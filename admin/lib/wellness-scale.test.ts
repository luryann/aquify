import test from 'node:test'
import assert from 'node:assert/strict'

import { localToday, readiness, readinessStep } from './wellness-scale.ts'

test('readiness mirrors the server formula', () => {
  assert.equal(readiness({ sleep_quality: 5, soreness: 1, energy: 5, stress: 1, mood: 5 }), 5)
  assert.equal(readiness({ sleep_quality: 1, soreness: 5, energy: 1, stress: 5, mood: 1 }), 1)
  assert.equal(readiness({ sleep_quality: 4, soreness: 2, energy: 4, stress: 2, mood: 4 }), 4)
})

test('readinessStep buckets scores and handles gaps', () => {
  assert.equal(readinessStep(null), null)
  assert.equal(readinessStep(1.2), 1)
  assert.equal(readinessStep(2.5), 2)
  assert.equal(readinessStep(3), 3)
  assert.equal(readinessStep(4), 4)
  assert.equal(readinessStep(4.6), 5)
})

test('localToday formats the local calendar date', () => {
  assert.equal(localToday(new Date(2026, 0, 5, 23, 30)), '2026-01-05')
})
