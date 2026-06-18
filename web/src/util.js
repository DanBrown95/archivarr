export function formatBytes(n) {
  if (n == null) return '—'
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB', 'PB']
  let v = n
  let i = -1
  do {
    v /= 1024
    i++
  } while (v >= 1024 && i < units.length - 1)
  return `${v.toFixed(1)} ${units[i]}`
}

// usedPercent returns the 0..100 used percentage given free and capacity bytes.
export function usedPercent(free, capacity) {
  if (!capacity || free == null) return 0
  return Math.max(0, Math.min(100, Math.round(((capacity - free) / capacity) * 100)))
}

export function formatTime(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d)) return iso
  return d.toLocaleString()
}
