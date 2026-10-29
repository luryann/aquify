'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useSession } from '@/lib/session'
import { parseBackendDate, localToday } from '@/lib/dates'
import { useApiClient } from '@/lib/api'
import { Icon } from '@/components/ui/icons'
import { displayNameForUser, fetchAuditEvents, fetchGroups, fetchHouseholds, fetchInvoices, fetchMeetCommitments, fetchMeetEntries, fetchMeets, fetchPerformanceBestTimes, fetchPractices, fetchSeasons, fetchSwimmers, fetchUsers, type AuditEventRecord, type BestTimeRecord, type GroupRecord, type HouseholdRecord, type InvoiceRecord, type MeetRecord, type PracticeRecord, type SeasonRecord, type SwimmerRecord, type UserRecord } from '@/lib/adminData'
import './page.inline.css'

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

function ActivityItem({ description, time, actor }: { description: string; time: string; actor?: string }) {
  return (
    <div className="u-gap-12px u-p-12px-0" style={{ display: 'flex', alignItems: 'flex-start', borderBottom: '1px solid var(--border)' }}>
      <div className="u-mt-4px" style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--accent)', flexShrink: 0 }} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <p className="u-fs-fs-base u-lh-1_4" style={{ color: 'var(--text)' }}>
          {actor && (
            <>
              <span className="u-fw-500" style={{ color: 'var(--text-subtle)' }}>{actor}</span>
              {' '}
            </>
          )}
          <span style={{ color: actor ? 'var(--text-muted)' : 'var(--text)' }}>{description}</span>
        </p>
      </div>
      <p className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)', flexShrink: 0, whiteSpace: 'nowrap' }}>
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
    <div className="card-elevated u-p-20px-24px" >
      <div className="u-mb-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <p className="u-fs-fs-md u-fw-600 u-ff-font-body" style={{ color: 'var(--text)' }}>
          Performance snapshot
        </p>
        <a href="/performance/visualization" className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>
          full view →
        </a>
      </div>

      {loading ? (
        <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
          {[1,2,3,4].map((i) => (
            <div key={i} style={{ height: '28px', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.08}s` }} />
          ))}
        </div>
      ) : topEvents.length === 0 ? (
        <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>
          No performance data yet.
        </p>
      ) : (
        <div className="cols-2 u-gap-20px" >
          {/* Top events */}
          <div>
            <p className="u-fs-fs-sm u-ff-font-body u-mb-12px" style={{ color: 'var(--text-muted)' }}>
              Top events
            </p>
            <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
              {topEvents.slice(0, 5).map((ev) => (
                <div key={ev.event}>
                  <div className="u-mb-4px" style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <span className="u-fs-fs-sm u-fw-500" style={{ color: 'var(--text)' }}>{ev.event}</span>
                    <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
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
            <p className="u-fs-fs-sm u-ff-font-body u-mb-12px" style={{ color: 'var(--text-muted)' }}>
              Most events swum
            </p>
            <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
              {topSwimmers.slice(0, 5).map((sw, i) => (
                <div key={sw.name} className="u-gap-8px" style={{ display: 'flex', alignItems: 'center' }}>
                  <span className="u-fs-fs-2xs u-ff-font-body" style={{ color: 'var(--text-muted)', width: '14px', textAlign: 'right', flexShrink: 0 }}>
                    {i + 1}
                  </span>
                  <span className="u-fs-fs-sm" style={{ color: 'var(--text)', flex: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {sw.name}
                  </span>
                  <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--accent-text, var(--accent))', flexShrink: 0 }}>
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
  const [practices, setPractices] = useState<PracticeRecord[]>([])
  const [entriesAwaiting, setEntriesAwaiting] = useState(0)
  const [awaitingResponses, setAwaitingResponses] = useState(0)
  const [loadFailed, setLoadFailed] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  // Invoices, the audit log, and the user directory are admin-only APIs; coaches
  // must not see placeholder billing totals or an empty "recent activity" feed.
  const roles = session?.user.roles ?? (session?.user.role ? [session.user.role] : [])
  const isAdmin = roles.includes('admin')

  useEffect(() => {
    let cancelled = false
    async function run() {
      if (!session?.user.orgId) { setLoading(false); return }
      try {
        setLoading(true)
        setLoadFailed(false)
        let failures = 0
        const [sw, hh, mm, inv, gr, audit, bt, users, sn, pr] = await Promise.all([
          fetchSwimmers(api, session.user.orgId).catch(() => { failures++; return [] as SwimmerRecord[] }),
          fetchHouseholds(api, session.user.orgId).catch(() => { failures++; return [] as HouseholdRecord[] }),
          fetchMeets(api, session.user.orgId).catch(() => { failures++; return [] as MeetRecord[] }),
          isAdmin ? fetchInvoices(api, session.user.orgId).catch(() => [] as InvoiceRecord[]) : Promise.resolve([] as InvoiceRecord[]),
          fetchGroups(api, session.user.orgId).catch(() => [] as GroupRecord[]),
          isAdmin ? fetchAuditEvents(api, session.user.orgId).catch(() => ({ events: [] as AuditEventRecord[] })) : Promise.resolve({ events: [] as AuditEventRecord[] }),
          fetchPerformanceBestTimes(api, session.user.orgId).catch(() => [] as BestTimeRecord[]),
          isAdmin ? fetchUsers(api, session.user.orgId, { limit: 200 }).catch(() => ({ users: [] as UserRecord[] })) : Promise.resolve({ users: [] as UserRecord[] }),
          fetchSeasons(api, session.user.orgId).catch(() => [] as SeasonRecord[]),
          fetchPractices(api, session.user.orgId).catch(() => [] as PracticeRecord[]),
        ])
        const orgId = session.user.orgId
        const live = mm.filter((m) => UPCOMING_STATUSES.has(m.status)).slice(0, 6)
        const entryLists = await Promise.all(live.map((m) => fetchMeetEntries(api, orgId, m.id).catch(() => [])))
        const openMeets = live.filter((m) => m.status === 'commitment_open')
        const commitmentLists = await Promise.all(openMeets.map((m) => fetchMeetCommitments(api, orgId, m.id).catch(() => [])))
        if (cancelled) return
        setPractices(pr)
        setEntriesAwaiting(entryLists.flat().filter((e) => e.status === 'submitted').length)
        setAwaitingResponses(commitmentLists.flat().filter((c) => c.status === 'invited').length)
        setLoadFailed(failures >= 2)
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
  }, [session?.user.orgId, api, isAdmin, reloadKey])

  // Derived stats
  const activeSwimmers = swimmers.filter((s) => s.registration_status === 'active').length
  const totalSwimmers = swimmers.length
  const upcomingMeetsList = meets
    .filter((m) => UPCOMING_STATUSES.has(m.status))
    .sort((a, b) => (a.start_date ?? '').localeCompare(b.start_date ?? ''))
    .slice(0, 3)
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

  const activeSeason = seasons.find((s) => s.status === 'active') ?? null
  const seasonLabel = activeSeason ? activeSeason.name : 'No active season'

  const todayIso = localToday()
  const upcomingPractice = practices
    .filter((pr) => pr.date >= todayIso && pr.type !== 'rest')
    .sort((x, y) => x.date.localeCompare(y.date))[0]
  const groupName = (id?: string) => groups.find((g) => g.id === id)?.name ?? 'All groups'
  const practiceWhen = upcomingPractice
    ? (upcomingPractice.date === todayIso ? 'Today' : parseBackendDate(upcomingPractice.date).toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' }))
    : ''
  const isEmptyClub = !loading && !loadFailed && totalSwimmers === 0 && households.length === 0 && meets.length === 0 && groups.length === 0

  type TodayRow = { key: string; count: string; title: string; detail: string; href: string; urgent: boolean; show: boolean }
  const todayRows: TodayRow[] = [
    {
      key: 'practice', count: upcomingPractice ? practiceWhen : '0',
      title: upcomingPractice ? `Next practice: ${groupName(upcomingPractice.group_id)}` : 'No practice scheduled',
      detail: upcomingPractice ? (upcomingPractice.description || `${upcomingPractice.yardage_target.toLocaleString()} yards`) : 'Schedule one in Training.',
      href: '/training', urgent: false, show: true,
    },
    { key: 'entries', count: String(entriesAwaiting), title: 'Entries awaiting approval', detail: entriesAwaiting > 0 ? 'Review and approve before the entry file is finalized.' : 'Nothing waiting for review.', href: '/entries', urgent: entriesAwaiting > 0, show: true },
    {
      key: 'commitments', count: nextCommitmentDeadlineDays != null ? `${nextCommitmentDeadlineDays}d` : String(awaitingResponses),
      title: 'Commitments due',
      detail: nextCommitmentDeadlineDays != null ? `${awaitingResponses} families have not responded. Deadline in ${nextCommitmentDeadlineDays} days.` : (awaitingResponses > 0 ? `${awaitingResponses} families have not responded.` : 'No open commitment deadlines.'),
      href: '/commitments', urgent: nextCommitmentDeadlineDays != null && nextCommitmentDeadlineDays <= 3 || awaitingResponses > 0, show: true,
    },
    { key: 'overdue', count: String(overdueInvoices.length), title: 'Overdue invoices', detail: overdueInvoices.length > 0 ? `${formatCurrency(overdueAmount)} past due.` : 'No overdue balances.', href: '/billing?tab=invoices', urgent: overdueInvoices.length > 0, show: isAdmin },
  ]
  todayRows.sort((x, y) => Number(y.urgent) - Number(x.urgent))

  const setupSteps = [
    { label: 'Create a season', href: '/seasons', done: seasons.length > 0 },
    { label: 'Add training groups', href: '/members?tab=groups', done: groups.length > 0 },
    { label: 'Add households and swimmers', href: '/members?tab=households', done: households.length > 0 || totalSwimmers > 0 },
    { label: 'Schedule your first meet', href: '/meets/new', done: meets.length > 0 },
  ]

  return (
    <div className="page" style={{ animation: 'slideUp 0.35s var(--ease-out) both' }}>
      
      <div className="page-heading">
        <h1 className="page-title">Today</h1>
        <p className="page-subtitle">{loading ? '\u00a0' : seasonLabel}</p>
      </div>

      {loadFailed && (
        <div role="alert" className="row row-between u-p-12px-16px u-mb-24px" style={{ border: '1px solid var(--error)', borderRadius: 'var(--r-md, 12px)', background: 'var(--error-bg)' }}>
          <span className="u-fs-fs-base" style={{ color: 'var(--text)' }}>Some dashboard data could not be loaded. Check your connection and try again.</span>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setReloadKey((k) => k + 1)}>Retry</button>
        </div>
      )}

      {isEmptyClub ? (
        <div className="card-elevated u-p-24px u-mb-24px" >
          <h2 className="section-title">Set up your club</h2>
          <p className="page-subtitle u-mb-16px" >Nothing is scheduled yet. Work through these to get started.</p>
          <ol className="stack u-p-0" style={{ listStyle: 'none' }}>
            {setupSteps.map((st) => (
              <li key={st.href}>
                <Link href={st.href} className="today-row">
                  <span aria-hidden style={{ width: '20px', height: '20px', borderRadius: '50%', border: '2px solid var(--border-strong)', background: st.done ? 'var(--success)' : 'transparent', flexShrink: 0 }} />
                  <span className="u-fs-fs-base u-fw-600" >{st.label}</span>
                  {st.done && <span className="kbd-hint u-ml-auto" >Done</span>}
                </Link>
              </li>
            ))}
          </ol>
        </div>
      ) : (
        <section aria-label="Today" className="stack u-mb-32px" >
          {loading ? (
            [1, 2, 3, 4].map((i) => <div key={i} style={{ height: '56px', background: 'var(--bg-elevated)', borderRadius: '12px', animation: 'pulse 1.5s ease-in-out infinite' }} />)
          ) : (
            todayRows.filter((row) => row.show).map((row) => (
              <Link key={row.key} href={row.href} className={`today-row${row.urgent ? ' is-urgent' : ''}`}>
                <span className="today-count">{row.count}</span>
                <span style={{ flex: 1, minWidth: 0 }}>
                  <span className="u-fs-fs-base u-fw-600" style={{ display: 'block' }}>{row.title}</span>
                  <span className="u-fs-fs-sm" style={{ display: 'block', color: 'var(--text-subtle)' }}>{row.detail}</span>
                </span>
                <Icon name="chevron-right" size={16} />
              </Link>
            ))
          )}
        </section>
      )}

      {/* Secondary counts */}
      {!loading && !isEmptyClub && (
        <p className="u-fs-fs-sm u-mb-24px" style={{ color: 'var(--text-subtle)' }}>
          {activeSwimmers} active swimmers, {households.length} households, {activeGroups} groups, {pendingRegistrations} pending registrations, {recentBestTimes} new best times in 30 days{isAdmin ? `, ${formatCurrency(openInvoiceAmount)} open` : ''}
        </p>
      )}

      {/* Upcoming Meets + Quick Actions */}
      <div className="split-aside u-gap-16px u-mb-16px" style={{ alignItems: 'stretch' }}>
        {/* Upcoming Meets list */}
        <div className="card-elevated u-p-20px-24px" >
          <div className="u-mb-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <p className="u-fs-fs-md u-fw-600 u-ff-font-body" style={{ color: 'var(--text)' }}>
              Upcoming meets
            </p>
            <Link href="/meets" className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>
              all meets →
            </Link>
          </div>
          {loading ? (
            <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
              {[1, 2, 3].map((i) => (
                <div key={i} style={{ height: '52px', background: 'var(--bg-elevated)', borderRadius: '8px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.1}s` }} />
              ))}
            </div>
          ) : upcomingMeetsList.length === 0 ? (
            <p className="u-fs-fs-base u-p-8px-0" style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>
              No upcoming meets scheduled.
            </p>
          ) : (
            <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
              {upcomingMeetsList.map((meet) => (
                <Link
                  key={meet.id}
                  href={`/meets/${meet.id}`}
                  className="u-p-12px-16px hv-64a866" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderRadius: '8px', background: 'var(--bg-surface)', border: '1px solid var(--border)', textDecoration: 'none', transition: 'border-color 0.15s' }}
                >
                  <div>
                    <p className="u-fs-fs-base u-fw-600 u-mb-2px" style={{ color: 'var(--text)' }}>{meet.name}</p>
                    <p className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
                      {meet.start_date ? parseBackendDate(meet.start_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : 'Date TBD'}
                      {meet.course && <span style={{ marginLeft: '8px', opacity: 0.7 }}>· {meet.course}</span>}
                    </p>
                  </div>
                  <span className="u-fs-fs-2xs u-ff-font-body u-fw-700 u-ls-0_05em u-p-4px-8px" style={{ borderRadius: '6px', textTransform: 'uppercase', background: meet.status === 'commitment_open' ? 'var(--success-bg)' : meet.status === 'entry_finalized' ? 'var(--info-bg)' : 'var(--bg-elevated)', color: meet.status === 'commitment_open' ? 'var(--success)' : meet.status === 'entry_finalized' ? 'var(--info)' : 'var(--text-muted)' }}>
                    {meet.status.replace(/_/g, ' ')}
                  </span>
                </Link>
              ))}
            </div>
          )}
        </div>

        {/* Quick Actions */}
        <div className="card-elevated u-p-20px-24px" >
          <div className="u-gap-6px" style={{ display: 'flex', flexDirection: 'column' }}>
            {[
              { label: 'New meet', href: '/meets/new', icon: '＋', roles: ['admin', 'coach'] },
              { label: 'Add swimmer', href: '/members?tab=swimmers', icon: '＋', roles: ['admin', 'coach'] },
              { label: 'New household', href: '/members?tab=households', icon: '＋', roles: ['admin'] },
              { label: 'New invoice', href: '/billing?tab=invoices', icon: '＋', roles: ['admin'] },
            ]
              .filter((action) => roles.some((role) => action.roles.includes(role)))
              .map((action) => (
              <Link
                key={action.href}
                href={action.href}
                className="u-gap-12px u-p-8px-12px" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', background: 'var(--bg-surface)', border: '1px solid var(--border)', textDecoration: 'none', transition: 'all 0.15s', color: 'var(--text)' }}
                onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent)'; e.currentTarget.style.background = 'var(--accent-subtle)' }}
                onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.background = 'var(--bg-surface)' }}
              >
                <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--accent-text, var(--accent))', width: '14px', textAlign: 'center', flexShrink: 0 }}>
                  {action.icon}
                </span>
                <span className="u-fs-fs-base u-fw-500" >{action.label}</span>
              </Link>
            ))}
          </div>
        </div>
      </div>

      {/* Performance snapshot — full width */}
      <PerfSnapshot loading={loading} topEvents={topEvents} topSwimmers={topSwimmers} maxCount={maxCount} />

      {/* Recent activity — full width, bottom of page (admin-only audit feed) */}
      {isAdmin && (
      <div className="card-elevated u-p-20px-24px u-mt-16px" >
        <div className="u-mb-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <p className="u-fs-fs-md u-fw-600 u-ff-font-body" style={{ color: 'var(--text)' }}>
            Recent activity
          </p>
          <a href="/audit" className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>
            view audit log →
          </a>
        </div>
        {loading ? (
          <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
            {[1,2,3,4,5].map((i) => (
              <div key={i} style={{ height: '20px', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.1}s` }} />
            ))}
          </div>
        ) : auditEvents.length === 0 ? (
          <p className="u-fs-fs-base u-p-8px-0" style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>
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
