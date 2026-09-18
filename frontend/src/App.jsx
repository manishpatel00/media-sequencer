import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchWindows, addMedia, deleteMedia, triggerSync, connectSocket } from './api.js'
import MediaWindow from './components/MediaWindow.jsx'
import AddMediaForm from './components/AddMediaForm.jsx'
import SyncPanel from './components/SyncPanel.jsx'

const POLL_INTERVAL_MS = 25_000

export default function App() {
  const [windows, setWindows] = useState([])
  const [serverOffsetMs, setServerOffsetMs] = useState(0)
  const [syncState, setSyncState] = useState({ active: false })
  const [error, setError] = useState(null)
  const [connectionStatus, setConnectionStatus] = useState('connecting')

  // Mirrors serverOffsetMs in a ref so callbacks created once (the
  // WebSocket message handler, in particular) always read the latest
  // offset instead of the value captured at the time they were created.
  // Using only React state here would cause a stale-closure bug: the
  // socket handler is set up once in the effect below, so if it read
  // `serverOffsetMs` directly it would keep using whatever the offset was
  // on the very first render (0) for the lifetime of the connection.
  const offsetRef = useRef(0)

  // Timers scheduled to flip syncState active/inactive at the exact
  // server-provided instant. Kept in a ref so they can be cleared when a
  // newer sync event supersedes an older one.
  const syncTimers = useRef([])

  // Schedules local timers to flip syncState on/off at the server's
  // starts_at/ends_at, corrected for this client's clock offset. Safe to
  // call both for a fresh "sync_start" push AND for catching up on a sync
  // that was already in progress when this client loaded/reconnected (in
  // which case localStartMs may already be in the past — Math.max(0, ...)
  // below makes that fire immediately rather than "in the past").
  const scheduleSync = useCallback((data, offsetMs) => {
    syncTimers.current.forEach(clearTimeout)
    syncTimers.current = []

    const localStartMs = new Date(data.starts_at).getTime() - offsetMs
    const localEndMs = new Date(data.ends_at).getTime() - offsetMs
    const now = Date.now()

    if (localEndMs <= now) {
      // Already over by the time we learned about it — nothing to do.
      setSyncState({ active: false })
      return
    }

    const activate = () =>
      setSyncState({
        active: true,
        type: data.type,
        url: data.url,
        mediaId: data.media_id,
        endsAt: data.ends_at,
      })
    const deactivate = () => setSyncState({ active: false })

    syncTimers.current.push(setTimeout(activate, Math.max(0, localStartMs - now)))
    syncTimers.current.push(setTimeout(deactivate, Math.max(0, localEndMs - now)))
  }, [])

  const refresh = useCallback(async () => {
    try {
      const data = await fetchWindows()
      setWindows(data)

      let offsetMs = offsetRef.current
      if (data.length > 0) {
        offsetMs = new Date(data[0].server_time).getTime() - Date.now()
        offsetRef.current = offsetMs
        setServerOffsetMs(offsetMs)
      }

      // Catch up on any sync that's already active or scheduled — this
      // matters when a client loads (or reconnects) mid-sync, e.g. a new
      // window opened right after someone else triggered one.
      const inProgress = data.find((w) => w.sync?.active)?.sync
      if (inProgress) {
        scheduleSync(inProgress, offsetMs)
      }

      setError(null)
    } catch (err) {
      setError(`Could not reach backend: ${err.message}`)
    }
  }, [scheduleSync])

  useEffect(() => {
    refresh()
    const poll = setInterval(refresh, POLL_INTERVAL_MS)

    const disconnect = connectSocket((event) => {
      setConnectionStatus('connected')
      if (event.type === 'playlist_updated') {
        refresh()
      } else if (event.type === 'sync_start') {
        scheduleSync(event.data, offsetRef.current)
      } else if (event.type === 'sync_end') {
        syncTimers.current.forEach(clearTimeout)
        syncTimers.current = []
        setSyncState({ active: false })
      }
    })

    return () => {
      clearInterval(poll)
      disconnect()
      syncTimers.current.forEach(clearTimeout)
    }
  }, [refresh, scheduleSync])

  const handleAddMedia = async (windowId, payload) => {
    await addMedia(windowId, payload)
    await refresh() // WS broadcast will also trigger this; refresh now for snappy UI feedback
  }

  const handleRemoveMedia = async (windowId, mediaId) => {
    await deleteMedia(windowId, mediaId)
    await refresh()
  }

  const handleTriggerSync = async ({ mediaId, durationSeconds }) => {
    await triggerSync({ mediaId, durationSeconds })
  }

  return (
    <div className="app">
      <header className="app__header">
        <div>
          <h1>⚡ Multi-Window Media Sequencer</h1>
          <p className="app__subtitle">
            Each window loops its own 5-hour playlist independently. Trigger a sync to show
            one item across every window at once.
          </p>
        </div>
        <span className={`status-pill status-pill--${connectionStatus}`}>
          {connectionStatus === 'connected' ? 'live' : 'connecting…'}
        </span>
      </header>

      {error && <div className="alert">{error}</div>}

      <main className="windows-grid">
        {windows.map((w) => (
          <MediaWindow
            key={w.window.id}
            win={w}
            serverOffsetMs={serverOffsetMs}
            syncState={syncState}
            onRemoveMedia={handleRemoveMedia}
          />
        ))}
        {windows.length === 0 && !error && <p className="muted">Loading windows…</p>}
      </main>

      <section className="controls-grid">
        {windows.length > 0 && <AddMediaForm windows={windows} onAdd={handleAddMedia} />}
        {windows.length > 0 && (
          <SyncPanel windows={windows} syncState={syncState} onTrigger={handleTriggerSync} />
        )}
      </section>

      <footer className="app__footer">
        <span>Backend: {import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'}</span>
      </footer>
    </div>
  )
}
