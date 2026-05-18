import test from 'node:test'
import assert from 'node:assert/strict'

import { ApiError } from './api.ts'

test('ApiError.fromResponse unwraps the backend nested error envelope', async () => {
  const response = new Response(JSON.stringify({
    error: {
      code: 'validation_failed',
      message: 'Household name is required.',
    },
  }), {
    status: 400,
    statusText: 'Bad Request',
    headers: { 'content-type': 'application/json' },
  })

  const err = await ApiError.fromResponse(response)

  assert.equal(err.status, 400)
  assert.equal(err.code, 'validation_failed')
  assert.equal(err.message, 'Household name is required.')
})

test('ApiError.fromResponse still accepts flat code and message payloads', async () => {
  const response = new Response(JSON.stringify({
    code: 'conflict',
    message: 'A record already exists.',
  }), {
    status: 409,
    statusText: 'Conflict',
    headers: { 'content-type': 'application/json' },
  })

  const err = await ApiError.fromResponse(response)

  assert.equal(err.status, 409)
  assert.equal(err.code, 'conflict')
  assert.equal(err.message, 'A record already exists.')
})
