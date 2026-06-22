<script setup>
import { computed, onMounted, ref } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { api } from '../api'
import { useAuthStore } from '../stores/auth'

const message = useMessage()
const dialog = useDialog()
const auth = useAuthStore()
const loading = ref(false)
const saving = ref(false)

// --- Account (username / password) ---
const account = ref({ username: '', currentPassword: '', newPassword: '', confirm: '' })
const savingAccount = ref(false)

// --- API key ---
const apiKey = ref('')
const apiKeyRevealed = ref(false)
const regeneratingKey = ref(false)

async function loadApiKey() {
  try {
    const r = await api.apiKey()
    apiKey.value = r.apiKey
  } catch (e) {
    message.error(String(e).replace(/^Error:\s*/, ''))
  }
}

async function copyApiKey() {
  try {
    await navigator.clipboard.writeText(apiKey.value)
    message.success('API key copied')
  } catch {
    message.error('Could not copy to clipboard')
  }
}

function confirmRegenerateKey() {
  dialog.warning({
    title: 'Regenerate API key?',
    content:
      'The current key stops working immediately. Any dashboards, scripts, or ' +
      'monitors using it will need to be updated with the new key.',
    positiveText: 'Regenerate',
    negativeText: 'Cancel',
    onPositiveClick: async () => {
      regeneratingKey.value = true
      try {
        const r = await api.regenerateApiKey()
        apiKey.value = r.apiKey
        apiKeyRevealed.value = true
        message.success('API key regenerated')
      } catch (e) {
        message.error(String(e).replace(/^Error:\s*/, ''))
      } finally {
        regeneratingKey.value = false
      }
    },
  })
}

onMounted(() => {
  account.value.username = auth.username
  loadApiKey()
})

const accountPwTooShort = computed(
  () => account.value.newPassword.length > 0 && account.value.newPassword.length < 8,
)
const accountMismatch = computed(
  () => account.value.confirm.length > 0 && account.value.confirm !== account.value.newPassword,
)

async function saveAccount() {
  if (!account.value.currentPassword) {
    message.warning('Enter your current password to confirm changes')
    return
  }
  if (account.value.newPassword && account.value.newPassword.length < 8) {
    message.warning('New password must be at least 8 characters')
    return
  }
  if (account.value.newPassword && account.value.newPassword !== account.value.confirm) {
    message.warning('New passwords do not match')
    return
  }
  savingAccount.value = true
  try {
    const r = await api.updateAccount({
      username: account.value.username,
      currentPassword: account.value.currentPassword,
      newPassword: account.value.newPassword || undefined,
    })
    auth.username = r.username
    account.value.currentPassword = ''
    account.value.newPassword = ''
    account.value.confirm = ''
    message.success('Account updated')
  } catch (e) {
    message.error(String(e).replace(/^Error:\s*/, ''))
  } finally {
    savingAccount.value = false
  }
}

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

// --- Import legacy backup_tracking.db ---
const importFile = ref('backup_tracking.db')
const importDryRun = ref(true)
const importing = ref(false)
const importResult = ref(null)

async function runImport() {
  if (!importFile.value.trim()) {
    message.warning('Enter the file name (it must live in your /config directory)')
    return
  }
  importing.value = true
  importResult.value = null
  try {
    const r = await api.importLegacy({
      file: importFile.value.trim(),
      dryRun: importDryRun.value,
    })
    importResult.value = r
    const s = r.stats
    message.success(
      r.dryRun
        ? `Dry run: ${s.backupsInserted} backup(s) would be recorded`
        : `Imported ${s.backupsInserted} backup(s)`,
    )
  } catch (e) {
    message.error(String(e).replace(/^Error:\s*/, ''))
  } finally {
    importing.value = false
  }
}
</script>

