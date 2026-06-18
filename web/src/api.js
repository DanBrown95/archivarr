// Thin fetch wrapper around the Archivarr REST API.

const base = '/api'

async function req(method, path, body) {
  const opts = { method, headers: {} }
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(base + path, opts)
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new Error((data && data.error) || `HTTP ${res.status}`)
  }
  return data
}

function qs(params) {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params || {})) {
    if (v !== undefined && v !== null && v !== '') p.set(k, v)
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

export const api = {
  health: () => req('GET', '/health'),

  stats: () => req('GET', '/stats'),
  media: (params) => req('GET', '/media' + qs(params)),

  drives: () => req('GET', '/drives'),
  drive: (id) => req('GET', `/drives/${id}`),
  createDrive: (b) => req('POST', '/drives', b),
  deleteDrive: (id) => req('DELETE', `/drives/${id}`),
  discovered: () => req('GET', '/drives/discovered'),
  register: (b) => req('POST', '/drives/register', b),

  sourceRecovery: (id) => req('GET', `/recovery/source/${id}`),
  requeueDestination: (id) => req('POST', `/recovery/destination/${id}`),

  getSettings: () => req('GET', '/settings'),
  saveSettings: (b) => req('PUT', '/settings', b),

  jobs: () => req('GET', '/jobs'),
  job: (id) => req('GET', `/jobs/${id}`),
  createJob: (b) => req('POST', '/jobs', b),
  cancelJob: (id) => req('DELETE', `/jobs/${id}`),

  automation: () => req('GET', '/automation'),
  pause: (seconds) => req('POST', '/automation/pause', seconds ? { seconds } : {}),
  resume: () => req('POST', '/automation/resume'),
}
