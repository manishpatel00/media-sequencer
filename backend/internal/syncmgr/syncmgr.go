// Package syncmgr holds the live "is a sync currently overriding every
// window's display?" state and the logic to trigger one.
//
// Design summary (see README for the full explanation):
//   - A sync never touches a window's own playlist or its CycleAnchor. It
//     is purely a temporary *display override* layered on top of normal
//     playback.
//   - Triggering a sync picks a start time slightly in the future
//     (LeadTime, e.g. 2s) and broadcasts it to every connected client over
//     WebSocket immediately. Every client therefore learns the same
//     absolute start time and schedules a local timer against it, so all
//     windows switch to the synced item at effectively the same instant
//     regardless of small network latency differences.
//   - Because each window's normal playback position is always computed
//     fresh from its untouched CycleAnchor (see internal/sequencer), the
//     moment the sync's end time passes, every window simply resumes
//     showing whatever its own 5-hour cycle says it should be showing at
//     that real timestamp — nothing was lost or rewound.
package syncmgr

import (
	"sync"
	"time"

	"github.com/evabharat/media-sequencer/backend/internal/models"
	"github.com/evabharat/media-sequencer/backend/internal/wsHub"
)

// State describes an in-progress or scheduled sync.
type State struct {
	Active   bool      `json:"active"`
	MediaID  *int64    `json:"media_id,omitempty"`
	Type     string    `json:"type,omitempty"`
	URL      string    `json:"url,omitempty"`
	StartsAt time.Time `json:"starts_at,omitempty"`
	EndsAt   time.Time `json:"ends_at,omitempty"`
}

type Manager struct {
	hub *wsHub.Hub

	mu      sync.RWMutex
	current State
	timer   *time.Timer
}

func New(hub *wsHub.Hub) *Manager {
	return &Manager{hub: hub}
}

// Trigger schedules a sync of item to start after leadTime and stay active
// for duration, then broadcasts the start immediately so every client can
// count down to the same absolute instant.
func (m *Manager) Trigger(item models.MediaItem, duration, leadTime time.Duration) State {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.timer != nil {
		m.timer.Stop()
	}

	startsAt := time.Now().UTC().Add(leadTime)
	endsAt := startsAt.Add(duration)

	var mediaID *int64
	id := item.ID
	if id != 0 {
		mediaID = &id
	}

	m.current = State{
		Active:   true,
		MediaID:  mediaID,
		Type:     string(item.Type),
		URL:      item.URL,
		StartsAt: startsAt,
		EndsAt:   endsAt,
	}

	m.hub.Broadcast(wsHub.Event{Type: "sync_start", Data: m.current})

	// Schedule automatic clearing + a "sync_end" notification once the
	// window closes, so clients that (for whatever reason) missed doing
	// the math themselves still get an explicit signal to resume normal
	// playback.
	m.timer = time.AfterFunc(time.Until(endsAt), func() {
		m.mu.Lock()
		wasThisSync := m.current.StartsAt.Equal(startsAt)
		if wasThisSync {
			m.current = State{Active: false}
		}
		m.mu.Unlock()
		if wasThisSync {
			m.hub.Broadcast(wsHub.Event{Type: "sync_end"})
		}
	})

	return m.current
}

// Status returns whether a sync is currently active *right now* (i.e. the
// current wall-clock time falls within [StartsAt, EndsAt)), along with its
// details. It is used both for the polling-friendly GET /api/sync/status
// endpoint and could be reused by any additional consumer.
func (m *Manager) Status() State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.current.Active {
		return State{Active: false}
	}
	// A sync that has already ended is reported inactive to any snapshot
	// caller, even before the cleanup timer above has fired. A sync that
	// is scheduled but hasn't started yet (now < StartsAt) is still
	// reported active with its StartsAt/EndsAt so a client that only just
	// connected (e.g. reloaded the page) can still catch up and schedule
	// its local timer correctly.
	if time.Now().UTC().After(m.current.EndsAt) {
		return State{Active: false}
	}
	return m.current
}
