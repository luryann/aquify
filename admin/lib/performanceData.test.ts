import test from 'node:test'
import assert from 'node:assert/strict'
import { formatDrop, formatSwimTime, genderLabel, parseSwimTime } from './performanceData'
import { defaultComparison, goalTone, periodRange } from './performance-periods'
import { relayLegLabel, toggleLock } from './relay-builder'

test('swim times format and parse round-trip', () => {
  assert.equal(formatSwimTime(58120), '58.12')
  assert.equal(formatSwimTime(62340), '1:02.34')
  assert.equal(formatSwimTime(5000), '5.00')
  assert.equal(formatSwimTime(null), 'NT')
  assert.equal(parseSwimTime('58.12'), 58120)
  assert.equal(parseSwimTime('1:02.34'), 62340)
  assert.equal(parseSwimTime('1:02'), 62000)
  assert.equal(parseSwimTime('58.1'), 58100)
  assert.equal(parseSwimTime('1:75.00'), null)
  assert.equal(parseSwimTime('abc'), null)
  assert.equal(parseSwimTime('0'), null)
})

test('drops carry a sign and gender labels follow the age group', () => {
  assert.equal(formatDrop(1230), '−1.23')
  assert.equal(formatDrop(-400), '+0.40')
  assert.equal(genderLabel('F', 'OPEN'), 'Women')
  assert.equal(genderLabel('M', '11-12'), 'Boys')
  assert.equal(genderLabel('X', '11-12'), 'Mixed')
})

test('period presets and default comparison windows', () => {
  const today = new Date(2026, 9, 26)
  assert.deepEqual(periodRange('all', today), { from: '', to: '' })
  assert.deepEqual(periodRange('ytd', today), { from: '2026-01-01', to: '2026-10-26' })
  assert.deepEqual(periodRange('12m', today), { from: '2025-10-26', to: '2026-10-26' })
  assert.deepEqual(defaultComparison(today), { from_a: '2024-10-27', to_a: '2025-10-26', from_b: '2025-10-27', to_b: '2026-10-26' })
})

test('goal tone clamps progress and flags overdue goals', () => {
  assert.deepEqual(goalTone({ achieved: true, overdue: false, progress_percent: 40 }), { fill: 100, tone: 'success' })
  assert.deepEqual(goalTone({ achieved: false, overdue: true, progress_percent: 140 }), { fill: 100, tone: 'warning' })
  assert.deepEqual(goalTone({ achieved: false, overdue: false }), { fill: 0, tone: 'accent' })
})

test('relay leg labels', () => {
  assert.equal(relayLegLabel('medley', 1), 'Back')
  assert.equal(relayLegLabel('medley', 3), 'Fly')
  assert.equal(relayLegLabel('free', 1), 'Lead-off')
  assert.equal(relayLegLabel('free', 4), 'Anchor')
})

test('relay locks: one leg per swimmer, one swimmer per leg, toggle off', () => {
  let locks = toggleLock([], { team: 0, leg: 1, swimmer_id: 'a' })
  assert.deepEqual(locks, [{ team: 0, leg: 1, swimmer_id: 'a' }])
  locks = toggleLock(locks, { team: 1, leg: 2, swimmer_id: 'a' })
  assert.deepEqual(locks, [{ team: 1, leg: 2, swimmer_id: 'a' }])
  locks = toggleLock(locks, { team: 1, leg: 2, swimmer_id: 'b' })
  assert.deepEqual(locks, [{ team: 1, leg: 2, swimmer_id: 'b' }])
  assert.deepEqual(toggleLock(locks, { team: 1, leg: 2, swimmer_id: 'b' }), [])
})
