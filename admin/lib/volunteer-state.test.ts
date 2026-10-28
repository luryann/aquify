import test from 'node:test'
import assert from 'node:assert/strict'

import {
  currentLedgerRow,
  fillPercent,
  formatCents,
  formatHours,
  groupInOrder,
  hoursProgress,
  localInputToISO,
} from './volunteer-state.ts'

test('formatHours trims trailing zeros and pluralises', () => {
  assert.equal(formatHours(1), '1 hr')
  assert.equal(formatHours(3), '3 hrs')
  assert.equal(formatHours(2.5), '2.5 hrs')
  assert.equal(formatHours(5.25), '5.25 hrs')
})

test('formatCents renders dollars', () => {
  assert.equal(formatCents(13125), '$131.25')
})

test('hoursProgress clamps earned and scheduled shares', () => {
  assert.deepEqual(hoursProgress(10, 4, 3), { earned: 0.4, scheduled: 0.3 })
  assert.deepEqual(hoursProgress(10, 12, 3), { earned: 1, scheduled: 0 })
  const capped = hoursProgress(10, 8, 5)
  assert.ok(Math.abs(capped.earned + capped.scheduled - 1) < 1e-9)
  assert.deepEqual(hoursProgress(0, 0, 0), { earned: 1, scheduled: 0 })
})

test('fillPercent handles empty capacity', () => {
  assert.equal(fillPercent(3, 4), 75)
  assert.equal(fillPercent(0, 0), 0)
})

test('groupInOrder preserves first-seen order', () => {
  const groups = groupInOrder(['b1', 'a1', 'b2'], (s) => s[0])
  assert.deepEqual(groups.map((g) => g.key), ['b', 'a'])
  assert.deepEqual(groups[0].items, ['b1', 'b2'])
})

test('currentLedgerRow prefers the active season', () => {
  const rows = [{ season_status: 'upcoming', id: 1 }, { season_status: 'active', id: 2 }]
  assert.equal(currentLedgerRow(rows)?.id, 2)
  assert.equal(currentLedgerRow([{ season_status: 'completed', id: 3 }])?.id, 3)
  assert.equal(currentLedgerRow([]), null)
})

test('localInputToISO returns empty for blank or invalid input', () => {
  assert.equal(localInputToISO(''), '')
  assert.equal(localInputToISO('nope'), '')
  assert.match(localInputToISO('2026-03-07T08:00'), /^2026-03-0[67]T\d\d:00:00\.000Z$/)
})
