import type { Session } from '@/lib/session'
import type { Role } from '@/lib/roles'

export type SidebarSessionProps = {
  role: Role
  userName?: string
  orgId: string
}

export function buildSidebarSessionProps(session: Session): SidebarSessionProps {
  return {
    role: session.user.role,
    userName: session.user.avatarInitials ?? session.user.name?.slice(0, 2),
    orgId: session.user.orgId,
  }
}

export function shouldFetchPendingApprovals(role: Role, orgId?: string): orgId is string {
  return Boolean(orgId) && (role === 'owner' || role === 'admin')
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
