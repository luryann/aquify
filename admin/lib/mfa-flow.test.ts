import test from 'node:test'
import assert from 'node:assert/strict'

import { resolveMfaBootstrapState, type MfaFactorSummary } from './mfa-flow.ts'

test('first-time enrollment starts with fresh totp setup', () => {
  assert.deepEqual(resolveMfaBootstrapState([]), { kind: 'enroll-totp' })
})

test('stale unverified totp does not skip the qr setup flow', () => {
  const factors: MfaFactorSummary[] = [
    { id: 'totp-stale', factor_type: 'totp', is_verified: false },
  ]

  assert.deepEqual(resolveMfaBootstrapState(factors), { kind: 'enroll-totp' })
})

test('verified totp without backup codes resumes at backup-code setup', () => {
  const factors: MfaFactorSummary[] = [
    { id: 'totp-1', factor_type: 'totp', is_verified: true },
  ]

  assert.deepEqual(resolveMfaBootstrapState(factors), {
    kind: 'setup-backup',
    totpFactorId: 'totp-1',
  })
})

test('completed setup prefers normal totp verification with backup fallback available', () => {
  const factors: MfaFactorSummary[] = [
    { id: 'totp-1', factor_type: 'totp', is_verified: true },
    { id: 'backup-1', factor_type: 'backup_code', is_verified: false },
  ]

  assert.deepEqual(resolveMfaBootstrapState(factors), {
    kind: 'verify',
    mode: 'totp',
    totpFactorId: 'totp-1',
    backupFactorId: 'backup-1',
  })
})

test('backup-only users can still sign in with a saved backup code', () => {
  const factors: MfaFactorSummary[] = [
    { id: 'backup-1', factor_type: 'backup_code', is_verified: true },
  ]

  assert.deepEqual(resolveMfaBootstrapState(factors), {
    kind: 'verify',
    mode: 'backup',
    backupFactorId: 'backup-1',
    totpFactorId: undefined,
  })
})
