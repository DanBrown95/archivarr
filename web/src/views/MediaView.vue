<script setup>
import { computed, h, onMounted, ref, watch } from 'vue'
import { NButton, NDropdown, NProgress, NSpace, NTag, useMessage } from 'naive-ui'
import { api } from '../api'
import { formatBytes, formatTime, usedPercent } from '../util'

const message = useMessage()

const stats = ref(null)
const media = ref({ total: 0, limit: 50, offset: 0, items: [] })
const loadingMedia = ref(false)
const scanning = ref(false)

// Filters
const sourceFilter = ref(null) // driveId or null
const statusFilter = ref('all') // all | backed | pending
const search = ref('')
const page = ref(1)
const pageSize = 50

const statusOptions = [
  { label: 'All', value: 'all' },
  { label: 'Backed up', value: 'backed' },
  { label: 'Not backed up', value: 'pending' },
]
const sourceOptions = computed(() => {
  const opts = [{ label: 'All sources', value: null }]
  for (const s of stats.value?.sources || []) opts.push({ label: s.label, value: s.driveId })
  return opts
})

const destOptions = computed(() => {
  const online = (stats.value?.destinations || []).filter((d) => d.online)
  return online.length
    ? online.map((d) => ({ label: `→ ${d.label}`, key: String(d.driveId) }))
    : [{ label: 'No destinations online', key: 'none', disabled: true }]
})

const backedPct = computed(() => {
  const t = stats.value?.totals
  if (!t || !t.files) return 0
  return Math.round((t.backedFiles / t.files) * 100)
})

// Whether any filter/search is narrowing the list (drives the empty-state copy).
const filtersActive = computed(
  () => !!search.value || statusFilter.value !== 'all' || sourceFilter.value !== null,
)

async function loadStats() {
  try {
    stats.value = await api.stats()
  } catch (e) {
    message.error(String(e))
  }
}

async function loadMedia() {
  loadingMedia.value = true
  try {
    media.value = await api.media({
      sourceDriveId: sourceFilter.value ?? '',
      status: statusFilter.value === 'all' ? '' : statusFilter.value,
      q: search.value,
      limit: pageSize,
      offset: (page.value - 1) * pageSize,
    })
  } catch (e) {
    message.error(String(e))
  } finally {
    loadingMedia.value = false
  }
}

function refresh() {
  loadStats()
  loadMedia()
}

async function waitForJob(id, tries = 120) {
  for (let i = 0; i < tries; i++) {
    const j = await api.job(id)
    if (['done', 'failed', 'cancelled'].includes(j.status)) return j
    await new Promise((r) => setTimeout(r, 500))
  }
  return null // still running after the wait window
}

// scanSources rescans the selected source (or all sources), waits for the
// scan(s) to finish, then reloads the list. This reads the disk, unlike Refresh.
async function scanSources() {
  const targets = sourceFilter.value
    ? [sourceFilter.value]
    : (stats.value?.sources || []).map((s) => s.driveId)
  if (!targets.length) {
    message.warning('No source drives to scan — add one on the Drives page first.')
    return
  }
  scanning.value = true
  try {
    const ids = []
    for (const id of targets) {
      const job = await api.createJob({ type: 'scan', driveId: id })
      ids.push(job.id)
    }
    let timedOut = false
    for (const id of ids) {
      if (!(await waitForJob(id))) timedOut = true
    }
    await Promise.all([loadStats(), loadMedia()])
    if (timedOut) {
      message.warning('Scan is still running in the background. Select Refresh to see results once it finishes.')
    } else {
      message.success('Scan complete')
    }
  } catch (e) {
    message.error(String(e))
  } finally {
    scanning.value = false
  }
}

// Back up a single file to a chosen destination.
async function backupItem(m, key) {
  if (key === 'none') return
  if (!m.sourceDriveId) {
    message.error('This file has no source drive')
    return
  }
  try {
    const job = await api.createJob({
      type: 'backup',
      sourceDriveId: m.sourceDriveId,
      destDriveId: Number(key),
      mediaItemIds: [m.id],
    })
    message.success(`Backing up "${m.relPath}" (job #${job.id})`)
    await waitForJob(job.id)
    await Promise.all([loadStats(), loadMedia()])
  } catch (e) {
    message.error(String(e))
  }
}

