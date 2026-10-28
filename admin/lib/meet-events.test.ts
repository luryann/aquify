import test from 'node:test'
import assert from 'node:assert/strict'

import { formatAgeGroup, formatEventLabel, formatStroke } from './meet-events.ts'

test('formatStroke accepts API long names and legacy short codes', () => {
  assert.equal(formatStroke('freestyle'), 'Freestyle')
  assert.equal(formatStroke('free'), 'Freestyle')
  assert.equal(formatStroke('individual_medley'), 'IM')
  assert.equal(formatStroke('medley_relay'), 'Medley Relay')
  assert.equal(formatStroke(undefined), '')
})

test('formatEventLabel includes gender and age bounds', () => {
  assert.equal(formatEventLabel({ distance: 50, stroke: 'freestyle', gender: 'F', min_age: 11, max_age: 12 }), '50 Freestyle (F 11-12)')
  assert.equal(formatEventLabel({ distance: 200, stroke: 'individual_medley', gender: 'M' }), '200 IM (M Open)')
  assert.equal(formatAgeGroup(15, null), '15&O')
  assert.equal(formatAgeGroup(null, 10), '10&U')
})
