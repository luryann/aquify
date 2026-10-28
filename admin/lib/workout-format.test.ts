import test from 'node:test'
import assert from 'node:assert/strict'

import {
  addDaysISO, formatClock, formatDuration, formatSetText, parseTags, setDistance, setHeadline,
  weekStartOf, zoneColor, zoneShares,
} from './workout-format.ts'

test('formatClock renders send-offs', () => {
  assert.equal(formatClock(90), '1:30')
  assert.equal(formatClock(45), ':45')
  assert.equal(formatClock(600), '10:00')
  assert.equal(formatClock(undefined), ':00')
})

test('formatDuration rounds to minutes', () => {
  assert.equal(formatDuration(1800), '30m')
  assert.equal(formatDuration(4500), '1h 15m')
  assert.equal(formatDuration(7200), '2h')
})

test('set text covers leaf sets and nested rounds', () => {
  const leaf = { repeats: 8, distance: 100, stroke: 'free', interval_seconds: 90, zone: 'EN2' as const }
  assert.equal(setHeadline(leaf), '8 × 100 free')
  assert.equal(formatSetText(leaf), '8 × 100 free EN2 @ 1:30')
  const round = {
    repeats: 3, rest_seconds: 60,
    sets: [leaf, { repeats: 1, distance: 100, stroke: 'back', equipment: ['pull_buoy'] }],
  }
  assert.equal(setHeadline(round), '3 rounds')
  assert.equal(formatSetText(round), '3 × {8 × 100 free EN2 @ 1:30; 100 back buoy} 1:00 rest')
  assert.equal(setDistance(round), 2700)
})

test('zoneShares orders by intensity and drops empty zones', () => {
  const shares = zoneShares({ SP3: 100, EN1: 300, unassigned: 100, EN2: 0 })
  assert.deepEqual(shares.map((s) => s.zone), ['EN1', 'SP3', 'unassigned'])
  assert.equal(shares[0].share, 0.6)
  assert.deepEqual(zoneShares({}), [])
})

test('zoneColor ramps with intensity and falls back for unknown zones', () => {
  assert.match(zoneColor('REC'), /18%/)
  assert.match(zoneColor('SP3'), /100%/)
  assert.equal(zoneColor('nope'), 'var(--border-strong)')
})

test('parseTags normalises and de-duplicates', () => {
  assert.deepEqual(parseTags(' Threshold, senior ,threshold,, '), ['threshold', 'senior'])
})

test('week helpers are Monday-based and timezone safe', () => {
  assert.equal(weekStartOf('2026-11-08'), '2026-11-02') // Sunday
  assert.equal(weekStartOf('2026-11-02'), '2026-11-02') // Monday
  assert.equal(addDaysISO('2026-12-30', 3), '2027-01-02')
})
