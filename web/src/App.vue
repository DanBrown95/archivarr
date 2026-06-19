<script setup>
import { computed, watchEffect } from 'vue'
import { darkTheme } from 'naive-ui'
import { useRoute } from 'vue-router'
import AppLayout from './components/AppLayout.vue'
import { useThemeStore } from './stores/theme'

const route = useRoute()
const theme = useThemeStore()

// null theme = Naive UI's default (light); darkTheme = dark.
const naiveTheme = computed(() => (theme.isDark ? darkTheme : null))

// Reflect the mode on <html> so custom CSS (body bg, brand text, code blocks)
// can flip via [data-theme] variables.
watchEffect(() => {
  document.documentElement.dataset.theme = theme.mode
})

// Accent matches an *arr-style dashboard.
const themeOverrides = {
  common: {
    primaryColor: '#3b82f6',
    primaryColorHover: '#60a5fa',
    primaryColorPressed: '#2563eb',
    primaryColorSuppl: '#3b82f6',
    borderRadius: '6px',
  },
}
</script>

<template>
  <n-config-provider :theme="naiveTheme" :theme-overrides="themeOverrides">
    <n-message-provider>
      <n-dialog-provider>
        <!-- Login / setup render standalone; everything else gets the app shell. -->
        <router-view v-if="route.meta.public" />
        <AppLayout v-else />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
