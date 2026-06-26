<script setup>
import { computed, h, onMounted, onUnmounted, reactive, ref } from 'vue'
import { NButton, NProgress, NTag, useDialog, useMessage } from 'naive-ui'
import { useBreakpoints } from '@vueuse/core'
import { api } from '../api'
import { breakpoints } from '../breakpoints'
import { cap, formatTime } from '../util'

const message = useMessage()
const dialog = useDialog()
const isMobile = useBreakpoints(breakpoints).smaller('s')
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

// Column defs for the jobs table.
const jobColumns = [
  { title: '#', key: 'id', width: 60 },
  {
    title: 'Type',
    key: 'type',
    width: 100,
    render: (row) => h(NTag, { size: 'small', bordered: false }, { default: () => cap(row.type) }),
  },
  {
    title: 'Trigger',
    key: 'origin',
    width: 120,
    render: (row) =>
      h(
        NTag,
        { size: 'small', bordered: false, type: row.origin === 'auto' ? 'info' : 'default' },
        { default: () => originLabel(row.origin) },
      ),
  },
  {
    title: 'Status',
    key: 'status',
    width: 110,
    render: (row) =>
      h(NTag, { size: 'small', type: statusType[row.status] || 'default' }, { default: () => cap(row.status) }),
  },
  {
    title: 'Progress',
    key: 'progress',
    width: 180,
    render: (row) =>
      row.status === 'running'
        ? h(NProgress, {
          type: 'line',
          percentage: Math.round((row.progress || 0) * 100),
          height: 8,
          processing: true,
        })
        : h('span', { class: 'muted' }, row.status === 'done' ? '100%' : '—'),
  },
  {
    title: 'Summary',
    key: 'summary',
    minWidth: 240,
    render: (row) => h('span', { class: 'muted' }, summary(row)),
  },
  {
    title: 'Started',
    key: 'started',
    width: 170,
    render: (row) => h('span', { class: 'muted' }, formatTime(row.startedAt || row.createdAt)),
  },
  {
    title: 'Actions',
    key: 'actions',
    width: 160,
    align: 'right',
    render: (row) =>
      h('div', { class: 'row-actions' }, [
        h(NButton, { size: 'small', tertiary: true, onClick: () => openDetail(row) }, { default: () => 'Log' }),
        row.status === 'running' || row.status === 'queued'
          ? h(
            NButton,
            { size: 'small', type: 'error', ghost: true, onClick: () => cancel(row) },
            { default: () => 'Cancel' },
          )
          : null,
      ]),
  },
]

// Derived from the columns so the horizontal-scroll threshold can't drift out of
// sync with the column widths.
const jobScrollX = jobColumns.reduce((total, c) => total + (c.width || c.minWidth || 0), 0)

// Client-side pagination for the jobs table. A reactive object (controlled mode)
// so the current page survives the 2s polling refresh instead of resetting.
const pagination = reactive({
  page: 1,
  pageSize: 15,
  showSizePicker: true,
  pageSizes: [15, 30, 50],
  prefix: ({ itemCount }) => `${itemCount} job(s)`,
  onUpdatePage: (p) => {
    pagination.page = p
  },
  onUpdatePageSize: (ps) => {
    pagination.pageSize = ps
    pagination.page = 1
  },
})

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
      <n-data-table :columns="jobColumns" :data="jobs" :row-key="(row) => row.id" :pagination="pagination"
        :bordered="false" :single-line="false" :scroll-x="jobScrollX">
        <template #empty>
          <n-empty description="No jobs yet." />
        </template>
      </n-data-table>
    </n-card>

    <n-drawer v-model:show="showDrawer" :width="isMobile ? '100%' : 520">
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
