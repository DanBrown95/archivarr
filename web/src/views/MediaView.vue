<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
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
      message.warning('Scan still running (is automation paused?). It will appear once it finishes — hit Refresh.')
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
      </div>
      <n-space>
        <n-button type="primary" :loading="scanning" @click="scanSources">Scan sources</n-button>
        <n-button quaternary :loading="loadingMedia" @click="refresh">Refresh</n-button>
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
                <template #suffix><span class="muted">&nbsp;· {{ formatBytes(stats?.totals.pendingBytes) }}</span></template>
              </n-statistic>
            </n-card>
          </n-gi>
          <n-gi span="4 m:1">
            <n-card>
              <div class="muted" style="font-size: 12px; margin-bottom: 6px">Coverage</div>
              <n-progress type="line" :percentage="backedPct" :height="12" />
            </n-card>
          </n-gi>
        </n-grid>

        <n-card>
          <n-space align="center" style="margin-bottom: 12px" :wrap="true">
            <n-select v-model:value="sourceFilter" :options="sourceOptions" style="width: 200px" />
            <n-select v-model:value="statusFilter" :options="statusOptions" style="width: 170px" />
            <n-input
              v-model:value="search"
              placeholder="Search path…"
              clearable
              style="width: 260px"
              @keyup.enter="(page = 1), loadMedia()"
            />
            <n-button @click="(page = 1), loadMedia()">Search</n-button>
          </n-space>

          <n-spin :show="loadingMedia">
            <n-empty v-if="!media.items.length" description="No matching media." />
            <n-table v-else :bordered="false" :single-line="false">
              <thead>
                <tr>
                  <th>Path</th>
                  <th style="width: 100px">Size</th>
                  <th style="width: 140px">Source</th>
                  <th style="width: 110px">Status</th>
                  <th>Backed up to</th>
                  <th style="width: 170px">Last backup</th>
                  <th style="width: 120px; text-align: right">Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="m in media.items" :key="m.id">
                  <td class="mono">{{ m.relPath }}</td>
                  <td class="muted">{{ formatBytes(m.size) }}</td>
                  <td class="muted">{{ m.sourceLabel || '—' }}</td>
                  <td>
                    <n-tag size="small" :type="m.backedUp ? 'success' : 'warning'" :bordered="false">
                      {{ m.backedUp ? 'backed up' : 'not backed up' }}
                    </n-tag>
                  </td>
                  <td>
                    <n-space v-if="m.backups.length" :size="4">
                      <n-tag v-for="b in m.backups" :key="b.driveId" size="small" :bordered="false">{{ b.label }}</n-tag>
                    </n-space>
                    <span v-else class="muted">—</span>
                  </td>
                  <td class="muted">{{ formatTime(m.lastCopiedAt) }}</td>
                  <td>
                    <div style="text-align: right">
                      <n-dropdown trigger="click" :options="destOptions" @select="(k) => backupItem(m, k)">
                        <n-button size="small" type="primary" :disabled="!m.sourceDriveId">Back up ▾</n-button>
                      </n-dropdown>
                    </div>
                  </td>
                </tr>
              </tbody>
            </n-table>
          </n-spin>

          <n-space justify="space-between" align="center" style="margin-top: 12px">
            <span class="muted">{{ media.total }} file(s)</span>
            <n-pagination
              v-model:page="page"
              :page-size="pageSize"
              :item-count="media.total"
              :page-slot="7"
            />
          </n-space>
        </n-card>
      </n-tab-pane>

      <!-- ============ DESTINATIONS ============ -->
      <n-tab-pane name="destinations" tab="Destination drives">
        <n-card>
          <n-empty
            v-if="!stats?.destinations.length"
            description="No destination drives registered yet."
          />
          <n-table v-else :bordered="false" :single-line="false">
            <thead>
              <tr>
                <th>Drive</th>
                <th style="width: 110px">Status</th>
                <th style="width: 120px">Files stored</th>
                <th style="width: 120px">Data stored</th>
                <th style="width: 240px">Capacity</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="d in stats.destinations" :key="d.driveId">
                <td>{{ d.label }}</td>
                <td>
                  <n-tag size="small" :type="d.online ? 'success' : 'default'" :bordered="false">
                    {{ d.online ? 'online' : 'offline' }}
                  </n-tag>
                </td>
                <td>{{ d.files }}</td>
                <td class="muted">{{ formatBytes(d.bytes) }}</td>
                <td>
                  <template v-if="d.capacityBytes">
                    <n-progress type="line" :percentage="usedPercent(d.freeBytes, d.capacityBytes)" :height="8" :show-indicator="false" />
                    <span class="muted mono">{{ formatBytes(d.freeBytes) }} free / {{ formatBytes(d.capacityBytes) }}</span>
                  </template>
                  <span v-else class="muted">—</span>
                </td>
              </tr>
            </tbody>
          </n-table>
        </n-card>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>
