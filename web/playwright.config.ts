import process from 'node:process'
import { defineConfig, devices } from '@playwright/test'

/**
 * See https://playwright.dev/docs/test-configuration.
 */
export default defineConfig({
  testDir: './e2e',
  /* Maximum time one test can run for. */
  timeout: 30 * 1000,
  expect: {
    /**
     * Maximum time expect() should wait for the condition to be met.
     * For example in `await expect(locator).toHaveText();`
     */
    timeout: 5000,
  },
  /* Fail the build on CI if you accidentally left test.only in the source code. */
  forbidOnly: !!process.env.CI,
  /* Retry on CI only */
  retries: process.env.CI ? 2 : 0,
  /* Opt out of parallel tests on CI. */
  workers: process.env.CI ? 1 : undefined,
  /* Reporter to use. See https://playwright.dev/docs/test-reporters */
  reporter: 'html',

  // On CI the backend is started by the workflow directly; locally we use
  // docker/podman compose via global-setup/teardown.
  ...(process.env.CI
    ? {}
    : {
        globalSetup: './e2e/global-setup.ts',
        globalTeardown: './e2e/global-teardown.ts',
      }),

  /* Shared settings for all the projects below. See https://playwright.dev/docs/api/class-testoptions. */
  use: {
    /* Maximum time each action such as `click()` can take. Defaults to 0 (no limit). */
    actionTimeout: 0,
    /* Both CI and local: backend serves frontend + API on one port. */
    baseURL: 'http://127.0.0.1:3333',

    /* Collect trace when retrying the failed test. See https://playwright.dev/docs/trace-viewer */
    trace: 'on-first-retry',

    headless: true,
  },

  /* Configure projects for major browsers */
  projects: [
    {
      name: 'setup',
      testMatch: /setup-flow\.spec\.ts/,
      use: {
        ...devices['Desktop Chrome'],
      },
    },
    {
      // settings.spec.ts mutates the shared global poster_source via UI
      // auto-save.  Run it first so it doesn't race with key-settings and
      // key-auth-api tests that depend on the global default being 't'.
      name: 'settings',
      dependencies: ['setup'],
      // Only settings.spec.ts: a bare /settings\.spec\.ts$/ also matched
      // key-settings.spec.ts, whose failures then blocked every chromium test.
      testMatch: /(^|[\\/])settings\.spec\.ts$/,
      use: {
        ...devices['Desktop Chrome'],
      },
    },
    {
      name: 'chromium',
      dependencies: ['setup', 'settings'],
      testIgnore: [/setup-flow\.spec\.ts/, /live-api\.spec\.ts/, /(^|[\\/])settings\.spec\.ts$/],
      use: {
        ...devices['Desktop Chrome'],
      },
    },
    {
      name: 'live',
      dependencies: ['setup'],
      testMatch: /live-api\.spec\.ts/,
      timeout: 120 * 1000,
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],

  // No webServer needed — backend serves the frontend via STATIC_DIR.
})
