import test from 'node:test'
import assert from 'node:assert/strict'

import { backoffMs, clearRollCallStorage, dueMarks, effectiveStatus, enqueueMark, failMark, isRetryable, loadQueue, rollCallRosterKey, rollCallScope, saveQueue, settleMark, type RollCallQueue } from './roll-call-queue.ts'

const SCOPE = rollCallScope('org-a', 'user-1')

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
  assert.ok(isRetryable(401) && isRetryable(403, 'csrf_invalid') && !isRetryable(403, 'forbidden'))
})

test('same-tick taps and clock rollback cannot settle or delay a newer mark', () => {
  for (const now of [100, 90]) {
    const initial = enqueueMark({}, 'p', 's', 'present', 100)
    const sent = dueMarks(initial, 100)[0]
    const updated = enqueueMark(initial, 'p', 's', 'absent', now)
    assert.equal(dueMarks(updated, now).length, 1)
    assert.deepEqual(settleMark(updated, sent), updated)
    assert.deepEqual(failMark(updated, sent, 'offline', 200), updated)
  }
})

test('invalid stored records cannot crash roll call or block valid queued writes', () => {
  const valid = enqueueMark({}, 'p', 's', 'present', 10)
  const storage = { getItem: () => JSON.stringify({ ...valid, broken: null, invalid: { status: 'other' } }) }
  assert.deepEqual(loadQueue(storage, SCOPE), valid)
  assert.deepEqual(loadQueue({ getItem: () => '[]' }, SCOPE), {})
  assert.deepEqual(dueMarks(loadQueue(storage, SCOPE), 10), Object.values(valid))
})

test('queue persists through storage and tolerates garbage', () => {
  const store = new Map<string, string>()
  const storage = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v) }
  const q = enqueueMark({}, 'p', 's', 'excused', 5)
  saveQueue(storage, q, SCOPE)
  assert.deepEqual(loadQueue(storage, SCOPE), q)
  store.set([...store.keys()][0], '{not json')
  assert.deepEqual(loadQueue(storage, SCOPE), {})
  assert.deepEqual(loadQueue(undefined, SCOPE), {})
})

test('another org or user never sees a queued mark, and sign-out wipes every roll-call key', () => {
  const store = new Map<string, string>()
  const storage = {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, v),
    removeItem: (k: string) => void store.delete(k),
    get length() { return store.size },
    key: (i: number) => [...store.keys()][i] ?? null,
  }
  saveQueue(storage, enqueueMark({}, 'p', 's', 'present', 1), SCOPE)
  store.set(rollCallRosterKey(SCOPE, 'p'), '[{"swimmer":"A"}]')
  store.set('aquify:roll-call-queue:v1', '{"legacy":true}')
  store.set('aquify:theme', 'dark')

  assert.deepEqual(loadQueue(storage, rollCallScope('org-b', 'user-1')), {})
  assert.deepEqual(loadQueue(storage, rollCallScope('org-a', 'user-2')), {})
  assert.notEqual(rollCallRosterKey(SCOPE, 'p'), rollCallRosterKey(rollCallScope('org-b', 'user-1'), 'p'))

  clearRollCallStorage(storage)
  assert.deepEqual([...store.keys()], ['aquify:theme'])
})
