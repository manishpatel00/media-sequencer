package sequencer

import (
	"testing"
	"time"
)

func mkItems() []Item {
	return []Item{
		{ID: 1, Type: "image", URL: "a.jpg", Duration: 10 * time.Second},
		{ID: 2, Type: "video", URL: "b.mp4", Duration: 20 * time.Second},
		{ID: 3, Type: "image", URL: "c.jpg", Duration: 10 * time.Second},
	}
}

func TestResolve_EmptyPlaylist(t *testing.T) {
	_, ok := Resolve(nil, time.Now(), time.Now())
	if ok {
		t.Fatal("expected ok=false for an empty playlist")
	}
}

func TestResolve_AllZeroDurations(t *testing.T) {
	items := []Item{{ID: 1, Duration: 0}, {ID: 2, Duration: -5 * time.Second}}
	_, ok := Resolve(items, time.Now(), time.Now())
	if ok {
		t.Fatal("expected ok=false when no item has a positive duration")
	}
}

func TestResolve_FirstItemAtAnchor(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	state, ok := Resolve(mkItems(), anchor, anchor)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if state.Index != 0 || state.Item.ID != 1 {
		t.Fatalf("expected first item at t=anchor, got index=%d id=%d", state.Index, state.Item.ID)
	}
	if state.RemainingInItem != 10*time.Second {
		t.Fatalf("expected 10s remaining, got %s", state.RemainingInItem)
	}
}

func TestResolve_MidSecondItem(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// playlist: [0,10)=item1 [10,30)=item2 [30,40)=item3, total=40s
	now := anchor.Add(15 * time.Second)
	state, ok := Resolve(mkItems(), anchor, now)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if state.Item.ID != 2 {
		t.Fatalf("expected item 2 at t=+15s, got id=%d", state.Item.ID)
	}
	if state.ElapsedInItem != 5*time.Second {
		t.Fatalf("expected 5s elapsed into item 2, got %s", state.ElapsedInItem)
	}
	if state.RemainingInItem != 15*time.Second {
		t.Fatalf("expected 15s remaining, got %s", state.RemainingInItem)
	}
}

func TestResolve_LoopsWithinCycle(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// total playlist duration = 40s. At t=+45s we should be 5s into a second
	// full loop of the playlist, i.e. back inside item 1.
	now := anchor.Add(45 * time.Second)
	state, ok := Resolve(mkItems(), anchor, now)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if state.Item.ID != 1 {
		t.Fatalf("expected playlist to have looped back to item 1, got id=%d", state.Item.ID)
	}
	if state.ElapsedInItem != 5*time.Second {
		t.Fatalf("expected 5s into the looped item, got %s", state.ElapsedInItem)
	}
}

func TestResolve_RestartsAtFiveHourBoundary(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// 40s playlist does not evenly divide 5h (18000s); 18000 % 40 == 0 in
	// this particular case actually divides evenly (18000/40=450), so pick a
	// playlist whose duration does NOT divide 5h evenly to exercise the
	// boundary-cut behaviour.
	items := []Item{
		{ID: 1, Type: "image", Duration: 10 * time.Second},
		{ID: 2, Type: "image", Duration: 13 * time.Second}, // total = 23s
	}
	// 18000 % 23 = 18000 - 782*23 = 18000-17986 = 14 -> at t just before the
	// 5h mark, cycleElapsed is close to 18000s, posInPlaylist = cycleElapsed % 23.
	// One second before the boundary:
	justBefore := anchor.Add(CycleDuration - time.Second)
	state, ok := Resolve(items, anchor, justBefore)
	if !ok {
		t.Fatal("expected ok=true")
	}
	// Remaining must never exceed the time left until the cycle boundary.
	if state.RemainingInItem > time.Second {
		t.Fatalf("expected remaining to be capped by the cycle boundary (<=1s), got %s", state.RemainingInItem)
	}

	// Exactly at the boundary, the cycle has restarted: we must be back at
	// item 0 with a fresh, full remaining duration.
	atBoundary := anchor.Add(CycleDuration)
	state2, ok := Resolve(items, anchor, atBoundary)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if state2.Index != 0 {
		t.Fatalf("expected cycle restart at item 0 exactly at the 5h boundary, got index=%d", state2.Index)
	}
}

func TestResolve_SkipsZeroDurationEntriesButKeepsOthersPlayable(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []Item{
		{ID: 1, Duration: 0}, // misconfigured, should be skipped
		{ID: 2, Duration: 10 * time.Second},
	}
	state, ok := Resolve(items, anchor, anchor)
	if !ok {
		t.Fatal("expected ok=true, one item is valid")
	}
	if state.Item.ID != 2 {
		t.Fatalf("expected the only positive-duration item to play, got id=%d", state.Item.ID)
	}
}

func TestResolve_NegativeElapsedClampsToZero(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := anchor.Add(-time.Hour) // now before anchor: clock skew scenario
	state, ok := Resolve(mkItems(), anchor, now)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if state.Index != 0 {
		t.Fatalf("expected clamped-to-zero elapsed to resolve to the first item, got index=%d", state.Index)
	}
}
