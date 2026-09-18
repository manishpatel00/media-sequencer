import { useState } from 'react'

export default function SyncPanel({ windows, syncState, onTrigger }) {
  const allMedia = windows.flatMap((w) =>
    w.playlist.map((m) => ({ ...m, windowLabel: w.window.id }))
  )
  const [mediaId, setMediaId] = useState('')
  const [duration, setDuration] = useState(10)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(null)

  if (allMedia.length > 0 && !mediaId) {
    setMediaId(String(allMedia[0].id))
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError(null)
    if (!mediaId) {
      setError('Add at least one media item first.')
      return
    }
    setBusy(true)
    try {
      await onTrigger({ mediaId, durationSeconds: Number(duration) })
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="panel" onSubmit={handleSubmit}>
      <h3>Sync playback</h3>
      <p className="panel__hint">
        Pick any configured media item. Every window will switch to it at the same
        moment, then return to its own sequence automatically.
      </p>

      <label className="field-block">
        Media item
        <select value={mediaId} onChange={(e) => setMediaId(e.target.value)}>
          {allMedia.map((m) => (
            <option key={m.id} value={m.id}>
              {m.windowLabel} · #{m.id} · {m.type} {m.type !== 'blank' ? `(${shortUrl(m.url)})` : ''}
            </option>
          ))}
        </select>
      </label>

      <label className="field-block">
        Sync duration (seconds)
        <input
          type="number"
          min="1"
          value={duration}
          onChange={(e) => setDuration(e.target.value)}
        />
      </label>

      {error && <p className="form-error">{error}</p>}

      <button type="submit" className="btn btn--accent" disabled={busy}>
        {busy ? 'Syncing…' : 'Sync all windows now'}
      </button>

      {syncState?.active && (
        <p className="panel__status">
          Live sync active · ends {new Date(syncState.endsAt).toLocaleTimeString()}
        </p>
      )}
    </form>
  )
}

function shortUrl(url) {
  try {
    const u = new URL(url)
    return u.pathname.split('/').pop() || u.hostname
  } catch {
    return url
  }
}
