import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// The SPA is compiled into the Go binary via go:embed, so its build output goes
// into the httpserver package's embed directory rather than a local dist/.
//
// base: './' makes every asset reference relative, so the SAME build works no
// matter what URL prefix the server is mounted under (BASE_PATH="/breadbot" or
// root). The app reads import.meta.env.BASE_URL at runtime for API/image URLs;
// with a relative base that resolves against the document location, which the
// server sets correctly via the SPA fallback.
export default defineConfig({
  plugins: [svelte()],
  base: './',
  build: {
    outDir: '../internal/httpserver/frontend/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    // In dev, proxy /api and /healthz to the Go server so relative fetches work
    // without CORS. The Go server also enables permissive CORS when DEBUG=1, so
    // either path works.
    proxy: {
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
})
