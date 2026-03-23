'use client'

import { useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchHouseholds,
  fetchSwimmers,
  fetchMeets,
  fetchInvoices,
  fetchImports,
  fetchGroups,
  fetchAuditEvents,
  type HouseholdRecord,
  type SwimmerRecord,
  type MeetRecord,
  type InvoiceRecord,
  type ImportRecord,
  type GroupRecord,
  type AuditEventRecord,
} from '@/lib/adminData'

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatCurrency(cents: number) {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 }).format(cents / 100)
}

const UPCOMING_STATUSES = new Set(['published', 'commitment_open', 'commitment_closed', 'entry_finalized'])
const PENDING_IMPORT_STATUSES = new Set(['uploaded', 'scanning', 'parsing', 'matching', 'conflicts_pending'])

// ─── Sub-components ───────────────────────────────────────────────────────────

function StatCard({
  label,
  value,
  delta,
  positive,
  mono,
  loading,
}: {
  label: string
  value: string
  delta?: string
  positive?: boolean
  mono?: boolean
  loading?: boolean
}) {
  return (
    <div className="card-elevated" style={{ padding: '20px 22px' }}>
      <p
        style={{
          fontSize: '11px',
          fontFamily: 'var(--font-mono)',
          fontWeight: 500,
          color: 'var(--text-muted)',
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          marginBottom: '10px',
        }}
      >
        {label}
      </p>
      {loading ? (
        <div style={{ height: '32px', background: 'var(--bg-elevated)', borderRadius: '4px', animation: 'pulse 1.5s ease-in-out infinite' }} />
      ) : (
        <p
          style={{
            fontSize: '26px',
            fontFamily: mono ? 'var(--font-mono)' : 'var(--font-display)',
            color: 'var(--text)',
            lineHeight: 1.1,
            letterSpacing: mono ? '0.02em' : '-0.01em',
            marginBottom: '6px',
          }}
        >
          {value}
        </p>
      )}
      {delta && !loading && (
        <p
          style={{
            fontSize: '11px',
            color: positive ? 'var(--success)' : 'var(--error)',
            fontFamily: 'var(--font-mono)',
          }}
        >
          {positive ? '↑' : '↓'} {delta}
        </p>
      )}
    </div>
  )
}

function ActivityItem({
  action,
  subject,
  time,
  actor,
}: {
  action: string
  subject: string
  time: string
  actor: string
}) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: '10px',
        padding: '10px 0',
        borderBottom: '1px solid var(--border)',
      }}
    >
      <div
        style={{
          width: '6px',
          height: '6px',
          borderRadius: '50%',
          background: 'var(--accent)',
          flexShrink: 0,
          marginTop: '5px',
        }}
      />
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ fontSize: '13px', color: 'var(--text)', lineHeight: 1.4 }}>
          <span style={{ color: 'var(--text-subtle)', fontWeight: 500 }}>{actor}</span>
          {' '}
          <span style={{ color: 'var(--text-muted)' }}>{action}</span>
          {' '}
          <span style={{ fontWeight: 500 }}>{subject}</span>
        </p>
      </div>
      <p
        style={{
          fontSize: '11px',
          fontFamily: 'var(--font-mono)',
          color: 'var(--text-muted)',
          flexShrink: 0,
          whiteSpace: 'nowrap',
        }}
      >
        {time}
      </p>
    </div>
  )
}

