<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { formatBytes, formatTime } from '../util'

const router = useRouter()
const drives = ref([])
const jobs = ref([])
const error = ref(null)

const onlineDrives = computed(() => drives.value.filter((d) => d.online).length)
const sources = computed(() => drives.value.filter((d) => d.role === 'source' || d.role === 'both').length)
const dests = computed(() => drives.value.filter((d) => d.role === 'destination' || d.role === 'both').length)
const destCapacity = computed(() =>
  drives.value
    .filter((d) => (d.role === 'destination' || d.role === 'both') && d.online)
    .reduce((a, d) => a + (d.capacityBytes || 0), 0),
)
const destFree = computed(() =>
  drives.value
    .filter((d) => (d.role === 'destination' || d.role === 'both') && d.online)
    .reduce((a, d) => a + (d.freeBytes || 0), 0),
)
const recentJobs = computed(() => jobs.value.slice(0, 6))
const activeJobs = computed(() => jobs.value.filter((j) => j.status === 'running' || j.status === 'queued').length)

const statusType = {
  done: 'success',
  failed: 'error',
  running: 'info',
  queued: 'default',
  cancelled: 'warning',
}

async function load() {
  try {
    const [d, j] = await Promise.all([api.drives(), api.jobs()])
    drives.value = d || []
    jobs.value = j || []
    error.value = null
  } catch (e) {
    error.value = String(e)
  }
}
onMounted(load)
</script>

<template>
  <div class="page">
    <h1 class="page-title">Dashboard</h1>
    <p class="page-subtitle">Backup state across your drives — visible even when they're unplugged.</p>

    <n-alert v-if="error" type="error" style="margin-bottom: 16px">{{ error }}</n-alert>

    <n-grid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
      <n-gi span="4 m:1">
        <n-card><n-statistic label="Drives online" :value="onlineDrives">
          <template #suffix><span class="muted"> / {{ drives.length }}</span></template>
        </n-statistic></n-card>
      </n-gi>
      <n-gi span="4 m:1">
        <n-card><n-statistic label="Source drives" :value="sources" /></n-card>
      </n-gi>
      <n-gi span="4 m:1">
        <n-card><n-statistic label="Destination drives" :value="dests" /></n-card>
      </n-gi>
      <n-gi span="4 m:1">
        <n-card><n-statistic label="Active jobs" :value="activeJobs" /></n-card>
      </n-gi>
    </n-grid>

    <n-grid :cols="2" :x-gap="16" :y-gap="16" responsive="screen" item-responsive style="margin-top: 16px">
      <n-gi span="2 m:1">
        <n-card title="Destination capacity (online)">
          <n-statistic label="Free">
            {{ formatBytes(destFree) }}
            <template #suffix><span class="muted"> / {{ formatBytes(destCapacity) }}</span></template>
          </n-statistic>
          <n-progress
            type="line"
            style="margin-top: 12px"
            :percentage="destCapacity ? Math.round(((destCapacity - destFree) / destCapacity) * 100) : 0"
            :height="10"
            :show-indicator="false"
          />
        </n-card>
      </n-gi>

      <n-gi span="2 m:1">
        <n-card title="Recent activity">
          <template #header-extra>
            <n-button text type="primary" @click="router.push('/activity')">View all</n-button>
          </template>
          <n-empty v-if="!recentJobs.length" description="No jobs yet" />
          <n-list v-else>
            <n-list-item v-for="j in recentJobs" :key="j.id">
              <n-space align="center" justify="space-between" style="width: 100%">
                <span><n-tag size="small" :bordered="false">{{ j.type }}</n-tag> #{{ j.id }}</span>
                <span class="muted">{{ formatTime(j.finishedAt || j.createdAt) }}</span>
                <n-tag size="small" :type="statusType[j.status] || 'default'">{{ j.status }}</n-tag>
              </n-space>
            </n-list-item>
          </n-list>
        </n-card>
      </n-gi>
    </n-grid>
  </div>
</template>
