/** Roles an admin can assign, in display order. */
export const ROLE_OPTIONS: { value: string; label: string }[] = [
  { value: 'admin', label: 'Admin' },
  { value: 'coach', label: 'Coach' },
  { value: 'parent', label: 'Parent' },
  { value: 'swimmer', label: 'Swimmer' },
]

/** Badge colors per role on the users screens. */
export const ROLE_COLORS: Record<string, { bg: string; color: string }> = {
  admin:   { bg: 'rgba(251,191,36,0.12)',  color: '#fbbf24' },
  coach:   { bg: 'rgba(74,222,128,0.10)',  color: '#4ade80' },
  parent:  { bg: 'rgba(196,148,255,0.10)', color: '#c484ff' },
  swimmer: { bg: 'rgba(248,113,113,0.10)', color: '#f87171' },
}
