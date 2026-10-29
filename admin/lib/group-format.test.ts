import test from 'node:test'
import assert from 'node:assert/strict'

import { formatAgeRange } from './group-format.ts'

test('formatAgeRange handles open, one-sided and bounded groups', () => {
  assert.equal(formatAgeRange(null, null), 'All ages')
  assert.equal(formatAgeRange(undefined, 12), 'Up to 12')
  assert.equal(formatAgeRange(13, null), '13+')
  assert.equal(formatAgeRange(9, 12), '9-12')
})
