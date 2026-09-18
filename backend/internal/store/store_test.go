package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/manishpatel00/media-sequencer/backend/internal/db"
	"github.com/manishpatel00/media-sequencer/backend/internal/models"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(conn)
}

func TestAddMedia_UnknownWindow(t *testing.T) {
	s := newTestStore(t)
	_, err := s.AddMedia("does-not-exist", models.MediaImage, "http://x/a.jpg", 5)
	if !errors.Is(err, ErrWindowNotFound) {
		t.Fatalf("expected ErrWindowNotFound, got %v", err)
	}
}

func TestAddMedia_PositionsIncrement(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateWindow("W1", "Window 1", time.Now()); err != nil {
		t.Fatalf("create window: %v", err)
	}
	m1, err := s.AddMedia("W1", models.MediaImage, "http://x/1.jpg", 5)
	if err != nil {
		t.Fatalf("add media 1: %v", err)
	}
	m2, err := s.AddMedia("W1", models.MediaImage, "http://x/2.jpg", 5)
	if err != nil {
		t.Fatalf("add media 2: %v", err)
	}
	if m1.Position != 0 || m2.Position != 1 {
		t.Fatalf("expected positions 0,1 got %d,%d", m1.Position, m2.Position)
	}
}

// TestOwnershipIsEnforced is the key regression test for the assignment's
// explicit "clean ownership checks" requirement: a media item created
// under window A must be untouchable through window B's endpoints, even
// though both are valid windows and the media ID itself is valid.
func TestOwnershipIsEnforced(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateWindow("W1", "Window 1", time.Now()); err != nil {
		t.Fatalf("create W1: %v", err)
	}
	if _, err := s.CreateWindow("W2", "Window 2", time.Now()); err != nil {
		t.Fatalf("create W2: %v", err)
	}

	item, err := s.AddMedia("W1", models.MediaImage, "http://x/1.jpg", 5)
	if err != nil {
		t.Fatalf("add media to W1: %v", err)
	}

	// Reading W1's item "as if" it belonged to W2 must fail.
	if _, err := s.GetOwnedMedia("W2", item.ID); !errors.Is(err, ErrMediaNotOwned) {
		t.Fatalf("expected ErrMediaNotOwned reading cross-window, got %v", err)
	}

	// Deleting via W2's scope must fail and must NOT delete the row.
	if err := s.DeleteMedia("W2", item.ID); !errors.Is(err, ErrMediaNotOwned) {
		t.Fatalf("expected ErrMediaNotOwned deleting cross-window, got %v", err)
	}
	stillThere, err := s.GetOwnedMedia("W1", item.ID)
	if err != nil {
		t.Fatalf("expected item to still exist under W1, got err: %v", err)
	}
	if stillThere.ID != item.ID {
		t.Fatalf("expected same item back")
	}

	// Deleting via the correct owning window succeeds.
	if err := s.DeleteMedia("W1", item.ID); err != nil {
		t.Fatalf("expected delete via correct owner to succeed, got %v", err)
	}
	if _, err := s.GetMediaByID(item.ID); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected item to be gone, got %v", err)
	}
}

func TestReorder_RejectsForeignID(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateWindow("W1", "Window 1", time.Now()); err != nil {
		t.Fatalf("create W1: %v", err)
	}
	if _, err := s.CreateWindow("W2", "Window 2", time.Now()); err != nil {
		t.Fatalf("create W2: %v", err)
	}
	a, _ := s.AddMedia("W1", models.MediaImage, "http://x/1.jpg", 5)
	_, _ = s.AddMedia("W1", models.MediaImage, "http://x/2.jpg", 5)
	foreign, _ := s.AddMedia("W2", models.MediaImage, "http://x/3.jpg", 5)

	err := s.Reorder("W1", []int64{foreign.ID, a.ID})
	if err == nil {
		t.Fatal("expected reorder with a foreign ID to fail")
	}
}

func TestListMedia_UnknownWindow(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.ListMedia("nope"); !errors.Is(err, ErrWindowNotFound) {
		t.Fatalf("expected ErrWindowNotFound, got %v", err)
	}
}
