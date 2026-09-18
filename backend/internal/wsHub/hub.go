// Package wsHub implements a small WebSocket "fan-out" hub. Its only job is
// to let the backend push two kinds of events to every connected frontend
// the instant they happen, instead of making the frontend poll for them:
//
//  1. "playlist_updated" — a window's media list changed, so any client
//     should re-fetch that window's playlist.
//  2. "sync_start" / "sync_end" — a sync action began or ended, with the
//     exact server timestamp it started/ends at, so every window can render
//     the synced item at (as close as networking allows) the same instant.
//
// This is what makes "every window should display M2 at the same time"
// reliable: rather than each client deciding independently when to switch,
// they all receive the same authoritative starts_at timestamp from the
// server and schedule a local timer against it.
package wsHub

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Event is the JSON envelope sent to every connected client.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type Hub struct {
	upgrader websocket.Upgrader

	mu      sync.Mutex
	clients map[*websocket.Conn]chan Event
}

func New(allowedOrigin string) *Hub {
	return &Hub{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// The API already enforces CORS for regular HTTP requests; for
			// the WS upgrade we simply allow any origin, mirroring the same
			// permissive policy used elsewhere in this demo deployment.
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		clients: make(map[*websocket.Conn]chan Event),
	}
}

// ServeWS upgrades an HTTP connection to a WebSocket and registers it as a
// broadcast recipient until it disconnects.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	send := make(chan Event, 16)
	h.mu.Lock()
	h.clients[conn] = send
	h.mu.Unlock()

	// Writer goroutine: serialises all writes to this connection so we
	// never call WriteJSON concurrently on the same conn.
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
		}()
		for ev := range send {
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		}
	}()

	// We don't need anything from the client, but we must keep reading so
	// the connection's close/ping-pong control frames are processed and a
	// dead client is detected promptly.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			h.mu.Lock()
			if ch, ok := h.clients[conn]; ok {
				close(ch)
				delete(h.clients, conn)
			}
			h.mu.Unlock()
			return
		}
	}
}

// Broadcast sends ev to every currently connected client. Slow/blocked
// clients are dropped rather than allowed to stall the broadcast for
// everyone else.
func (h *Hub) Broadcast(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn, ch := range h.clients {
		select {
		case ch <- ev:
		default:
			log.Printf("ws client too slow, dropping connection")
			close(ch)
			delete(h.clients, conn)
		}
	}
}

// MustJSON is a small helper for logging/debugging event payloads; it is
// not used on the hot broadcast path (WriteJSON handles real encoding).
func MustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