const coverageColor = computed(() => {
  const p = backedPct.value
  if (p == 100) return 'green'
  if (p >= 50) return 'orange'
  return 'red'
})

// Column defs for the n-data-table render of the source-media list. Computed so
// the per-row "Back up" dropdown picks up changes to the online destinations.
const mediaColumns = computed(() => [
  {
    title: 'Path',
    key: 'relPath',
    width: 320,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'mono' }, row.relPath),
  },
  {
    title: 'Size',
    key: 'size',
    width: 110,
    render: (row) => h('span', { class: 'muted' }, formatBytes(row.size)),
  },
  {
    title: 'Source',
    key: 'sourceLabel',
    width: 150,
    render: (row) => h('span', { class: 'muted' }, row.sourceLabel || '—'),
  },
  {
    title: 'Status',
    key: 'backedUp',
    width: 130,
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.backedUp ? 'success' : 'warning', bordered: false },
        { default: () => (row.backedUp ? 'Backed up' : 'Not backed up') },
      ),
  },
  {
    title: 'Backed up to',
    key: 'backups',
    width: 200,
    render: (row) =>
      row.backups.length
        ? h(
          NSpace,
          { size: 4 },
          {
            default: () =>
              row.backups.map((b) =>
                h(NTag, { key: b.driveId, size: 'small', bordered: false }, { default: () => b.label }),
              ),
          },
        )
        : h('span', { class: 'muted' }, '—'),
  },
  {
    title: 'Last backup',
    key: 'lastCopiedAt',
    width: 180,
    render: (row) => h('span', { class: 'muted' }, formatTime(row.lastCopiedAt)),
  },
  {
    title: 'Actions',
    key: 'actions',
    width: 130,
    align: 'right',
    render: (row) =>
      h(
        NDropdown,
        { trigger: 'click', options: destOptions.value, onSelect: (k) => backupItem(row, k) },
        {
          default: () =>
            h(
              NButton,
              { size: 'small', type: 'primary', disabled: !row.sourceDriveId },
              { default: () => 'Back up ▾' },
            ),
        },
      ),
  },
])

// Column defs for the destinations table.
const destColumns = [
  { title: 'Drive', key: 'label', minWidth: 160 },
  {
    title: 'Status',
    key: 'online',
    width: 110,
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.online ? 'success' : 'default', bordered: false },
        { default: () => (row.online ? 'online' : 'offline') },
      ),
  },
  { title: 'Files stored', key: 'files', width: 120 },
  {
    title: 'Data stored',
    key: 'bytes',
    width: 120,
    render: (row) => h('span', { class: 'muted' }, formatBytes(row.bytes)),
  },
  {
    title: 'Capacity',
    key: 'capacity',
    width: 260,
    render: (row) =>
      row.capacityBytes
        ? h('div', null, [
          h(NProgress, {
            type: 'line',
            percentage: usedPercent(row.freeBytes, row.capacityBytes),
            height: 8,
            showIndicator: false,
          }),
          h(
            'span',
            { class: 'muted mono' },
            `${formatBytes(row.freeBytes)} free / ${formatBytes(row.capacityBytes)}`,
          ),
        ])
        : h('span', { class: 'muted' }, '—'),
  },
]

watch([sourceFilter, statusFilter], () => {
  page.value = 1
  loadMedia()
})
watch(page, loadMedia)

onMounted(() => {
  loadStats()
  loadMedia()
})
</script>

