<script setup>
import { computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAutomationStore } from '../stores/automation'

const route = useRoute()
const router = useRouter()
const automation = useAutomationStore()

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
        </div>
      </n-layout-header>

      <n-layout-content content-style="padding: 24px" :native-scrollbar="false">
        <n-alert
          v-if="automation.paused"
          type="warning"
          style="margin-bottom: 16px"
          title="Automation is paused"
        >
          Scans, backups, and drive detection are paused. Queued jobs will wait until you resume.
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
  color: #f0f0f0;
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