<template>
  <div class="page" style="max-width: 760px">
    <h1 class="page-title">Settings</h1>
    <p class="page-subtitle">Control what gets tracked and how often scans run automatically.</p>

    <n-card title="Account" style="margin-bottom: 16px">
      <n-form label-placement="top" @submit.prevent="saveAccount">
        <n-form-item label="Username">
          <n-input v-model:value="account.username" placeholder="Username" />
        </n-form-item>
        <n-form-item label="Current password" required>
          <n-input
            v-model:value="account.currentPassword"
            type="password"
            show-password-on="click"
            placeholder="Required to confirm any change"
          />
        </n-form-item>
        <n-form-item
          label="New password"
          :validation-status="accountPwTooShort ? 'error' : undefined"
          :feedback="accountPwTooShort ? 'At least 8 characters' : undefined"
        >
          <n-input
            v-model:value="account.newPassword"
            type="password"
            show-password-on="click"
            placeholder="Leave blank to keep current password"
          />
        </n-form-item>
        <n-form-item
          label="Confirm new password"
          :validation-status="accountMismatch ? 'error' : undefined"
          :feedback="accountMismatch ? 'Passwords do not match' : undefined"
        >
          <n-input
            v-model:value="account.confirm"
            type="password"
            show-password-on="click"
            placeholder="Re-enter new password"
          />
        </n-form-item>
        <n-space justify="end">
          <n-button type="primary" :loading="savingAccount" @click="saveAccount">Update account</n-button>
        </n-space>
      </n-form>
    </n-card>

    <n-card title="API key" style="margin-bottom: 16px">
      <p class="muted" style="margin-top: 0">
        For automation and dashboards (e.g. Homepage/Homarr widgets) or scripts that trigger jobs.
        Send it in the <span class="mono">X-Api-Key</span> header (or
        <span class="mono">Authorization: Bearer &lt;key&gt;</span>). Keep it secret — it grants full
        access to your data. Liveness monitoring doesn't need it; use the public
        <span class="mono">/api/health</span> endpoint.
      </p>
      <n-input-group>
        <n-input
          :value="apiKey"
          :type="apiKeyRevealed ? 'text' : 'password'"
          readonly
          placeholder="Loading…"
          class="mono"
        />
        <n-button tertiary @click="apiKeyRevealed = !apiKeyRevealed">
          {{ apiKeyRevealed ? 'Hide' : 'Reveal' }}
        </n-button>
        <n-button tertiary :disabled="!apiKey" @click="copyApiKey">Copy</n-button>
        <n-button type="warning" :loading="regeneratingKey" @click="confirmRegenerateKey">
          Regenerate
        </n-button>
      </n-input-group>
    </n-card>

    <n-card title="Import legacy backup data" style="margin-bottom: 16px">
      <p class="muted" style="margin-top: 0">
        Import the old backup script's <span class="mono">backup_tracking.db</span> so previously
        backed-up files show as covered instead of pending. Place the file in your
        <span class="mono">/config</span> directory and enter its name below. Destination drives are
        created automatically from the file's labels; rows attach to your source drive.
        <strong>Scan your source first</strong>, then run a dry run before the real import.
      </p>
      <n-form label-placement="top">
        <n-form-item label="File name (in /config)">
          <n-input v-model:value="importFile" placeholder="backup_tracking.db" class="mono" />
        </n-form-item>
        <n-form-item label="Dry run (preview only, writes nothing)">
          <n-switch v-model:value="importDryRun" />
        </n-form-item>
        <n-space justify="end">
          <n-button type="primary" :loading="importing" @click="runImport">
            {{ importDryRun ? 'Preview import' : 'Run import' }}
          </n-button>
        </n-space>
      </n-form>

      <n-alert
        v-if="importResult"
        :type="importResult.dryRun ? 'info' : 'success'"
        :title="importResult.dryRun ? 'Dry run — nothing written' : 'Import complete'"
        style="margin-top: 12px"
      >
        <div class="mono" style="font-size: 13px; line-height: 1.6">
          Lines: {{ importResult.stats.linesTotal }}
          (parsed {{ importResult.stats.parsed }},
          skipped {{ importResult.stats.skipped }},
          errors {{ importResult.stats.errors }})<br />
          Destination drives created: {{ importResult.stats.destDrivesCreated }}<br />
          Media items: {{ importResult.stats.mediaMatched }} matched,
          {{ importResult.stats.mediaCreated }} created<br />
          Backups: {{ importResult.stats.backupsInserted }}
          {{ importResult.dryRun ? 'would be recorded' : 'recorded' }},
          {{ importResult.stats.backupsDuplicate }} already present
        </div>
      </n-alert>
    </n-card>

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
