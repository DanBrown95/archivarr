<script setup>
import { computed, onMounted, ref } from 'vue'
import { useMessage, useDialog } from 'naive-ui'
import { api } from '../api'
import { formatBytes } from '../util'

const message = useMessage()
const dialog = useDialog()

const drives = ref([])
const stats = ref(null)

const sourceDrives = computed(() => drives.value.filter((d) => d.role === 'source' || d.role === 'both'))
const destDrives = computed(() => drives.value.filter((d) => d.role === 'destination' || d.role === 'both'))
const driveOptions = (list) => list.map((d) => ({ label: d.label, value: d.id }))

async function loadDrives() {
  try {
    drives.value = (await api.drives()) || []
    stats.value = await api.stats()
  } catch (e) {
    message.error(String(e))
  }
}
onMounted(loadDrives)

/* ---- Source failure ---- */
const selectedSource = ref(null)
const report = ref(null)
const reportLoading = ref(false)
async function runReport() {
  if (!selectedSource.value) return
  reportLoading.value = true
  try {
    report.value = await api.sourceRecovery(selectedSource.value)
  } catch (e) {
    message.error(String(e))
  } finally {
    reportLoading.value = false
  }
}

/* ---- Destination failure ---- */
const selectedDest = ref(null)
const destInfo = computed(() => stats.value?.destinations.find((d) => d.driveId === selectedDest.value) || null)

function confirmRequeue() {
  if (!selectedDest.value) return
  const info = destInfo.value
  dialog.warning({
    title: 'Re-queue destination for backup',
    content: `Remove the backup records for "${info?.label}" (${info?.files ?? 0} file(s))? Those files will become "not backed up" again so you can copy them to a replacement drive. Nothing is deleted from any drive.`,
    positiveText: 'Re-queue',
    negativeText: 'Cancel',
    onPositiveClick: async () => {
      try {
        const res = await api.requeueDestination(selectedDest.value)
        message.success(`Re-queued ${res.removed} file(s)`)
        await loadDrives()
      } catch (e) {
        message.error(String(e))
      }
    },
  })
}

function confirmRemove(list) {
  const id = list === 'source' ? selectedSource.value : selectedDest.value
  const d = drives.value.find((x) => x.id === id)
  if (!d) {
    message.warning('Select a drive first')
    return
  }
  dialog.error({
    title: `Remove drive "${d.label}"`,
    content:
      'This permanently removes the drive and its tracking data from Archivarr (backup records, and for a source its media entries). Files already on physical drives are NOT touched. Continue?',
    positiveText: 'Remove',
    negativeText: 'Cancel',
    onPositiveClick: async () => {
      try {
        await api.deleteDrive(id)
        message.success('Drive removed')
        report.value = null
        selectedSource.value = null
        selectedDest.value = null
        await loadDrives()
      } catch (e) {
        message.error(String(e))
      }
    },
  })
}
</script>

<template>
  <div class="page">
    <h1 class="page-title">Recovery</h1>
    <p class="page-subtitle">Plan for — and recover from — a failed drive.</p>

    <n-tabs type="line" animated>
      <!-- ============ SOURCE FAILURE ============ -->
      <n-tab-pane name="source" tab="Source drive failed">
        <n-card title="What was on a source drive, and where are the copies?">
          <n-space align="center">
            <n-select
              v-model:value="selectedSource"
              :options="driveOptions(sourceDrives)"
              placeholder="Select source drive"
              style="width: 240px"
            />
            <n-button type="primary" :loading="reportLoading" :disabled="!selectedSource" @click="runReport">
              Generate report
            </n-button>
            <n-button quaternary :disabled="!selectedSource" @click="confirmRemove('source')">Remove drive…</n-button>
          </n-space>

          <template v-if="report">
            <n-grid :cols="3" :x-gap="16" :y-gap="16" style="margin-top: 20px">
              <n-gi><n-card><n-statistic label="Files tracked" :value="report.totalTracked" /></n-card></n-gi>
              <n-gi
                ><n-card
                  ><n-statistic label="Recoverable" :value="report.recoverableFiles"
                    ><template #suffix><span class="muted">&nbsp;· {{ formatBytes(report.recoverableBytes) }}</span></template></n-statistic
                  ></n-card
                ></n-gi
              >
              <n-gi>
                <n-card>
                  <n-statistic label="Lost (no backup)" :value="report.lostFiles">
                    <template #suffix><span class="muted">&nbsp;· {{ formatBytes(report.lostBytes) }}</span></template>
                  </n-statistic>
                </n-card>
              </n-gi>
            </n-grid>

            <n-divider title-placement="left">Where to recover from</n-divider>
            <n-alert v-if="!report.perDestination.length" type="warning">
              No backups exist for this source — nothing can be recovered.
            </n-alert>
            <n-table v-else :bordered="false" :single-line="false">
              <thead>
                <tr><th>Plug in this drive</th><th>Files to restore</th><th>Data</th></tr>
              </thead>
              <tbody>
                <tr v-for="d in report.perDestination" :key="d.driveId">
                  <td>{{ d.label }}</td>
                  <td>{{ d.files }}</td>
                  <td class="muted">{{ formatBytes(d.bytes) }}</td>
                </tr>
              </tbody>
            </n-table>

            <n-divider title-placement="left">Lost files (no backup anywhere)</n-divider>
            <n-alert v-if="!report.lost.length" type="success">
              Everything on this source is backed up — nothing is lost.
            </n-alert>
            <pre v-else class="lostbox mono">{{ report.lost.map((l) => l.relPath).join('\n') }}</pre>
          </template>
        </n-card>
      </n-tab-pane>

      <!-- ============ DESTINATION FAILURE ============ -->
      <n-tab-pane name="destination" tab="Destination drive failed">
        <n-card title="A backup drive died — re-queue its files">
          <n-space align="center">
            <n-select
              v-model:value="selectedDest"
              :options="driveOptions(destDrives)"
              placeholder="Select destination drive"
              style="width: 240px"
            />
            <n-button type="warning" :disabled="!selectedDest" @click="confirmRequeue">Re-queue for backup</n-button>
            <n-button quaternary :disabled="!selectedDest" @click="confirmRemove('dest')">Remove drive…</n-button>
          </n-space>

          <n-descriptions v-if="destInfo" label-placement="left" :columns="2" style="margin-top: 20px" bordered>
            <n-descriptions-item label="Files stored">{{ destInfo.files }}</n-descriptions-item>
            <n-descriptions-item label="Data stored">{{ formatBytes(destInfo.bytes) }}</n-descriptions-item>
            <n-descriptions-item label="Status">{{ destInfo.online ? 'online' : 'offline' }}</n-descriptions-item>
          </n-descriptions>

          <n-alert type="info" style="margin-top: 16px">
            Re-queuing removes Archivarr's record of these files being on this drive, so they show as
            "not backed up" again and can be copied to a replacement. It never deletes data from any drive.
          </n-alert>
        </n-card>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<style scoped>
.lostbox {
  background: #18181c;
  border: 1px solid #2a2a30;
  border-radius: 6px;
  padding: 10px;
  max-height: 300px;
  overflow: auto;
  white-space: pre-wrap;
}
</style>
