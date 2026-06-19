import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api'

export const useAuthStore = defineStore('auth', () => {
  const ready = ref(false) // status has been fetched at least once
  const authenticated = ref(false)
  const setupRequired = ref(false)
  const username = ref('')

  function apply(s) {
    authenticated.value = !!s.authenticated
    setupRequired.value = !!s.setupRequired
    username.value = s.username || ''
  }

  // refresh fetches the current auth status. Cached after the first call via
  // `ready`; pass force to re-fetch.
  async function refresh(force = false) {
    if (ready.value && !force) return
    apply(await api.authStatus())
    ready.value = true
  }

  async function setup(user, password) {
    const r = await api.setup(user, password)
    authenticated.value = true
    setupRequired.value = false
    username.value = r.username
  }

  async function login(user, password) {
    const r = await api.login(user, password)
    authenticated.value = true
    setupRequired.value = false
    username.value = r.username
  }

  async function logout() {
    try {
      await api.logout()
    } finally {
      markLoggedOut()
    }
  }

  // markLoggedOut clears local auth state without an API call (used when the
  // server reports the session is gone).
  function markLoggedOut() {
    authenticated.value = false
    username.value = ''
  }

  return { ready, authenticated, setupRequired, username, refresh, setup, login, logout, markLoggedOut }
})