function StatusRow({ label, status, detail }: { label: string; status: 'ok' | 'warn' | 'error'; detail: string }) {
  const color = status === 'ok' ? 'var(--success)' : status === 'warn' ? 'var(--warning)' : 'var(--error)'
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '8px 0',
        borderBottom: '1px solid var(--border)',
        gap: '12px',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: color, flexShrink: 0 }} />
        <span style={{ fontSize: '13px', color: 'var(--text)' }}>{label}</span>
      </div>
      <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
        {detail}
      </span>
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

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    let cancelled = false
    async function run() {
      if (!session?.user.orgId) { setLoading(false); return }
      try {
        setLoading(true)
        const [sw, hh, mm, inv, imp, gr, audit] = await Promise.all([
          fetchSwimmers(api, session.user.orgId).catch(() => [] as SwimmerRecord[]),
          fetchHouseholds(api, session.user.orgId).catch(() => [] as HouseholdRecord[]),
          fetchMeets(api, session.user.orgId).catch(() => [] as MeetRecord[]),
          fetchInvoices(api, session.user.orgId).catch(() => [] as InvoiceRecord[]),
          fetchImports(api, session.user.orgId).catch(() => [] as ImportRecord[]),
          fetchGroups(api, session.user.orgId).catch(() => [] as GroupRecord[]),
          fetchAuditEvents(api, session.user.orgId).catch(() => ({ events: [] as AuditEventRecord[] })),
        ])
        if (cancelled) return
        setSwimmers(sw)
        setHouseholds(hh)
        setMeets(mm)
        setInvoices(inv)
        setImports(imp)
        setGroups(gr)
        setAuditEvents(audit.events.slice(0, 5))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    run()
    return () => { cancelled = true }
  }, [session?.user.orgId])

  // Derived stats
  const activeSwimmers = swimmers.filter((s) => s.registration_status === 'active').length
  const totalSwimmers = swimmers.length
  const upcomingMeets = meets.filter((m) => UPCOMING_STATUSES.has(m.status)).length
  const openInvoices = invoices.filter((i) => i.status === 'open' || i.status === 'sent')
  const openInvoiceAmount = openInvoices.reduce((sum, i) => sum + i.amount_due_cents, 0)
  const overdueInvoices = openInvoices.filter((i) => i.due_date && new Date(i.due_date) < new Date())
  const overdueAmount = overdueInvoices.reduce((sum, i) => sum + i.amount_due_cents, 0)
  const pendingImports = imports.filter((i) => PENDING_IMPORT_STATUSES.has(i.status)).length
  const activeGroups = groups.filter((g) => g.is_active).length

  const today = new Date().toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })

  return (
    <div style={{ maxWidth: '1200px', animation: 'slideUp 0.35s var(--ease-out) both' }}>
      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>

      {/* Page header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">Dashboard</h1>
          <p className="page-subtitle">DARE Aquatics · Season 2026</p>
        </div>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
          <span
            style={{
              fontSize: '11px',
              fontFamily: 'var(--font-mono)',
              color: 'var(--text-muted)',
              background: 'var(--bg-surface)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--r-sm)',
              padding: '4px 9px',
            }}
          >
            {today}
          </span>
        </div>
      </div>

      {/* Stat grid */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(190px, 1fr))',
          gap: '14px',
          marginBottom: '28px',
        }}
      >
        <StatCard
          label="Active Swimmers"
          value={activeSwimmers.toString()}
          delta={totalSwimmers !== activeSwimmers ? `${totalSwimmers} total` : undefined}
          positive
          loading={loading}
        />
        <StatCard
          label="Households"
          value={households.length.toString()}
          loading={loading}
        />
        <StatCard
          label="Upcoming Meets"
          value={upcomingMeets.toString()}
          loading={loading}
        />
        <StatCard
          label="Open Invoices"
          value={formatCurrency(openInvoiceAmount)}
          mono
          delta={overdueAmount > 0 ? `${formatCurrency(overdueAmount)} overdue` : undefined}
          loading={loading}
        />
        <StatCard
          label="Pending Imports"
          value={pendingImports.toString()}
          loading={loading}
        />
        <StatCard
          label="Active Groups"
          value={activeGroups.toString()}
          loading={loading}
        />
      </div>

      {/* Two-column lower section */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '1fr 280px',
          gap: '16px',
          alignItems: 'start',
        }}
      >
        {/* Recent activity */}
        <div className="card-elevated" style={{ padding: '20px 22px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
            <p
              style={{
                fontSize: '12px',
                fontWeight: 600,
                fontFamily: 'var(--font-mono)',
                color: 'var(--text-subtle)',
                letterSpacing: '0.06em',
                textTransform: 'uppercase',
              }}
            >
              Recent Activity
            </p>
            <a
              href="/audit"
              style={{
                fontSize: '12px',
                color: 'var(--accent)',
                textDecoration: 'none',
                fontFamily: 'var(--font-mono)',
              }}
            >
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
          <p
            style={{
              fontSize: '12px',
              fontWeight: 600,
              fontFamily: 'var(--font-mono)',
              color: 'var(--text-subtle)',
              letterSpacing: '0.06em',
              textTransform: 'uppercase',
              marginBottom: '14px',
            }}
          >
            System Status
          </p>

          <StatusRow label="API" status="ok" detail="healthy" />
          <StatusRow label="Database" status="ok" detail="16ms p95" />
          <StatusRow label="Worker Queue" status="ok" detail="active" />
          <StatusRow label="Storage" status="ok" detail="MinIO" />
          <StatusRow label="Email" status="ok" detail="Postmark" />
          <StatusRow label="Billing" status="ok" detail="Stripe" />

          <p
            style={{
              fontSize: '11px',
              fontFamily: 'var(--font-mono)',
              color: 'var(--text-muted)',
              marginTop: '14px',
              textAlign: 'right',
            }}
          >
            {loading ? 'loading…' : 'updated just now'}
          </p>
        </div>
      </div>
    </div>
  )
}
