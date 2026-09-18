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

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Media Sequencer API</title>
    <style>
        body { margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background-color: #09090b; color: #fafafa; display: flex; align-items: center; justify-content: center; height: 100vh; }
        .container { text-align: center; padding: 2.5rem 2rem; background: #18181b; border-radius: 16px; border: 1px solid #27272a; box-shadow: 0 4px 32px rgba(0,0,0,0.4); max-width: 400px; width: 90%; }
        h1 { margin-top: 0; font-size: 1.5rem; font-weight: 600; letter-spacing: -0.025em; }
        .status { display: inline-flex; align-items: center; gap: 8px; margin: 1rem 0; padding: 8px 16px; background: rgba(34, 197, 94, 0.1); color: #4ade80; border-radius: 999px; font-size: 0.875rem; font-weight: 500; border: 1px solid rgba(34, 197, 94, 0.2); }
        .dot { width: 8px; height: 8px; background-color: #22c55e; border-radius: 50%; box-shadow: 0 0 12px #22c55e; animation: pulse 2s infinite; }
        @keyframes pulse { 0% { opacity: 1; transform: scale(1); } 50% { opacity: 0.5; transform: scale(1.2); } 100% { opacity: 1; transform: scale(1); } }
        p { color: #a1a1aa; margin-bottom: 2rem; font-size: 0.95rem; line-height: 1.5; }
        a { display: inline-block; background: #fafafa; color: #09090b; text-decoration: none; padding: 10px 24px; border-radius: 8px; font-weight: 600; font-size: 0.95rem; transition: background 0.2s; }
        a:hover { background: #e4e4e7; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Media Sequencer</h1>
        <div class="status">
            <div class="dot"></div>
            API is online
        </div>
        <p>This is the backend server. To use the sequencer, please visit the frontend application.</p>
        <a href="https://frontend-drab-nine-hyk0lvk13g.vercel.app">Go to Frontend App</a>
    </div>
</body>
</html>`))
	})

	mux.HandleFunc("GET /api/health", a.Health)

	mux.HandleFunc("GET /api/windows", a.ListWindows)
	mux.HandleFunc("GET /api/windows/{id}", a.GetWindow)
	mux.HandleFunc("GET /api/windows/{id}/now-playing", a.NowPlaying)
	mux.HandleFunc("POST /api/windows/{id}/media", a.withAdminAuth(a.AddMedia))
	mux.HandleFunc("DELETE /api/windows/{id}/media/{mediaId}", a.withAdminAuth(a.DeleteMedia))
	mux.HandleFunc("PUT /api/windows/{id}/media/reorder", a.withAdminAuth(a.ReorderMedia))

	mux.HandleFunc("POST /api/sync", a.withAdminAuth(a.TriggerSync))
	mux.HandleFunc("GET /api/sync/status", a.SyncStatus)

	mux.HandleFunc("GET /ws", a.WS)

	return a.withMiddleware(mux)
}

func (a *API) withMiddleware(h http.Handler) http.Handler {
	return withLogging(a.withCORS(h))
}

func (a *API) withAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.AdminToken != "" {
			token := r.Header.Get("X-Admin-Token")
			if token != a.cfg.AdminToken {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}
		next(w, r)
	}
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
