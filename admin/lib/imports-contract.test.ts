import test from 'node:test'
import assert from 'node:assert/strict'

import type { ApiClient } from './api.ts'
import { finalizeImport, requestImportUploadUrl } from './adminData.ts'

type RecordedCall = {
  method: 'POST'
  path: string
  body?: unknown
  headers?: Record<string, string>
}

function createMockApi() {
  const calls: RecordedCall[] = []
  const responses = new Map<string, unknown>()

  const api = {
    post: async <T>(path: string, body?: unknown, headers?: Record<string, string>) => {
      calls.push({ method: 'POST', path, body, headers })
      return responses.get(path) as T
    },
    calls,
    respond(path: string, value: unknown) {
      responses.set(path, value)
    },
  }

  return api
}

test('requestImportUploadUrl decodes the nested import response envelope', async () => {
  const mock = createMockApi()
  mock.respond('/api/v1/imports/upload-url', {
    import: {
      id: 'imp_123',
      filename: 'meet.hyv',
      file_type: 'hyv',
      status: 'uploaded',
      upload_url: 'https://uploads.example.test/imports/imp_123',
      storage_key: 'imports/imp_123/meet.hyv',
      row_count: 0,
      error_count: 0,
      conflict_count: 0,
      created_at: '2026-03-26T00:00:00Z',
      created_by: '00000000-0000-0000-0000-00000000a102',
      org_id: '00000000-0000-0000-0000-00000000a001',
    },
  })

  const api = mock as unknown as ApiClient
  const result = await requestImportUploadUrl(api, 'org_local', {
    filename: 'meet.hyv',
    size_bytes: 1024,
    mime_type: 'application/octet-stream',
    meet_id: 'meet_1',
  })

  assert.deepEqual(result, {
    import_id: 'imp_123',
    upload_url: 'https://uploads.example.test/imports/imp_123',
    storage_key: 'imports/imp_123/meet.hyv',
  })
  assert.deepEqual(mock.calls, [{
    method: 'POST',
    path: '/api/v1/imports/upload-url',
    body: {
      filename: 'meet.hyv',
      size_bytes: 1024,
      mime_type: 'application/octet-stream',
      meet_id: 'meet_1',
    },
    headers: { 'X-Org-Id': 'org_local' },
  }])
})

test('finalizeImport uses the server persisted import metadata without an object key payload', async () => {
  const mock = createMockApi()
  mock.respond('/api/v1/imports/imp_123/finalize', {
    import: {
      id: 'imp_123',
      filename: 'meet.hyv',
      file_type: 'hyv',
      status: 'parsing',
      row_count: 0,
      error_count: 0,
      conflict_count: 0,
      created_at: '2026-03-26T00:00:00Z',
      created_by: '00000000-0000-0000-0000-00000000a102',
      org_id: '00000000-0000-0000-0000-00000000a001',
    },
  })

  const api = mock as unknown as ApiClient
  const result = await finalizeImport(api, 'org_local', 'imp_123')

  assert.equal(result.id, 'imp_123')
  assert.equal(result.status, 'parsing')
  assert.deepEqual(mock.calls, [{
    method: 'POST',
    path: '/api/v1/imports/imp_123/finalize',
    body: undefined,
    headers: { 'X-Org-Id': 'org_local' },
  }])
})
