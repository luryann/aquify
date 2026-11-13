import test from 'node:test'
import assert from 'node:assert/strict'

import { formatDeadline } from './dates.ts'

test('formatDeadline shows a bare date without a time and a timestamp with one', () => {
  assert.equal(formatDeadline(''), '—')
  assert.equal(formatDeadline(undefined), '—')
  assert.match(formatDeadline('2026-11-20'), /^Nov 20, 2026$/)
  assert.match(formatDeadline('2026-11-21T01:00:00Z'), /2026/)
  assert.match(formatDeadline('2026-11-21T01:00:00Z'), /\d{2}:\d{2}/)
  assert.equal(formatDeadline('not a date'), 'not a date')
})
