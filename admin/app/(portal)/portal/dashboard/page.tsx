'use client'

import Link from 'next/link'
import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { parseBackendDate } from '@/lib/dates'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  fetchSwimmers,
  fetchMeets,
  fetchMeetCommitments,
  fetchInvoicesForHousehold,
  type CommitmentRecord,
} from '@/lib/adminData'
import { VolunteerHoursStrip } from '@/components/volunteer/VolunteerHoursStrip'

// ── Helpers ─────────────────────────────────────────────────────────────────
function fmtMeetDate(iso?: string): string {
  if (!iso) return '—'
  return parseBackendDate(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function calcAge(dob: string): number {
  const d = parseBackendDate(dob)
  const now = new Date()
  let age = now.getFullYear() - d.getFullYear()
  const m = now.getMonth() - d.getMonth()
  if (m < 0 || (m === 0 && now.getDate() < d.getDate())) age--
  return age
}

const GRADIENTS = [
  'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
  'linear-gradient(135deg, #10b981 0%, #059669 100%)',
  'linear-gradient(135deg, #f59e0b 0%, #d97706 100%)',
  'linear-gradient(135deg, #ef4444 0%, #dc2626 100%)',
]

// ── Derived types ─────────────────────────────────────────────────────────────
type DashAlert = {
  id: string
  type: 'invoice' | 'commitment' | 'document'
  title: string
  body: string
  detail: string
  urgency: 'high' | 'medium' | 'low'
  cta: string
  ctaPath: string
}

type DashSwimmer = {
  id: string
  name: string
  age: number
  group: string
  status: string
  gradient: string
  initials: string
  pendingCommits: number
  nextMeet: string
}

type DashMeet = {
  id: string
  name: string
  date: string
  course: string
  location: string
  commitment: 'pending' | 'committed' | 'open'
  swimmers: string[]
}

// ── Alert urgency config ─────────────────────────────────────────────────────
const URGENCY_CONFIG = {
  high: {
    borderColor: 'var(--error-border)',
    iconBg: 'var(--error-bg)',
    iconColor: 'var(--error)',
  },
  medium: {
    borderColor: 'var(--warning-border)',
    iconBg: 'var(--warning-bg)',
    iconColor: 'var(--warning)',
  },
  low: {
    borderColor: 'var(--info-border)',
    iconBg: 'var(--info-bg)',
    iconColor: 'var(--info)',
  },
}

const ALERT_ICON: Record<string, string> = {
  invoice:    'file-text',
  commitment: 'calendar',
  document:   'clipboard-list',
}

const COMMIT_STATUS = {
  pending:   { label: 'Response needed', color: 'var(--warning)', bg: 'var(--warning-bg)', border: '#fde68a' },
  committed: { label: 'Committed',       color: 'var(--success)', bg: 'var(--success-bg)', border: 'var(--success-bg)' },
  open:      { label: 'Not yet open',    color: 'var(--text-muted)', bg: 'var(--bg-surface)', border: 'var(--border)' },
}

// ── Skeleton ─────────────────────────────────────────────────────────────────
function Skeleton({ w = '100%', h = 16, r = 8 }: { w?: string | number; h?: number; r?: number }) {
  return (
    <div
      style={{
        width: w,
        height: h,
        borderRadius: r,
        background: 'linear-gradient(90deg, #f0efee 25%, #e6e5e3 50%, #f0efee 75%)',
        backgroundSize: '200% 100%',
        animation: 'shimmer 1.5s infinite',
      }}
    />
  )
}

// ── Components ─────────────────────────────────────────────────────────────
function AlertCard({ alert }: { alert: DashAlert }) {
  const cfg = URGENCY_CONFIG[alert.urgency]
  const iconName = ALERT_ICON[alert.type] as 'file-text' | 'calendar' | 'clipboard-list'

  return (
    <div
      style={{
        background: 'var(--bg-elevated)',
        borderRadius: '12px',
        border: `1px solid ${cfg.borderColor}`,
        padding: '16px 20px',
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        gap: '16px',
        animation: 'slideUp 0.35s var(--ease-out) both',
        transition: 'box-shadow 150ms ease, transform 150ms ease',
      }}
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.boxShadow = '0 6px 20px rgba(0,0,0,0.10)'
        el.style.transform = 'translateY(-1px)'
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.boxShadow = 'none'
        el.style.transform = 'translateY(0)'
      }}
    >
      <div
        style={{
          width: '40px',
          height: '40px',
          borderRadius: '12px',
          background: cfg.iconBg,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: cfg.iconColor,
          flexShrink: 0,
        }}
      >
        <Icon name={iconName} size={18} />
      </div>

      <div style={{ flex: '1 1 240px', minWidth: 0 }}>
        <p style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: 'var(--text)', lineHeight: 1.2 }}>
          {alert.body}
        </p>
        <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', marginTop: '2px' }}>
          {alert.title} · {alert.detail}
        </p>
      </div>

      <Link
        href={alert.ctaPath}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '6px',
          padding: '8px 16px',
          background: 'var(--text)',
          color: 'var(--bg-elevated)',
          borderRadius: '8px',
          fontSize: 'var(--fs-base)',
          fontWeight: 600,
          textDecoration: 'none',
          flexShrink: 0,
          transition: 'background 120ms ease',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--accent)')}
        onMouseLeave={(e) => (e.currentTarget.style.background = 'var(--text)')}
      >
        {alert.cta}
        <Icon name="chevron-right" size={13} />
      </Link>
    </div>
  )
}

