'use client'

import { useEffect, useState } from 'react'
import type { ClubActivityEvent, PlatformOrg } from './tenancyData'

// TEMP (design preview): sample operator data, development only. Visit /platform?mock=1 to turn it on
// (it sticks for the browser tab), /platform?mock=0 to turn it off. Delete this file and its callers to remove.
const day = 86_400_000
const ago = (ms: number) => new Date(Date.now() - ms).toISOString()

export const MOCK_ORGS: PlatformOrg[] = [
  { id: 'mock-platform', slug: 'aquify', name: 'Aquify Platform', status: 'active', created_at: '2026-01-05T12:00:00Z' },
  { id: 'mock-1', slug: 'harbor-sharks', name: 'Harbor Sharks Swim Club', status: 'active', created_at: '2026-03-14T12:00:00Z' },
  { id: 'mock-2', slug: 'lakeside-aquatics', name: 'Lakeside Aquatics', status: 'active', created_at: '2026-06-02T12:00:00Z' },
  { id: 'mock-3', slug: 'ridgeview-rays', name: 'Ridgeview Rays', status: 'suspended', created_at: '2026-07-21T12:00:00Z' },
  { id: 'mock-4', slug: 'north-shore-dolphins', name: 'North Shore Dolphins', status: 'provisioning', created_at: ago(2 * 3_600_000) },
  { id: 'mock-5', slug: 'old-mill-marlins', name: 'Old Mill Marlins', status: 'offboarding', offboarding_at: ago(16 * day), created_at: '2025-09-30T12:00:00Z' },
  { id: 'mock-6', slug: 'cedar-park-cudas', name: 'Cedar Park Cudas', status: 'offboarding', offboarding_at: ago(4 * day), created_at: '2026-02-11T12:00:00Z' },
]

export const MOCK_PLATFORM_ORG_ID = 'mock-platform'

export const MOCK_EVENTS: Record<string, ClubActivityEvent[]> = {
  'mock-1': [
    { id: 'e1', action: 'platform.user.password_reset', actor_user_id: 'op', occurred_at: ago(3 * 3_600_000), resource_id: 'u1', metadata: { target_email: 'coach@harborsharks.org', reason: 'Locked out before Saturday’s meet' } },
    { id: 'e2', action: 'platform.org.domain_verify', actor_user_id: 'op', occurred_at: ago(2 * day), resource_id: 'd1' },
  ],
  'mock-3': [{ id: 'e3', action: 'platform.org.suspend', actor_user_id: 'op', occurred_at: ago(9 * day), resource_id: 'mock-3', metadata: { reason: 'Unpaid platform invoice' } }],
  'mock-6': [{ id: 'e4', action: 'platform.org.offboard', actor_user_id: 'op', occurred_at: ago(4 * day), resource_id: 'mock-6', metadata: { reason: 'Club merged with Lakeside' } }],
  'mock-2': [{ id: 'e5', action: 'platform.user.admin_add', actor_user_id: 'op', occurred_at: ago(5 * day), resource_id: 'u9', metadata: { target_email: 'dana@lakesideaquatics.org', reason: 'New head coach' } }],
}

export const MOCK_CHECKS: Record<string, string> = {
  database: 'ready', redis: 'ready', storage: 'ready', keycloak: 'ready', stripe: 'disabled', email: 'ready (in-memory)',
}

/** null until the browser has been read, then whether sample data is on. Always false outside development. */
export function usePlatformMock(): boolean | null {
  const [on, setOn] = useState<boolean | null>(null)
  useEffect(() => {
    if (process.env.NODE_ENV !== 'development') { setOn(false); return }
    try {
      const q = new URLSearchParams(window.location.search).get('mock')
      if (q === '1') window.sessionStorage.setItem('platform-mock', '1')
      if (q === '0') window.sessionStorage.removeItem('platform-mock')
      setOn(window.sessionStorage.getItem('platform-mock') === '1')
    } catch { setOn(false) }
  }, [])
  return on
}
