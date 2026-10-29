import test from 'node:test'
import assert from 'node:assert/strict'

import { QUICK_REACTIONS, formatMessageTime } from './message-format.ts'

test('formatMessageTime formats a timestamp and passes through unparseable text', () => {
  assert.equal(formatMessageTime('not a date'), 'not a date')
  assert.match(formatMessageTime('2026-07-04T15:05:00'), /^Jul 4, 3:05\s?PM$/)
})

test('QUICK_REACTIONS offers six emoji', () => {
  assert.equal(QUICK_REACTIONS.length, 6)
})
