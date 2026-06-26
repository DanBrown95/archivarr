<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { cap, formatBytes, formatTime } from '../util'

const router = useRouter()
const drives = ref([])
const jobs = ref([])
const error = ref(null)

const onlineDrives = computed(() => drives.value.filter((d) => d.online).length)
const sources = computed(() => drives.value.filter((d) => d.role === 'source').length)
const dests = computed(() => drives.value.filter((d) => d.role === 'destination').length)
const destCapacity = computed(() =>
  drives.value
    .filter((d) => d.role === 'destination' && d.online)
    .reduce((a, d) => a + (d.capacityBytes || 0), 0),
)
const destFree = computed(() =>
  drives.value
    .filter((d) => d.role === 'destination' && d.online)
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

const setupSteps = [
  { title: 'Connect your drives', desc: 'Plug in the drive that holds your media and the external drive(s) you back up to.' },
  { title: 'Add your source', desc: 'On the Drives page, add a source pointing at your media folder (e.g. /mnt/Media).' },
  { title: 'Register a destination', desc: 'On Drives, use Discover destinations, plug in a backup drive, and register it — Archivarr writes a small marker so it is recognized at any mount path.' },
  { title: 'Tune what is tracked (optional)', desc: 'In Settings, adjust the include/exclude patterns to control which files are scanned and backed up.' },
  { title: 'Scan the source', desc: 'Run a scan to index your media and detect what needs backing up.' },
  { title: 'Back up', desc: 'Run a backup to copy everything not yet on a destination; it resumes on the next drive when one fills.' },
]
const importSteps = [
  { title: 'Add and scan your source', desc: 'So there is something to match against — add the source on Drives, then run a scan.' },
  { title: 'Register the backup drive', desc: 'On Drives, discover and register the existing backup drive as a destination.' },
  { title: 'Import existing backups', desc: 'Use Import existing on that destination. Files on it that match your source — by path, or by content hash if the drive has an Archivarr snapshot — are recorded as backups so they are not re-copied. Unmatched files are reported, not added.' },
]

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
          <n-progress type="line" style="margin-top: 12px"
            :percentage="destCapacity ? Math.round(((destCapacity - destFree) / destCapacity) * 100) : 0" :height="10"
            :show-indicator="false" />
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
                <span><n-tag size="small" :bordered="false">{{ cap(j.type) }}</n-tag> #{{ j.id }}</span>
                <span class="muted">{{ formatTime(j.finishedAt || j.createdAt) }}</span>
                <n-tag size="small" :type="statusType[j.status] || 'default'">{{ cap(j.status) }}</n-tag>
              </n-space>
            </n-list-item>
          </n-list>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card title="Getting started" style="margin-top: 16px" class="getting-started">
      <n-tabs type="segment" animated>
        <n-tab-pane name="setup" tab="Set up backups">
          <ol class="steps">
            <li v-for="(s, i) in setupSteps" :key="i" class="step">
              <div class="step-num">{{ i + 1 }}</div>
              <div class="step-body">
                <div class="step-title">{{ s.title }}</div>
                <div class="step-desc muted">{{ s.desc }}</div>
              </div>
            </li>
          </ol>
        </n-tab-pane>
        <n-tab-pane name="import" tab="Import an existing drive">
          <ol class="steps">
            <li v-for="(s, i) in importSteps" :key="i" class="step">
              <div class="step-num">{{ i + 1 }}</div>
              <div class="step-body">
                <div class="step-title">{{ s.title }}</div>
                <div class="step-desc muted">{{ s.desc }}</div>
              </div>
            </li>
          </ol>
        </n-tab-pane>
      </n-tabs>
    </n-card>
  </div>
</template>

<style scoped>
.steps {
  list-style: none;
  margin: 8px 0 0;
  padding: 0;
}

.step {
  display: flex;
  gap: 12px;
  padding: 12px 0;
}

.step+.step {
  border-top: 1px solid var(--code-border);
}

.step-num {
  flex: 0 0 auto;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  background: #3b82f6;
}

.step-title {
  font-weight: 600;
  margin-bottom: 2px;
}

.step-desc {
  font-size: 13px;
  line-height: 1.45;
}

/* On phones the two segment tabs ("Set up backups" / "Import an existing drive")
   are too wide to sit side by side, so stack them full-width. The segment's
   sliding capsule is positioned with a horizontal transform that no longer maps
   to a vertical layout, so we hide it and highlight the active tab directly. */
@media (max-width: 640px) {
  .getting-started :deep(.n-tabs--segment-type .n-tabs-rail) {
    flex-direction: column;
    align-items: stretch;
    gap: 4px;
  }

  .getting-started :deep(.n-tabs--segment-type .n-tabs-tab-wrapper) {
    flex-grow: 0;
    flex-basis: auto;
  }

  .getting-started :deep(.n-tabs--segment-type .n-tabs-capsule) {
    display: none;
  }

  .getting-started :deep(.n-tabs--segment-type .n-tabs-tab--active) {
    background-color: var(--n-tab-color-segment);
    box-shadow: 0 1px 3px 0 rgba(0, 0, 0, 0.08);
  }
}
</style>
