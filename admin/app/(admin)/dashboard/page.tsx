'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useSession } from '@/lib/session'
import { parseBackendDate } from '@/lib/dates'
import { useApiClient } from '@/lib/api'
import {
  fetchHouseholds,
  fetchSwimmers,
  fetchMeets,
  fetchInvoices,
  fetchGroups,
  fetchAuditEvents,
  fetchPerformanceBestTimes,
  fetchUsers,
  fetchSeasons,
  type HouseholdRecord,
  type SwimmerRecord,
  type MeetRecord,
  type InvoiceRecord,
  type GroupRecord,
  type AuditEventRecord,
  type BestTimeRecord,
  type UserRecord,
  type SeasonRecord,
} from '@/lib/adminData'

function displayNameForUser(user: UserRecord): string {
  const name = `${user.firstName ?? ''} ${user.lastName ?? ''}`.trim()
  return name || user.email || user.id
}

// A dev-stub/system principal ID renders as all zeros (e.g.
// "00000000-0000-0000-0000-000000000000") — treat it as an unattributed
// system actor rather than showing the raw ID fragment as if it were a name.
function isSystemActorId(id: string): boolean {
  return /^[0-]+$/.test(id)
}

function titleCase(words: string): string {
  return words.replace(/\b\w/g, (c) => c.toUpperCase())
}

// Report-run audit events use a fixed slug ("payment_reconciliation") as
// their resource_id rather than a real record ID, so truncating it like a
// UUID produces garbage ("#payment_"). Describe these as a single generated-
// report line instead of the generic actor/action/subject shape.
function describeReportEvent(action: string): string {
  const slug = action.split('.')[1] ?? action
  return `${titleCase(slug.replace(/_/g, ' '))} report generated`
}

// Turns an internal action identifier like "meet.published" into a
// readable phrase ("meet published") for the recent-activity feed —
// admins reviewing "who did what" shouldn't have to parse dotted job codes.
function humanizeAction(action: string): string {
  return action.replace(/[._]/g, ' ')
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatCurrency(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 }).format(cents / 100)
}

function formatSwimTime(ms?: number | null): string {
  if (ms == null) return 'NT'
  const totalSeconds = ms / 1000
  if (totalSeconds < 60) return totalSeconds.toFixed(2)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = (totalSeconds % 60).toFixed(2).padStart(5, '0')
  return `${minutes}:${seconds}`
}

const UPCOMING_STATUSES = new Set(['published', 'commitment_open', 'commitment_closed', 'entry_finalized'])

// ─── Sub-components ───────────────────────────────────────────────────────────

