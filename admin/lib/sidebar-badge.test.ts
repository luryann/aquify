import test from 'node:test'
import assert from 'node:assert/strict'

import type { Session } from './session.tsx'
import {
  buildSidebarSessionProps,
  pendingApprovalsRequestInit,
  shouldFetchPendingApprovals,
} from './sidebar-badge.ts'

const session: Session = {
  token: '',
  expiresAt: Date.now() + 60_000,
  user: {
    id: 'user-1',
    email: 'admin@example.com',
    name: 'Ada Lovelace',
    role: 'admin',
    roles: ['admin'],
    orgId: 'org-42',
    avatarInitials: 'AL',
  },
}

test('buildSidebarSessionProps forwards org scope for the approvals badge', () => {
  assert.deepEqual(buildSidebarSessionProps(session), {
    role: 'admin',
    roles: ['admin'],
    userName: 'AL',
    orgId: 'org-42',
  })
})

test('shouldFetchPendingApprovals only enables privileged scoped sessions', () => {
  assert.equal(shouldFetchPendingApprovals('admin', 'org-42'), true)
  assert.equal(shouldFetchPendingApprovals('coach', 'org-42'), false)
  assert.equal(shouldFetchPendingApprovals(['coach', 'admin'], 'org-42'), true)
  assert.equal(shouldFetchPendingApprovals('admin', ''), false)
})

test('pendingApprovalsRequestInit uses cookie-backed auth with org scoping', () => {
  assert.deepEqual(pendingApprovalsRequestInit('org-42'), {
    cache: 'no-store',
    credentials: 'include',
    headers: {
      'X-Org-Id': 'org-42',
    },
  })
})
