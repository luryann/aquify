import type { Role } from './roles'

const MFA_REQUIRED_ROLES: Role[] = ['owner', 'admin', 'coach', 'billing']

export function roleRequiresMFA(role: Role): boolean {
  return MFA_REQUIRED_ROLES.includes(role)
}

export function destinationForRole(role: string): string {
  if (role === 'parent') return '/portal/dashboard'
  if (role === 'swimmer') return '/swim'
  return '/dashboard'
}