function StatCard({
  label, value, delta, positive, caption, loading,
}: {
  label: string; value: string; delta?: string; positive?: boolean; caption?: string; loading?: boolean
}) {
  return (
    <div className="card-elevated" style={{ padding: '20px 24px' }}>
      <p style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', fontWeight: 500, color: 'var(--text-muted)', marginBottom: '12px' }}>
        {label}
      </p>
      {loading ? (
        <div style={{ height: '32px', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite' }} />
      ) : (
        <p style={{ fontSize: 'var(--fs-3xl)', fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', color: 'var(--text)', lineHeight: 1.1, letterSpacing: 'var(--display-tracking)', marginBottom: '6px' }}>
          {value}
        </p>
      )}
      {delta && !loading && (
        <p style={{ fontSize: 'var(--fs-xs)', color: positive ? 'var(--success)' : 'var(--error)', fontFamily: 'var(--font-body)' }}>
          {positive ? '↑' : '↓'} {delta}
        </p>
      )}
      {caption && !loading && (
        <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
          {caption}
        </p>
      )}
    </div>
  )
}

function ActivityItem({ description, time, actor }: { description: string; time: string; actor?: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px', padding: '12px 0', borderBottom: '1px solid var(--border)' }}>
      <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--accent)', flexShrink: 0, marginTop: '4px' }} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text)', lineHeight: 1.4 }}>
          {actor && (
            <>
              <span style={{ color: 'var(--text-subtle)', fontWeight: 500 }}>{actor}</span>
              {' '}
            </>
          )}
          <span style={{ color: actor ? 'var(--text-muted)' : 'var(--text)' }}>{description}</span>
        </p>
      </div>
      <p style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', flexShrink: 0, whiteSpace: 'nowrap' }}>
        {time}
      </p>
    </div>
  )
}

function timeAgo(isoString: string): string {
  const diff = Date.now() - new Date(isoString).getTime()
  const m = Math.floor(diff / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

// ─── Performance snapshot section ─────────────────────────────────────────────

type EventStat = { event: string; count: number; fastest?: number | null }
type SwimmerStat = { name: string; count: number }

function PerfSnapshot({
  loading,
  topEvents,
  topSwimmers,
  maxCount,
}: {
  loading: boolean
  topEvents: EventStat[]
  topSwimmers: SwimmerStat[]
  maxCount: number
}) {
  return (
    <div className="card-elevated" style={{ padding: '20px 24px' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
        <p style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)' }}>
          Performance snapshot
        </p>
        <a href="/performance/visualization" style={{ fontSize: 'var(--fs-sm)', color: 'var(--accent)', textDecoration: 'none', fontFamily: 'var(--font-body)' }}>
          full view →
        </a>
      </div>

      {loading ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          {[1,2,3,4].map((i) => (
            <div key={i} style={{ height: '28px', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.08}s` }} />
          ))}
        </div>
      ) : topEvents.length === 0 ? (
        <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', fontStyle: 'italic' }}>
          No performance data yet.
        </p>
      ) : (
        <div className="cols-2" style={{ gap: '20px' }}>
          {/* Top events */}
          <div>
            <p style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '12px' }}>
              Top events
            </p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {topEvents.slice(0, 5).map((ev) => (
                <div key={ev.event}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '4px' }}>
                    <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text)', fontWeight: 500 }}>{ev.event}</span>
                    <span style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)' }}>
                      {ev.fastest ? formatSwimTime(ev.fastest) : '—'}&nbsp;<span style={{ color: 'var(--text-muted)', opacity: 0.6 }}>·</span>&nbsp;{ev.count}
                    </span>
                  </div>
                  <div style={{ height: '3px', background: 'var(--bg-elevated)', borderRadius: '2px', overflow: 'hidden' }}>
                    <div style={{
                      height: '100%',
                      width: `${maxCount > 0 ? (ev.count / maxCount) * 100 : 0}%`,
                      background: 'var(--accent)',
                      borderRadius: '2px',
                      transition: 'width 0.6s var(--ease-out)',
                      opacity: 0.8,
                    }} />
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Top swimmers */}
          <div>
            <p style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '12px' }}>
              Most events swum
            </p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {topSwimmers.slice(0, 5).map((sw, i) => (
                <div key={sw.name} style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', width: '14px', textAlign: 'right', flexShrink: 0 }}>
                    {i + 1}
                  </span>
                  <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text)', flex: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {sw.name}
                  </span>
                  <span style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: 'var(--accent)', flexShrink: 0 }}>
                    {sw.count}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function DashboardPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)

  const [loading, setLoading] = useState(true)
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [meets, setMeets] = useState<MeetRecord[]>([])
  const [invoices, setInvoices] = useState<InvoiceRecord[]>([])
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [auditEvents, setAuditEvents] = useState<AuditEventRecord[]>([])
  const [bestTimes, setBestTimes] = useState<BestTimeRecord[]>([])
  const [actorNames, setActorNames] = useState<Record<string, string>>({})
  const [seasons, setSeasons] = useState<SeasonRecord[]>([])
  // Invoices, the audit log, and the user directory are admin-only APIs; coaches
  // must not see placeholder billing totals or an empty "recent activity" feed.
  const isAdmin = (session?.user.roles ?? (session?.user.role ? [session.user.role] : [])).includes('admin')

  useEffect(() => {
    let cancelled = false
    async function run() {
      if (!session?.user.orgId) { setLoading(false); return }
      try {
        setLoading(true)
        const [sw, hh, mm, inv, gr, audit, bt, users, sn] = await Promise.all([
          fetchSwimmers(api, session.user.orgId).catch(() => [] as SwimmerRecord[]),
          fetchHouseholds(api, session.user.orgId).catch(() => [] as HouseholdRecord[]),
          fetchMeets(api, session.user.orgId).catch(() => [] as MeetRecord[]),
          isAdmin ? fetchInvoices(api, session.user.orgId).catch(() => [] as InvoiceRecord[]) : Promise.resolve([] as InvoiceRecord[]),
          fetchGroups(api, session.user.orgId).catch(() => [] as GroupRecord[]),
          isAdmin ? fetchAuditEvents(api, session.user.orgId).catch(() => ({ events: [] as AuditEventRecord[] })) : Promise.resolve({ events: [] as AuditEventRecord[] }),
          fetchPerformanceBestTimes(api, session.user.orgId).catch(() => [] as BestTimeRecord[]),
          isAdmin ? fetchUsers(api, session.user.orgId, { limit: 200 }).catch(() => ({ users: [] as UserRecord[] })) : Promise.resolve({ users: [] as UserRecord[] }),
          fetchSeasons(api, session.user.orgId).catch(() => [] as SeasonRecord[]),
        ])
        if (cancelled) return
        setSwimmers(sw)
        setHouseholds(hh)
        setMeets(mm)
        setInvoices(inv)
        setGroups(gr)
        setAuditEvents(audit.events.slice(0, 5))
        setBestTimes(bt)
        setActorNames(Object.fromEntries(users.users.map((u) => [u.id, displayNameForUser(u)])))
        setSeasons(sn)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    run()
    return () => { cancelled = true }
  }, [session?.user.orgId, api, isAdmin])

  // Derived stats
  const activeSwimmers = swimmers.filter((s) => s.registration_status === 'active').length
  const totalSwimmers = swimmers.length
  const upcomingMeetsList = meets
    .filter((m) => UPCOMING_STATUSES.has(m.status))
    .sort((a, b) => (a.start_date ?? '').localeCompare(b.start_date ?? ''))
    .slice(0, 3)
  const upcomingMeets = meets.filter((m) => UPCOMING_STATUSES.has(m.status)).length
  const openInvoices = invoices.filter((i) => i.status === 'issued' || i.status === 'partially_paid')
  const openInvoiceAmount = openInvoices.reduce((sum, i) => sum + i.amount_due_cents, 0)
  const overdueInvoices = openInvoices.filter((i) => i.due_date && parseBackendDate(i.due_date) < new Date())
  const overdueAmount = overdueInvoices.reduce((sum, i) => sum + i.amount_due_cents, 0)
  const activeGroups = groups.filter((g) => g.is_active).length
  const pendingRegistrations = swimmers.filter((s) => s.registration_status === 'pending').length
  const thirtyDaysAgo = Date.now() - 30 * 24 * 60 * 60 * 1000
  const recentBestTimes = bestTimes.filter((bt) => bt.recorded_at && new Date(bt.recorded_at).getTime() >= thirtyDaysAgo).length
  const nextCommitmentDeadlineDays = meets
    .filter((m) => UPCOMING_STATUSES.has(m.status) && m.commitment_deadline)
    .map((m) => Math.ceil((new Date(m.commitment_deadline as string).getTime() - Date.now()) / (24 * 60 * 60 * 1000)))
    .filter((days) => days >= 0)
    .sort((a, b) => a - b)[0]

  // Performance snapshot derivations
  const swimmerNameMap = new Map(swimmers.map((s) => [s.id, `${s.first_name} ${s.last_name}`]))

  const eventMap = new Map<string, { count: number; fastest?: number | null }>()
  const swimmerBtCount = new Map<string, number>()
  for (const bt of bestTimes) {
    const key = `${bt.event_distance} ${bt.stroke}`
    const cur = eventMap.get(key) ?? { count: 0, fastest: undefined }
    cur.count += 1
    if (bt.best_time_ms != null) {
      cur.fastest = cur.fastest == null ? bt.best_time_ms : Math.min(cur.fastest, bt.best_time_ms)
    }
    eventMap.set(key, cur)
    swimmerBtCount.set(bt.swimmer_id, (swimmerBtCount.get(bt.swimmer_id) ?? 0) + 1)
  }

  const topEvents: EventStat[] = [...eventMap.entries()]
    .map(([event, v]) => ({ event, count: v.count, fastest: v.fastest }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 5)

  const topSwimmers: SwimmerStat[] = [...swimmerBtCount.entries()]
    .map(([id, count]) => ({ name: swimmerNameMap.get(id) ?? id, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 5)

  const maxCount = topEvents.reduce((m, e) => Math.max(m, e.count), 0)

  const today = new Date().toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric', year: 'numeric' })
  // Derive the dashboard subtitle from a real Season record instead of the
  // current calendar year — an implied season the Seasons page doesn't
  // actually have would mislead an owner looking for "the current season."
  const activeSeason = seasons.find((s) => s.status === 'active') ?? null
  const seasonLabel = activeSeason ? activeSeason.name : 'No active season'

  return (
    <div className="page" style={{ animation: 'slideUp 0.35s var(--ease-out) both' }}>
      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>

      {/* Page header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">Dashboard</h1>
          <p className="page-subtitle">{loading ? '\u00a0' : seasonLabel}</p>
        </div>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
          <span style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-sm)', padding: '4px 8px' }}>
            {today}
          </span>
        </div>
      </div>

      {/* Stat grid */}
      <div className="cols-4" style={{ gap: '16px', marginBottom: '24px' }}>
        <StatCard label="Active swimmers" value={activeSwimmers.toString()} delta={totalSwimmers !== activeSwimmers ? `${totalSwimmers} total` : undefined} positive loading={loading} />
        <StatCard label="Households" value={households.length.toString()} loading={loading} />
        <StatCard label="Upcoming meets" value={upcomingMeets.toString()} loading={loading} />
        {isAdmin && (
          <StatCard label="Open invoices" value={formatCurrency(openInvoiceAmount)} delta={overdueAmount > 0 ? `${formatCurrency(overdueAmount)} overdue` : undefined} loading={loading} />
        )}
        <StatCard label="Active groups" value={activeGroups.toString()} loading={loading} />
        <StatCard label="New best times" value={recentBestTimes.toString()} caption="last 30 days" loading={loading} />
        <StatCard label="Pending registrations" value={pendingRegistrations.toString()} loading={loading} />
        <StatCard
          label="Next commitment due"
          value={nextCommitmentDeadlineDays != null ? `${nextCommitmentDeadlineDays}d` : '—'}
          caption={nextCommitmentDeadlineDays != null ? 'until deadline' : 'no deadline set'}
          loading={loading}
        />
      </div>

      {/* Upcoming Meets + Quick Actions */}
      <div className="split-aside" style={{ gap: '16px', marginBottom: '16px', alignItems: 'stretch' }}>
        {/* Upcoming Meets list */}
        <div className="card-elevated" style={{ padding: '20px 24px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
            <p style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)' }}>
              Upcoming meets
            </p>
            <Link href="/meets" style={{ fontSize: 'var(--fs-sm)', color: 'var(--accent)', textDecoration: 'none', fontFamily: 'var(--font-body)' }}>
              all meets →
            </Link>
          </div>
          {loading ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[1, 2, 3].map((i) => (
                <div key={i} style={{ height: '52px', background: 'var(--bg-elevated)', borderRadius: '8px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.1}s` }} />
              ))}
            </div>
          ) : upcomingMeetsList.length === 0 ? (
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', fontStyle: 'italic', padding: '8px 0' }}>
              No upcoming meets scheduled.
            </p>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {upcomingMeetsList.map((meet) => (
                <Link
                  key={meet.id}
                  href={`/meets/${meet.id}`}
                  style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 16px', borderRadius: '8px', background: 'var(--bg-surface)', border: '1px solid var(--border)', textDecoration: 'none', transition: 'border-color 0.15s' }}
                  onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent)' }}
                  onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                >
                  <div>
                    <p style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--text)', marginBottom: '2px' }}>{meet.name}</p>
                    <p style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)' }}>
                      {meet.start_date ? parseBackendDate(meet.start_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : 'Date TBD'}
                      {meet.course && <span style={{ marginLeft: '8px', opacity: 0.7 }}>· {meet.course}</span>}
                    </p>
                  </div>
                  <span style={{
                    fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', fontWeight: 700, letterSpacing: '0.05em',
                    padding: '4px 8px', borderRadius: '6px', textTransform: 'uppercase',
                    background: meet.status === 'commitment_open' ? 'var(--success-bg)' : meet.status === 'entry_finalized' ? 'var(--info-bg)' : 'var(--bg-elevated)',
                    color: meet.status === 'commitment_open' ? 'var(--success)' : meet.status === 'entry_finalized' ? 'var(--info)' : 'var(--text-muted)',
                  }}>
                    {meet.status.replace(/_/g, ' ')}
                  </span>
                </Link>
              ))}
            </div>
          )}
        </div>

        {/* Quick Actions */}
        <div className="card-elevated" style={{ padding: '20px 24px' }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
            {[
              { label: 'New meet', href: '/meets/new', icon: '＋', roles: ['admin', 'coach'] },
              { label: 'Add swimmer', href: '/members?tab=swimmers', icon: '＋', roles: ['admin', 'coach'] },
              { label: 'New household', href: '/members?tab=households', icon: '＋', roles: ['admin'] },
              { label: 'New invoice', href: '/billing?tab=invoices', icon: '＋', roles: ['admin'] },
            ]
              .filter((action) => {
                const roles = session?.user.roles ?? (session?.user.role ? [session.user.role] : [])
                return roles.some((role) => action.roles.includes(role))
              })
              .map((action) => (
              <Link
                key={action.href}
                href={action.href}
                style={{
                  display: 'flex', alignItems: 'center', gap: '12px',
                  padding: '8px 12px', borderRadius: '8px',
                  background: 'var(--bg-surface)', border: '1px solid var(--border)',
                  textDecoration: 'none', transition: 'all 0.15s',
                  color: 'var(--text)',
                }}
                onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent)'; e.currentTarget.style.background = 'var(--accent-subtle)' }}
                onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.background = 'var(--bg-surface)' }}
              >
                <span style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--accent)', width: '14px', textAlign: 'center', flexShrink: 0 }}>
                  {action.icon}
                </span>
                <span style={{ fontSize: 'var(--fs-base)', fontWeight: 500 }}>{action.label}</span>
              </Link>
            ))}
          </div>
        </div>
      </div>

      {/* Performance snapshot — full width */}
      <PerfSnapshot loading={loading} topEvents={topEvents} topSwimmers={topSwimmers} maxCount={maxCount} />

      {/* Recent activity — full width, bottom of page (admin-only audit feed) */}
      {isAdmin && (
      <div className="card-elevated" style={{ padding: '20px 24px', marginTop: '16px' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
          <p style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)' }}>
            Recent activity
          </p>
          <a href="/audit" style={{ fontSize: 'var(--fs-sm)', color: 'var(--accent)', textDecoration: 'none', fontFamily: 'var(--font-body)' }}>
            view audit log →
          </a>
        </div>
        {loading ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {[1,2,3,4,5].map((i) => (
              <div key={i} style={{ height: '20px', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.1}s` }} />
            ))}
          </div>
        ) : auditEvents.length === 0 ? (
          <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', fontStyle: 'italic', padding: '8px 0' }}>
            No recent activity recorded yet.
          </p>
        ) : (
          auditEvents.map((ev) => {
            const isReport = ev.resource_type === 'report'
            if (isReport) {
              return (
                <ActivityItem
                  key={ev.id}
                  description={describeReportEvent(ev.action)}
                  time={timeAgo(ev.occurred_at)}
                />
              )
            }
            const actor = ev.actor_user_id
              ? (actorNames[ev.actor_user_id] ?? (isSystemActorId(ev.actor_user_id) ? 'System' : 'A team member'))
              : 'System'
            return (
              <ActivityItem
                key={ev.id}
                actor={actor}
                description={`${humanizeAction(ev.action)} ${ev.resource_type}${ev.resource_id ? ` #${ev.resource_id.slice(0, 8)}` : ''}`}
                time={timeAgo(ev.occurred_at)}
              />
            )
          })
        )}
      </div>
      )}
    </div>
  )
}