function SwimmerCard({ swimmer }: { swimmer: DashSwimmer }) {
  return (
    <div
      className="card"
      style={{
        padding: '24px',
        display: 'flex',
        gap: '16px',
        alignItems: 'flex-start',
        transition: 'box-shadow 150ms ease, transform 150ms ease',
        animation: 'slideUp 0.4s var(--ease-out) 0.1s both',
      }}
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.boxShadow = '0 8px 24px rgba(0,0,0,0.10)'
        el.style.transform = 'translateY(-2px)'
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.boxShadow = 'none'
        el.style.transform = 'translateY(0)'
      }}
    >
      <div
        style={{
          width: '52px',
          height: '52px',
          borderRadius: '50%',
          background: swimmer.gradient,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'var(--bg-elevated)',
          fontSize: 'var(--fs-lg)',
          fontWeight: 700,
          fontFamily: 'var(--font-body)',
          flexShrink: 0,
        }}
      >
        {swimmer.initials}
      </div>

      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '2px' }}>
          <p style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {swimmer.name}
          </p>
          <span style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: 'var(--success)', background: 'var(--success-bg)', padding: '2px 8px', borderRadius: '999px', fontWeight: 500, flexShrink: 0 }}>
            Active
          </span>
        </div>
        <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', marginBottom: '12px' }}>
          Age {swimmer.age}{swimmer.group ? ` · ${swimmer.group}` : ''}
        </p>

        {swimmer.pendingCommits > 0 ? (
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', background: 'var(--warning-bg)', border: '1px solid #fde68a', borderRadius: '8px', padding: '4px 12px', fontSize: 'var(--fs-sm)', color: '#92400e' }}>
            <Icon name="alert-circle" size={13} style={{ color: 'var(--warning)' }} />
            {swimmer.pendingCommits} commitment pending
          </div>
        ) : (
          <div className="card" style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', padding: '4px 12px', fontSize: 'var(--fs-sm)', color: '#374151' }}>
            <Icon name="calendar" size={13} style={{ color: 'var(--text-muted)' }} />
            {swimmer.nextMeet || 'No upcoming meets'}
          </div>
        )}
      </div>

      <Link
        href={`/portal/household/swimmers/${swimmer.id}`}
        aria-label={`View ${swimmer.name}`}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '4px',
          height: '32px',
          padding: '0 12px',
          borderRadius: '8px',
          background: 'rgba(0,0,0,0.04)',
          color: 'var(--text-subtle)',
          fontSize: 'var(--fs-sm)',
          fontWeight: 600,
          flexShrink: 0,
          textDecoration: 'none',
          whiteSpace: 'nowrap',
          transition: 'background 120ms ease, color 120ms ease',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.background = 'var(--accent)'
          e.currentTarget.style.color = 'var(--bg-elevated)'
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.background = 'rgba(0,0,0,0.04)'
          e.currentTarget.style.color = 'var(--text-subtle)'
        }}
      >
        View
        <Icon name="chevron-right" size={13} />
      </Link>
    </div>
  )
}

