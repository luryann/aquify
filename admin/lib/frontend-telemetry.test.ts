import test from 'node:test'
import assert from 'node:assert/strict'

import { ApiError } from './api.ts'
import {
  NETWORK_ERROR_MESSAGE,
  PERMISSION_DENIED_MESSAGE,
  SESSION_EXPIRED_MESSAGE,
  reportUIError,
  uiErrorMessage,
} from './frontend-telemetry.ts'

test('uiErrorMessage passes human API messages through and falls back otherwise', () => {
  assert.equal(uiErrorMessage(new ApiError(409, 'conflict', 'That season already exists.'), 'Fallback'), 'That season already exists.')
  assert.equal(uiErrorMessage(new ApiError(502, 'bad_gateway', 'Backend failed'), 'Fallback'), 'Fallback')
  assert.equal(uiErrorMessage(new ApiError(400, 'invalid_request', 'swimmer_id is required.'), 'Fallback'), 'Fallback')
  assert.equal(uiErrorMessage(new ApiError(400, 'bad_request', 'Invalid JSON body.'), 'Fallback'), 'Fallback')
  assert.equal(uiErrorMessage(new Error('Plain error'), 'Fallback'), 'Plain error')
  assert.equal(uiErrorMessage(null, 'Fallback'), 'Fallback')
})

test('uiErrorMessage uses shared copy for auth, permission, and network failures', () => {
  assert.equal(uiErrorMessage(new ApiError(401, 'unauthorized', 'Authentication context is missing.'), 'Fallback'), SESSION_EXPIRED_MESSAGE)
  assert.equal(uiErrorMessage(new ApiError(403, 'forbidden', 'Insufficient role.'), 'Fallback'), PERMISSION_DENIED_MESSAGE)
  assert.equal(
    uiErrorMessage(new ApiError(403, 'forbidden', 'Only admins may manage time standards.'), 'Fallback'),
    'Only admins may manage time standards.',
  )
  assert.equal(uiErrorMessage(new TypeError('Failed to fetch'), 'Fallback'), NETWORK_ERROR_MESSAGE)
})

test('reportUIError dispatches browser telemetry event when window is available', () => {
  const events: Event[] = []
  const previousWindow = globalThis.window

  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: {
      dispatchEvent(event: Event) {
        events.push(event)
        return true
      },
    },
  })

  const message = reportUIError(
    'portal.dashboard.load',
    new ApiError(404, 'meet_not_found', 'Meet was not found.'),
    'Fallback',
    { orgId: 'org_test' },
  )

  assert.equal(message, 'Meet was not found.')
  assert.equal(events.length, 1)
  const customEvent = events[0] as CustomEvent
  assert.equal(customEvent.type, 'aquify:ui-error')
  assert.equal(customEvent.detail.scope, 'portal.dashboard.load')
  assert.equal(customEvent.detail.status, 404)
  assert.equal(customEvent.detail.code, 'meet_not_found')
  assert.deepEqual(customEvent.detail.meta, { orgId: 'org_test' })

  if (previousWindow === undefined) {
    // @ts-expect-error cleanup test shim
    delete globalThis.window
  } else {
    Object.defineProperty(globalThis, 'window', {
      configurable: true,
      value: previousWindow,
    })
  }
})
