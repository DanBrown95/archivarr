import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

const STORAGE_KEY = 'archivarr-theme'

// initialMode resolves the startup theme: a saved choice wins, otherwise we
// follow the OS preference, defaulting to dark (the app's original look).
function initialMode() {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved === 'light' || saved === 'dark') return saved
  if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) {
    return 'light'
  }
  return 'dark'
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref(initialMode())
  const isDark = computed(() => mode.value === 'dark')

  function set(next) {
    mode.value = next === 'light' ? 'light' : 'dark'
    localStorage.setItem(STORAGE_KEY, mode.value)
  }
  function toggle() {
    set(isDark.value ? 'light' : 'dark')
  }

  return { mode, isDark, set, toggle }
})
