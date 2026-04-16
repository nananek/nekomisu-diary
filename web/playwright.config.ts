import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'mobile',
      use: {
        ...devices['Pixel 7'],
      },
    },
    {
      name: 'desktop',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