function MeetRow({ meet }: { meet: DashMeet }) {
  const status = COMMIT_STATUS[meet.commitment]
  return (
    <div
      className="meet-row"
      style={{
        padding: '16px 20px',
        borderBottom: '1px solid rgba(0,0,0,0.06)',
        transition: 'background 120ms ease',
      }}
      onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.02)')}
      onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
    >
      <div>
        <p style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: 'var(--accent)', fontWeight: 500, letterSpacing: '0.02em' }}>
          {meet.date}
        </p>
        <p style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginTop: '2px' }}>
          {meet.course}
        </p>
      </div>

      <div>
        <p style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)', marginBottom: '2px' }}>{meet.name}</p>
        <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>{meet.location}</p>
      </div>

      <span
        style={{
          fontSize: 'var(--fs-xs)',
          fontWeight: 500,
          color: status.color,
          background: status.bg,
          border: `1px solid ${status.border}`,
          padding: '4px 12px',
          borderRadius: '999px',
          whiteSpace: 'nowrap',
        }}
      >
        {status.label}
      </span>

      {meet.commitment === 'pending' ? (
        <Link
          href="/portal/meets"
          className="btn btn-primary btn-sm"
          style={{ textDecoration: 'none' }}
        >
          Respond
        </Link>
      ) : (
        <Link
          href="/portal/meets"
          style={{ padding: '6px 16px', background: 'transparent', color: 'var(--text-subtle)', border: '1px solid rgba(0,0,0,0.12)', borderRadius: '8px', fontSize: 'var(--fs-sm)', fontWeight: 500, textDecoration: 'none', whiteSpace: 'nowrap', transition: 'border-color 120ms ease, color 120ms ease' }}
          onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--text)'; e.currentTarget.style.borderColor = 'rgba(0,0,0,0.24)' }}
          onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-subtle)'; e.currentTarget.style.borderColor = 'rgba(0,0,0,0.12)' }}
        >
          View
        </Link>
      )}
    </div>
  )
}

function SectionHeader({ title, action, actionPath }: { title: string; action?: string; actionPath?: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
      <h2 className="section-title">
        {title}
      </h2>
      {action && actionPath && (
        <Link
          href={actionPath}
          style={{ fontSize: 'var(--fs-base)', fontWeight: 500, color: 'var(--accent)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px', transition: 'color 120ms ease' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = '#d44f14')}
          onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--accent)')}
        >
          {action}
          <Icon name="chevron-right" size={14} />
        </Link>
      )}
    </div>
  )
}

