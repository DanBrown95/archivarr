<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { api } from '../api'
import { cap, formatTime } from '../util'

const message = useMessage()
const dialog = useDialog()
const jobs = ref([])
const selected = ref(null)
const showDrawer = ref(false)

const statusType = {
  done: 'success',
  failed: 'error',
  running: 'info',
  queued: 'default',
  cancelled: 'warning',
}

const queuedCount = computed(() => jobs.value.filter((j) => j.status === 'queued').length)

function originLabel(o) {
  return o === 'auto' ? 'Scheduled' : 'Manual'
}

function summary(job) {
  const s = job.stats
  if (!s) return ''
  if (job.type === 'backup') {
    let t = `copied ${s.copied}/${s.total}`
    if (s.adopted) t += `, ${s.adopted} adopted`
    if (s.conflicts) t += `, ${s.conflicts} conflicts`
    if (s.failed) t += `, ${s.failed} failed`
    if (s.stoppedFull) t += ' (destination full)'
    return t
  }
  if (job.type === 'scan') {
    return `${s.new} new, ${s.changed} changed, ${s.missing} missing, ${s.hashed} hashed`
  }
  if (job.type === 'import') {
    let t = `imported ${s.imported}`
    if (s.alreadyKnown) t += `, ${s.alreadyKnown} already known`
    if (s.unmatched) t += `, ${s.unmatched} unmatched`
    if (s.missing) t += `, ${s.missing} missing from drive`
    if (s.sizeMismatch) t += `, ${s.sizeMismatch} size mismatch`
    if (s.hashMismatch) t += `, ${s.hashMismatch} hash mismatch`
    return t
  }
  return ''
}

async function load() {
  try {
    jobs.value = (await api.jobs()) || []
    if (selected.value) {
      const fresh = jobs.value.find((j) => j.id === selected.value.id)
      if (fresh) selected.value = fresh
    }
  } catch (e) {
    /* transient; ignore during polling */
  }
}

async function cancel(job) {
  try {
    await api.cancelJob(job.id)
    message.info(`Cancelling job #${job.id}`)
    load()
  } catch (e) {
    message.error(String(e))
  }
}

function confirmClearQueue() {
  dialog.warning({
    title: 'Clear the queue?',
    content: `Cancel all ${queuedCount.value} queued job(s) that haven't started yet. Running jobs are not affected.`,
    positiveText: 'Clear queue',
    negativeText: 'Cancel',
    onPositiveClick: async () => {
      try {
        const r = await api.clearQueue()
        message.info(`Cleared ${r.cancelled} queued job(s)`)
        load()
      } catch (e) {
        message.error(String(e))
      }
    },
  })
}

function openDetail(job) {
  selected.value = job
  showDrawer.value = true
}

let timer
onMounted(() => {
  load()
  timer = setInterval(load, 2000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div class="page">
    <n-space justify="space-between" align="center" style="margin-bottom: 12px">
      <div>
        <h1 class="page-title">Activity</h1>
        <p class="page-subtitle" style="margin: 0">Background jobs — scans, backups, and imports.</p>
      </div>
      <n-space>
        <n-button v-if="queuedCount" type="error" ghost @click="confirmClearQueue">
          Clear queue ({{ queuedCount }})
        </n-button>
        <n-button quaternary @click="load">Refresh</n-button>
      </n-space>
    </n-space>

    <n-card>
      <n-empty v-if="!jobs.length" description="No jobs yet." />
      <n-table v-else :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th style="width: 60px">#</th>
            <th>Type</th>
            <th style="width: 110px">Trigger</th>
            <th>Status</th>
            <th style="width: 180px">Progress</th>
            <th>Summary</th>
            <th>Started</th>
            <th style="text-align: right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="j in jobs" :key="j.id">
            <td>{{ j.id }}</td>
            <td><n-tag size="small" :bordered="false">{{ cap(j.type) }}</n-tag></td>
            <td>
              <n-tag size="small" :bordered="false" :type="j.origin === 'auto' ? 'info' : 'default'">
                {{ originLabel(j.origin) }}
              </n-tag>
            </td>
            <td><n-tag size="small" :type="statusType[j.status] || 'default'">{{ cap(j.status) }}</n-tag></td>
            <td>
              <n-progress
                v-if="j.status === 'running'"
                type="line"
                :percentage="Math.round((j.progress || 0) * 100)"
                :height="8"
                processing
              />
              <span v-else class="muted">{{ j.status === 'done' ? '100%' : '—' }}</span>
            </td>
            <td class="muted">{{ summary(j) }}</td>
            <td class="muted">{{ formatTime(j.startedAt || j.createdAt) }}</td>
            <td>
              <div class="row-actions">
                <n-button size="small" tertiary @click="openDetail(j)">Log</n-button>
                <n-button
                  v-if="j.status === 'running' || j.status === 'queued'"
                  size="small"
                  type="error"
                  ghost
                  @click="cancel(j)"
                >
                  Cancel
                </n-button>
              </div>
            </td>
          </tr>
        </tbody>
      </n-table>
    </n-card>

    <n-drawer v-model:show="showDrawer" :width="520">
      <n-drawer-content v-if="selected" :title="`Job #${selected.id} — ${selected.type}`" closable>
        <n-space vertical>
          <div>
            <n-tag size="small" :type="statusType[selected.status] || 'default'">{{ cap(selected.status) }}</n-tag>
          </div>
          <div v-if="summary(selected)" class="muted">{{ summary(selected) }}</div>
          <n-divider title-placement="left" style="margin: 8px 0">Log</n-divider>
          <pre class="logbox mono">{{ selected.log || '(no output)' }}</pre>
          <n-divider title-placement="left" style="margin: 8px 0">Stats</n-divider>
          <pre class="logbox mono">{{ JSON.stringify(selected.stats || {}, null, 2) }}</pre>
        </n-space>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.logbox {
  background: var(--code-bg);
  border: 1px solid var(--code-border);
  border-radius: 6px;
  padding: 10px;
  max-height: 320px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
