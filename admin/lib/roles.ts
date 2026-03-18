export type Role = 'owner' | 'admin' | 'coach' | 'parent' | 'swimmer' | 'billing' | 'read-only'

export const ROLE_LABELS: Record<Role, string> = {
  owner:       'Owner',
  admin:       'Admin',
  coach:       'Coach',
  parent:      'Parent',
  swimmer:     'Swimmer',
  billing:     'Billing',
  'read-only': 'Read-Only',
}

export const ROLE_HIERARCHY: Record<Role, number> = {
  owner:       6,
  admin:       5,
  coach:       4,
  billing:     3,
  'read-only': 2,
  parent:      1,
  swimmer:     0,
}

export type NavItem = {
  id: string
  label: string
  path: string
  icon: string
  allowedRoles: Role[]
  section?: string
}

export const NAV_SECTIONS = [
  'Overview',
  'Members',
  'Competition',
  'Performance',
  'Files',
  'Billing',
  'Communications',
  'Compliance',
  'System',
] as const

export type NavSection = (typeof NAV_SECTIONS)[number]

export const NAV_ITEMS: NavItem[] = [
  // Overview
  {
    id: 'dashboard', label: 'Dashboard', path: '/dashboard', icon: 'layout-grid',
    allowedRoles: ['owner', 'admin', 'coach', 'billing', 'read-only'],
    section: 'Overview',
  },
  // Members
  {
    id: 'households', label: 'Households', path: '/members/households', icon: 'home',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Members',
  },
  {
    id: 'swimmers', label: 'Swimmers', path: '/members/swimmers', icon: 'users',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Members',
  },
  {
    id: 'groups', label: 'Groups', path: '/members/groups', icon: 'layers',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Members',
  },
  {
    id: 'registrations', label: 'Registrations', path: '/members/registrations', icon: 'clipboard-list',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Members',
  },
  // Competition
  {
    id: 'meets', label: 'Meets', path: '/meets', icon: 'calendar',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Competition',
  },
  {
    id: 'commitments', label: 'Commitments', path: '/commitments', icon: 'clipboard-list',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Competition',
  },
  {
    id: 'entries', label: 'Entries', path: '/entries', icon: 'check-square',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Competition',
  },
  {
    id: 'practices', label: 'Practice Scheduler', path: '/practices', icon: 'clock',
    allowedRoles: ['owner', 'admin', 'coach'],
    section: 'Competition',
  },
  // Performance
  {
    id: 'time-standards', label: 'Time Standards', path: '/performance/time-standards', icon: 'clipboard-list',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Performance',
  },
  {
    id: 'best-times', label: 'Best Times', path: '/performance/best-times', icon: 'zap',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Performance',
  },
  {
    id: 'visualization', label: 'Visualization', path: '/performance/visualization', icon: 'bar-chart-2',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Performance',
  },
  // Files
  {
    id: 'imports', label: 'Imports', path: '/imports', icon: 'upload-cloud',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Files',
  },
  {
    id: 'exports', label: 'Exports', path: '/exports', icon: 'download-cloud',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Files',
  },
  // Billing
  {
    id: 'billing', label: 'Overview', path: '/billing', icon: 'dollar-sign',
    allowedRoles: ['owner', 'admin', 'billing'],
    section: 'Billing',
  },
  {
    id: 'autopay', label: 'Autopay', path: '/billing/autopay', icon: 'refresh-cw',
    allowedRoles: ['owner', 'admin', 'billing'],
    section: 'Billing',
  },
  {
    id: 'invoices', label: 'Invoices', path: '/invoices', icon: 'file-text',
    allowedRoles: ['owner', 'admin', 'billing', 'read-only'],
    section: 'Billing',
  },
  {
    id: 'ledger', label: 'Ledger', path: '/ledger', icon: 'book-open',
    allowedRoles: ['owner', 'admin', 'billing'],
    section: 'Billing',
  },
  {
    id: 'stripe', label: 'Stripe', path: '/billing/stripe', icon: 'credit-card',
    allowedRoles: ['owner', 'admin', 'billing'],
    section: 'Billing',
  },
  // Communications
  {
    id: 'announcements', label: 'Announcements', path: '/announcements', icon: 'megaphone',
    allowedRoles: ['owner', 'admin', 'coach', 'read-only'],
    section: 'Communications',
  },
  {
    id: 'campaigns', label: 'Email Campaigns', path: '/campaigns', icon: 'mail',
    allowedRoles: ['owner', 'admin', 'read-only'],
    section: 'Communications',
  },
  {
    id: 'suppression', label: 'Suppression List', path: '/comms/suppression', icon: 'slash',
    allowedRoles: ['owner', 'admin'],
    section: 'Communications',
  },
  // Compliance
  {
    id: 'data-exports', label: 'Data Exports', path: '/compliance/data-exports', icon: 'download',
    allowedRoles: ['owner', 'admin'],
    section: 'Compliance',
  },
  {
    id: 'data-deletions', label: 'Data Deletions', path: '/compliance/data-deletions', icon: 'trash-2',
    allowedRoles: ['owner', 'admin'],
    section: 'Compliance',
  },
  // System
  {
    id: 'audit', label: 'Audit Log', path: '/audit', icon: 'shield-check',
    allowedRoles: ['owner', 'admin'],
    section: 'System',
  },
  {
    id: 'jobs', label: 'Background Jobs', path: '/jobs', icon: 'cpu',
    allowedRoles: ['owner', 'admin'],
    section: 'System',
  },
  {
    id: 'reports', label: 'Reports', path: '/reports', icon: 'bar-chart-2',
    allowedRoles: ['owner', 'admin', 'billing', 'read-only'],
    section: 'System',
  },
  {
    id: 'settings', label: 'Settings', path: '/settings', icon: 'settings',
    allowedRoles: ['owner', 'admin'],
    section: 'System',
  },
  {
    id: 'roles', label: 'Role Management', path: '/settings/roles', icon: 'users',
    allowedRoles: ['owner', 'admin'],
    section: 'System',
  },
]

export function hasRole(userRole: Role, requiredRole: Role): boolean {
  return ROLE_HIERARCHY[userRole] >= ROLE_HIERARCHY[requiredRole]
}

export function canAccess(userRole: Role, allowedRoles: Role[]): boolean {
  return allowedRoles.includes(userRole)
}

export function filterNavItems(userRole: Role): NavItem[] {
  return NAV_ITEMS.filter((item) => canAccess(userRole, item.allowedRoles))
}

export function groupNavItems(items: NavItem[]): Record<string, NavItem[]> {
  const groups: Record<string, NavItem[]> = {}
  for (const item of items) {
    const section = item.section ?? 'Other'
    if (!groups[section]) groups[section] = []
    groups[section].push(item)
  }
  return groups
}