// ── Page ─────────────────────────────────────────────────────────────────────
export default function ParentDashboardPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''
  const firstName = session?.user.name?.split(' ')[0] ?? 'there'

  const [alerts, setAlerts] = useState<DashAlert[]>([])
  const [dashSwimmers, setDashSwimmers] = useState<DashSwimmer[]>([])
  const [dashMeets, setDashMeets] = useState<DashMeet[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId || !householdId) {
      setLoading(false)
      setLoadError('Parent dashboard is unavailable because household context is missing.')
      return
    }
    setLoading(true)
    setLoadError(null)

    Promise.all([
      fetchSwimmers(api, orgId),
      fetchMeets(api, orgId),
      fetchInvoicesForHousehold(api, orgId, householdId),
    ])
      .then(async ([allSwimmers, allMeets, invs]) => {
        const hhSwimmers = allSwimmers.filter((s) => s.household_id === householdId)
        const swimmerIds = new Set(hhSwimmers.map((s) => s.id))

        // Sort meets by start_date, filter upcoming
        const now = new Date()
        const upcoming = allMeets
          .filter((m) => m.start_date && parseBackendDate(m.start_date) >= now)
          .sort((a, b) => parseBackendDate(a.start_date!).getTime() - parseBackendDate(b.start_date!).getTime())
          .slice(0, 5)

        // Fetch commitments for commitment_open meets
        const openMeets = allMeets.filter((m) =>
          ['commitment_open'].includes(m.status)
        )
        let allCommitments: CommitmentRecord[] = []
        if (openMeets.length > 0) {
          const arrays = await Promise.all(
            openMeets.slice(0, 8).map((m) =>
              fetchMeetCommitments(api, orgId, m.id)
            )
          )
          allCommitments = arrays.flat()
        }

        // Build swimmer dashboard cards
        const ds: DashSwimmer[] = hhSwimmers.map((sw, idx) => {
          const swCommits = allCommitments.filter(
            (c) => c.swimmer_id === sw.id && c.status === 'invited'
          )
          const nextMeet = upcoming.find(() => true)
          return {
            id: sw.id,
            name: `${sw.first_name} ${sw.last_name}`,
            age: calcAge(sw.date_of_birth),
            group: '',
            status: sw.registration_status,
            gradient: GRADIENTS[idx % GRADIENTS.length],
            initials: `${sw.first_name[0] ?? ''}${sw.last_name[0] ?? ''}`.toUpperCase(),
            pendingCommits: swCommits.length,
            nextMeet: nextMeet?.name ?? '',
          }
        })

        // Build meet dashboard rows
        const dm: DashMeet[] = upcoming.map((m) => {
          const meetCommits = allCommitments.filter((c) => c.meet_id === m.id)
          const hhCommits = meetCommits.filter((c) => swimmerIds.has(c.swimmer_id))
          let commitment: 'pending' | 'committed' | 'open' = 'open'
          if (m.status === 'commitment_open') {
            const hasPending = hhCommits.some((c) => c.status === 'invited')
            const hasCommitted = hhCommits.some((c) =>
              ['committed_yes', 'coach_reviewed', 'locked'].includes(c.status)
            )
            if (hasPending) commitment = 'pending'
            else if (hasCommitted) commitment = 'committed'
          }
          const swimmerNames = hhSwimmers
            .filter((sw) => hhCommits.some((c) => c.swimmer_id === sw.id && ['committed_yes', 'coach_reviewed', 'locked'].includes(c.status)))
            .map((sw) => sw.first_name)

          return {
            id: m.id,
            name: m.name,
            date: fmtMeetDate(m.start_date),
            course: m.course,
            location: m.location ?? '—',
            commitment,
            swimmers: swimmerNames,
          }
        })

        // Build alerts
        const newAlerts: DashAlert[] = []
        const dueInvoices = invs.filter(
          (inv) => (inv.status === 'issued' || inv.status === 'partially_paid') && inv.amount_due_cents > 0
        )
        if (dueInvoices.length > 0) {
          const inv = dueInvoices[0]
          newAlerts.push({
            id: `inv-${inv.id}`,
            type: 'invoice',
            title: 'Invoice due',
            body: `$${(inv.amount_due_cents / 100).toFixed(2)} outstanding`,
            detail: inv.notes ?? `Invoice ${inv.invoice_number}`,
            urgency: 'high',
            cta: 'Pay Now',
            ctaPath: '/portal/billing',
          })
        }
        const pendingMeets = dm.filter((m) => m.commitment === 'pending')
        if (pendingMeets.length > 0) {
          const pm = pendingMeets[0]
          newAlerts.push({
            id: `commit-${pm.id}`,
            type: 'commitment',
            title: 'Meet response needed',
            body: pm.name,
            detail: 'Commitment deadline approaching',
            urgency: 'medium',
            cta: 'Respond',
            ctaPath: '/portal/meets',
          })
        }

        setAlerts(newAlerts)
        setDashSwimmers(ds)
        setDashMeets(dm)
      })
      .catch((error) => {
        setAlerts([])
        setDashSwimmers([])
        setDashMeets([])
        setLoadError(reportUIError(
          'portal-dashboard-load',
          error,
          'Parent dashboard data is unavailable right now.',
          { orgId, householdId },
        ))
      })
      .finally(() => setLoading(false))
  }, [api, orgId, householdId, reloadKey])

  const now = new Date()
  const hour = now.getHours()
  const greeting = hour < 12 ? 'Good morning' : hour < 17 ? 'Good afternoon' : 'Good evening'

  return (
    <div>
      <style>{`
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
      `}</style>

      {/* Page greeting */}
      <div className="page-heading">
        <h1 className="page-title">
          {greeting}, {firstName}.
        </h1>
      </div>

      {loadError && (
        <div
          style={{
            marginBottom: '24px',
            background: 'var(--warning-bg)',
            border: '1px solid #fdba74',
            borderRadius: '12px',
            padding: '16px 16px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: '16px',
          }}
        >
          <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-start' }}>
            <Icon name="alert-circle" size={16} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
            <p style={{ margin: 0, fontSize: 'var(--fs-base)', color: 'var(--warning)', lineHeight: 1.5 }}>{loadError}</p>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{
              padding: '8px 16px',
              borderRadius: '8px',
              border: '1px solid #fdba74',
              background: 'var(--bg-elevated)',
              color: 'var(--warning)',
              fontSize: 'var(--fs-sm)',
              fontWeight: 600,
              cursor: 'pointer',
              whiteSpace: 'nowrap',
            }}
          >
            Retry
          </button>
        </div>
      )}

      <VolunteerHoursStrip />

      {/* Needs Attention */}
      <section style={{ marginBottom: 'var(--space-8)' }}>
        <SectionHeader title="Needs your attention" />
        {loading ? (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: '16px' }}>
            {[0, 1].map((i) => (
              <div key={i} className="card" style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
                <Skeleton w={38} h={38} r={10} />
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                  <Skeleton w="40%" h={12} />
                  <Skeleton w="70%" h={20} />
                  <Skeleton w="60%" h={13} />
                </div>
                <Skeleton w={90} h={34} r={8} />
              </div>
            ))}
          </div>
        ) : loadError ? (
          <div
            style={{
              background: 'var(--warning-bg)',
              border: '1px solid #fdba74',
              borderRadius: '12px',
              padding: '20px 24px',
              display: 'flex',
              alignItems: 'center',
              gap: '12px',
              color: 'var(--warning)',
              fontSize: 'var(--fs-md)',
            }}
          >
            <Icon name="alert-circle" size={18} style={{ color: 'var(--warning)' }} />
            Parent dashboard actions are unavailable until the latest data loads successfully.
          </div>
        ) : shouldShowCollectionEmptyState(loading, loadError, alerts.length) ? (
          <div
            style={{
              background: 'var(--success-bg)',
              border: '1px solid #bbf7d0',
              borderRadius: '12px',
              padding: '20px 24px',
              display: 'flex',
              alignItems: 'center',
              gap: '12px',
              color: '#166534',
              fontSize: 'var(--fs-md)',
            }}
          >
            <Icon name="check-circle" size={18} style={{ color: 'var(--success)' }} />
            You&apos;re all caught up — no pending actions right now.
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            {alerts.map((alert, i) => (
              <div key={alert.id} style={{ animationDelay: `${i * 60}ms`, animation: 'slideUp 0.35s var(--ease-out) both' }}>
                <AlertCard alert={alert} />
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Swimmers */}
      <section style={{ marginBottom: 'var(--space-8)' }}>
        <SectionHeader title="Your swimmers" action="View all" actionPath="/portal/household" />
        {loading ? (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: '16px' }}>
            {[0, 1].map((i) => (
              <div key={i} className="card" style={{ padding: '24px', display: 'flex', gap: '16px' }}>
                <Skeleton w={52} h={52} r={26} />
                <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px' }}>
                  <Skeleton w="50%" h={15} />
                  <Skeleton w="70%" h={12} />
                </div>
              </div>
            ))}
          </div>
        ) : loadError ? (
          <div style={{ color: 'var(--text-muted)', fontSize: 'var(--fs-md)' }}>Household swimmer data is unavailable right now.</div>
        ) : shouldShowCollectionEmptyState(loading, loadError, dashSwimmers.length) ? (
          <div style={{ color: 'var(--text-muted)', fontSize: 'var(--fs-md)' }}>No swimmers found for your household.</div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: '16px' }}>
            {dashSwimmers.map((swimmer) => (
              <SwimmerCard key={swimmer.id} swimmer={swimmer} />
            ))}
          </div>
        )}
      </section>

      {/* Upcoming meets */}
      <section>
        <SectionHeader title="Upcoming meets" action="See all meets" actionPath="/portal/meets" />
        {loading ? (
          <div className="card" style={{ overflow: 'hidden' }}>
            {[0, 1, 2].map((i) => (
              <div key={i} style={{ display: 'grid', gridTemplateColumns: '90px 1fr auto auto', gap: '16px', padding: '16px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)', alignItems: 'center' }}>
                <Skeleton w={80} h={14} />
                <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                  <Skeleton w="60%" h={14} />
                  <Skeleton w="40%" h={12} />
                </div>
                <Skeleton w={90} h={26} r={999} />
                <Skeleton w={70} h={30} r={7} />
              </div>
            ))}
          </div>
        ) : loadError ? (
          <div className="card" style={{ padding: '32px', textAlign: 'center', color: 'var(--text-muted)', fontSize: 'var(--fs-md)' }}>
            Upcoming meet data is unavailable right now.
          </div>
        ) : shouldShowCollectionEmptyState(loading, loadError, dashMeets.length) ? (
          <div className="card" style={{ padding: '32px', textAlign: 'center', color: 'var(--text-muted)', fontSize: 'var(--fs-md)' }}>
            No upcoming meets scheduled.
          </div>
        ) : (
          <div className="card" style={{ overflow: 'hidden', animation: 'slideUp 0.45s var(--ease-out) 0.15s both' }}>
            {dashMeets.map((meet, i) => (
              <div key={meet.id} style={{ animation: `slideUp 0.3s var(--ease-out) ${0.2 + i * 0.07}s both` }}>
                <MeetRow meet={meet} />
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
