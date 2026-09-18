// Package models defines the core domain types used throughout the backend:
// display Windows, the MediaItems that make up a window's playlist, and the
// SyncEvent record used to audit sync actions. Keeping these types in one
// place (instead of scattering ad-hoc structs across packages) makes the
// data shape easy to find and easy to reason about for anyone reading the
// code for the first time.
package models

import "time"

// MediaType enumerates the kinds of content a playlist slot can hold.
type MediaType string

const (
	MediaImage MediaType = "image"
	MediaVideo MediaType = "video"
	MediaBlank MediaType = "blank" // an explicit "show nothing" slot
)

// Valid reports whether t is one of the supported media types.
func (t MediaType) Valid() bool {
	switch t {
	case MediaImage, MediaVideo, MediaBlank:
		return true
	default:
		return false
	}
}

// Window represents one physical/virtual display that plays its own
// looping playlist. CycleAnchor is a fixed point in time, set once when the
// window is created, that every playback calculation is measured from. It
// never changes (not even during a sync), which is what lets a window
// "pick up where it left off" once a sync ends.
type Window struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	CycleAnchor time.Time `json:"cycle_anchor"`
	CreatedAt   time.Time `json:"created_at"`
}

// MediaItem is a single entry in a window's playlist.
//
// Ownership note: every MediaItem belongs to exactly one Window
// (WindowID). All mutating operations on a media item must verify that the
// item actually belongs to the window given in the request path before
// acting on it — see internal/store for the enforced checks.
type MediaItem struct {
	ID              int64     `json:"id"`
	WindowID        string    `json:"window_id"`
	Type            MediaType `json:"type"`
	URL             string    `json:"url"`
	DurationSeconds int       `json:"duration_seconds"`
	Position        int       `json:"position"`
	CreatedAt       time.Time `json:"created_at"`
}

// SyncEvent is a persisted audit record of a sync action, kept mainly for
// history/debugging. Live sync state (what's playing right now) is held in
// memory by internal/sync for speed, and is reconstructable from the most
// recent still-active row here if the server restarts mid-sync.
type SyncEvent struct {
	ID              int64     `json:"id"`
	MediaID         *int64    `json:"media_id,omitempty"`
	Type            MediaType `json:"type"`
	URL             string    `json:"url"`
	DurationSeconds int       `json:"duration_seconds"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	CreatedAt       time.Time `json:"created_at"`
}
