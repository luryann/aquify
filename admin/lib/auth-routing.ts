import type { Role } from './roles'

const MFA_REQUIRED_ROLES: Role[] = ['admin', 'coach']

function normalizeRoles(roles: Role | Role[]): Role[] {
  return Array.isArray(roles) ? roles : [roles]
}

export function roleRequiresMFA(roles: Role | Role[]): boolean {
  return normalizeRoles(roles).some((role) => MFA_REQUIRED_ROLES.includes(role))
}

export function destinationForRole(roles: string | string[]): string {
  const allRoles = Array.isArray(roles) ? roles : [roles]
  if (allRoles.includes('parent')) return '/portal/dashboard'
  if (allRoles.includes('swimmer')) return '/swim'
  return '/dashboard'
}
