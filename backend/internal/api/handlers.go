// Package api wires HTTP requests to the store/sequencer/syncmgr packages.
// Handlers are intentionally thin: parse + validate the request, call the
// store (which owns correctness/ownership rules), translate the result (or
// error) into JSON. Business logic itself lives in the packages this one
// imports, not here.
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/evabharat/media-sequencer/backend/internal/config"
	"github.com/evabharat/media-sequencer/backend/internal/models"
	"github.com/evabharat/media-sequencer/backend/internal/sequencer"
	"github.com/evabharat/media-sequencer/backend/internal/store"
	"github.com/evabharat/media-sequencer/backend/internal/syncmgr"
	"github.com/evabharat/media-sequencer/backend/internal/wsHub"
)

type API struct {
	cfg   config.Config
	store *store.Store
	hub   *wsHub.Hub
	sync  *syncmgr.Manager
}

func New(cfg config.Config, st *store.Store, hub *wsHub.Hub, sm *syncmgr.Manager) *API {
	return &API{cfg: cfg, store: st, hub: hub, sync: sm}
}

// --- shared response helpers ---------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json response: %v", err)
	}
}

type apiError struct {
	Error string `json:"error"`
}

// maxRequestBody caps the size of JSON request bodies this API will read.
// None of our payloads are legitimately large (a handful of short fields),
// so a generous-but-finite 64KB limit is purely defensive: it stops a
// misbehaving or malicious client from streaming an unbounded body at a
// handler that just calls json.Decode.
const maxRequestBody = 64 * 1024

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	return json.NewDecoder(r.Body).Decode(dst)
}

// writeStoreError maps the sentinel errors defined in internal/store (plus
// a couple of local ones) to the right HTTP status, so every handler that
// touches the store gets consistent, correct status codes for free.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrWindowNotFound):
		writeJSON(w, http.StatusNotFound, apiError{"window not found"})
	case errors.Is(err, store.ErrMediaNotFound):
		writeJSON(w, http.StatusNotFound, apiError{"media item not found"})
	case errors.Is(err, store.ErrMediaNotOwned):
		// 404, not 403: we deliberately don't reveal that the ID exists
		// but belongs to someone else's window.
		writeJSON(w, http.StatusNotFound, apiError{"media item not found for this window"})
	default:
		log.Printf("internal error: %v", err)
		writeJSON(w, http.StatusInternalServerError, apiError{"internal server error"})
	}
}

// --- health ---------------------------------------------------------------

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- windows ---------------------------------------------------------------

// windowView is the JSON shape returned for a window: its metadata, its
// current playlist, the server-resolved "now playing" state (handy for
// clients that don't want to duplicate the cycle math), and the server's
// own clock so the frontend can correct for clock drift.
type windowView struct {
	Window     models.Window      `json:"window"`
	Playlist   []models.MediaItem `json:"playlist"`
	NowPlaying *nowPlayingView    `json:"now_playing,omitempty"`
	ServerTime time.Time          `json:"server_time"`
	CycleSecs  int                `json:"cycle_duration_seconds"`
	Sync       syncmgr.State      `json:"sync"`
}

type nowPlayingView struct {
	MediaID          int64     `json:"media_id"`
	Type             string    `json:"type"`
	URL              string    `json:"url"`
	Index            int       `json:"index"`
	ElapsedSeconds   int       `json:"elapsed_seconds"`
	RemainingSeconds int       `json:"remaining_seconds"`
	NextChangeAt     time.Time `json:"next_change_at"`
}

func (a *API) buildWindowView(w models.Window) (windowView, error) {
	playlist, err := a.store.ListMedia(w.ID)
	if err != nil {
		return windowView{}, err
	}

	view := windowView{
		Window:     w,
		Playlist:   playlist,
		ServerTime: time.Now().UTC(),
		CycleSecs:  int(sequencer.CycleDuration.Seconds()),
		Sync:       a.sync.Status(),
	}

	items := make([]sequencer.Item, len(playlist))
	for i, m := range playlist {
		items[i] = sequencer.Item{
			ID: m.ID, Type: string(m.Type), URL: m.URL,
			Duration: time.Duration(m.DurationSeconds) * time.Second,
		}
	}
	if state, ok := sequencer.Resolve(items, w.CycleAnchor, view.ServerTime); ok {
		view.NowPlaying = &nowPlayingView{
			MediaID:          state.Item.ID,
			Type:             state.Item.Type,
			URL:              state.Item.URL,
			Index:            state.Index,
			ElapsedSeconds:   int(state.ElapsedInItem.Seconds()),
			RemainingSeconds: int(state.RemainingInItem.Seconds()),
			NextChangeAt:     state.NextChangeAt,
		}
	}
	return view, nil
}

func (a *API) ListWindows(w http.ResponseWriter, r *http.Request) {
	windows, err := a.store.ListWindows()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	views := make([]windowView, 0, len(windows))
	for _, win := range windows {
		v, err := a.buildWindowView(win)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, views)
}

func (a *API) GetWindow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	win, err := a.store.GetWindow(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	view, err := a.buildWindowView(win)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *API) NowPlaying(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	win, err := a.store.GetWindow(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	view, err := a.buildWindowView(win)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"now_playing": view.NowPlaying,
		"server_time": view.ServerTime,
		"sync":        view.Sync,
	})
}

