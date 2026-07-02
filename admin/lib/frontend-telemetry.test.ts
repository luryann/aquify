import test from 'node:test'
import assert from 'node:assert/strict'

import { ApiError } from './api.ts'
import { reportUIError, uiErrorMessage } from './frontend-telemetry.ts'

test('uiErrorMessage prefers ApiError and Error messages before fallback', () => {
  assert.equal(uiErrorMessage(new ApiError(502, 'bad_gateway', 'Backend failed'), 'Fallback'), 'Backend failed')
  assert.equal(uiErrorMessage(new Error('Plain error'), 'Fallback'), 'Plain error')
  assert.equal(uiErrorMessage(null, 'Fallback'), 'Fallback')
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
    new ApiError(503, 'service_unavailable', 'Failed to load dashboard'),
    'Fallback',
    { orgId: 'org_test' },
  )

  assert.equal(message, 'Failed to load dashboard')
  assert.equal(events.length, 1)
  const customEvent = events[0] as CustomEvent
  assert.equal(customEvent.type, 'aquify:ui-error')
  assert.equal(customEvent.detail.scope, 'portal.dashboard.load')
  assert.equal(customEvent.detail.status, 503)
  assert.equal(customEvent.detail.code, 'service_unavailable')
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
