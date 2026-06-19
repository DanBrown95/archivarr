// Thin fetch wrapper around the Archivarr REST API.

const base = '/api'

// onUnauthorized is invoked whenever a protected request comes back 401, so the
// app can bounce the user to the login page. Wired up in main.js.
let onUnauthorized = null
export function setUnauthorizedHandler(fn) {
  onUnauthorized = fn
}

async function req(method, path, body, opts = {}) {
  const reqOpts = { method, headers: {} }
  if (body !== undefined) {
    reqOpts.headers['Content-Type'] = 'application/json'
    reqOpts.body = JSON.stringify(body)
  }
  const res = await fetch(base + path, reqOpts)
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    // A 401 on a normal request means the session is gone — surface it globally.
    // The auth bootstrap calls opt out so a bad-login 401 just shows an error.
    if (res.status === 401 && !opts.skipAuthRedirect && onUnauthorized) {
      onUnauthorized()
    }
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

  // Auth bootstrap — these skip the global 401 redirect.
  authStatus: () => req('GET', '/auth/status', undefined, { skipAuthRedirect: true }),
  setup: (username, password) =>
    req('POST', '/auth/setup', { username, password }, { skipAuthRedirect: true }),
  login: (username, password) =>
    req('POST', '/auth/login', { username, password }, { skipAuthRedirect: true }),
  logout: () => req('POST', '/auth/logout'),
  updateAccount: (b) => req('PUT', '/auth/account', b),
  apiKey: () => req('GET', '/auth/apikey'),
  regenerateApiKey: () => req('POST', '/auth/apikey/regenerate'),

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
