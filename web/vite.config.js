import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// During `npm run build`, output goes to dist/, which Go embeds.
// During `npm run dev`, /api requests are proxied to the Go backend.
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:7979',
    },
  },
})
