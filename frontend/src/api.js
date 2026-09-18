// Thin wrapper around the backend's REST + WebSocket API. Every function
// here does exactly one HTTP call and throws a readable Error on failure,
// so components can stay focused on rendering rather than fetch plumbing.

const API_BASE = (import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080').replace(/\/$/, '')
const WS_BASE = API_BASE.replace(/^http/, 'ws')

async function request(path, options = {}) {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const body = await res.json()
      if (body?.error) message = body.error
    } catch {
      // response wasn't JSON; keep the default status-based message
    }
    throw new Error(message)
  }
  if (res.status === 204) return null
  return res.json()
}

export function fetchWindows() {
  return request('/api/windows')
}

export function fetchWindow(id) {
  return request(`/api/windows/${encodeURIComponent(id)}`)
}

export function addMedia(windowId, { type, url, duration_seconds }) {
  return request(`/api/windows/${encodeURIComponent(windowId)}/media`, {
    method: 'POST',
    body: JSON.stringify({ type, url, duration_seconds }),
  })
}

export function deleteMedia(windowId, mediaId) {
  return request(`/api/windows/${encodeURIComponent(windowId)}/media/${mediaId}`, {
    method: 'DELETE',
  })
}

export function triggerSync({ mediaId, type, url, durationSeconds }) {
  const body = {}
  if (mediaId != null) {
    body.media_id = Number(mediaId)
  } else {
    body.type = type
    body.url = url
  }
  if (durationSeconds) body.duration_seconds = Number(durationSeconds)
  return request('/api/sync', { method: 'POST', body: JSON.stringify(body) })
}

export function fetchSyncStatus() {
  return request('/api/sync/status')
}

// connectSocket opens the WebSocket used for realtime playlist/sync push
// notifications and reconnects automatically (with simple backoff) if the
// connection drops — network blips shouldn't require a page reload.
export function connectSocket(onEvent) {
  let socket
  let closedByCaller = false
  let attempt = 0

  const connect = () => {
    socket = new WebSocket(`${WS_BASE}/ws`)
    socket.onopen = () => {
      attempt = 0
    }
    socket.onmessage = (msg) => {
      try {
        onEvent(JSON.parse(msg.data))
      } catch (err) {
        console.error('failed to parse ws message', err)
      }
    }
    socket.onclose = () => {
      if (closedByCaller) return
      attempt += 1
      const delay = Math.min(1000 * attempt, 8000)
      setTimeout(connect, delay)
    }
    socket.onerror = () => {
      socket.close()
    }
  }

  connect()

  return () => {
    closedByCaller = true
    socket?.close()
  }
}
