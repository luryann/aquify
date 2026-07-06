export type Role = 'admin' | 'coach' | 'parent' | 'swimmer'

export const ROLE_LABELS: Record<Role, string> = {
  admin:   'Admin',
  coach:   'Coach',
  parent:  'Parent',
  swimmer: 'Swimmer',
}

export const ROLE_HIERARCHY: Record<Role, number> = {
  admin:   3,
  coach:   2,
  parent:  1,
  swimmer: 0,
}

export type NavItem = {
  id: string
  label: string
  path: string
  icon: string
  allowedRoles: Role[]
  section?: string
}

function normalizeRoles(userRoles: Role | Role[]): Role[] {
  return Array.isArray(userRoles) ? userRoles : [userRoles]
}

export const NAV_SECTIONS = [
  'Overview',
  'Members',
  'Competition',
  'Training',
  'Performance',
  'Billing',
  'Communications',
  'Compliance',
  'System',
  'Advanced',
] as const

export type NavSection = (typeof NAV_SECTIONS)[number]

export const NAV_ITEMS: NavItem[] = [
  // Overview
  {
    id: 'dashboard', label: 'Dashboard', path: '/dashboard', icon: 'layout-grid',
    allowedRoles: ['admin', 'coach'],
    section: 'Overview',
  },
  // Members
  {
    id: 'members', label: 'Members', path: '/members', icon: 'users',
    allowedRoles: ['admin', 'coach'],
    section: 'Members',
  },
  {
    id: 'approvals', label: 'Approvals', path: '/members/approvals', icon: 'check-circle',
    allowedRoles: ['admin'],
    section: 'Members',
  },
  // Competition
  {
    id: 'meets', label: 'Meets', path: '/meets', icon: 'calendar',
    allowedRoles: ['admin', 'coach'],
    section: 'Competition',
  },
  // Training
  {
    id: 'training', label: 'Training', path: '/training', icon: 'activity',
    allowedRoles: ['admin', 'coach'],
    section: 'Training',
  },
  // Performance
  {
    id: 'seasons', label: 'Seasons', path: '/seasons', icon: 'calendar',
    allowedRoles: ['admin'],
    section: 'Performance',
  },
  {
    id: 'time-standards', label: 'Time Standards', path: '/performance/time-standards', icon: 'clipboard-list',
    allowedRoles: ['admin', 'coach'],
    section: 'Performance',
  },
  {
    id: 'best-times', label: 'Best Times', path: '/performance/best-times', icon: 'zap',
    allowedRoles: ['admin', 'coach'],
    section: 'Performance',
  },
  {
    id: 'visualization', label: 'Team Coverage', path: '/performance/visualization', icon: 'bar-chart-2',
    allowedRoles: ['admin', 'coach'],
    section: 'Performance',
  },
  // Billing
  {
    id: 'billing', label: 'Billing', path: '/billing', icon: 'dollar-sign',
    allowedRoles: ['admin'],
    section: 'Billing',
  },
  {
    id: 'stripe', label: 'Stripe', path: '/billing/stripe', icon: 'credit-card',
    allowedRoles: ['admin'],
    section: 'Billing',
  },
  // Communications
  {
    id: 'messages', label: 'Messages', path: '/messages', icon: 'mail',
    allowedRoles: ['admin', 'coach'],
    section: 'Communications',
  },
  {
    id: 'announcements', label: 'Announcements', path: '/announcements', icon: 'megaphone',
    allowedRoles: ['admin', 'coach'],
    section: 'Communications',
  },
  {
    id: 'campaigns', label: 'Email Campaigns', path: '/campaigns', icon: 'mail',
    allowedRoles: ['admin'],
    section: 'Communications',
  },
  // Compliance
  {
    id: 'data-exports', label: 'Data Exports', path: '/compliance/data-exports', icon: 'download',
    allowedRoles: ['admin'],
    section: 'Compliance',
  },
  {
    id: 'data-deletions', label: 'Data Deletions', path: '/compliance/data-deletions', icon: 'trash-2',
    allowedRoles: ['admin'],
    section: 'Compliance',
  },
  // System
  {
    id: 'reports', label: 'Reports', path: '/reports', icon: 'bar-chart-2',
    allowedRoles: ['admin'],
    section: 'System',
  },
  {
    id: 'settings', label: 'Settings', path: '/settings', icon: 'settings',
    allowedRoles: ['admin'],
    section: 'System',
  },
  {
    id: 'users', label: 'Users & Roles', path: '/users', icon: 'user-check',
    allowedRoles: ['admin'],
    section: 'System',
  },
  // Advanced (engineering/ops tooling — not club-administration tasks)
  {
    id: 'audit', label: 'Audit Log', path: '/audit', icon: 'shield-check',
    allowedRoles: ['admin'],
    section: 'Advanced',
  },
  {
    id: 'jobs', label: 'Background Jobs', path: '/jobs', icon: 'cpu',
    allowedRoles: ['admin'],
    section: 'Advanced',
  },
  {
    id: 'system-diagnostics', label: 'System Diagnostics', path: '/settings/system', icon: 'cpu',
    allowedRoles: ['admin'],
    section: 'Advanced',
  },
]

export function hasRole(userRoles: Role | Role[], requiredRole: Role): boolean {
  return normalizeRoles(userRoles).some((userRole) => ROLE_HIERARCHY[userRole] >= ROLE_HIERARCHY[requiredRole])
}

export function canAccess(userRoles: Role | Role[], allowedRoles: Role[]): boolean {
  return normalizeRoles(userRoles).some((userRole) => allowedRoles.includes(userRole))
}

export type NavItemWithAccess = NavItem & { locked: boolean }

// Returns every nav item annotated with whether the current role can access it, rather than
// dropping inaccessible items — callers render locked items grayed-out with an explanation
// instead of silently omitting them, so a role can see what it doesn't have access to.
export function filterNavItems(userRoles: Role | Role[]): NavItemWithAccess[] {
  return NAV_ITEMS.map((item) => ({ ...item, locked: !canAccess(userRoles, item.allowedRoles) }))
}

export function groupNavItems(items: NavItemWithAccess[]): Record<string, NavItemWithAccess[]> {
  const groups: Record<string, NavItemWithAccess[]> = {}
  for (const item of items) {
    const section = item.section ?? 'Other'
    if (!groups[section]) groups[section] = []
    groups[section].push(item)
  }
  return groups
}
