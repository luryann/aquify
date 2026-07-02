import test from 'node:test'
import assert from 'node:assert/strict'

import { ApiError } from './api.ts'
import type { ApiClient } from './api.ts'
import {
  DEFAULT_PORTAL_COMMUNICATION_PREFERENCES,
  loadPortalCommunicationPreferences,
  mergePortalCommunicationPreferences,
  portalCommunicationPreferencesChanged,
  portalCommunicationPreferencesErrorMessage,
  portalCommunicationPreferencesPayload,
  savePortalCommunicationPreferences,
} from './portal-communications-preferences.ts'

test('mergePortalCommunicationPreferences overlays persisted settings onto defaults', () => {
  const merged = mergePortalCommunicationPreferences([
    {
      event_id: 'announcement',
      label: 'Club Announcements',
      description: 'General announcements from club staff',
      email_enabled: false,
      push_enabled: true,
    },
  ])

  assert.equal(merged.length, DEFAULT_PORTAL_COMMUNICATION_PREFERENCES.length)
  assert.deepEqual(merged.find((preference) => preference.id === 'announcement'), {
    id: 'announcement',
    label: 'Club Announcements',
    description: 'General announcements from club staff',
    email: false,
    push: true,
  })
  assert.deepEqual(merged.find((preference) => preference.id === 'invoice_due'), DEFAULT_PORTAL_COMMUNICATION_PREFERENCES.find((preference) => preference.id === 'invoice_due'))
})

test('portalCommunicationPreferencesChanged compares persisted payload shape', () => {
  const current = structuredClone(DEFAULT_PORTAL_COMMUNICATION_PREFERENCES)
  const saved = structuredClone(DEFAULT_PORTAL_COMMUNICATION_PREFERENCES)

  assert.equal(portalCommunicationPreferencesChanged(current, saved), false)
  current[0] = { ...current[0], push: !current[0].push }
  assert.equal(portalCommunicationPreferencesChanged(current, saved), true)
  assert.deepEqual(portalCommunicationPreferencesPayload(current)[0], {
    event_id: current[0].id,
    email_enabled: current[0].email,
    push_enabled: current[0].push,
  })
})

test('load and save portal communication preferences use the live API contract', async () => {
  const calls: Array<{ method: string; path: string; body?: unknown; headers?: Record<string, string> }> = []
  const api = {
    get: async (path: string, _signal?: AbortSignal, headers?: Record<string, string>) => {
      calls.push({ method: 'GET', path, headers })
      return {
        preferences: [
          {
            event_id: 'invoice_due',
            label: 'Invoice Due Reminders',
            description: 'When an invoice is due or overdue',
            email_enabled: false,
            push_enabled: true,
          },
        ],
      }
    },
    put: async (path: string, body?: unknown, headers?: Record<string, string>) => {
      calls.push({ method: 'PUT', path, body, headers })
      return {
        preferences: [
          {
            event_id: 'invoice_due',
            label: 'Invoice Due Reminders',
            description: 'When an invoice is due or overdue',
            email_enabled: false,
            push_enabled: true,
          },
        ],
      }
    },
  } as unknown as ApiClient

  const loaded = await loadPortalCommunicationPreferences(api, 'org_portal')
  const saved = await savePortalCommunicationPreferences(api, 'org_portal', loaded)

  assert.equal(calls[0]?.path, '/api/v1/communication-preferences')
  assert.deepEqual(calls[0]?.headers, { 'X-Org-Id': 'org_portal' })
  assert.equal(calls[1]?.path, '/api/v1/communication-preferences')
  assert.deepEqual(calls[1]?.headers, { 'X-Org-Id': 'org_portal' })
  assert.deepEqual(calls[1]?.body, {
    preferences: portalCommunicationPreferencesPayload(loaded),
  })
  assert.deepEqual(saved, loaded)
})

test('portalCommunicationPreferencesErrorMessage surfaces API errors and hides unknown failures', () => {
  assert.equal(
    portalCommunicationPreferencesErrorMessage(new ApiError(422, 'validation_error', 'Request validation failed.')),
    'Request validation failed.',
  )
  assert.equal(
    portalCommunicationPreferencesErrorMessage(new Error('unexpected')),
    'Unable to save communication preferences right now.',
  )
})
