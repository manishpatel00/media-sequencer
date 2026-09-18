// Command server boots the Multi-Window Media Sequencer backend: it opens
// the SQLite database, applies the schema, seeds demo data on first run,
// and starts the HTTP + WebSocket server.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/evabharat/media-sequencer/backend/internal/api"
	"github.com/evabharat/media-sequencer/backend/internal/config"
	"github.com/evabharat/media-sequencer/backend/internal/db"
	"github.com/evabharat/media-sequencer/backend/internal/seed"
	"github.com/evabharat/media-sequencer/backend/internal/store"
	"github.com/evabharat/media-sequencer/backend/internal/syncmgr"
	"github.com/evabharat/media-sequencer/backend/internal/wsHub"
)

func main() {
	cfg := config.Load()

	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	st := store.New(conn)

	if cfg.SeedOnEmpty {
		if err := seed.Run(st); err != nil {
			log.Fatalf("failed to seed database: %v", err)
		}
	}

	hub := wsHub.New(cfg.AllowedOrigin)
	sm := syncmgr.New(hub)
	a := api.New(cfg, st, hub, sm)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: a.NewRouter(),
		// ReadHeaderTimeout guards against slow-header (slowloris-style)
		// connections holding a goroutine open indefinitely.
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Run the server in a goroutine so the main goroutine is free to wait
	// for an interrupt/terminate signal and shut down cleanly — important
	// in containerized deployments, which send SIGTERM on redeploy and
	// expect the process to stop accepting new work and exit promptly.
	go func() {
		log.Printf("media-sequencer backend listening on :%s (db=%s)", cfg.Port, cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Printf("shutdown signal received, draining connections...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown did not complete cleanly: %v", err)
	}
}
