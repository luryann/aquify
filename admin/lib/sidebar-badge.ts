import type { Session } from '@/lib/session'
import type { Role } from '@/lib/roles'

export type SidebarSessionProps = {
  role: Role
  roles: Role[]
  userName?: string
  orgId: string
  token?: string
}

export function buildSidebarSessionProps(session: Session): SidebarSessionProps {
  return {
    role: session.user.role,
    roles: session.user.roles,
    userName: session.user.avatarInitials ?? session.user.name?.slice(0, 2),
    orgId: session.user.orgId,
    token: session.token || undefined,
  }
}

export function shouldFetchPendingApprovals(roles: Role | Role[], orgId?: string): orgId is string {
  const allRoles = Array.isArray(roles) ? roles : [roles]
  return Boolean(orgId) && (allRoles.includes('admin'))
}

// Cookie-backed sessions authenticate via credentials; bearer sessions (dev
// stub tokens) must also forward the token or the badge requests 401.
export function pendingApprovalsRequestInit(orgId: string, token?: string): RequestInit {
  const headers: Record<string, string> = { 'X-Org-Id': orgId }
  if (token) headers['Authorization'] = `Bearer ${token}`
  return {
    cache: 'no-store',
    credentials: 'include',
    headers,
  }
}
