# Agent Instructions: Push & Deploy the Media Sequencer

This file is written to be handed directly to an AI coding agent with
shell access (GitHub Copilot agent mode, Google Antigravity, Claude Code,
Cursor agent, etc.) so it can execute the repo-push and deployment steps
itself. It is a runbook, not marketing copy: every step names its
prerequisite, its command, and how to verify it worked before moving on.
If a step fails, stop and report the exact error rather than guessing at a
fix and continuing.

**Do not fabricate success.** After every command, check its actual exit
code / HTTP response before claiming the step is done.

---

## Prerequisites (human must provide these; do not invent values)

| Needed | How the human gets it | Used for |
|---|---|---|
| GitHub account + `gh` CLI installed | `gh auth login` (interactive, one time) | creating & pushing the repo |
| Render.com account | render.com signup (free tier is enough) | backend deploy |
| Vercel account | vercel.com signup (free tier is enough) | frontend deploy |

If any of these tools/accounts aren't available in the current
environment, stop and ask the human to complete that one prerequisite,
rather than skipping the step silently.

---

## Step 1 — Verify the codebase before touching git or any remote

```bash
cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./... && cd ..
cd frontend && npm install && npm run build && cd ..
```

**Pass condition:** `go build`/`go vet`/`go test` exit 0, `gofmt -l .`
prints nothing, `npm run build` exits 0 and produces `frontend/dist/`.
**If anything fails, stop here** — do not push broken code.

---

## Step 2 — Create the GitHub repository and push

```bash
gh auth status || gh auth login   # confirms/establishes GitHub auth

# Create the repo AND push the existing local commit history in one step:
gh repo create media-sequencer --public --source=. --remote=origin --push
```

**Pass condition:** the command prints a `https://github.com/<user>/media-sequencer`
URL, and `git log --oneline -1` matches what `git log --oneline -1 origin/main`
shows after `git fetch`.

**Verify:**
```bash
git remote -v
gh repo view --web=false --json url -q .url
```

If `gh` isn't available, fall back to the manual two-step:
```bash
# human creates an empty repo at github.com/new (no README/license, so it
# doesn't conflict with this repo's own files), then:
git remote add origin https://github.com/<username>/media-sequencer.git
git branch -M main
git push -u origin main
```

---

## Step 3 — Deploy the backend to Render

Render's Blueprint flow (reads `render.yaml` at the repo root) requires a
one-time browser OAuth to connect the GitHub account — an agent cannot
complete that headlessly. Do this step as a guided handoff:

1. Tell the human: "Open https://dashboard.render.com/blueprints, click
   **New Blueprint Instance**, select the `media-sequencer` repo you just
   pushed. Render will read `render.yaml` and show a preview of one web
   service (`media-sequencer-backend`) with a 1GB disk at `/app/data`.
   Leave `ALLOWED_ORIGIN` blank for now. Click **Apply**."
2. Wait for the human to confirm the deploy finished, then ask them to
   paste the resulting URL (looks like
   `https://media-sequencer-backend.onrender.com`).
3. **Verify it yourself** once you have the URL:
   ```bash
   curl -s https://<render-url>/api/health
   curl -s https://<render-url>/api/windows | head -c 300
   ```
   **Pass condition:** health returns `{"status":"ok"}`, and `/api/windows`
   returns a non-empty JSON array (the seeded W1/W2/W3 windows).

Record `<render-url>` — the next step needs it.

---

## Step 4 — Deploy the frontend to Vercel

```bash
npm install -g vercel   # if the CLI isn't already available
vercel login            # interactive, one time
cd frontend
vercel --prod \
  --build-env VITE_API_BASE_URL=https://<render-url> \
  --env VITE_API_BASE_URL=https://<render-url> \
  --yes
cd ..
```

`vercel --prod` prints the deployed URL on success (looks like
`https://media-sequencer-<hash>.vercel.app` or a custom alias if one is
configured).

**Verify:**
```bash
curl -s -o /dev/null -w "%{http_code}\n" https://<vercel-url>
```
**Pass condition:** `200`.

If the CLI isn't usable in this environment, fall back to the guided
handoff: "Open https://vercel.com/new, import the `media-sequencer` repo,
set **Root Directory** to `frontend`, add env var
`VITE_API_BASE_URL=https://<render-url>`, click Deploy." Then ask for the
resulting URL and verify it the same way.

Record `<vercel-url>` — the next step needs it.

---

## Step 5 — Close the CORS loop

The backend was deployed with `ALLOWED_ORIGIN` blank/`*`. Now that the
frontend's real URL exists, lock it down:

- **Render dashboard:** open the `media-sequencer-backend` service →
  Environment → set `ALLOWED_ORIGIN=https://<vercel-url>` (exact origin,
  no trailing slash) → **Save, rebuild and deploy**.
- Wait for the redeploy to finish, then verify CORS is actually applied:
  ```bash
  curl -s -i -H "Origin: https://<vercel-url>" https://<render-url>/api/windows \
    | grep -i "access-control-allow-origin"
  ```
  **Pass condition:** the header value equals `https://<vercel-url>`.

---

## Step 6 — End-to-end smoke test against the real deployment

```bash
# 1. Windows load and are seeded
curl -s https://<render-url>/api/windows | python3 -c "import json,sys; d=json.load(sys.stdin); print(len(d), 'windows')"

# 2. Add media works
curl -s -X POST https://<render-url>/api/windows/W1/media \
  -H "Content-Type: application/json" \
  -d '{"type":"image","url":"https://picsum.photos/id/237/800/600","duration_seconds":6}'

# 3. Sync works and broadcasts (fire-and-check-status)
curl -s -X POST https://<render-url>/api/sync \
  -H "Content-Type: application/json" \
  -d '{"media_id":1,"duration_seconds":6}'
sleep 3
curl -s https://<render-url>/api/sync/status
```

**Pass condition:** step 1 prints `3 windows`, step 2 returns `201` with
the created item, step 3's first call returns a scheduled sync and the
status call (taken 3s later, inside the sync window) shows `"active":true`.

Then open `https://<vercel-url>` in an actual browser and confirm visually:
windows are looping, the item just added to W1 eventually appears in its
rotation, and triggering a sync from the UI flips all three windows to the
same item within ~`SYNC_LEAD_SECONDS`.

---

## Step 7 — Report back

Summarize for the human, plainly, with the two URLs and the commit hash
that's live:

```
GitHub:   https://github.com/<username>/media-sequencer
Backend:  https://<render-url>
Frontend: https://<vercel-url>
Deployed commit: <git rev-parse HEAD>
All smoke tests: PASS/FAIL (list any FAIL explicitly)
```

Do not mark this complete if any verification step above failed — report
the failing step and its actual output instead.
