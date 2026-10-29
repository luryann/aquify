import os from 'node:os'
import path from 'node:path'

/**
 * Per-run bookkeeping file shared by start-api (writes) and global-teardown
 * (reads). playwright.config.ts sets PLAYWRIGHT_E2E_STATE_FILE once in the main
 * process (webServer processes and workers inherit it), so concurrent runs and
 * separate worktrees never share or clobber each other's `startedPostgres` flag.
 */
export function e2eStateFile() {
  return process.env.PLAYWRIGHT_E2E_STATE_FILE
    ?? path.join(os.tmpdir(), `aquify-playwright-state-${process.pid}.json`)
}
