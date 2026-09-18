// A small client-side mirror of the backend's internal/sequencer.Resolve
// logic (see backend/internal/sequencer/sequencer.go for the authoritative
// version + its unit tests). Keeping the same algorithm on both sides lets
// the frontend schedule item changes locally — via setTimeout, recomputed
// from the window's fixed cycle_anchor every time — instead of polling the
// backend every second just to know when to flip images. The backend's
// own `now_playing` field is still used as the source of truth on initial
// load; this function is what keeps playback moving smoothly in between
// fetches.
export const CYCLE_DURATION_MS = 5 * 60 * 60 * 1000

/**
 * @param {Array<{id:number,type:string,url:string,duration_seconds:number}>} items
 * @param {number} anchorMs - epoch ms of the window's cycle_anchor
 * @param {number} nowMs - epoch ms to resolve playback for
 * @returns {{item:object,index:number,elapsedMs:number,remainingMs:number,nextChangeAtMs:number}|null}
 */
export function resolvePlayback(items, anchorMs, nowMs) {
  const playable = items.filter((it) => it.duration_seconds > 0)
  if (playable.length === 0) return null

  const totalMs = playable.reduce((sum, it) => sum + it.duration_seconds * 1000, 0)
  if (totalMs <= 0) return null

  let elapsed = nowMs - anchorMs
  if (elapsed < 0) elapsed = 0

  const cycleElapsed = elapsed % CYCLE_DURATION_MS
  const posInPlaylist = cycleElapsed % totalMs

  let walked = 0
  for (let i = 0; i < items.length; i += 1) {
    const it = items[i]
    if (it.duration_seconds <= 0) continue
    const durMs = it.duration_seconds * 1000
    const itemEnd = walked + durMs
    if (posInPlaylist < itemEnd) {
      const elapsedMs = posInPlaylist - walked
      const itemRemainingMs = durMs - elapsedMs
      const cycleBoundaryRemainingMs = CYCLE_DURATION_MS - cycleElapsed
      const remainingMs = Math.min(itemRemainingMs, cycleBoundaryRemainingMs)
      return {
        item: it,
        index: i,
        elapsedMs,
        remainingMs,
        nextChangeAtMs: nowMs + remainingMs,
      }
    }
    walked = itemEnd
  }
  return null
}
