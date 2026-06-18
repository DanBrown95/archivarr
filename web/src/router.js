import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from './views/DashboardView.vue'
import DrivesView from './views/DrivesView.vue'
import MediaView from './views/MediaView.vue'
import ActivityView from './views/ActivityView.vue'
import RecoveryView from './views/RecoveryView.vue'
import SettingsView from './views/SettingsView.vue'

const routes = [
  { path: '/', name: 'dashboard', component: DashboardView, meta: { title: 'Dashboard' } },
  { path: '/media', name: 'media', component: MediaView, meta: { title: 'Media' } },
  { path: '/drives', name: 'drives', component: DrivesView, meta: { title: 'Drives' } },
  { path: '/activity', name: 'activity', component: ActivityView, meta: { title: 'Activity' } },
  { path: '/recovery', name: 'recovery', component: RecoveryView, meta: { title: 'Recovery' } },
  { path: '/settings', name: 'settings', component: SettingsView, meta: { title: 'Settings' } },
]

export default createRouter({
  history: createWebHistory(),
  routes,
})
