import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  build: { rollupOptions: { input: { app: 'index.html', docs: 'docs/index.html' } } },
  plugins: [vue()],
  // Never load local dotenv files. Runtime secrets belong to the API process.
  envDir: false,
  server: { proxy: { '/api': { target: 'http://127.0.0.1:8080', rewrite: path => path.replace(/^\/api/, '') } } },
  test: { environment: 'happy-dom', include: ['src/**/*.test.ts'], restoreMocks: true },
})
