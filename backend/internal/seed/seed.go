// Package seed inserts demo windows and playlists the first time the
// server runs against an empty database, so the assignment's "seed data
// matching the example windows and media lists" deliverable is satisfied
// out of the box with zero manual setup.
//
// The sample media URLs point at freely-licensed, publicly hosted demo
// assets (Google's public GTV test-video bucket for video, and Picsum for
// placeholder photos) purely so the seeded playlists play something real
// in a browser. Swap them for your own media at any time via the "add
// media" API/UI — nothing about the sequencing logic depends on these
// specific URLs.
package seed

import (
	"log"
	"time"

	"github.com/manishpatel00/media-sequencer/backend/internal/models"
	"github.com/manishpatel00/media-sequencer/backend/internal/store"
)

// Run seeds three demo windows (W1, W2, W3) if, and only if, no windows
// exist yet. It is safe to call on every startup.
func Run(st *store.Store) error {
	existing, err := st.ListWindows()
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		log.Printf("seed: %d window(s) already present, skipping", len(existing))
		return nil
	}

	log.Printf("seed: no windows found, inserting demo data")
	now := time.Now().UTC()

	type seedMedia struct {
		Type     models.MediaType
		URL      string
		Duration int
	}
	windows := []struct {
		ID, Name string
		Media    []seedMedia
	}{
		{
			ID: "W1", Name: "Window 1: Lobby Display",
			Media: []seedMedia{
				{models.MediaImage, "https://picsum.photos/id/1015/1280/720", 8},
				{models.MediaVideo, "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4", 30},
				{models.MediaImage, "https://picsum.photos/id/1025/1280/720", 8},
			},
		},
		{
			ID: "W2", Name: "Window 2: Storefront",
			Media: []seedMedia{
				{models.MediaImage, "https://picsum.photos/id/1043/1280/720", 10},
				{models.MediaImage, "https://picsum.photos/id/1050/1280/720", 10},
				{models.MediaVideo, "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ElephantsDream.mp4", 25},
				{models.MediaBlank, "", 5},
			},
		},
		{
			ID: "W3", Name: "Window 3: Reception",
			Media: []seedMedia{
				{models.MediaVideo, "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ForBiggerBlazes.mp4", 15},
				{models.MediaImage, "https://picsum.photos/id/1069/1280/720", 8},
			},
		},
	}

	for _, w := range windows {
		if _, err := st.CreateWindow(w.ID, w.Name, now); err != nil {
			return err
		}
		for _, m := range w.Media {
			if _, err := st.AddMedia(w.ID, m.Type, m.URL, m.Duration); err != nil {
				return err
			}
		}
	}

	log.Printf("seed: inserted %d demo windows", len(windows))
	return nil
}