<template>
  <div class="page">
    <n-space justify="space-between" align="center" style="margin-bottom: 12px">
      <div>
        <h1 class="page-title">Media</h1>
        <p class="page-subtitle" style="margin: 0">Backup coverage across your source library.</p>
        <p class="muted" style="margin: 4px 0 0; font-size: 12px">
          Last scan: {{ stats?.lastScanAt ? formatTime(stats.lastScanAt) : 'never' }}
        </p>
      </div>
      <n-space>
        <n-tooltip>
          <template #trigger>
            <n-button type="primary" :loading="scanning" @click="scanSources">Scan sources</n-button>
          </template>
          Re-read your source drives from disk to pick up new, changed, or removed files.
        </n-tooltip>
        <n-tooltip>
          <template #trigger>
            <n-button quaternary :loading="loadingMedia" @click="refresh">Refresh</n-button>
          </template>
          Reload this list from the database (fast — doesn't read the disk).
        </n-tooltip>
      </n-space>
    </n-space>

    <n-tabs type="line" animated>
      <!-- ============ SOURCE MEDIA ============ -->
      <n-tab-pane name="media" tab="Source media">
        <n-grid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive style="margin-bottom: 16px">
          <n-gi span="4 m:1">
            <n-card><n-statistic label="Total files" :value="stats?.totals.files ?? 0">
                <template #suffix><span class="muted">&nbsp;· {{ formatBytes(stats?.totals.bytes) }}</span></template>
              </n-statistic></n-card>
          </n-gi>
          <n-gi span="4 m:1">
            <n-card><n-statistic label="Backed up" :value="stats?.totals.backedFiles ?? 0">
                <template #suffix><span class="muted">&nbsp;· {{ backedPct }}%</span></template>
              </n-statistic></n-card>
          </n-gi>
          <n-gi span="4 m:1">
            <n-card>
              <n-statistic label="Not backed up" :value="stats?.totals.pendingFiles ?? 0">
                <template #suffix><span class="muted">&nbsp;· {{ formatBytes(stats?.totals.pendingBytes)
                    }}</span></template>
              </n-statistic>
            </n-card>
          </n-gi>
          <n-gi span="4 m:1">
            <n-card>
              <div class="muted" style="font-size: 12px; margin-bottom: 6px">Coverage</div>
              <n-progress type="line" :percentage="backedPct" :color="coverageColor" :height="12" />
            </n-card>
          </n-gi>
        </n-grid>

        <n-card>
          <n-grid :cols="24" :x-gap="12" :y-gap="12" responsive="screen" item-responsive style="margin-bottom: 12px">
            <n-gi span="24 s:12 l:6">
              <n-select v-model:value="sourceFilter" :options="sourceOptions" placeholder="Source" />
            </n-gi>
            <n-gi span="24 s:12 l:6">
              <n-select v-model:value="statusFilter" :options="statusOptions" placeholder="Status" />
            </n-gi>
            <n-gi span="24 s:18 l:8">
              <n-input v-model:value="search" placeholder="Search path…" clearable
                @keyup.enter="(page = 1), loadMedia()" />
            </n-gi>
            <n-gi span="24 s:6 l:4">
              <n-button block @click="(page = 1), loadMedia()">Search</n-button>
            </n-gi>
          </n-grid>

          <n-data-table :columns="mediaColumns" :data="media.items" :row-key="(row) => row.id" :loading="loadingMedia"
            :bordered="false" :single-line="false" :scroll-x="1220" :pagination="false">
            <template #empty>
              <n-empty :description="filtersActive
                ? 'No media matches your filters.'
                : 'No media tracked yet — add a source on the Drives page, then run Scan sources.'
                " />
            </template>
          </n-data-table>

          <n-space justify="space-between" align="center" style="margin-top: 12px">
            <span class="muted">{{ media.total }} file(s)</span>
            <n-pagination v-model:page="page" :page-size="pageSize" :item-count="media.total" :page-slot="7" />
          </n-space>
        </n-card>
      </n-tab-pane>

      <!-- ============ DESTINATIONS ============ -->
      <n-tab-pane name="destinations" tab="Destination drives">
        <n-card>
          <n-data-table :columns="destColumns" :data="stats?.destinations || []" :row-key="(row) => row.driveId"
            :bordered="false" :single-line="false" :scroll-x="770">
            <template #empty>
              <n-empty description="No destination drives registered yet." />
            </template>
          </n-data-table>
        </n-card>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>
