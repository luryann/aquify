import { defineConfig, devices } from '@playwright/test'

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
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  globalTeardown: './scripts/e2e/global-teardown.mjs',
  webServer: [
    {
      command: 'node ./scripts/e2e/start-api.mjs',
      cwd: '.',
      url: 'http://localhost:8080/api/v1/health/live',
      reuseExistingServer: !process.env.CI,
      stdout: 'pipe',
      stderr: 'pipe',
      env: {
        ...process.env,
        APP_ENV: 'development',
        ADDR: ':8080',
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
      cwd: '.',
      url: 'http://localhost:3000/login',
      reuseExistingServer: !process.env.CI,
      stdout: 'pipe',
      stderr: 'pipe',
      env: {
        ...process.env,
        NEXT_PUBLIC_API_URL: 'http://localhost:8080',
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
