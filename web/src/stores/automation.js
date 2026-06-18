import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '../api'

export const useAutomationStore = defineStore('automation', () => {
  const paused = ref(false)
  const pausedUntil = ref(null)

  function apply(s) {
    paused.value = !!s.paused
    pausedUntil.value = s.pausedUntil || null
  }

  async function load() {
    apply(await api.automation())
  }
  async function pause(seconds) {
    apply(await api.pause(seconds))
  }
  async function resume() {
    apply(await api.resume())
  }

  return { paused, pausedUntil, load, pause, resume }
})
