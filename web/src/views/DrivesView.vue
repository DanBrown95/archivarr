<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage, useDialog } from 'naive-ui'
import { api } from '../api'
import { cap, formatBytes, formatTime, usedPercent } from '../util'

const router = useRouter()
const message = useMessage()
const dialog = useDialog()

function confirmRemove(d) {
  dialog.error({
    title: `Remove "${d.label}"`,
    content:
      'Permanently removes this drive and its tracking data from Archivarr — backup records, and for a source its media entries. Files already on your physical drives are NOT touched.',
    positiveText: 'Remove',
    negativeText: 'Cancel',
    onPositiveClick: async () => {
      try {
        await api.deleteDrive(d.id)
        message.success('Drive removed')
        await load()
      } catch (e) {
        message.error(String(e))
      }
    },
  })
}

const drives = ref([])
const loading = ref(false)

const onlineDests = computed(() =>
  drives.value.filter((d) => d.role === 'destination' && d.online),
)
const sources = computed(() => drives.value.filter((d) => d.role === 'source'))

const roleType = { source: 'info', destination: 'success' }
const scanOptions = [
  { label: 'Scan (quick)', key: 'scan' },
  { label: 'Scan + hash', key: 'hash' },
]
const backupOptions = computed(() =>
  onlineDests.value.length
    ? onlineDests.value.map((d) => ({ label: `→ ${d.label}`, key: String(d.id) }))
    : [{ label: 'No destinations online', key: 'none', disabled: true }],
)

