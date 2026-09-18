# Multi-Window Media Sequencer with Sync Playback

A full-stack app where multiple display windows continuously loop their own
media playlists, support dynamic playlist edits, and can be **synced** so
every window shows one chosen item at the same moment before resuming their
own sequences.

- **Backend:** Go (standard library `net/http` with Go 1.22 route patterns),
  SQLite (`mattn/go-sqlite3`) for persistence, `gorilla/websocket` for
  real-time push.
- **Frontend:** React (Vite).
- **Storage:** SQLite file on disk (see [Assumptions](#assumptions--tradeoffs)
  for why).

```
media-sequencer/
├── backend/     Go API + WebSocket server, SQLite storage, seed data
├── frontend/    React app (Vite)
└── docker-compose.yml
```

---

## Contents
- [Quick start](#quick-start)
- [How the 5-hour cycle works](#how-the-5-hour-cycle-works)
- [How sync works](#how-sync-works)
- [API documentation](#api-documentation)
- [Configuration reference](#configuration-reference)
- [Deployment](#deployment)
- [Assumptions & tradeoffs](#assumptions--tradeoffs)
- [Testing](#testing)

---

## Quick start

### Option A — Docker Compose (recommended, one command)

Requires Docker + Docker Compose.

```bash
docker compose up --build
```

- Frontend: <http://localhost:5173>
- Backend:  <http://localhost:8080>

The database is seeded automatically on first run with three demo windows
(`W1`, `W2`, `W3`) and sample image/video/blank playlists. Data persists in
a named Docker volume (`backend-data`) across restarts.

### Option B — Run natively

**Backend** (requires Go 1.22+; `mattn/go-sqlite3` needs a C compiler, e.g.
`gcc`, available on the machine — it's present by default on macOS/Linux
dev machines):

```bash
cd backend
cp .env.example .env      # optional, defaults already work
go run ./cmd/server
# -> listening on :8080, seeds demo data into ./data/sequencer.db
```

**Frontend** (requires Node 18+):

```bash
cd frontend
cp .env.example .env.local   # VITE_API_BASE_URL=http://localhost:8080
npm install
npm run dev
# -> http://localhost:5173
```

Open the frontend, and you'll see three windows looping their seeded
playlists, plus two controls at the bottom: **Add media to a window** and
**Sync playback**.

---

## How the 5-hour cycle works

Per the brief: *"The total play size for each window must be treated as 5
hours. Each window should keep playing its configured list again and again
within that cycle."*

Each window has a fixed **cycle anchor** — a timestamp set once, when the
window is created, that never changes (not even during a sync). All
playback is computed as a pure function of `(playlist, anchor, now)`:

1. `elapsed = now - anchor`
2. `cycleElapsed = elapsed mod 5 hours`
3. `posInPlaylist = cycleElapsed mod (sum of the playlist's item durations)`
4. Walk the playlist to find which item `posInPlaylist` falls inside, and
   how far into it we are.

This has a few consequences worth calling out explicitly:

- **The playlist loops continuously** to fill the 5-hour cycle (step 3), so
  a 40-second playlist repeats ~450 times within one 5-hour window — there
  is never a moment where playback "runs out" before restarting.
- **Blank is only shown when explicitly configured** as a playlist item
  (`type: "blank"`). A window with no blank entries never falls back to a
  blank screen mid-cycle — it just keeps looping its real items.
- **The cycle itself restarts at the 5-hour mark.** If the playlist's total
  duration doesn't divide evenly into 5 hours, the item that happens to be
  playing exactly as the 5-hour mark is crossed is cut short, and playback
  restarts from the first item. This keeps every window's 5-hour boundary
  perfectly predictable (useful for testing/verification) at the cost of
  that one boundary item occasionally not finishing — see
  [Assumptions](#assumptions--tradeoffs).
- **Dynamic playlist changes take effect immediately** without losing the
  window's position in time: adding/removing an item just changes what
  `posInPlaylist` resolves to on the next recomputation. The backend
  broadcasts a `playlist_updated` WebSocket event so the frontend refetches
  immediately.

This logic lives in [`backend/internal/sequencer/sequencer.go`](backend/internal/sequencer/sequencer.go)
as a pure, fully unit-tested function (`Resolve`), and is mirrored on the
client in [`frontend/src/sequencer.js`](frontend/src/sequencer.js) so the UI
can schedule its own item-change timers locally instead of polling the
server every second. Both implementations are covered by tests asserting
they agree (see [Testing](#testing)).

---

## How sync works

Per the brief: *"When a user triggers sync for a media item, for example
M2, every window should display M2 at the same time. After the configured
sync duration ends, each window should continue its own normal sequence
without losing its playlist configuration."*

**A sync is a temporary display override, not a change to any window's
playlist or cycle anchor.**

1. The client calls `POST /api/sync` with either an existing media item's
   ID, or an ad-hoc `{type, url}` pair.
2. The server picks a start time slightly in the future
   (`SYNC_LEAD_SECONDS`, default 2s) — **not** "now" — specifically so that
   every connected client has time to receive the broadcast *before* the
   synced start moment arrives.
3. The server broadcasts `{"type":"sync_start","data":{...,"starts_at":...,"ends_at":...}}`
   to every connected WebSocket client immediately.
4. Every frontend client receives the **same absolute `starts_at` /
   `ends_at` timestamps** and schedules two local timers: one to switch to
   the synced item, one to switch back. Because all clients are scheduling
   against the same server-issued timestamps (with a small clock-offset
   correction — see below), every window displays the synced item at
   effectively the same instant, independent of small differences in
   network latency between clients.
5. When the sync's `ends_at` passes, the server also broadcasts
   `sync_end` as a belt-and-braces signal, and each window simply resumes
   showing whatever its own `Resolve(playlist, anchor, now)` says it should
   be showing *right now* — since the anchor was never touched, nothing was
   lost or rewound; the window picks up exactly where its own independent
   cycle already was.

**Clock-offset correction:** every `GET /api/windows` response includes the
server's own `server_time`. The frontend computes
`offset = server_time - Date.now()` once on load and applies it whenever it
schedules a sync timer or resolves current playback, so a client whose
system clock is a few seconds off from the server still shows perfectly
synced windows.

**A synced item need not belong to any window's playlist.** `POST
/api/sync` accepts either `media_id` (look up an existing item, wherever it
lives) or a raw `{type, url}` pair, so you can sync an item that isn't
configured into any window's list at all.

---

## API documentation

Base URL defaults to `http://localhost:8080`. All responses are JSON.
Timestamps are RFC3339 UTC.

### `GET /api/health`
Liveness check. `200 {"status":"ok"}`.

### `GET /api/windows`
List every window with its playlist, server-resolved `now_playing`,
current sync status, and the server's clock. This is the main payload the
frontend loads on startup.

```json
[
  {
    "window": { "id": "W1", "name": "Window 1 — Lobby Display", "cycle_anchor": "...", "created_at": "..." },
    "playlist": [
      { "id": 1, "window_id": "W1", "type": "image", "url": "https://...", "duration_seconds": 8, "position": 0, "created_at": "..." }
    ],
    "now_playing": {
      "media_id": 1, "type": "image", "url": "https://...",
      "index": 0, "elapsed_seconds": 3, "remaining_seconds": 5,
      "next_change_at": "..."
    },
    "server_time": "2026-09-19T10:00:00Z",
    "cycle_duration_seconds": 18000,
    "sync": { "active": false }
  }
]
```

### `GET /api/windows/{id}`
Same shape as one entry above, for a single window. `404` if unknown.

### `GET /api/windows/{id}/now-playing`
Lightweight polling endpoint returning just `{now_playing, server_time, sync}`.

### `POST /api/windows/{id}/media`
Add a media item to the end of a window's playlist (dynamic update).

Request:
```json
{ "type": "image", "url": "https://example.com/pic.jpg", "duration_seconds": 10 }
```
- `type`: `"image"`, `"video"`, or `"blank"`.
- `url`: required for `image`/`video`; ignored (may be omitted) for `blank`.
- `duration_seconds`: required, positive integer.

`201` with the created item, `400` on validation failure, `404` if the
window doesn't exist.

### `DELETE /api/windows/{id}/media/{mediaId}`
Removes an item — **only if `mediaId` actually belongs to `id`**. If it
belongs to a different window, this returns `404` rather than succeeding or
leaking that the ID exists elsewhere (see [ownership
checks](#ownership-checks)). `204` on success.

### `PUT /api/windows/{id}/media/reorder`
Rewrites playlist order. Body: `{ "ordered_ids": [3, 1, 2] }` — must
contain exactly the window's current item IDs (all-or-nothing; a foreign or
partial ID list is rejected without changing anything).

### `POST /api/sync`
Trigger a sync. Either reference an existing item:
```json
{ "media_id": 2, "duration_seconds": 10 }
```
or supply one ad-hoc:
```json
{ "type": "image", "url": "https://example.com/breaking-news.jpg", "duration_seconds": 15 }
```
`duration_seconds` (top-level) is how long the sync stays on screen;
defaults to `DEFAULT_SYNC_DURATION_SECONDS` if omitted. Returns the
scheduled sync state, including its `starts_at`/`ends_at`.

### `GET /api/sync/status`
Current/scheduled sync state, for polling clients that don't use the
WebSocket.

### `GET /ws`
WebSocket endpoint. Sends JSON events:
- `{"type":"playlist_updated","data":{"window_id":"W1"}}`
- `{"type":"sync_start","data":{"active":true,"media_id":2,"type":"video","url":"...","starts_at":"...","ends_at":"..."}}`
- `{"type":"sync_end"}`

### Ownership checks
Every endpoint shaped like `/windows/{windowId}/media/{mediaId}` verifies,
server-side, that `mediaId` actually belongs to `windowId` before reading
or mutating it — see `Store.GetOwnedMedia` / `Store.DeleteMedia` in
[`backend/internal/store/store.go`](backend/internal/store/store.go), and
the regression tests in
[`backend/internal/store/store_test.go`](backend/internal/store/store_test.go)
(`TestOwnershipIsEnforced`) that assert a media item created under one
window is untouchable through another window's endpoints.

---

## Configuration reference

### Backend (env vars, see `backend/.env.example`)
| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP/WebSocket listen port |
| `DB_PATH` | `./data/sequencer.db` | SQLite file path (parent dir auto-created) |
| `ALLOWED_ORIGIN` | `*` | CORS origin allowed to call the API |
| `SEED_ON_EMPTY` | `true` | Insert demo windows/media on first run |
| `SYNC_LEAD_SECONDS` | `2` | Delay before a triggered sync actually starts |
| `DEFAULT_SYNC_DURATION_SECONDS` | `10` | Fallback sync duration if not specified per-request |

### Frontend (env vars, see `frontend/.env.example`)
| Variable | Default | Purpose |
|---|---|---|
| `VITE_API_BASE_URL` | `http://localhost:8080` | Backend base URL (WebSocket URL is derived from it) |

Vite env vars are **compile-time**: for a Docker image, pass
`--build-arg VITE_API_BASE_URL=...`; for most static hosts, set it as a
build-time environment variable in the project's dashboard.

---

## Deployment

Any host that runs a Go binary + persists a file, and any static host, will
work. This repo includes deploy-as-code configs (`render.yaml`,
`frontend/vercel.json`) for the fastest path; manual steps are below too.

### Backend — Render (Blueprint, fastest)
1. Push this repo to GitHub (see [Pushing to GitHub](#pushing-to-github)).
2. Render dashboard → **New → Blueprint** → connect the repo. Render reads
   `render.yaml` at the repo root and provisions the web service, the
   `/app/data` persistent disk, and every env var except `ALLOWED_ORIGIN`
   automatically.
3. Deploy first with `ALLOWED_ORIGIN=*` (or leave it blank), note the
   resulting URL (e.g. `https://media-sequencer-backend.onrender.com`),
   then come back after deploying the frontend and set it to the
   frontend's exact origin (see step 4 below), and redeploy.

<details>
<summary>Manual alternative (no Blueprint)</summary>

1. Render dashboard → New → **Web Service** → connect the repo, root
   directory `backend`, environment **Docker** (it'll find the Dockerfile).
2. Add a **persistent disk** mounted at `/app/data` — this is what makes
   the SQLite file survive redeploys.
3. Set the env vars listed in [Configuration reference](#configuration-reference).
4. Deploy.
</details>

### Frontend — Vercel (fastest)
1. Vercel dashboard → **Add New → Project** → import the same repo, set
   **Root Directory** to `frontend`. Vercel reads `frontend/vercel.json`
   and the Vite framework preset automatically.
2. Add the env var `VITE_API_BASE_URL` = your Render backend URL from
   above.
3. Deploy → note the resulting URL, then go back to Render and set
   `ALLOWED_ORIGIN` to this exact URL (scheme + host, no trailing slash),
   then redeploy the backend so CORS allows it.

<details>
<summary>Or, containerized (e.g. Fly.io, Render static/web service)</summary>

```bash
docker build \
  --build-arg VITE_API_BASE_URL=https://media-sequencer-backend.onrender.com \
  -t media-sequencer-frontend ./frontend
```
then deploy that image, serving on port 80 via the included nginx config.
</details>

### After deploying both
- Confirm CORS: `ALLOWED_ORIGIN` on the backend must exactly match the
  frontend's deployed origin once you move off `*`.
- Confirm the WebSocket path (`wss://<backend-host>/ws`) isn't blocked by
  any proxy in front of the backend — Render/Railway/Fly.io all support
  WebSocket upgrades on their standard HTTP(S) ports without extra config.
- Smoke-test: open the frontend URL, confirm all three windows are
  looping, add a media item, and trigger a sync — it should apply to every
  window within `SYNC_LEAD_SECONDS`.

### Pushing to GitHub
```bash
# from the repo root, after unzipping / cloning locally
git remote add origin https://github.com/<your-username>/media-sequencer.git
git branch -M main
git push -u origin main
```
Use a GitHub Personal Access Token (Settings → Developer settings →
Personal access tokens, `repo` scope) as the password if prompted, or
`gh repo create media-sequencer --public --source=. --push` if you have
the GitHub CLI installed and authenticated (`gh auth login`).

For a fully scripted, end-to-end run of every step above (repo creation,
push, both deployments, and verification), see
[`AGENT_INSTRUCTIONS.md`](AGENT_INSTRUCTIONS.md) — it's written to be
handed directly to an AI coding agent (GitHub Copilot, Google Antigravity,
Claude Code, etc.) with shell/CLI access.

---

## Assumptions & tradeoffs

- **Persistent storage:** SQLite (a single file via `mattn/go-sqlite3`) was
  chosen over a hosted database because it needs no external service to
  stand up, is trivially backed by a persistent volume/disk in any
  container host, and comfortably handles this assignment's scale (a
  handful of windows, tens of media items each). Swapping it for
  Postgres/MySQL later would only mean changing `internal/db` and the SQL
  placeholder syntax in `internal/store` — the rest of the app is
  storage-agnostic.
- **5-hour boundary cuts off the in-progress item.** As explained above,
  when the playlist's total duration doesn't evenly divide 5 hours, the
  item playing exactly at the boundary is cut short so the next cycle
  starts cleanly at item 0. The alternative (let the item finish, then
  restart) would make the "5 hours" a soft minimum rather than a hard,
  predictable boundary — I judged predictability more valuable here, and
  it's isolated to one function (`sequencer.Resolve`) if a different
  behaviour is preferred.
- **Video item duration is admin-supplied, not read from the file.** The
  assignment doesn't require inspecting video metadata, and doing so
  reliably (for arbitrary hosted URLs) is out of scope; `duration_seconds`
  is simply part of the playlist entry, same as for images. A video is
  restarted from 0:00 whenever it becomes the active item and is switched
  away from at its configured duration regardless of its real length.
- **Sync targets are looked up globally, not scoped to one window.** The
  brief's example ("sync M2 across all windows") implies a sync target can
  be any known media item, not only one already in every window's own
  list — `POST /api/sync` accordingly accepts any existing `media_id` *or*
  a completely ad-hoc `{type, url}` pair.
- **No authentication.** The brief doesn't ask for it, and none of the
  deployment guidance above assumes it; if this were a real product, I'd
  add at minimum an admin token on the mutating endpoints
  (`POST`/`DELETE`/`PUT`).
- **CORS defaults to `*`** for ease of local grading; tighten
  `ALLOWED_ORIGIN` in any real deployment (see [Configuration
  reference](#configuration-reference)).
- **WebSocket with polling fallback.** The frontend also polls
  `GET /api/windows` every 25s regardless of WebSocket state, so a dropped
  connection (which also auto-reconnects with backoff) never leaves the UI
  stale for long.
- **Sync catch-up on load/reconnect.** `GET /api/windows` includes each
  window's current `sync` status, and the frontend checks it on every
  fetch (not just on the `sync_start` WebSocket push) — so a browser tab
  opened, or reconnected, in the middle of an already-triggered sync still
  joins it correctly instead of only picking up the *next* one.
- **Production hardening included but kept minimal:** the server performs
  a graceful shutdown on `SIGTERM`/`SIGINT` (finishes in-flight requests
  before exiting — important so container platforms don't kill mid-request
  during a redeploy), and request bodies are capped at 64KB via
  `http.MaxBytesReader` as a defensive limit against oversized payloads.

---

## Testing

Backend:
```bash
cd backend
go test ./...          # unit tests for the cycle math (internal/sequencer)
                        # + ownership-check regression tests (internal/store)
go vet ./...
gofmt -l .              # should print nothing
```

The most important tests to read first, since they cover the two trickiest
correctness requirements in the brief:
- [`backend/internal/sequencer/sequencer_test.go`](backend/internal/sequencer/sequencer_test.go) —
  the 5-hour cycle/loop/restart behaviour.
- [`backend/internal/store/store_test.go`](backend/internal/store/store_test.go) —
  `TestOwnershipIsEnforced`, the cross-window ownership guarantee.

Frontend:
```bash
cd frontend
npm run build           # production build; also catches JSX/type errors
```
