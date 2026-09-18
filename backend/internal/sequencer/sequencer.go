// Package sequencer contains the pure math that decides "what should this
// window be showing right now, and when does that change?". It has no
// dependency on the database, HTTP, or time.Now() being called anywhere
// hidden inside it — every function takes "now" as an explicit argument.
// That is deliberate: it makes the trickiest part of the assignment (the
// 5-hour looping cycle and the sync override) fully unit-testable without
// mocking a clock or spinning up a server.
package sequencer

import "time"

// CycleDuration is the fixed length of one playback cycle for every
// window, per the assignment brief: "The total play size for each window
// must be treated as 5 hours."
const CycleDuration = 5 * time.Hour

// Item is the minimal shape the sequencer needs from a playlist entry.
// internal/models.MediaItem is converted into this before being resolved,
// keeping this package decoupled from the storage layer.
type Item struct {
	ID       int64
	Type     string
	URL      string
	Duration time.Duration
}

// State describes what a window should be displaying at a given instant,
// plus enough information for a caller (or a frontend, via the API) to know
// exactly when to move on to the next item.
type State struct {
	Item            Item
	Index           int           // position of Item within the supplied playlist
	ElapsedInItem   time.Duration // how far into this item we are
	RemainingInItem time.Duration // time left before a change is due
	CycleElapsed    time.Duration // elapsed time within the current 5h cycle
	NextChangeAt    time.Time     // absolute wall-clock time of the next change
}

// Resolve computes which playlist item is active "now" for a window whose
// cycle started at anchor.
//
// Behaviour (documented deliberately, since it's the crux of the
// assignment):
//   - The playlist loops continuously to fill up a fixed CycleDuration
//     (5 hours) window, then the cycle restarts from the first item again.
//   - If the sum of item durations divides evenly into 5 hours, looping is
//     seamless. If it does not, the item that is playing exactly as the
//     5-hour mark is crossed gets cut short and the cycle restarts at item
//     0 — this keeps every window's "5-hour mark" perfectly aligned, which
//     matters for predictable sync/testing behaviour, at the cost of that
//     one boundary item occasionally not finishing. This is called out in
//     the README as a documented assumption.
//   - If the playlist's own total duration is longer than 5 hours, only the
//     first 5 hours of it ever plays before the cycle wraps back to item 0.
//
// Resolve returns ok=false if items is empty (nothing to play) or every
// item has a non-positive duration (a misconfigured playlist).
func Resolve(items []Item, anchor time.Time, now time.Time) (State, bool) {
	if len(items) == 0 {
		return State{}, false
	}

	total := time.Duration(0)
	for _, it := range items {
		if it.Duration > 0 {
			total += it.Duration
		}
	}
	if total <= 0 {
		return State{}, false
	}

	elapsedSinceAnchor := now.Sub(anchor)
	if elapsedSinceAnchor < 0 {
		// Clock skew or an anchor set in the future: treat as "just started".
		elapsedSinceAnchor = 0
	}

	cycleElapsed := elapsedSinceAnchor % CycleDuration
	posInPlaylist := cycleElapsed % total

	// Walk the (looped) playlist to find which item posInPlaylist lands on.
	var walked time.Duration
	for idx, it := range items {
		if it.Duration <= 0 {
			continue // skip misconfigured zero/negative-duration entries
		}
		itemEnd := walked + it.Duration
		if posInPlaylist < itemEnd {
			elapsedInItem := posInPlaylist - walked
			itemRemaining := it.Duration - elapsedInItem
			cycleBoundaryRemaining := CycleDuration - cycleElapsed

			// The change happens at whichever comes first: the item simply
			// finishing, or the 5-hour cycle boundary forcing a restart.
			remaining := itemRemaining
			if cycleBoundaryRemaining < remaining {
				remaining = cycleBoundaryRemaining
			}

			return State{
				Item:            it,
				Index:           idx,
				ElapsedInItem:   elapsedInItem,
				RemainingInItem: remaining,
				CycleElapsed:    cycleElapsed,
				NextChangeAt:    now.Add(remaining),
			}, true
		}
		walked = itemEnd
	}

	// Should not be reachable given posInPlaylist < total by construction,
	// but fall back to the first playable item defensively rather than
	// panicking or returning a zero-value State.
	for idx, it := range items {
		if it.Duration > 0 {
			return State{
				Item:            it,
				Index:           idx,
				RemainingInItem: it.Duration,
				NextChangeAt:    now.Add(it.Duration),
			}, true
		}
	}
	return State{}, false
}
