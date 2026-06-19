import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from './views/DashboardView.vue'
import DrivesView from './views/DrivesView.vue'
import MediaView from './views/MediaView.vue'
import ActivityView from './views/ActivityView.vue'
import RecoveryView from './views/RecoveryView.vue'
import SettingsView from './views/SettingsView.vue'
import LoginView from './views/LoginView.vue'
import SetupView from './views/SetupView.vue'
import { useAuthStore } from './stores/auth'

const routes = [
  // Public routes render without the app shell (no sidebar/header).
  { path: '/login', name: 'login', component: LoginView, meta: { title: 'Sign in', public: true } },
  { path: '/setup', name: 'setup', component: SetupView, meta: { title: 'First-run setup', public: true } },

  { path: '/', name: 'dashboard', component: DashboardView, meta: { title: 'Dashboard' } },
  { path: '/media', name: 'media', component: MediaView, meta: { title: 'Media' } },
  { path: '/drives', name: 'drives', component: DrivesView, meta: { title: 'Drives' } },
  { path: '/activity', name: 'activity', component: ActivityView, meta: { title: 'Activity' } },
  { path: '/recovery', name: 'recovery', component: RecoveryView, meta: { title: 'Recovery' } },
  { path: '/settings', name: 'settings', component: SettingsView, meta: { title: 'Settings' } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Auth gate: force first-run setup, require a session for protected pages, and
// keep authenticated users out of the login/setup pages.
router.beforeEach(async (to) => {
  const auth = useAuthStore()
  try {
    await auth.refresh()
  } catch {
    // If status can't be fetched, fall through; protected calls will 401.
  }

  if (auth.setupRequired) {
    return to.name === 'setup' ? true : { name: 'setup' }
  }

  // Setup already done — no reason to visit the setup page.
  if (to.name === 'setup') {
    return auth.authenticated ? { name: 'dashboard' } : { name: 'login' }
  }

  if (!auth.authenticated && !to.meta.public) {
    return { name: 'login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : undefined }
  }

  if (auth.authenticated && to.name === 'login') {
    return { name: 'dashboard' }
  }

  return true
})

export default router
