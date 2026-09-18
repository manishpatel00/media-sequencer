import { useState } from 'react'

export default function AddMediaForm({ windows, onAdd }) {
  const [windowId, setWindowId] = useState(windows[0]?.window.id ?? '')
  const [type, setType] = useState('image')
  const [url, setUrl] = useState('')
  const [duration, setDuration] = useState(10)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState(null)

  // Keep the selected window valid as the window list loads/changes.
  if (windows.length > 0 && !windows.some((w) => w.window.id === windowId)) {
    setWindowId(windows[0].window.id)
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError(null)
    if (type !== 'blank' && !url.trim()) {
      setError('URL is required for image/video items.')
      return
    }
    if (!duration || Number(duration) <= 0) {
      setError('Duration must be a positive number of seconds.')
      return
    }
    let finalUrl = type === 'blank' ? '' : url.trim()

    if (type === 'video') {
      const isYt = finalUrl.includes('youtube.com') || finalUrl.includes('youtu.be')
      const isVimeo = finalUrl.includes('vimeo.com')
      
      if (isYt) {
        const ytMatch = finalUrl.match(/(?:youtube\.com\/(?:[^\/]+\/.+\/|(?:v|e(?:mbed)?)\/|.*[?&]v=)|youtu\.be\/)([^"&?\/\s]{11})/i)
        if (ytMatch && ytMatch[1]) {
          finalUrl = `https://www.youtube-nocookie.com/embed/${ytMatch[1]}?autoplay=1&mute=1&loop=1&playlist=${ytMatch[1]}`
        } else {
          setError('Paste a direct video file link, or a normal youtube.com/watch or youtu.be link')
          return
        }
      } else if (isVimeo) {
        const vimeoMatch = finalUrl.match(/(?:vimeo\.com\/|player\.vimeo\.com\/video\/)([0-9]+)/i)
        if (vimeoMatch && vimeoMatch[1]) {
          finalUrl = `https://player.vimeo.com/video/${vimeoMatch[1]}?autoplay=1&muted=1&loop=1&autopause=0`
        } else {
          setError('Paste a direct video file link, or a normal vimeo.com link')
          return
        }
      }
    }

    setBusy(true)
    try {
      await onAdd(windowId, {
        type,
        url: finalUrl,
        duration_seconds: Number(duration),
      })
      setUrl('')
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="panel" onSubmit={handleSubmit}>
      <h3>Add media to a window</h3>
      <div className="field-row">
        <label>
          Window
          <select value={windowId} onChange={(e) => setWindowId(e.target.value)}>
            {windows.map((w) => (
              <option key={w.window.id} value={w.window.id}>
                {w.window.id}: {w.window.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Type
          <select value={type} onChange={(e) => setType(e.target.value)}>
            <option value="image">image</option>
            <option value="video">video</option>
            <option value="blank">blank</option>
          </select>
        </label>
      </div>

      {type !== 'blank' && (
        <label className="field-block">
          Media URL
          <input
            type="text"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://example.com/media.jpg"
          />
        </label>
      )}

      <label className="field-block">
        Duration (seconds)
        <input
          type="number"
          min="1"
          value={duration}
          onChange={(e) => setDuration(e.target.value)}
        />
      </label>

      {error && <p className="form-error">{error}</p>}

      <button type="submit" className="btn btn--primary" disabled={busy}>
        {busy ? 'Adding…' : 'Add to playlist'}
      </button>
    </form>
  )
}
