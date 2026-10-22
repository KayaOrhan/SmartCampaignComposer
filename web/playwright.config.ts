import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e', timeout: 240000, workers: 1,
  use: { baseURL: 'http://127.0.0.1:5173', browserName: 'chromium', viewport: { width: 1440, height: 1000 }, screenshot: 'only-on-failure', trace: 'off' },
})
