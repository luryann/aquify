import { coolingOffDaysLeft, type ClubActivityEvent, type PlatformOrg } from './tenancyData.ts'

/** A setup that has run this long without finishing is treated as stuck. */
export const STUCK_SETUP_MINUTES = 15

export type AttentionKind = 'delete_ready' | 'setup_stuck' | 'closing' | 'paused'
export type AttentionItem = { org: PlatformOrg; kind: AttentionKind; title: string; detail: string; tone: 'error' | 'warning' | 'default' }

const ORDER: AttentionKind[] = ['delete_ready', 'setup_stuck', 'closing', 'paused']

/** Clubs that need an operator, most urgent first. Healthy clubs and the platform org never appear. */
export function attentionItems(orgs: PlatformOrg[], platformOrgId: string, now: number = Date.now()): AttentionItem[] {
  const items: AttentionItem[] = []
  for (const org of orgs) {
    if (org.id === platformOrgId) continue
    if (org.status === 'offboarding') {
      const left = coolingOffDaysLeft(org.offboarding_at, 14, now)
      items.push(left === 0
        ? { org, kind: 'delete_ready', tone: 'error', title: 'Ready to delete', detail: 'The waiting period is over. Download the data export, then delete the club.' }
        : { org, kind: 'closing', tone: 'warning', title: 'Closing', detail: `${left} ${left === 1 ? 'day' : 'days'} left before its data can be deleted. You can still keep the club.` })
    } else if (org.status === 'provisioning') {
      const minutes = (now - Date.parse(org.created_at)) / 60_000
      if (minutes >= STUCK_SETUP_MINUTES) {
        items.push({ org, kind: 'setup_stuck', tone: 'error', title: 'Setup didn’t finish', detail: 'On Clubs, submit the same details again to finish setting it up.' })
      }
    } else if (org.status === 'suspended') {
      items.push({ org, kind: 'paused', tone: 'default', title: 'Paused', detail: 'Everyone at this club is locked out until you resume it.' })
    }
  }
  return items.sort((a, b) => ORDER.indexOf(a.kind) - ORDER.indexOf(b.kind) || a.org.name.localeCompare(b.org.name))
}

export type StatusCounts = Record<PlatformOrg['status'], number>

export function statusCounts(orgs: PlatformOrg[], platformOrgId: string): StatusCounts {
  const counts: StatusCounts = { provisioning: 0, active: 0, suspended: 0, offboarding: 0, deleted: 0 }
  for (const org of orgs) if (org.id !== platformOrgId) counts[org.status] += 1
  return counts
}

export type OperatorEvent = ClubActivityEvent & { orgId: string; orgName: string }

/** Merges every club's operator activity into one newest-first feed. */
export function mergeActivity(perClub: { org: Pick<PlatformOrg, 'id' | 'name'>; events: ClubActivityEvent[] }[], limit = 200): OperatorEvent[] {
  return perClub
    .flatMap(({ org, events }) => events.map((ev) => ({ ...ev, orgId: org.id, orgName: org.name })))
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))
    .slice(0, limit)
}

export type HealthCheck = { name: string; state: 'ok' | 'off' | 'down'; detail: string }

const CHECK_LABELS: Record<string, string> = {
  database: 'Database', postgres: 'Database', redis: 'Job queue (Redis)', storage: 'File storage', object_storage: 'File storage',
  keycloak: 'Sign-in (Keycloak)', stripe: 'Payments (Stripe)', email: 'Email',
}

/** Turns the readiness checks ("ready", "disabled", "ready (stub)", anything else = failing) into plain rows. */
export function healthChecks(checks: Record<string, string>): HealthCheck[] {
  return Object.entries(checks).map(([key, raw]) => {
    const text = raw.trim()
    const lower = text.toLowerCase()
    const state: HealthCheck['state'] = lower.startsWith('ready') ? 'ok' : lower.startsWith('disabled') || lower === 'not-configured' ? 'off' : 'down'
    const label = CHECK_LABELS[key] ?? key.replace(/[_-]+/g, ' ').replace(/^./, (c) => c.toUpperCase())
    return { name: label, state, detail: state === 'ok' ? (lower === 'ready' ? 'Working' : text.replace(/^ready\s*/i, '').replace(/[()]/g, '') || 'Working') : state === 'off' ? 'Not turned on' : text }
  }).sort((a, b) => ({ down: 0, off: 1, ok: 2 }[a.state] - { down: 0, off: 1, ok: 2 }[b.state]) || a.name.localeCompare(b.name))
}