// --- media -------------------------------------------------------------

type addMediaRequest struct {
	Type            string `json:"type"`
	URL             string `json:"url"`
	DurationSeconds int    `json:"duration_seconds"`
}

func (a *API) AddMedia(w http.ResponseWriter, r *http.Request) {
	windowID := r.PathValue("id")

	var req addMediaRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{"invalid JSON body"})
		return
	}

	mt := models.MediaType(req.Type)
	if !mt.Valid() {
		writeJSON(w, http.StatusBadRequest, apiError{"type must be one of: image, video, blank"})
		return
	}
	if mt != models.MediaBlank && req.URL == "" {
		writeJSON(w, http.StatusBadRequest, apiError{"url is required for image/video items"})
		return
	}
	if req.DurationSeconds <= 0 {
		writeJSON(w, http.StatusBadRequest, apiError{"duration_seconds must be a positive integer"})
		return
	}

	item, err := a.store.AddMedia(windowID, mt, req.URL, req.DurationSeconds)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	a.hub.Broadcast(wsHub.Event{Type: "playlist_updated", Data: map[string]string{"window_id": windowID}})
	writeJSON(w, http.StatusCreated, item)
}

func (a *API) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	windowID := r.PathValue("id")
	mediaIDStr := r.PathValue("mediaId")

	mediaID, err := strconv.ParseInt(mediaIDStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{"mediaId must be an integer"})
		return
	}

	// DeleteMedia itself performs the ownership check (media must belong
	// to windowID); we don't duplicate that logic here.
	if err := a.store.DeleteMedia(windowID, mediaID); err != nil {
		writeStoreError(w, err)
		return
	}

	a.hub.Broadcast(wsHub.Event{Type: "playlist_updated", Data: map[string]string{"window_id": windowID}})
	writeJSON(w, http.StatusNoContent, nil)
}

type reorderRequest struct {
	OrderedIDs []int64 `json:"ordered_ids"`
}

func (a *API) ReorderMedia(w http.ResponseWriter, r *http.Request) {
	windowID := r.PathValue("id")

	var req reorderRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{"invalid JSON body"})
		return
	}
	if len(req.OrderedIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, apiError{"ordered_ids must not be empty"})
		return
	}

	if err := a.store.Reorder(windowID, req.OrderedIDs); err != nil {
		if errors.Is(err, store.ErrMediaNotOwned) || errors.Is(err, store.ErrWindowNotFound) {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusBadRequest, apiError{err.Error()})
		return
	}

	a.hub.Broadcast(wsHub.Event{Type: "playlist_updated", Data: map[string]string{"window_id": windowID}})
	writeJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}

// --- sync ------------------------------------------------------------------

// syncRequest supports two ways to specify what to sync:
//  1. {"media_id": 7} — sync an existing media item, looked up regardless
//     of which window currently owns it in its playlist.
//  2. {"type": "image", "url": "...", "duration_seconds": 8} — sync an
//     ad-hoc item that need not belong to any window's playlist at all.
//
// duration_seconds (top-level) optionally overrides how long the sync stays
// on screen; if omitted, config.DefaultSyncDurationSeconds is used.
type syncRequest struct {
	MediaID         *int64 `json:"media_id,omitempty"`
	Type            string `json:"type,omitempty"`
	URL             string `json:"url,omitempty"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
}

func (a *API) TriggerSync(w http.ResponseWriter, r *http.Request) {
	var req syncRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{"invalid JSON body"})
		return
	}

	var item models.MediaItem
	if req.MediaID != nil {
		m, err := a.store.GetMediaByID(*req.MediaID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		item = m
	} else {
		mt := models.MediaType(req.Type)
		if !mt.Valid() {
			writeJSON(w, http.StatusBadRequest, apiError{"provide either media_id, or a valid type+url"})
			return
		}
		if mt != models.MediaBlank && req.URL == "" {
			writeJSON(w, http.StatusBadRequest, apiError{"url is required for image/video sync items"})
			return
		}
		item = models.MediaItem{Type: mt, URL: req.URL}
	}

	syncDuration := time.Duration(a.cfg.DefaultSyncDurationSeconds) * time.Second
	if req.DurationSeconds > 0 {
		syncDuration = time.Duration(req.DurationSeconds) * time.Second
	}
	leadTime := time.Duration(a.cfg.SyncLeadSeconds) * time.Second

	state := a.sync.Trigger(item, syncDuration, leadTime)

	var mediaIDPtr *int64
	if item.ID != 0 {
		id := item.ID
		mediaIDPtr = &id
	}
	if _, err := a.store.RecordSyncEvent(models.SyncEvent{
		MediaID: mediaIDPtr, Type: item.Type, URL: item.URL,
		DurationSeconds: int(syncDuration.Seconds()),
		StartsAt:        state.StartsAt, EndsAt: state.EndsAt,
	}); err != nil {
		// Non-fatal: the live sync already started/was broadcast; failing
		// to write the audit row shouldn't block the sync itself.
		log.Printf("record sync event: %v", err)
	}

	writeJSON(w, http.StatusOK, state)
}

func (a *API) SyncStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.sync.Status())
}

// --- websocket ---------------------------------------------------------

func (a *API) WS(w http.ResponseWriter, r *http.Request) {
	a.hub.ServeWS(w, r)
}
