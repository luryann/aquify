import { test as base } from '@playwright/test'

import { closeTrackedContexts } from './app'

/**
 * `test` with an automatic fixture that closes every context opened through
 * `newAuthedContext` when the test ends, pass or fail, so a failed assertion
 * cannot leak the admin and role contexts across retries and repeat runs.
 */
export const test = base.extend<{ trackedContexts: void }>({
  trackedContexts: [
    async ({}, use) => {
      await use()
      await closeTrackedContexts()
    },
    { auto: true },
  ],
})

export { expect } from '@playwright/test'
