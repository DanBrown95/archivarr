<script setup>
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NDropdown, NInput, NProgress, NSpace, NTag, useMessage, useDialog } from 'naive-ui'
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

// Column defs for the main drives table. Computed so the per-row "Back up"
// dropdown reflects which destinations are currently online.
const driveColumns = computed(() => [
  { title: 'Label', key: 'label', minWidth: 140 },
  {
    title: 'Role',
    key: 'role',
    width: 110,
    render: (row) =>
      h(NTag, { size: 'small', type: roleType[row.role] || 'default' }, { default: () => cap(row.role) }),
  },
  {
    title: 'Status',
    key: 'online',
    width: 100,
    render: (row) =>
      h(
        NTag,
        { size: 'small', type: row.online ? 'success' : 'default', bordered: false },
        { default: () => (row.online ? 'Online' : 'Offline') },
      ),
  },
  {
    title: 'Mount / path',
    key: 'path',
    width: 240,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'mono muted' }, row.rootPath || row.lastMountPath || '—'),
  },
  {
    title: 'Capacity',
    key: 'capacity',
    width: 220,
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
  {
    title: 'Last seen',
    key: 'lastSeenAt',
    width: 170,
    render: (row) => h('span', { class: 'muted' }, formatTime(row.lastSeenAt)),
  },
  {
    title: 'Actions',
    key: 'actions',
    width: 260,
    align: 'right',
    render: (row) =>
      h('div', { class: 'row-actions' }, [
        isSource(row)
          ? h(
            NDropdown,
            { trigger: 'click', options: scanOptions, onSelect: (k) => onScan(row, k) },
            { default: () => h(NButton, { size: 'small' }, { default: () => 'Scan ▾' }) },
          )
          : null,
        isSource(row)
          ? h(
            NDropdown,
            { trigger: 'click', options: backupOptions.value, onSelect: (k) => onBackup(row, k) },
            { default: () => h(NButton, { size: 'small', type: 'primary' }, { default: () => 'Back up ▾' }) },
          )
          : null,
        isDest(row) && row.online
          ? h(NButton, { size: 'small', onClick: () => openImport(row) }, { default: () => 'Import existing' })
          : null,
        h(
          NButton,
          { size: 'small', quaternary: true, type: 'error', onClick: () => confirmRemove(row) },
          { default: () => 'Remove' },
        ),
      ]),
  },
])

// Column defs for the discover-destinations table inside its modal.
const discoverColumns = [
  {
    title: 'Path',
    key: 'path',
    minWidth: 200,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'mono' }, row.path),
  },
  {
    title: 'Status',
    key: 'status',
    width: 140,
    render: (row) => {
      if (row.known) {
        return h(
          NTag,
          { size: 'small', type: 'success', bordered: false },
          { default: () => `registered #${row.driveId}` },
        )
      }
      if (row.hasMarker) {
        return h(NTag, { size: 'small', type: 'warning', bordered: false }, { default: () => 'has marker' })
      }
      return h(NTag, { size: 'small', bordered: false }, { default: () => 'unregistered' })
    },
  },
  {
    title: 'Register as',
    key: 'register',
    width: 240,
    align: 'right',
    render: (row) =>
      row.known
        ? h('span', { class: 'muted' }, '—')
        : h(NSpace, { justify: 'end', wrap: false }, {
          default: () => [
            h(NInput, {
              value: labels.value[row.path],
              size: 'small',
              placeholder: 'label',
              style: 'width: 160px',
              'onUpdate:value': (v) => {
                labels.value[row.path] = v
              },
            }),
            h(
              NButton,
              { size: 'small', type: 'primary', onClick: () => registerMount(row) },
              { default: () => 'Register' },
            ),
          ],
        }),
  },
]
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
      <n-data-table :columns="driveColumns" :data="drives" :row-key="(row) => row.id" :loading="loading"
        :bordered="false" :single-line="false" :scroll-x="1240">
        <template #empty>
          <n-empty description="No drives yet — add a source or discover a destination." />
        </template>
      </n-data-table>
    </n-card>

    <!-- Import existing backups modal -->
    <n-modal v-model:show="showImport" preset="card"
      :title="importTarget ? `Import existing backups — ${importTarget.label}` : 'Import existing backups'"
      style="width: 520px; max-width: calc(100vw - 32px)">
      <p class="muted" style="margin-top: 0">
        Registers files already on this destination as existing backups of the source you choose,
        so they aren't re-copied. Files are matched by path — and by content hash when available
        (e.g. from an Archivarr snapshot, which survives a reorganized source). Files that don't
        match the source are reported, not added; nothing is created.
      </p>
      <p class="muted" style="margin-top: 0; font-size: 12px">
        Tip: let Archivarr manage the layout — don't move, rename, or delete files (or the
        <span class="mono">.archivarr</span> folder) on a backup drive yourself, or it can lose
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
    <n-modal v-model:show="showAdd" preset="card" title="Add source drive"
      style="width: 480px; max-width: calc(100vw - 32px)">
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
    <n-modal v-model:show="showDiscover" preset="card" title="Discover destination drives"
      style="width: 640px; max-width: calc(100vw - 32px)">
      <p class="muted">Mount points found under the scan roots. Register one to write its marker and track it.</p>
      <n-data-table :columns="discoverColumns" :data="discovered" :row-key="(row) => row.path"
        :loading="discoverLoading" :bordered="false" :scroll-x="580">
        <template #empty>
          <n-empty description="No mount points found." />
        </template>
      </n-data-table>
    </n-modal>
  </div>
</template>
