import os from 'node:os'
import path from 'node:path'

import { defineConfig, devices } from '@playwright/test'

const apiPort = process.env.PLAYWRIGHT_API_PORT ?? '8080'
const adminPort = process.env.PLAYWRIGHT_ADMIN_PORT ?? '3000'
const apiOrigin = `http://localhost:${apiPort}`
const adminOrigin = `http://localhost:${adminPort}`

// One bookkeeping file per run (start-api writes it, global-teardown reads it).
// Set before any webServer starts so child processes and workers inherit it.
process.env.PLAYWRIGHT_E2E_STATE_FILE ??= path.join(os.tmpdir(), `aquify-playwright-state-${process.pid}-${Date.now()}.json`)

const databaseUrl = process.env.DATABASE_URL ?? 'postgres://postgres:postgres@127.0.0.1:5432/aquify_e2e?sslmode=disable'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  timeout: 90_000,
  expect: {
    timeout: 15_000,
  },
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: adminOrigin,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  // No `globalSetup`: database setup runs once, inside the API webServer (scripts/e2e/start-api.mjs).
  globalTeardown: './scripts/e2e/global-teardown.mjs',
  webServer: [
    {
      command: 'node ./scripts/e2e/start-api.mjs',
      // Database setup and Go compilation can be slow on a cold CI runner.
      timeout: 300_000,
      cwd: '.',
      url: `${apiOrigin}/api/v1/health/live`,
      reuseExistingServer: !process.env.CI,
      stdout: 'pipe',
      stderr: 'pipe',
      env: {
        ...process.env,
        APP_ENV: 'development',
        API_ADDR: `:${apiPort}`,
        // Empty values select the default; whitespace disables this listener.
        METRICS_ADDR: ' ',
        API_ALLOWED_ORIGINS: adminOrigin,
        AUTH_ENABLE_DEV_STUB_TOKENS: 'true',
        AUTH_ALLOW_DEV_SESSION_ISSUE: 'true',
        SECURITY_RATE_LIMIT_AUTH_REQUESTS: '1000',
        DATABASE_URL: databaseUrl,
        OIDC_ISSUER_URL: ' ',
        OIDC_CLIENT_ID: ' ',
        OIDC_REDIRECT_URL: ' ',
        OIDC_JWKS_URL: ' ',
        KEYCLOAK_ADMIN_URL: ' ',
        KEYCLOAK_REALM: ' ',
        KEYCLOAK_ADMIN_CLIENT_ID: ' ',
        KEYCLOAK_ADMIN_CLIENT_SECRET: ' ',
      },
    },
    {
      command: 'npm run dev',
      timeout: 180_000,
      cwd: '.',
      url: `${adminOrigin}/login`,
      reuseExistingServer: !process.env.CI,
      stdout: 'pipe',
      stderr: 'pipe',
      env: {
        ...process.env,
        PORT: adminPort,
        NEXT_PUBLIC_API_URL: apiOrigin,
        NEXT_PUBLIC_APP_ENV: 'development',
        NEXT_PUBLIC_AUTH_ENABLE_DEV_STUB_TOKENS: 'false',
      },
    },
  ],
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],
})
