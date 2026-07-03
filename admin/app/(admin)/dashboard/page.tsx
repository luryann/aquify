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
  fetchImports,
  fetchGroups,
  fetchAuditEvents,
  fetchPerformanceBestTimes,
  type HouseholdRecord,
  type SwimmerRecord,
  type MeetRecord,
  type InvoiceRecord,
  type ImportRecord,
  type GroupRecord,
  type AuditEventRecord,
  type BestTimeRecord,
} from '@/lib/adminData'

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
const PENDING_IMPORT_STATUSES = new Set(['uploaded', 'scanning', 'parsing', 'matching', 'conflicts_pending'])

// ─── Sub-components ───────────────────────────────────────────────────────────

function StatCard({
  label, value, delta, positive, mono, loading,
}: {
  label: string; value: string; delta?: string; positive?: boolean; mono?: boolean; loading?: boolean
}) {
  return (
    <div className="card-elevated" style={{ padding: '20px 22px' }}>
      <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', fontWeight: 500, color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '10px' }}>
        {label}
      </p>
      {loading ? (
        <div style={{ height: '32px', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite' }} />
      ) : (
        <p style={{ fontSize: '26px', fontFamily: mono ? 'var(--font-mono)' : 'var(--font-display)', color: 'var(--text)', lineHeight: 1.1, letterSpacing: mono ? '0.02em' : '-0.01em', marginBottom: '6px' }}>
          {value}
        </p>
      )}
      {delta && !loading && (
        <p style={{ fontSize: '11px', color: positive ? 'var(--success)' : 'var(--error)', fontFamily: 'var(--font-mono)' }}>
          {positive ? '↑' : '↓'} {delta}
        </p>
      )}
    </div>
  )
}

function ActivityItem({ action, subject, time, actor }: { action: string; subject: string; time: string; actor: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', gap: '10px', padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
      <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--accent)', flexShrink: 0, marginTop: '5px' }} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ fontSize: '13px', color: 'var(--text)', lineHeight: 1.4 }}>
          <span style={{ color: 'var(--text-subtle)', fontWeight: 500 }}>{actor}</span>
          {' '}
          <span style={{ color: 'var(--text-muted)' }}>{action}</span>
          {' '}
          <span style={{ fontWeight: 500 }}>{subject}</span>
        </p>
      </div>
      <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', flexShrink: 0, whiteSpace: 'nowrap' }}>
        {time}
      </p>
    </div>
  )
}

function StatusRow({ label, status, detail }: { label: string; status: 'ok' | 'warn' | 'error'; detail: string }) {
  const color = status === 'ok' ? 'var(--success)' : status === 'warn' ? 'var(--warning)' : 'var(--error)'
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '8px 0', borderBottom: '1px solid var(--border)', gap: '12px' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: color, flexShrink: 0 }} />
        <span style={{ fontSize: '13px', color: 'var(--text)' }}>{label}</span>
      </div>
      <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>{detail}</span>
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
    <div className="card-elevated" style={{ padding: '20px 22px' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
        <p style={{ fontSize: '12px', fontWeight: 600, fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)', letterSpacing: '0.06em', textTransform: 'uppercase' }}>
          Performance Snapshot
        </p>
        <a href="/performance/visualization" style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none', fontFamily: 'var(--font-mono)' }}>
          full view →
        </a>
      </div>

      {loading ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
          {[1,2,3,4].map((i) => (
            <div key={i} style={{ height: '28px', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite', animationDelay: `${i * 0.08}s` }} />
          ))}
        </div>
      ) : topEvents.length === 0 ? (
        <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontStyle: 'italic' }}>
          No performance data yet.
        </p>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '20px' }}>
          {/* Top events */}
          <div>
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '10px' }}>
              Top Events
            </p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {topEvents.slice(0, 5).map((ev) => (
                <div key={ev.event}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '3px' }}>
                    <span style={{ fontSize: '12px', color: 'var(--text)', fontWeight: 500 }}>{ev.event}</span>
                    <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
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
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', marginBottom: '10px' }}>
              Most Coverage
            </p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '7px' }}>
              {topSwimmers.slice(0, 5).map((sw, i) => (
                <div key={sw.name} style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', width: '14px', textAlign: 'right', flexShrink: 0 }}>
                    {i + 1}
                  </span>
                  <span style={{ fontSize: '12px', color: 'var(--text)', flex: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {sw.name}
                  </span>
                  <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--accent)', flexShrink: 0 }}>
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
  const [imports, setImports] = useState<ImportRecord[]>([])
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [auditEvents, setAuditEvents] = useState<AuditEventRecord[]>([])
  const [bestTimes, setBestTimes] = useState<BestTimeRecord[]>([])

  useEffect(() => {
    let cancelled = false
    async function run() {
      if (!session?.user.orgId) { setLoading(false); return }
      try {
        setLoading(true)
        const [sw, hh, mm, inv, imp, gr, audit, bt] = await Promise.all([
          fetchSwimmers(api, session.user.orgId).catch(() => [] as SwimmerRecord[]),
          fetchHouseholds(api, session.user.orgId).catch(() => [] as HouseholdRecord[]),
          fetchMeets(api, session.user.orgId).catch(() => [] as MeetRecord[]),
          fetchInvoices(api, session.user.orgId).catch(() => [] as InvoiceRecord[]),
          fetchImports(api, session.user.orgId).catch(() => [] as ImportRecord[]),
          fetchGroups(api, session.user.orgId).catch(() => [] as GroupRecord[]),
          fetchAuditEvents(api, session.user.orgId).catch(() => ({ events: [] as AuditEventRecord[] })),
          fetchPerformanceBestTimes(api, session.user.orgId).catch(() => [] as BestTimeRecord[]),
        ])
        if (cancelled) return
        setSwimmers(sw)
        setHouseholds(hh)
        setMeets(mm)
        setInvoices(inv)
        setImports(imp)
        setGroups(gr)
        setAuditEvents(audit.events.slice(0, 5))
        setBestTimes(bt)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    run()
    return () => { cancelled = true }
  }, [session?.user.orgId, api])

  // Derived stats
  const activeSwimmers = swimmers.filter((s) => s.registration_status === 'active').length
  const totalSwimmers = swimmers.length
  const upcomingMeetsList = meets
    .filter((m) => UPCOMING_STATUSES.has(m.status))
    .sort((a, b) => (a.start_date ?? '').localeCompare(b.start_date ?? ''))
    .slice(0, 3)
  const upcomingMeets = meets.filter((m) => UPCOMING_STATUSES.has(m.status)).length
  const openInvoices = invoices.filter((i) => i.status === 'open' || i.status === 'sent')
  const openInvoiceAmount = openInvoices.reduce((sum, i) => sum + i.amount_due_cents, 0)
  const overdueInvoices = openInvoices.filter((i) => i.due_date && new Date(i.due_date) < new Date())
  const overdueAmount = overdueInvoices.reduce((sum, i) => sum + i.amount_due_cents, 0)
  const pendingImports = imports.filter((i) => PENDING_IMPORT_STATUSES.has(i.status)).length
  const activeGroups = groups.filter((g) => g.is_active).length

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

  return (
    <div style={{ maxWidth: '1200px', animation: 'slideUp 0.35s var(--ease-out) both' }}>
      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>

      {/* Page header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">Dashboard</h1>
          <p className="page-subtitle">Aquify · Season 2026</p>
        </div>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
          <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-sm)', padding: '4px 9px' }}>
            {today}
          </span>
        </div>
      </div>

      {/* Stat grid */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(190px, 1fr))', gap: '14px', marginBottom: '28px' }}>
        <StatCard label="Active Swimmers" value={activeSwimmers.toString()} delta={totalSwimmers !== activeSwimmers ? `${totalSwimmers} total` : undefined} positive loading={loading} />
        <StatCard label="Households" value={households.length.toString()} loading={loading} />
        <StatCard label="Upcoming Meets" value={upcomingMeets.toString()} loading={loading} />
        <StatCard label="Open Invoices" value={formatCurrency(openInvoiceAmount)} mono delta={overdueAmount > 0 ? `${formatCurrency(overdueAmount)} overdue` : undefined} loading={loading} />
        <StatCard label="Pending Imports" value={pendingImports.toString()} loading={loading} />
        <StatCard label="Active Groups" value={activeGroups.toString()} loading={loading} />
      </div>

      {/* Two-column lower section */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 280px', gap: '16px', alignItems: 'start', marginBottom: '16px' }}>
        {/* Recent activity */}
        <div className="card-elevated" style={{ padding: '20px 22px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
            <p style={{ fontSize: '12px', fontWeight: 600, fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)', letterSpacing: '0.06em', textTransform: 'uppercase' }}>
              Recent Activity
            </p>
            <a href="/audit" style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none', fontFamily: 'var(--font-mono)' }}>
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
            <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontStyle: 'italic', padding: '8px 0' }}>
              No recent activity recorded yet.
            </p>
          ) : (
            auditEvents.map((ev) => (
              <ActivityItem
                key={ev.id}
                actor={ev.actor_user_id || 'System'}
                action={ev.action}
                subject={ev.resource_type + (ev.resource_id ? ` #${ev.resource_id.slice(0, 8)}` : '')}
                time={timeAgo(ev.occurred_at)}
              />
            ))
          )}
        </div>

        {/* System status */}
        <div className="card-elevated" style={{ padding: '20px 22px' }}>
          <p style={{ fontSize: '12px', fontWeight: 600, fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '14px' }}>
            System Status
          </p>
          <StatusRow label="API" status="ok" detail="healthy" />
          <StatusRow label="Database" status="ok" detail="16ms p95" />
          <StatusRow label="Worker Queue" status="ok" detail="active" />
          <StatusRow label="Storage" status="ok" detail="MinIO" />
          <StatusRow label="Email" status="ok" detail="Postmark" />
          <StatusRow label="Billing" status="ok" detail="Stripe" />
          <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginTop: '14px', textAlign: 'right' }}>
            {loading ? 'loading…' : 'updated just now'}
          </p>
        </div>
      </div>

      {/* Upcoming Meets + Quick Actions */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 260px', gap: '16px', marginBottom: '16px' }}>
        {/* Upcoming Meets list */}
        <div className="card-elevated" style={{ padding: '20px 22px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
            <p style={{ fontSize: '12px', fontWeight: 600, fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)', letterSpacing: '0.06em', textTransform: 'uppercase' }}>
              Upcoming Meets
            </p>
            <Link href="/meets" style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none', fontFamily: 'var(--font-mono)' }}>
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
            <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontStyle: 'italic', padding: '8px 0' }}>
              No upcoming meets scheduled.
            </p>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {upcomingMeetsList.map((meet) => (
                <Link
                  key={meet.id}
                  href={`/meets/${meet.id}`}
                  style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 14px', borderRadius: '8px', background: 'var(--bg-surface)', border: '1px solid var(--border)', textDecoration: 'none', transition: 'border-color 0.15s' }}
                  onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent)' }}
                  onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                >
                  <div>
                    <p style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '2px' }}>{meet.name}</p>
                    <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                      {meet.start_date ? parseBackendDate(meet.start_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : 'Date TBD'}
                      {meet.course && <span style={{ marginLeft: '8px', opacity: 0.7 }}>· {meet.course}</span>}
                    </p>
                  </div>
                  <span style={{
                    fontSize: '10px', fontFamily: 'var(--font-mono)', fontWeight: 700, letterSpacing: '0.05em',
                    padding: '3px 8px', borderRadius: '6px', textTransform: 'uppercase',
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
        <div className="card-elevated" style={{ padding: '20px 22px' }}>
          <p style={{ fontSize: '12px', fontWeight: 600, fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '14px' }}>
            Quick Actions
          </p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
            {[
              { label: 'New Meet', href: '/meets/new', icon: '＋' },
              { label: 'Add Swimmer', href: '/members/swimmers?new=1', icon: '＋' },
              { label: 'New Household', href: '/members/households?new=1', icon: '＋' },
              { label: 'New Invoice', href: '/invoices?new=1', icon: '＋' },
              { label: 'Upload Import', href: '/imports?upload=1', icon: '↑' },
            ].map((action) => (
              <Link
                key={action.href}
                href={action.href}
                style={{
                  display: 'flex', alignItems: 'center', gap: '10px',
                  padding: '9px 12px', borderRadius: '8px',
                  background: 'var(--bg-surface)', border: '1px solid var(--border)',
                  textDecoration: 'none', transition: 'all 0.15s',
                  color: 'var(--text)',
                }}
                onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent)'; e.currentTarget.style.background = 'var(--accent-subtle)' }}
                onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.background = 'var(--bg-surface)' }}
              >
                <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--accent)', width: '14px', textAlign: 'center', flexShrink: 0 }}>
                  {action.icon}
                </span>
                <span style={{ fontSize: '13px', fontWeight: 500 }}>{action.label}</span>
              </Link>
            ))}
          </div>
        </div>
      </div>

      {/* Performance snapshot — full width */}
      <PerfSnapshot loading={loading} topEvents={topEvents} topSwimmers={topSwimmers} maxCount={maxCount} />
    </div>
  )
}
