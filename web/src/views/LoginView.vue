<script setup>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const message = useMessage()

const username = ref('')
const password = ref('')
const loading = ref(false)

async function submit() {
  if (loading.value) return // guard against duplicate submissions
  if (!username.value || !password.value) {
    message.warning('Enter your username and password')
    return
  }
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    router.push(redirect)
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
      <n-card title="Sign in">
        <!-- A single submission path: the form's submit event (fired by Enter
             in an input or the submit button). No extra @click/@keyup handlers,
             which previously caused duplicate login attempts. -->
        <n-form @submit.prevent="submit">
          <n-form-item label="Username">
            <n-input v-model:value="username" placeholder="Username" autofocus />
          </n-form-item>
          <n-form-item label="Password">
            <n-input
              v-model:value="password"
              type="password"
              show-password-on="click"
              placeholder="Password"
            />
          </n-form-item>
          <n-button type="primary" block :loading="loading" attr-type="submit">
            Sign in
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
  max-width: 380px;
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
