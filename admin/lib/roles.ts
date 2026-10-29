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

/** Highest tier across a user's full role set; 0 when empty. */
export function highestRoleTier(userRoles: readonly string[] | undefined | null): number {
  return (userRoles ?? []).reduce((max, r) => Math.max(max, ROLE_HIERARCHY[r as Role] ?? 0), 0)
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
  {
    id: 'volunteer', label: 'Volunteers', path: '/volunteer', icon: 'user-check',
    allowedRoles: ['admin', 'coach'],
    section: 'Competition',
  },
  // Training
  {
    id: 'training', label: 'Training', path: '/training', icon: 'activity',
    allowedRoles: ['admin', 'coach'],
    section: 'Training',
  },
  {
    id: 'attendance', label: 'Roll call', path: '/attendance', icon: 'check-square',
    allowedRoles: ['admin', 'coach'],
    section: 'Training',
  },
  {
    id: 'wellness', label: 'Wellness', path: '/wellness', icon: 'activity',
    allowedRoles: ['admin', 'coach'],
    section: 'Training',
  },
  {
    id: 'club-events', label: 'Club events', path: '/events', icon: 'calendar',
    allowedRoles: ['admin', 'coach'],
    section: 'Communications',
  },
  // Performance
  {
    id: 'seasons', label: 'Seasons', path: '/seasons', icon: 'calendar',
    allowedRoles: ['admin'],
    section: 'Performance',
  },
  {
    id: 'time-standards', label: 'Time standards', path: '/performance/time-standards', icon: 'clipboard-list',
    allowedRoles: ['admin', 'coach'],
    section: 'Performance',
  },
  {
    id: 'best-times', label: 'Best times', path: '/performance/best-times', icon: 'zap',
    allowedRoles: ['admin', 'coach'],
    section: 'Performance',
  },
  {
    id: 'visualization', label: 'Team coverage', path: '/performance/visualization', icon: 'bar-chart-2',
    allowedRoles: ['admin', 'coach'],
    section: 'Performance',
  },
  {
    id: 'team-records', label: 'Team records', path: '/performance/records', icon: 'clock',
    allowedRoles: ['admin', 'coach'],
    section: 'Performance',
  },
  {
    id: 'top-times', label: 'Top times', path: '/performance/top-times', icon: 'layers',
    allowedRoles: ['admin', 'coach'],
    section: 'Performance',
  },
  {
    id: 'relay-builder', label: 'Relay builder', path: '/performance/relays', icon: 'users',
    allowedRoles: ['admin', 'coach'],
    section: 'Performance',
  },
  {
    id: 'progress-report', label: 'Progress report', path: '/performance/progress', icon: 'activity',
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
  {
    id: 'billing-schedules', label: 'Fee schedules', path: '/billing/schedules', icon: 'calendar',
    allowedRoles: ['admin'],
    section: 'Billing',
  },
  {
    id: 'billing-runs', label: 'Billing runs', path: '/billing/runs', icon: 'refresh-cw',
    allowedRoles: ['admin'],
    section: 'Billing',
  },
  {
    id: 'billing-reports', label: 'Finance reports', path: '/billing/reports', icon: 'activity',
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
    id: 'campaigns', label: 'Email campaigns', path: '/campaigns', icon: 'mail',
    allowedRoles: ['admin'],
    section: 'Communications',
  },
  // Compliance
  {
    id: 'data-exports', label: 'Data exports', path: '/compliance/data-exports', icon: 'download',
    allowedRoles: ['admin'],
    section: 'Compliance',
  },
  {
    id: 'data-deletions', label: 'Data deletions', path: '/compliance/data-deletions', icon: 'trash-2',
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
    id: 'users', label: 'Users & roles', path: '/users', icon: 'user-check',
    allowedRoles: ['admin'],
    section: 'System',
  },
  // Advanced (engineering/ops tooling — not club-administration tasks)
  {
    id: 'audit', label: 'Audit log', path: '/audit', icon: 'shield-check',
    allowedRoles: ['admin'],
    section: 'Advanced',
  },
  {
    id: 'jobs', label: 'Background jobs', path: '/jobs', icon: 'cpu',
    allowedRoles: ['admin'],
    section: 'Advanced',
  },
  {
    id: 'system-diagnostics', label: 'System diagnostics', path: '/settings/system', icon: 'cpu',
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

export type NavItemWithAccess = NavItem

// Returns only the items the role set can open. Inaccessible items are hidden rather than
// shown disabled, so a coach never sees billing/settings/etc.; sections left empty vanish too.
export function filterNavItems(userRoles: Role | Role[]): NavItemWithAccess[] {
  return NAV_ITEMS.filter((item) => canAccess(userRoles, item.allowedRoles))
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
