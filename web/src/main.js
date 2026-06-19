import { createApp } from 'vue'
import { createPinia } from 'pinia'
import naive from 'naive-ui'
import App from './App.vue'
import router from './router'
import { setUnauthorizedHandler } from './api'
import { useAuthStore } from './stores/auth'
import './styles.css'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(naive)

// When any protected request 401s, drop local auth state and return to login.
const auth = useAuthStore()
setUnauthorizedHandler(() => {
  auth.markLoggedOut()
  const name = router.currentRoute.value.name
  if (name !== 'login' && name !== 'setup') {
    const redirect = router.currentRoute.value.fullPath
    router.push({ name: 'login', query: redirect !== '/' ? { redirect } : undefined })
  }
})

app.mount('#app')
