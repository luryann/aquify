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

import { mergeStreamMessage } from './message-format.ts'

test('mergeStreamMessage keeps the viewer own reacted_by_me flags', () => {
  const prev = [{ id: 'm1', reactions: [{ emoji: 'a', count: 1, reacted_by_me: true }] }]
  const ev = { id: 'm1', reactions: [{ emoji: 'a', count: 2, reacted_by_me: false }, { emoji: 'b', count: 1, reacted_by_me: true }] }
  const out = mergeStreamMessage(prev, ev)
  assert.deepEqual(out[0].reactions, [
    { emoji: 'a', count: 2, reacted_by_me: true },
    { emoji: 'b', count: 1, reacted_by_me: false },
  ])
  assert.equal(mergeStreamMessage([], ev).length, 1)
})
