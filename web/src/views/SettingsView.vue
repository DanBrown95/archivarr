<script setup>
import { onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { api } from '../api'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)

const form = ref({
  scanExclude: [],
  scanIncludeExt: [],
  scanHashOnScan: false,
  scanIntervalMinutes: 0,
})

async function load() {
  loading.value = true
  try {
    const s = await api.getSettings()
    form.value = {
      scanExclude: s.scanExclude || [],
      scanIncludeExt: s.scanIncludeExt || [],
      scanHashOnScan: !!s.scanHashOnScan,
      scanIntervalMinutes: s.scanIntervalMinutes || 0,
    }
  } catch (e) {
    message.error(String(e))
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function save() {
  saving.value = true
  try {
    form.value = await api.saveSettings(form.value)
    message.success('Settings saved')
  } catch (e) {
    message.error(String(e))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page" style="max-width: 760px">
    <h1 class="page-title">Settings</h1>
    <p class="page-subtitle">Control what gets tracked and how often scans run automatically.</p>

    <n-spin :show="loading">
      <n-card title="Scanning" style="margin-bottom: 16px">
        <n-form label-placement="top">
          <n-form-item label="Exclude patterns">
            <div style="width: 100%">
              <n-dynamic-tags v-model:value="form.scanExclude" />
              <div class="muted" style="font-size: 12px; margin-top: 6px">
                Files are skipped if a pattern matches the file name or any folder in its path.
                Examples: <span class="mono">*.nfo</span>, <span class="mono">@eaDir</span>,
                <span class="mono">.DS_Store</span>, <span class="mono">*.tmp</span>.
              </div>
            </div>
          </n-form-item>

          <n-form-item label="Include only these extensions">
            <div style="width: 100%">
              <n-dynamic-tags v-model:value="form.scanIncludeExt" />
              <div class="muted" style="font-size: 12px; margin-top: 6px">
                Leave empty to track all files. Otherwise only these extensions are tracked
                (no dot, e.g. <span class="mono">mkv</span>, <span class="mono">mp4</span>,
                <span class="mono">flac</span>).
              </div>
            </div>
          </n-form-item>
        </n-form>
      </n-card>

      <n-card title="Automatic scans">
        <n-form label-placement="left" label-width="220">
          <n-form-item label="Auto-scan interval (minutes)">
            <n-input-number v-model:value="form.scanIntervalMinutes" :min="0" :step="15" style="width: 160px" />
            <span class="muted" style="margin-left: 12px">0 = disabled. Scans every source on this interval.</span>
          </n-form-item>
          <n-form-item label="Compute hashes on auto-scan">
            <n-switch v-model:value="form.scanHashOnScan" />
            <span class="muted" style="margin-left: 12px">Slower, but enables integrity checks. Manual scans choose this per run.</span>
          </n-form-item>
        </n-form>
      </n-card>

      <n-space justify="end" style="margin-top: 16px">
        <n-button @click="load">Reset</n-button>
        <n-button type="primary" :loading="saving" @click="save">Save settings</n-button>
      </n-space>
    </n-spin>
  </div>
</template>
