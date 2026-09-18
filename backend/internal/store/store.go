// Package store is the only part of the backend allowed to run SQL. Every
// exported method here is written so that a caller cannot accidentally (or
// maliciously, via a crafted request) act on data belonging to a different
// window than the one it claims to be operating on — see the "ownership"
// note on each method below.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/manishpatel00/media-sequencer/backend/internal/models"
)

// Sentinel errors. Handlers in internal/api map these to HTTP status codes,
// so adding a new one here should usually come with a matching case in the
// API layer's error translation.
var (
	ErrWindowNotFound = errors.New("window not found")
	ErrMediaNotFound  = errors.New("media item not found")
	// ErrMediaNotOwned is returned when a media item exists but belongs to
	// a *different* window than the one specified in the request. This is
	// the core "ownership check" the store enforces: a request scoped to
	// window A must never be able to read, delete, or otherwise affect a
	// media item that actually belongs to window B, even if the caller
	// somehow knows or guesses its numeric ID.
	ErrMediaNotOwned = errors.New("media item does not belong to the specified window")
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// --- Windows -----------------------------------------------------------

// CreateWindow inserts a new window with a fresh cycle anchor of "now".
// Used by the seed step and is exposed for completeness; the assignment's
// core dynamic-update requirement is about media items, not windows.
func (s *Store) CreateWindow(id, name string, anchor time.Time) (models.Window, error) {
	now := time.Now().UTC()
	if anchor.IsZero() {
		anchor = now
	}
	_, err := s.db.Exec(
		`INSERT INTO windows (id, name, cycle_anchor, created_at) VALUES (?, ?, ?, ?)`,
		id, name, anchor.UTC(), now,
	)
	if err != nil {
		return models.Window{}, fmt.Errorf("insert window: %w", err)
	}
	return models.Window{ID: id, Name: name, CycleAnchor: anchor.UTC(), CreatedAt: now}, nil
}

// GetWindow fetches a single window by ID, or ErrWindowNotFound.
func (s *Store) GetWindow(id string) (models.Window, error) {
	var w models.Window
	err := s.db.QueryRow(
		`SELECT id, name, cycle_anchor, created_at FROM windows WHERE id = ?`, id,
	).Scan(&w.ID, &w.Name, &w.CycleAnchor, &w.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Window{}, ErrWindowNotFound
	}
	if err != nil {
		return models.Window{}, fmt.Errorf("get window: %w", err)
	}
	return w, nil
}

// ListWindows returns every window, ordered by ID for stable output.
func (s *Store) ListWindows() ([]models.Window, error) {
	rows, err := s.db.Query(`SELECT id, name, cycle_anchor, created_at FROM windows ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list windows: %w", err)
	}
	defer rows.Close()

	var out []models.Window
	for rows.Next() {
		var w models.Window
		if err := rows.Scan(&w.ID, &w.Name, &w.CycleAnchor, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan window: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// --- Media items ---------------------------------------------------------

// ListMedia returns every media item belonging to windowID, ordered by
// playlist position. Returns ErrWindowNotFound if the window doesn't exist,
// so callers never silently get an empty list for a typo'd window ID.
func (s *Store) ListMedia(windowID string) ([]models.MediaItem, error) {
	if _, err := s.GetWindow(windowID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, window_id, type, url, duration_seconds, position, created_at
		 FROM media_items WHERE window_id = ? ORDER BY position ASC, id ASC`,
		windowID,
	)
	if err != nil {
		return nil, fmt.Errorf("list media: %w", err)
	}
	defer rows.Close()

	var out []models.MediaItem
	for rows.Next() {
		var m models.MediaItem
		if err := rows.Scan(&m.ID, &m.WindowID, &m.Type, &m.URL, &m.DurationSeconds, &m.Position, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan media: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMedia appends a new item to windowID's playlist, at the next available
// position. Ownership is trivially correct here because the new row is
// created with the caller-supplied windowID directly — there is nothing to
// mix up. The window's existence is still verified first so adding media to
// a non-existent window fails clearly instead of silently succeeding with
// an orphaned row (foreign keys also enforce this at the DB level, but the
// explicit check produces a clean ErrWindowNotFound for the API layer).
func (s *Store) AddMedia(windowID string, mediaType models.MediaType, url string, durationSeconds int) (models.MediaItem, error) {
	if _, err := s.GetWindow(windowID); err != nil {
		return models.MediaItem{}, err
	}

	var nextPos sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT MAX(position) FROM media_items WHERE window_id = ?`, windowID,
	).Scan(&nextPos); err != nil {
		return models.MediaItem{}, fmt.Errorf("compute next position: %w", err)
	}
	position := 0
	if nextPos.Valid {
		position = int(nextPos.Int64) + 1
	}

	now := time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT INTO media_items (window_id, type, url, duration_seconds, position, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		windowID, string(mediaType), url, durationSeconds, position, now,
	)
	if err != nil {
		return models.MediaItem{}, fmt.Errorf("insert media: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.MediaItem{}, fmt.Errorf("read inserted id: %w", err)
	}

	return models.MediaItem{
		ID: id, WindowID: windowID, Type: mediaType, URL: url,
		DurationSeconds: durationSeconds, Position: position, CreatedAt: now,
	}, nil
}

// GetMediaByID fetches a media item by its global ID regardless of which
// window owns it. Used by the sync flow, which is allowed to reference any
// media item in the system (see internal/api's sync handler), not just ones
// belonging to a particular window.
func (s *Store) GetMediaByID(mediaID int64) (models.MediaItem, error) {
	var m models.MediaItem
	err := s.db.QueryRow(
		`SELECT id, window_id, type, url, duration_seconds, position, created_at
		 FROM media_items WHERE id = ?`, mediaID,
	).Scan(&m.ID, &m.WindowID, &m.Type, &m.URL, &m.DurationSeconds, &m.Position, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.MediaItem{}, ErrMediaNotFound
	}
	if err != nil {
		return models.MediaItem{}, fmt.Errorf("get media: %w", err)
	}
	return m, nil
}

// GetOwnedMedia fetches a media item AND verifies it belongs to windowID.
// This is the ownership check the assignment specifically calls out: any
// endpoint shaped like /windows/{windowID}/media/{mediaID} must use this
// (never GetMediaByID) so a caller cannot delete or otherwise touch another
// window's item just by knowing its numeric ID.
func (s *Store) GetOwnedMedia(windowID string, mediaID int64) (models.MediaItem, error) {
	m, err := s.GetMediaByID(mediaID)
	if err != nil {
		return models.MediaItem{}, err
	}
	if m.WindowID != windowID {
		return models.MediaItem{}, ErrMediaNotOwned
	}
	return m, nil
}

// DeleteMedia removes a media item, but only after confirming it belongs to
// windowID (see GetOwnedMedia). This prevents a caller from deleting item 7
// via DELETE /windows/W1/media/7 when item 7 actually belongs to W2.
func (s *Store) DeleteMedia(windowID string, mediaID int64) error {
	if _, err := s.GetOwnedMedia(windowID, mediaID); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM media_items WHERE id = ? AND window_id = ?`, mediaID, windowID)
	if err != nil {
		return fmt.Errorf("delete media: %w", err)
	}
	return nil
}

// Reorder rewrites the position of every item in orderedIDs to match the
// order they're given in. It verifies every ID belongs to windowID before
// changing anything (all-or-nothing), so a partially-foreign ID list is
// rejected instead of partially applied.
func (s *Store) Reorder(windowID string, orderedIDs []int64) error {
	existing, err := s.ListMedia(windowID)
	if err != nil {
		return err
	}
	owned := make(map[int64]bool, len(existing))
	for _, m := range existing {
		owned[m.ID] = true
	}
	if len(orderedIDs) != len(existing) {
		return fmt.Errorf("reorder must include exactly the window's current %d item(s), got %d", len(existing), len(orderedIDs))
	}
	for _, id := range orderedIDs {
		if !owned[id] {
			return ErrMediaNotOwned
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a harmless no-op

	for pos, id := range orderedIDs {
		if _, err := tx.Exec(
			`UPDATE media_items SET position = ? WHERE id = ? AND window_id = ?`,
			pos, id, windowID,
		); err != nil {
			return fmt.Errorf("update position: %w", err)
		}
	}
	return tx.Commit()
}

// --- Sync audit trail ----------------------------------------------------

// RecordSyncEvent persists a record of a triggered sync for audit/history
// purposes. Live sync state is tracked separately (in memory, by
// internal/sync) for speed; this table is not on the hot path.
func (s *Store) RecordSyncEvent(e models.SyncEvent) (models.SyncEvent, error) {
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT INTO sync_events (media_id, type, url, duration_seconds, starts_at, ends_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.MediaID, string(e.Type), e.URL, e.DurationSeconds, e.StartsAt.UTC(), e.EndsAt.UTC(), now,
	)
	if err != nil {
		return models.SyncEvent{}, fmt.Errorf("insert sync event: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.SyncEvent{}, fmt.Errorf("read inserted id: %w", err)
	}
	e.ID = id
	e.CreatedAt = now
	return e, nil
}
