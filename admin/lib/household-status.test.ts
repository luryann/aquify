import test from 'node:test'
import assert from 'node:assert/strict'

import { normalizeHouseholdStatus } from './household-status.ts'

test('normalizeHouseholdStatus preserves supported household statuses', () => {
  assert.equal(normalizeHouseholdStatus('active'), 'active')
  assert.equal(normalizeHouseholdStatus('inactive'), 'inactive')
  assert.equal(normalizeHouseholdStatus('archived'), 'archived')
})

test('normalizeHouseholdStatus flags unrecognised statuses instead of showing them as active', () => {
  assert.equal(normalizeHouseholdStatus('pending'), 'unknown')
  assert.equal(normalizeHouseholdStatus('suspended'), 'unknown')
})

test('normalizeHouseholdStatus treats a missing status as active', () => {
  assert.equal(normalizeHouseholdStatus(undefined), 'active')
  assert.equal(normalizeHouseholdStatus(null), 'active')
  assert.equal(normalizeHouseholdStatus(''), 'active')
})
