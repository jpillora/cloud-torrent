import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The built SPA is embedded into the Go binary for production. In development
// the Vite server (port 5173) proxies backend routes (/sync, /api, /search,
// /download, /js/velox.js) to the Go server — that proxy is added in a later
// task (M0: embed frontend build + dev proxy).
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
  },
  build: {
    outDir: 'dist',
  },
})
