import { useEffect, useState } from 'react'
import { resolvePlayback } from '../sequencer.js'

function formatSeconds(ms) {
  const s = Math.max(0, Math.ceil(ms / 1000))
  const m = Math.floor(s / 60)
  const rem = s % 60
  return m > 0 ? `${m}m ${rem}s` : `${rem}s`
}

/**
 * Renders a single display window: it works out — purely from the window's
 * fixed cycle_anchor and its current playlist — what should be on screen
 * right now, shows it, and reschedules itself for the exact moment the
 * next item is due. No polling is needed for smooth playback; the
 * WebSocket (handled by the parent) only needs to tell us when the
 * playlist itself changes or a sync starts/ends.
 */
export default function MediaWindow({ win, serverOffsetMs, syncState, onRemoveMedia }) {
  const [current, setCurrent] = useState(null)

  useEffect(() => {
    let timer
    const anchorMs = new Date(win.window.cycle_anchor).getTime()

    const tick = () => {
      const now = Date.now() + serverOffsetMs
      const resolved = resolvePlayback(win.playlist, anchorMs, now)
      setCurrent(resolved)
      if (resolved) {
        // Re-resolve slightly after the item is due to change rather than
        // exactly at the boundary, and never busy-loop on a zero delay.
        const delay = Math.max(200, resolved.remainingMs + 30)
        timer = setTimeout(tick, delay)
      }
    }
    tick()
    return () => clearTimeout(timer)
    // win.playlist is a new array reference whenever the backend tells us
    // the playlist changed, which is exactly when we want to re-resolve.
  }, [win.playlist, win.window.cycle_anchor, serverOffsetMs])

  const isSyncing = Boolean(syncState?.active)
  const displayItem = isSyncing ? { type: syncState.type, url: syncState.url } : current?.item

  return (
    <section className="window-card">
      <header className="window-card__header">
        <div>
          <h2>{win.window.name}</h2>
          <span className="window-card__id">{win.window.id}</span>
        </div>
        {isSyncing ? (
          <span className="badge badge--sync">SYNCED</span>
        ) : (
          current && (
            <span className="badge">
              item {current.index + 1}/{win.playlist.length} · next in {formatSeconds(current.remainingMs)}
            </span>
          )
        )}
      </header>

      <div className="window-card__stage">
        <MediaStage item={displayItem} />
      </div>

      <ul className="window-card__playlist">
        {win.playlist.length === 0 && <li className="muted">No media configured yet.</li>}
        {win.playlist.map((m, idx) => (
          <li key={m.id} className={current?.item?.id === m.id && !isSyncing ? 'is-active' : ''}>
            <span className="playlist-index">{idx + 1}</span>
            <span className="playlist-type">{m.type}</span>
            <span className="playlist-url" title={m.url}>
              {m.type === 'blank' ? '(blank)' : m.url}
            </span>
            <span className="playlist-duration">{m.duration_seconds}s</span>
            <button
              type="button"
              className="icon-btn"
              aria-label={`Remove item ${idx + 1}`}
              onClick={() => onRemoveMedia(win.window.id, m.id)}
            >
              ✕
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}

function MediaStage({ item }) {
  if (!item) {
    return <div className="stage stage--blank">No playable media</div>
  }
  if (item.type === 'blank') {
    return <div className="stage stage--blank">(blank)</div>
  }
  if (item.type === 'image') {
    return (
      <div className="stage">
        <img src={item.url} alt="" className="stage__media" />
      </div>
    )
  }
  if (item.type === 'video') {
    return (
      <div className="stage">
        {/* key=url forces the <video> element to remount (and therefore
            restart) whenever the active item changes to a different URL. */}
        <video
          key={item.url}
          src={item.url}
          className="stage__media"
          autoPlay
          muted
          loop
          playsInline
        />
      </div>
    )
  }
  return <div className="stage stage--blank">Unsupported media type</div>
}
