package api

import (
	"log"
	"net/http"
	"time"
)

// NewRouter wires every route to its handler. Go 1.22's net/http.ServeMux
// supports method + wildcard path segments natively (e.g. "GET
// /api/windows/{id}"), so no external routing library is needed for a
// surface this small.
func (a *API) NewRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", a.Health)

	mux.HandleFunc("GET /api/windows", a.ListWindows)
	mux.HandleFunc("GET /api/windows/{id}", a.GetWindow)
	mux.HandleFunc("GET /api/windows/{id}/now-playing", a.NowPlaying)
	mux.HandleFunc("POST /api/windows/{id}/media", a.AddMedia)
	mux.HandleFunc("DELETE /api/windows/{id}/media/{mediaId}", a.DeleteMedia)
	mux.HandleFunc("PUT /api/windows/{id}/media/reorder", a.ReorderMedia)

	mux.HandleFunc("POST /api/sync", a.TriggerSync)
	mux.HandleFunc("GET /api/sync/status", a.SyncStatus)

	mux.HandleFunc("GET /ws", a.WS)

	return a.withMiddleware(mux)
}

func (a *API) withMiddleware(h http.Handler) http.Handler {
	return withLogging(a.withCORS(h))
}

// withCORS allows the frontend (deployed on a different origin) to call
// this API. AllowedOrigin is configurable via env var; "*" is fine for a
// read-mostly public demo like this one.
func (a *API) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", a.cfg.AllowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start))
	})
}
