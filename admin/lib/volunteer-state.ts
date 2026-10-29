// Pure view helpers for the volunteer jobs & service-hours screens. Kept free
// of React and API imports so they are unit-testable with node:test.

export type VolunteerRole =
  | 'timer' | 'concessions' | 'marshal' | 'official' | 'hospitality' | 'setup' | 'admin' | 'other'

export type ComplianceStatus = 'exempt' | 'met' | 'on_track' | 'short' | 'bought_out'

export const VOLUNTEER_ROLE_LABELS: Record<VolunteerRole, string> = {
  timer: 'Timer',
  concessions: 'Concessions',
  marshal: 'Marshal',
  official: 'Official',
  hospitality: 'Hospitality',
  setup: 'Setup',
  admin: 'Clerk / admin',
  other: 'Other',
}

export const COMPLIANCE_LABELS: Record<ComplianceStatus, string> = {
  exempt: 'No requirement',
  met: 'Met',
  on_track: 'On track',
  short: 'Short',
  bought_out: 'Bought out',
}

export function formatHours(hours: number): string {
  const rounded = Math.round(hours * 100) / 100
  const text = Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(2).replace(/0$/, '')
  return `${text} hr${rounded === 1 ? '' : 's'}`
}

export function formatCents(cents: number): string {
  return (cents / 100).toLocaleString('en-US', { style: 'currency', currency: 'USD' })
}

/** Share of the requirement covered by earned + scheduled hours, each clamped to 0..1. */
export function hoursProgress(required: number, earned: number, scheduled: number): { earned: number; scheduled: number } {
  if (required <= 0) return { earned: 1, scheduled: 0 }
  const e = Math.min(1, Math.max(0, earned / required))
  const s = Math.min(1 - e, Math.max(0, scheduled / required))
  return { earned: e, scheduled: s }
}

/** Fill rate as a whole percentage; 0 when there is no capacity. */
export function fillPercent(filled: number, capacity: number): number {
  if (capacity <= 0) return 0
  return Math.round((filled / capacity) * 100)
}

/** Group items by a key while preserving first-seen order. */
export function groupInOrder<T>(items: T[], keyOf: (item: T) => string): Array<{ key: string; items: T[] }> {
  const groups = new Map<string, T[]>()
  for (const item of items) {
    const key = keyOf(item)
    const bucket = groups.get(key)
    if (bucket) bucket.push(item)
    else groups.set(key, [item])
  }
  return Array.from(groups, ([key, list]) => ({ key, items: list }))
}

/** Local-calendar day key (YYYY-MM-DD) for an ISO timestamp. */
export function localDayKey(iso: string): string {
  const d = new Date(iso)
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

/** "Sat, Mar 7 · 8:00 AM – 12:00 PM" style shift label in the viewer's zone. */
export function shiftLabel(startsAt: string, endsAt: string): string {
  const s = new Date(startsAt)
  const e = new Date(endsAt)
  const day = s.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
  const t = (d: Date) => d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
  return `${day}, ${t(s)} – ${t(e)}`
}

/** Pick the ledger row a parent should see first: the active season, else the newest. */
export function currentLedgerRow<T extends { season_status: string }>(rows: T[]): T | null {
  return rows.find((r) => r.season_status === 'active') ?? rows[0] ?? null
}