async function load() {
  loading.value = true
  try {
    drives.value = (await api.drives()) || []
  } catch (e) {
    message.error(String(e))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function isSource(d) {
  return d.role === 'source'
}

function isDest(d) {
  return d.role === 'destination'
}

async function onScan(d, key) {
  try {
    const job = await api.createJob({ type: 'scan', driveId: d.id, hashOnScan: key === 'hash' })
    message.success(`Scan queued (job #${job.id})`)
  } catch (e) {
    message.error(String(e))
  }
}

async function onBackup(source, key) {
  if (key === 'none') return
  const destId = Number(key)
  if (destId === source.id) {
    message.error('Source and destination must differ')
    return
  }
  try {
    const job = await api.createJob({ type: 'backup', sourceDriveId: source.id, destDriveId: destId })
    message.success(`Backup queued (job #${job.id})`)
    router.push('/activity')
  } catch (e) {
    message.error(String(e))
  }
}

/* ---- Import existing backups (scan a destination, register matches) ---- */
const showImport = ref(false)
const importTarget = ref(null)
const importForm = ref({ sourceId: null, verify: false })
const importing = ref(false)
const sourceOptions = computed(() => sources.value.map((s) => ({ label: s.label, value: s.id })))

function openImport(d) {
  importTarget.value = d
  // Default to the sole source if there's exactly one.
  importForm.value = { sourceId: sources.value.length === 1 ? sources.value[0].id : null, verify: false }
  showImport.value = true
}

async function submitImport() {
  if (!importForm.value.sourceId) {
    message.error('Choose which source these backups belong to')
    return
  }
  importing.value = true
  try {
    const job = await api.createJob({
      type: 'import',
      destDriveId: importTarget.value.id,
      sourceDriveId: importForm.value.sourceId,
      verify: importForm.value.verify,
    })
    message.success(`Import queued (job #${job.id})`)
    showImport.value = false
    router.push('/activity')
  } catch (e) {
    message.error(String(e))
  } finally {
    importing.value = false
  }
}

/* ---- Add source ---- */
const showAdd = ref(false)
const addForm = ref({ label: '', rootPath: '' })
async function submitAdd() {
  if (!addForm.value.label || !addForm.value.rootPath) {
    message.error('Label and path are required')
    return
  }
  try {
    await api.createDrive({ label: addForm.value.label, role: 'source', rootPath: addForm.value.rootPath })
    showAdd.value = false
    addForm.value = { label: '', rootPath: '' }
    await load()
    message.success('Source drive added')
  } catch (e) {
    message.error(String(e))
  }
}

/* ---- Discover / register destinations ---- */
const showDiscover = ref(false)
const discovered = ref([])
const discoverLoading = ref(false)
const labels = ref({}) // path -> proposed label

async function openDiscover() {
  showDiscover.value = true
  discoverLoading.value = true
  try {
    discovered.value = (await api.discovered()) || []
  } catch (e) {
    message.error(String(e))
  } finally {
    discoverLoading.value = false
  }
}
async function registerMount(m) {
  const label = labels.value[m.path]
  if (!label) {
    message.error('Enter a label first')
    return
  }
  try {
    await api.register({ path: m.path, label })
    message.success(`Registered ${label}`)
    await Promise.all([openDiscover(), load()])
  } catch (e) {
    message.error(String(e))
  }
}
</script>

<template>
  <div class="page">
    <n-space justify="space-between" align="center" style="margin-bottom: 12px">
      <div>
        <h1 class="page-title">Drives</h1>
        <p class="page-subtitle" style="margin: 0">Sources hold your media; destinations receive backups.</p>
      </div>
      <n-space>
        <n-button @click="openDiscover">Discover destinations</n-button>
        <n-button type="primary" @click="showAdd = true">Add source</n-button>
        <n-button quaternary :loading="loading" @click="load">Refresh</n-button>
      </n-space>
    </n-space>

    <n-card>
      <n-empty v-if="!drives.length && !loading" description="No drives yet — add a source or discover a destination." />
      <n-table v-else :bordered="false" :single-line="false">
        <thead>
          <tr>
            <th>Label</th>
            <th>Role</th>
            <th>Status</th>
            <th>Mount / path</th>
            <th>Capacity</th>
            <th>Last seen</th>
            <th style="text-align: right">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="d in drives" :key="d.id">
            <td>{{ d.label }}</td>
            <td><n-tag size="small" :type="roleType[d.role] || 'default'">{{ cap(d.role) }}</n-tag></td>
            <td>
              <n-tag size="small" :type="d.online ? 'success' : 'default'" :bordered="false">
                {{ d.online ? 'Online' : 'Offline' }}
              </n-tag>
            </td>
            <td class="mono muted">{{ d.rootPath || d.lastMountPath || '—' }}</td>
            <td style="min-width: 160px">
              <template v-if="d.capacityBytes">
                <n-progress
                  type="line"
                  :percentage="usedPercent(d.freeBytes, d.capacityBytes)"
                  :height="8"
                  :show-indicator="false"
                />
                <span class="muted mono">{{ formatBytes(d.freeBytes) }} free / {{ formatBytes(d.capacityBytes) }}</span>
              </template>
              <span v-else class="muted">—</span>
            </td>
            <td class="muted">{{ formatTime(d.lastSeenAt) }}</td>
            <td>
              <div class="row-actions">
                <template v-if="isSource(d)">
                  <n-dropdown trigger="click" :options="scanOptions" @select="(k) => onScan(d, k)">
                    <n-button size="small">Scan ▾</n-button>
                  </n-dropdown>
                  <n-dropdown trigger="click" :options="backupOptions" @select="(k) => onBackup(d, k)">
                    <n-button size="small" type="primary">Back up ▾</n-button>
                  </n-dropdown>
                </template>
                <n-button v-if="isDest(d) && d.online" size="small" @click="openImport(d)">
                  Import existing
                </n-button>
                <n-button size="small" quaternary type="error" @click="confirmRemove(d)">Remove</n-button>
              </div>
            </td>
          </tr>
        </tbody>
      </n-table>
    </n-card>

    <!-- Import existing backups modal -->
    <n-modal
      v-model:show="showImport"
      preset="card"
      :title="importTarget ? `Import existing backups — ${importTarget.label}` : 'Import existing backups'"
      style="width: 520px"
    >
      <p class="muted" style="margin-top: 0">
        Registers files already on this destination as existing backups of the source you choose,
        so they aren't re-copied. Files are matched by path — and by content hash when available
        (e.g. from an Archivarr snapshot, which survives a reorganized source). Files that don't
        match the source are reported, not added; nothing is created.
      </p>
      <p class="muted" style="margin-top: 0; font-size: 12px">
        Tip: let Archivarr manage the layout — don't move, rename, or delete files (or the
        <span class="mono">_backup_meta</span> folder) on a backup drive yourself, or it can lose
        track of what's stored where.
      </p>
      <n-alert v-if="!sourceOptions.length" type="warning" :bordered="false" style="margin-bottom: 12px">
        No source drives yet — add and scan a source first so there's something to match against.
      </n-alert>
      <n-form label-placement="top">
        <n-form-item label="These backups belong to source">
          <n-select v-model:value="importForm.sourceId" :options="sourceOptions" placeholder="Choose a source" />
        </n-form-item>
        <n-form-item label="Verify with hashes">
          <n-switch v-model:value="importForm.verify" />
          <span class="muted" style="margin-left: 12px">
            Filesystem imports only — reads and hashes each file to confirm it matches the source.
            Drives with an Archivarr snapshot use their stored hashes automatically.
          </span>
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showImport = false">Cancel</n-button>
          <n-button type="primary" :loading="importing" :disabled="!sourceOptions.length" @click="submitImport">
            Import
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Add source modal -->
    <n-modal v-model:show="showAdd" preset="card" title="Add source drive" style="width: 480px">
      <n-form>
        <n-form-item label="Label">
          <n-input v-model:value="addForm.label" placeholder="e.g. NAS_Media" />
        </n-form-item>
        <n-form-item label="Root path (inside the container)">
          <n-input v-model:value="addForm.rootPath" placeholder="e.g. /mnt/Media" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAdd = false">Cancel</n-button>
          <n-button type="primary" @click="submitAdd">Add</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Discover destinations modal -->
    <n-modal v-model:show="showDiscover" preset="card" title="Discover destination drives" style="width: 640px">
      <p class="muted">Mount points found under the scan roots. Register one to write its marker and track it.</p>
      <n-spin :show="discoverLoading">
        <n-empty v-if="!discovered.length && !discoverLoading" description="No mount points found." />
        <n-table v-else :bordered="false">
          <thead>
            <tr><th>Path</th><th>Status</th><th style="text-align: right">Register as</th></tr>
          </thead>
          <tbody>
            <tr v-for="m in discovered" :key="m.path">
              <td class="mono">{{ m.path }}</td>
              <td>
                <n-tag v-if="m.known" size="small" type="success" :bordered="false">registered #{{ m.driveId }}</n-tag>
                <n-tag v-else-if="m.hasMarker" size="small" type="warning" :bordered="false">has marker</n-tag>
                <n-tag v-else size="small" :bordered="false">unregistered</n-tag>
              </td>
              <td>
                <n-space v-if="!m.known" justify="end" :wrap="false">
                  <n-input v-model:value="labels[m.path]" size="small" placeholder="label" style="width: 160px" />
                  <n-button size="small" type="primary" @click="registerMount(m)">Register</n-button>
                </n-space>
                <span v-else class="muted">—</span>
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-spin>
    </n-modal>
  </div>
</template>
