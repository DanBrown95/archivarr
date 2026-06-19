<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const message = useMessage()

const username = ref('')
const password = ref('')
const confirm = ref('')
const loading = ref(false)

const passwordTooShort = computed(() => password.value.length > 0 && password.value.length < 8)
const mismatch = computed(() => confirm.value.length > 0 && confirm.value !== password.value)

async function submit() {
  if (loading.value) return // guard against duplicate submissions
  if (!username.value) {
    message.warning('Choose a username')
    return
  }
  if (password.value.length < 8) {
    message.warning('Password must be at least 8 characters')
    return
  }
  if (password.value !== confirm.value) {
    message.warning('Passwords do not match')
    return
  }
  loading.value = true
  try {
    await auth.setup(username.value, password.value)
    message.success('Admin account created')
    router.push('/')
  } catch (e) {
    message.error(String(e).replace(/^Error:\s*/, ''))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-wrap">
    <div class="auth-card">
      <div class="brand">
        <img src="/favicon.svg" alt="" class="brand-mark" />
        <span class="brand-text">Archi<span class="brand-accent">varr</span></span>
      </div>
      <n-card title="Create your admin account">
        <p class="muted" style="margin-top: 0">
          Archivarr is locked down by default. Set up the administrator account to get started.
        </p>
        <!-- Single submission path via the form's submit event; no duplicate
             @click/@keyup handlers (those previously fired setup twice, with the
             second POST hitting "setup already completed"). -->
        <n-form @submit.prevent="submit">
          <n-form-item label="Username">
            <n-input v-model:value="username" placeholder="Choose a username" autofocus />
          </n-form-item>
          <n-form-item
            label="Password"
            :validation-status="passwordTooShort ? 'error' : undefined"
            :feedback="passwordTooShort ? 'At least 8 characters' : undefined"
          >
            <n-input
              v-model:value="password"
              type="password"
              show-password-on="click"
              placeholder="At least 8 characters"
            />
          </n-form-item>
          <n-form-item
            label="Confirm password"
            :validation-status="mismatch ? 'error' : undefined"
            :feedback="mismatch ? 'Passwords do not match' : undefined"
          >
            <n-input
              v-model:value="confirm"
              type="password"
              show-password-on="click"
              placeholder="Re-enter password"
            />
          </n-form-item>
          <n-button type="primary" block :loading="loading" attr-type="submit">
            Create account
          </n-button>
        </n-form>
      </n-card>
    </div>
  </div>
</template>

<style scoped>
.auth-wrap {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}
.auth-card {
  width: 100%;
  max-width: 420px;
}
.brand {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  margin-bottom: 20px;
}
.brand-mark {
  width: 32px;
  height: 32px;
}
.brand-text {
  font-size: 22px;
  font-weight: 700;
  letter-spacing: 0.4px;
  color: var(--brand-text);
}
.brand-accent {
  color: #3b82f6;
}
</style>
