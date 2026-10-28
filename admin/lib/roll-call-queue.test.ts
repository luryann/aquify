import test from 'node:test'
import assert from 'node:assert/strict'

import { backoffMs, dueMarks, effectiveStatus, enqueueMark, failMark, isRetryable, loadQueue, saveQueue, settleMark, type RollCallQueue } from './roll-call-queue.ts'

test('latest tap wins and settles only the version that was sent', () => {
  let q: RollCallQueue = {}
  q = enqueueMark(q, 'p', 's', 'present', 1)
  const sent = dueMarks(q, 1)[0]
  q = enqueueMark(q, 'p', 's', 'absent', 2) // tapped again while in flight
  q = settleMark(q, sent)
  assert.equal(effectiveStatus(q, 'p', 's', 'present'), 'absent')
  q = settleMark(q, dueMarks(q, 2)[0])
  assert.deepEqual(q, {})
  assert.equal(effectiveStatus(q, 'p', 's', 'excused'), 'excused')
})

test('failures back off and are not due until the retry time', () => {
  let q = enqueueMark({}, 'p', 's', 'present', 0)
  q = failMark(q, dueMarks(q, 0)[0], 'offline', 0)
  assert.equal(dueMarks(q, 1000).length, 0)
  assert.equal(dueMarks(q, backoffMs(1)).length, 1)
  assert.equal(backoffMs(10), 60_000)
  assert.ok(isRetryable(undefined) && isRetryable(503) && isRetryable(429))
  assert.ok(!isRetryable(403) && !isRetryable(422))
})

test('queue persists through storage and tolerates garbage', () => {
  const store = new Map<string, string>()
  const storage = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v) }
  const q = enqueueMark({}, 'p', 's', 'excused', 5)
  saveQueue(storage, q)
  assert.deepEqual(loadQueue(storage), q)
  store.set('aquify:roll-call-queue:v1', '{not json')
  assert.deepEqual(loadQueue(storage), {})
  assert.deepEqual(loadQueue(undefined), {})
})
