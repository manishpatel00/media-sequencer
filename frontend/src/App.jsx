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
      <header className="app__header" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', padding: '24px 0', borderBottom: '1px solid #27272a', marginBottom: '32px' }}>
        <div style={{ display: 'flex', gap: '16px', alignItems: 'flex-start' }}>
          <div style={{ background: 'linear-gradient(135deg, #6366f1, #a855f7)', padding: '12px', borderRadius: '12px', display: 'flex', alignItems: 'center', justifyContent: 'center', boxShadow: '0 4px 12px rgba(99, 102, 241, 0.3)' }}>
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect>
              <line x1="8" y1="21" x2="16" y2="21"></line>
              <line x1="12" y1="17" x2="12" y2="21"></line>
            </svg>
          </div>
          <div>
            <h1 style={{ margin: '0 0 8px 0', fontSize: '1.75rem', fontWeight: '700', letterSpacing: '-0.025em', color: '#fafafa' }}>Multi-Window Media Sequencer</h1>
            <p className="app__subtitle" style={{ margin: 0, color: '#a1a1aa', fontSize: '0.95rem', maxWidth: '600px', lineHeight: '1.5' }}>
              Each window loops its own 5-hour playlist independently. Trigger a sync to show
              one item across every window at once.
            </p>
          </div>
        </div>
        <span className={`status-pill status-pill--${connectionStatus}`} style={{ marginTop: '8px' }}>
          {connectionStatus === 'connected' ? 'Live System' : 'Connecting…'}
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

      <footer className="app__footer" style={{ marginTop: '48px', paddingTop: '24px', borderTop: '1px solid #27272a', display: 'flex', justifyContent: 'center' }}>
        <a 
          href={import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'} 
          target="_blank" 
          rel="noreferrer"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '8px',
            padding: '8px 16px',
            backgroundColor: '#18181b',
            border: '1px solid #3f3f46',
            borderRadius: '999px',
            color: '#a1a1aa',
            textDecoration: 'none',
            fontSize: '0.875rem',
            transition: 'all 0.2s',
          }}
          onMouseOver={(e) => { e.currentTarget.style.borderColor = '#6366f1'; e.currentTarget.style.color = '#fafafa' }}
          onMouseOut={(e) => { e.currentTarget.style.borderColor = '#3f3f46'; e.currentTarget.style.color = '#a1a1aa' }}
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M5 12h14"></path>
            <path d="M12 5v14"></path>
          </svg>
          Backend API Node: {import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'}
        </a>
      </footer>
    </div>
  )
}
