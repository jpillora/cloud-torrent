import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The built SPA is embedded into the Go binary: `npm run build` writes to
// ../static/dist, which static/embed.go embeds. In development the Vite server
// (5173) proxies backend routes to the Go server (3000) so the SPA and the API
// share an origin.
const backend = 'http://localhost:3000'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/sync': { target: backend, ws: true },
      '/js/velox.js': { target: backend },
      '/api': { target: backend },
      '/search': { target: backend },
      '/download': { target: backend },
    },
  },
  build: {
    outDir: '../static/dist',
    emptyOutDir: false,
  },
})
