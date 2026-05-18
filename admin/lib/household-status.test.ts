import test from 'node:test'
import assert from 'node:assert/strict'

import { normalizeHouseholdStatus } from './household-status.ts'

test('normalizeHouseholdStatus preserves supported household statuses', () => {
  assert.equal(normalizeHouseholdStatus('active'), 'active')
  assert.equal(normalizeHouseholdStatus('inactive'), 'inactive')
  assert.equal(normalizeHouseholdStatus('archived'), 'archived')
})

test('normalizeHouseholdStatus falls back to active for unsupported values', () => {
  assert.equal(normalizeHouseholdStatus('pending'), 'active')
  assert.equal(normalizeHouseholdStatus('suspended'), 'active')
  assert.equal(normalizeHouseholdStatus(undefined), 'active')
  assert.equal(normalizeHouseholdStatus(null), 'active')
})
