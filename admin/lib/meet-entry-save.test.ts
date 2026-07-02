import test from 'node:test'
import assert from 'node:assert/strict'

import type { ApiClient } from './api.ts'
import { ApiError } from './api.ts'
import { formatMeetEntriesSaveError, saveMeetEntriesBatch } from './meet-entry-save.ts'

type RecordedCall = {
  method: 'POST'
  path: string
  body?: unknown
  headers?: Record<string, string>
}

function createMockApi() {
  const calls: RecordedCall[] = []
  const responses = new Map<string, unknown>()
  const failures = new Map<string, unknown>()

  function key(path: string) {
    return `POST ${path}`
  }

  const api = {
    post: async <T>(path: string, body?: unknown, headers?: Record<string, string>) => {
      calls.push({ method: 'POST', path, body, headers })
      if (failures.has(key(path))) {
        throw failures.get(key(path))
      }
      return responses.get(key(path)) as T
    },
    calls,
    respond(path: string, value: unknown) {
      responses.set(key(path), value)
    },
    fail(path: string, error: unknown) {
      failures.set(key(path), error)
    },
  }

  return api
}

test('saveMeetEntriesBatch writes every selected meet event and resolves only after all succeed', async () => {
  const mock = createMockApi()
  const orgId = 'org_entries'
  mock.respond('/api/v1/meets/meet_1/entries', {
    entry: { id: 'entry_1', meet_id: 'meet_1', swimmer_id: 'sw_1', meet_event_id: 'event_1', status: 'draft' },
  })

  const api = mock as unknown as ApiClient
  const savedCount = await saveMeetEntriesBatch(api, orgId, 'meet_1', 'sw_1', ['event_1', 'event_1'])

  assert.equal(savedCount, 2)
  assert.deepEqual(mock.calls, [
    {
      method: 'POST',
      path: '/api/v1/meets/meet_1/entries',
      body: { swimmer_id: 'sw_1', meet_event_id: 'event_1' },
      headers: { 'X-Org-Id': orgId },
    },
    {
      method: 'POST',
      path: '/api/v1/meets/meet_1/entries',
      body: { swimmer_id: 'sw_1', meet_event_id: 'event_1' },
      headers: { 'X-Org-Id': orgId },
    },
  ])
})

test('saveMeetEntriesBatch rejects when any selected meet entry write fails', async () => {
  const mock = createMockApi()
  const orgId = 'org_entries'
  mock.fail('/api/v1/meets/meet_1/entries', new ApiError(409, 'duplicate_entry', 'Duplicate entry already exists.'))

  const api = mock as unknown as ApiClient

  await assert.rejects(
    saveMeetEntriesBatch(api, orgId, 'meet_1', 'sw_1', ['event_ok']),
    (error: unknown) =>
      error instanceof ApiError &&
      error.status === 409 &&
      error.code === 'duplicate_entry',
  )
})

test('formatMeetEntriesSaveError surfaces a visible partial-failure message', () => {
  const message = formatMeetEntriesSaveError(
    new ApiError(409, 'duplicate_entry', 'Duplicate entry already exists.'),
    3,
  )

  assert.match(message, /Failed to save all 3 selected entries\./)
  assert.match(message, /Duplicate entry already exists\./)
  assert.match(message, /Some entries may have been created before the failure\./)
})
