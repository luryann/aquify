import type { Session } from '@/lib/session'
import type { Role } from '@/lib/roles'

export type SidebarSessionProps = {
  role: Role
  roles: Role[]
  userName?: string
  orgId: string
}

export function buildSidebarSessionProps(session: Session): SidebarSessionProps {
  return {
    role: session.user.role,
    roles: session.user.roles,
    userName: session.user.avatarInitials ?? session.user.name?.slice(0, 2),
    orgId: session.user.orgId,
  }
}

export function shouldFetchPendingApprovals(roles: Role | Role[], orgId?: string): orgId is string {
  const allRoles = Array.isArray(roles) ? roles : [roles]
  return Boolean(orgId) && (allRoles.includes('admin'))
}

export function pendingApprovalsRequestInit(orgId: string): RequestInit {
  return {
    cache: 'no-store',
    credentials: 'include',
    headers: {
      'X-Org-Id': orgId,
    },
  }
}
