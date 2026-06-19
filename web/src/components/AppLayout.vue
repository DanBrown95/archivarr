<script setup>
import { computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAutomationStore } from '../stores/automation'
import { useAuthStore } from '../stores/auth'
import { useThemeStore } from '../stores/theme'

const route = useRoute()
const router = useRouter()
const automation = useAutomationStore()
const auth = useAuthStore()
const theme = useThemeStore()

const userOptions = [
  { label: 'Account settings', key: 'account' },
  { label: 'Sign out', key: 'logout' },
]
async function onUser(key) {
  if (key === 'account') {
    router.push('/settings')
  } else if (key === 'logout') {
    await auth.logout()
    router.push({ name: 'login' })
  }
}

const menuOptions = [
  { label: 'Dashboard', key: '/' },
  { label: 'Media', key: '/media' },
  { label: 'Drives', key: '/drives' },
  { label: 'Activity', key: '/activity' },
  { label: 'Recovery', key: '/recovery' },
  { label: 'Settings', key: '/settings' },
]
const selectedKey = computed(() => route.path)
function onMenu(key) {
  if (key !== route.path) router.push(key)
}

const pauseOptions = [
  { label: 'Pause 15 minutes', key: '900' },
  { label: 'Pause 1 hour', key: '3600' },
  { label: 'Pause 8 hours', key: '28800' },
  { label: 'Pause indefinitely', key: '0' },
]
function onPause(key) {
  const secs = Number(key)
  automation.pause(secs > 0 ? secs : undefined)
}

let timer
onMounted(() => {
  automation.load().catch(() => {})
  timer = setInterval(() => automation.load().catch(() => {}), 5000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <n-layout has-sider style="height: 100vh">
    <n-layout-sider bordered :width="220" :native-scrollbar="false" content-style="padding-top: 12px">
      <div class="brand">
        <img src="/favicon.svg" alt="" class="brand-mark" />
        <span class="brand-text">Archi<span class="brand-accent">varr</span></span>
      </div>
      <n-menu :value="selectedKey" :options="menuOptions" @update:value="onMenu" />
    </n-layout-sider>

    <n-layout>
      <n-layout-header bordered class="header">
        <div class="header-title">{{ route.meta.title || 'Archivarr' }}</div>
        <div class="header-right">
          <template v-if="automation.paused">
            <n-tag type="warning" size="small" round>Automation paused</n-tag>
            <n-button size="small" type="primary" @click="automation.resume()">Resume</n-button>
          </template>
          <template v-else>
            <n-dropdown trigger="click" :options="pauseOptions" @select="onPause">
              <n-button size="small" tertiary>Pause automation</n-button>
            </n-dropdown>
          </template>
          <n-tooltip>
            <template #trigger>
              <n-button
                size="small"
                tertiary
                circle
                :aria-label="theme.isDark ? 'Switch to light mode' : 'Switch to dark mode'"
                @click="theme.toggle()"
              >
                <template #icon>
                  <n-icon>
                    <!-- Sun when dark (tap → light), moon when light (tap → dark). -->
                    <svg v-if="theme.isDark" viewBox="0 0 24 24" width="1em" height="1em">
                      <path
                        fill="currentColor"
                        d="M12 7a5 5 0 1 0 0 10a5 5 0 0 0 0-10m0-5a1 1 0 0 1 1 1v1a1 1 0 1 1-2 0V3a1 1 0 0 1 1-1m0 17a1 1 0 0 1 1 1v1a1 1 0 1 1-2 0v-1a1 1 0 0 1 1-1M4 11a1 1 0 1 1 0 2H3a1 1 0 1 1 0-2zm17 0a1 1 0 1 1 0 2h-1a1 1 0 1 1 0-2zM5.64 4.22l.7.71A1 1 0 0 1 4.93 6.34l-.71-.7a1 1 0 0 1 1.42-1.42m12.02 12.02l.71.7a1 1 0 0 1-1.42 1.42l-.7-.71a1 1 0 0 1 1.41-1.41M18.36 4.22a1 1 0 0 1 0 1.42l-.7.7a1 1 0 1 1-1.42-1.41l.71-.71a1 1 0 0 1 1.41 0M6.34 16.24a1 1 0 0 1 0 1.42l-.71.7a1 1 0 0 1-1.42-1.41l.7-.71a1 1 0 0 1 1.43 0"
                      />
                    </svg>
                    <svg v-else viewBox="0 0 24 24" width="1em" height="1em">
                      <path
                        fill="currentColor"
                        d="M12 3a9 9 0 1 0 9 9c0-.46-.04-.92-.1-1.36a5.39 5.39 0 0 1-4.4 2.26a5.4 5.4 0 0 1-4.4-8.51A9 9 0 0 0 12 3"
                      />
                    </svg>
                  </n-icon>
                </template>
              </n-button>
            </template>
            {{ theme.isDark ? 'Switch to light mode' : 'Switch to dark mode' }}
          </n-tooltip>
          <n-dropdown trigger="click" :options="userOptions" @select="onUser">
            <n-button size="small" tertiary>
              <template #icon>
                <n-icon><svg viewBox="0 0 24 24" width="1em" height="1em"><path fill="currentColor" d="M12 12a5 5 0 1 0 0-10a5 5 0 0 0 0 10m0 2c-4.42 0-8 2.69-8 6v2h16v-2c0-3.31-3.58-6-8-6"/></svg></n-icon>
              </template>
              {{ auth.username || 'Account' }}
            </n-button>
          </n-dropdown>
        </div>
      </n-layout-header>

      <n-layout-content content-style="padding: 24px" :native-scrollbar="false">
        <n-alert
          v-if="automation.paused"
          type="warning"
          style="margin-bottom: 16px"
          title="Automation is paused"
        >
          Scheduled scans and backups won't run while paused. Scans and backups you start
          manually still run, and drive detection stays active.
        </n-alert>
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<style scoped>
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 20px 18px;
}
.brand-mark {
  width: 28px;
  height: 28px;
}
.brand-text {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.4px;
  color: var(--brand-text);
}
.brand-accent {
  color: #3b82f6;
}
.header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
}
.header-title {
  font-size: 16px;
  font-weight: 600;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}
</style>
