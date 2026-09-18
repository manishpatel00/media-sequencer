// Package config centralises reading of environment variables so the rest
// of the codebase never calls os.Getenv directly. That keeps every
// configurable value discoverable in one file and one README table.
package config

import (
	"os"
	"strconv"
)

// Config holds every environment-tunable setting the server needs.
type Config struct {
	// Port the HTTP server listens on.
	Port string
	// DBPath is the filesystem path to the SQLite database file. The
	// directory is created automatically if it doesn't exist.
	DBPath string
	// AllowedOrigin is the value returned in Access-Control-Allow-Origin.
	// Use "*" for local development; set it to your deployed frontend's
	// origin in production.
	AllowedOrigin string
	// SeedOnEmpty controls whether demo windows/media are inserted the
	// first time the server runs against an empty database.
	SeedOnEmpty bool
	// SyncLeadSeconds is the small delay added before a sync actually
	// starts, giving every connected client time to receive the
	// "sync_start" websocket message before the target start time arrives.
	// This is what makes the sync feel simultaneous instead of racy.
	SyncLeadSeconds int
	// DefaultSyncDurationSeconds is used when a sync request does not
	// specify how long the synced item should stay on screen.
	DefaultSyncDurationSeconds int
}

func Load() Config {
	return Config{
		Port:                       getEnv("PORT", "8080"),
		DBPath:                     getEnv("DB_PATH", "./data/sequencer.db"),
		AllowedOrigin:              getEnv("ALLOWED_ORIGIN", "*"),
		SeedOnEmpty:                getBoolEnv("SEED_ON_EMPTY", true),
		SyncLeadSeconds:            getIntEnv("SYNC_LEAD_SECONDS", 2),
		DefaultSyncDurationSeconds: getIntEnv("DEFAULT_SYNC_DURATION_SECONDS", 10),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
