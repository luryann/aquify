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

// ── Helpers ─────────────────────────────────────────────────────────────────
function fmtMeetDate(iso?: string): string {
  if (!iso) return '—'
  return parseBackendDate(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function calcAge(dob: string): number {
  const d = new Date(dob)
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
    borderColor: '#ef4444',
    iconBg: '#fef2f2',
    iconColor: '#ef4444',
    badgeBg: '#fef2f2',
    badgeColor: '#dc2626',
    badgeText: 'Action Required',
  },
  medium: {
    borderColor: '#f59e0b',
    iconBg: '#fffbeb',
    iconColor: '#d97706',
    badgeBg: '#fffbeb',
    badgeColor: '#92400e',
    badgeText: 'Due Soon',
  },
  low: {
    borderColor: '#3b82f6',
    iconBg: '#eff6ff',
    iconColor: '#3b82f6',
    badgeBg: '#eff6ff',
    badgeColor: '#1d4ed8',
    badgeText: 'Upcoming',
  },
}

const ALERT_ICON: Record<string, string> = {
  invoice:    'file-text',
  commitment: 'calendar',
  document:   'clipboard-list',
}

const COMMIT_STATUS = {
  pending:   { label: 'Response Needed', color: '#d97706', bg: '#fffbeb', border: '#fde68a' },
  committed: { label: 'Committed',       color: '#16a34a', bg: '#f0fdf4', border: '#bbf7d0' },
  open:      { label: 'Not Yet Open',    color: '#6b7280', bg: '#f9fafb', border: '#e5e7eb' },
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
        background: '#ffffff',
        borderRadius: '12px',
        border: '1px solid rgba(0,0,0,0.08)',
        borderLeft: `3px solid ${cfg.borderColor}`,
        padding: '20px',
        boxShadow: '0 2px 8px rgba(0,0,0,0.06)',
        display: 'flex',
        flexDirection: 'column',
        gap: '14px',
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
        el.style.boxShadow = '0 2px 8px rgba(0,0,0,0.06)'
        el.style.transform = 'translateY(0)'
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div
          style={{
            width: '38px',
            height: '38px',
            borderRadius: '10px',
            background: cfg.iconBg,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: cfg.iconColor,
          }}
        >
          <Icon name={iconName} size={18} />
        </div>
        <span
          style={{
            fontSize: '10.5px',
            fontWeight: 600,
            fontFamily: 'var(--font-mono)',
            letterSpacing: '0.04em',
            textTransform: 'uppercase',
            color: cfg.badgeColor,
            background: cfg.badgeBg,
            padding: '3px 8px',
            borderRadius: '999px',
          }}
        >
          {cfg.badgeText}
        </span>
      </div>

      <div>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#A09D9B', letterSpacing: '0.04em', textTransform: 'uppercase', marginBottom: '4px' }}>
          {alert.title}
        </p>
        <p style={{ fontFamily: 'var(--font-display)', fontSize: '20px', color: '#1A1714', lineHeight: 1.2, marginBottom: '4px' }}>
          {alert.body}
        </p>
        <p style={{ fontSize: '12.5px', color: '#6B6866', lineHeight: 1.5 }}>
          {alert.detail}
        </p>
      </div>

      <Link
        href={alert.ctaPath}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '6px',
          padding: '8px 14px',
          background: '#1A1714',
          color: '#ffffff',
          borderRadius: '8px',
          fontSize: '13px',
          fontWeight: 600,
          textDecoration: 'none',
          alignSelf: 'flex-start',
          transition: 'background 120ms ease',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.background = '#eb5d1e')}
        onMouseLeave={(e) => (e.currentTarget.style.background = '#1A1714')}
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
      style={{
        background: '#ffffff',
        borderRadius: '14px',
        border: '1px solid rgba(0,0,0,0.08)',
        padding: '22px',
        boxShadow: '0 2px 8px rgba(0,0,0,0.05)',
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
        el.style.boxShadow = '0 2px 8px rgba(0,0,0,0.05)'
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
          color: '#ffffff',
          fontSize: '16px',
          fontWeight: 700,
          fontFamily: 'var(--font-mono)',
          flexShrink: 0,
          boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
        }}
      >
        {swimmer.initials}
      </div>

      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '2px' }}>
          <p style={{ fontSize: '15px', fontWeight: 600, color: '#1A1714', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {swimmer.name}
          </p>
          <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: '#16a34a', background: '#f0fdf4', padding: '2px 7px', borderRadius: '999px', fontWeight: 500, flexShrink: 0 }}>
            Active
          </span>
        </div>
        <p style={{ fontSize: '12px', color: '#6B6866', marginBottom: '12px' }}>
          Age {swimmer.age}{swimmer.group ? ` · ${swimmer.group}` : ''}
        </p>

        {swimmer.pendingCommits > 0 ? (
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', background: '#fffbeb', border: '1px solid #fde68a', borderRadius: '7px', padding: '5px 10px', fontSize: '12px', color: '#92400e' }}>
            <Icon name="alert-circle" size={13} style={{ color: '#d97706' }} />
            {swimmer.pendingCommits} commitment pending
          </div>
        ) : (
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', background: '#f9fafb', border: '1px solid #e5e7eb', borderRadius: '7px', padding: '5px 10px', fontSize: '12px', color: '#374151' }}>
            <Icon name="calendar" size={13} style={{ color: '#6b7280' }} />
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
          gap: '5px',
          height: '32px',
          padding: '0 10px',
          borderRadius: '8px',
          background: 'rgba(0,0,0,0.04)',
          color: '#6B6866',
          fontSize: '12px',
          fontWeight: 600,
          flexShrink: 0,
          textDecoration: 'none',
          whiteSpace: 'nowrap',
          transition: 'background 120ms ease, color 120ms ease',
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.background = '#eb5d1e'
          e.currentTarget.style.color = '#ffffff'
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.background = 'rgba(0,0,0,0.04)'
          e.currentTarget.style.color = '#6B6866'
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
      style={{
        display: 'grid',
        gridTemplateColumns: '90px 1fr auto auto',
        alignItems: 'center',
        gap: '16px',
        padding: '16px 20px',
        borderBottom: '1px solid rgba(0,0,0,0.06)',
        transition: 'background 120ms ease',
      }}
      onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.02)')}
      onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
    >
      <div>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: '#eb5d1e', fontWeight: 500, letterSpacing: '0.02em' }}>
          {meet.date}
        </p>
        <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: '#A09D9B', marginTop: '2px' }}>
          {meet.course}
        </p>
      </div>

      <div>
        <p style={{ fontSize: '14px', fontWeight: 600, color: '#1A1714', marginBottom: '2px' }}>{meet.name}</p>
        <p style={{ fontSize: '12px', color: '#9A9693' }}>{meet.location}</p>
      </div>

      <span
        style={{
          fontSize: '11.5px',
          fontWeight: 500,
          color: status.color,
          background: status.bg,
          border: `1px solid ${status.border}`,
          padding: '4px 10px',
          borderRadius: '999px',
          whiteSpace: 'nowrap',
        }}
      >
        {status.label}
      </span>

      {meet.commitment === 'pending' ? (
        <Link
          href="/portal/meets"
          style={{ padding: '6px 14px', background: '#eb5d1e', color: '#ffffff', borderRadius: '7px', fontSize: '12.5px', fontWeight: 600, textDecoration: 'none', whiteSpace: 'nowrap', transition: 'background 120ms ease' }}
          onMouseEnter={(e) => (e.currentTarget.style.background = '#d44f14')}
          onMouseLeave={(e) => (e.currentTarget.style.background = '#eb5d1e')}
        >
          Respond
        </Link>
      ) : (
        <Link
          href="/portal/meets"
          style={{ padding: '6px 14px', background: 'transparent', color: '#6B6866', border: '1px solid rgba(0,0,0,0.12)', borderRadius: '7px', fontSize: '12.5px', fontWeight: 500, textDecoration: 'none', whiteSpace: 'nowrap', transition: 'border-color 120ms ease, color 120ms ease' }}
          onMouseEnter={(e) => { e.currentTarget.style.color = '#1A1714'; e.currentTarget.style.borderColor = 'rgba(0,0,0,0.24)' }}
          onMouseLeave={(e) => { e.currentTarget.style.color = '#6B6866'; e.currentTarget.style.borderColor = 'rgba(0,0,0,0.12)' }}
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
      <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '22px', color: '#1A1714', letterSpacing: '-0.01em', lineHeight: 1.2 }}>
        {title}
      </h2>
      {action && actionPath && (
        <Link
          href={actionPath}
          style={{ fontSize: '13px', fontWeight: 500, color: '#eb5d1e', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px', transition: 'color 120ms ease' }}
          onMouseEnter={(e) => (e.currentTarget.style.color = '#d44f14')}
          onMouseLeave={(e) => (e.currentTarget.style.color = '#eb5d1e')}
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
          .filter((m) => m.start_date && new Date(m.start_date) >= now)
          .sort((a, b) => new Date(a.start_date!).getTime() - new Date(b.start_date!).getTime())
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
            title: 'Invoice Due',
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
            title: 'Respond to Meet',
            body: pm.name,
            detail: `Commitment deadline approaching`,
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
  const dateStr = now.toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' })
  const hour = now.getHours()
  const greeting = hour < 12 ? 'Good morning' : hour < 17 ? 'Good afternoon' : 'Good evening'

  return (
    <div>
      <style>{`
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
      `}</style>

      {/* Page greeting */}
      <div style={{ marginBottom: '36px', animation: 'slideUp 0.4s var(--ease-out) both' }}>
        <p style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: '#A09D9B', letterSpacing: '0.05em', marginBottom: '6px' }}>
          {dateStr}
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: 'clamp(28px, 4vw, 38px)', color: '#1A1714', letterSpacing: '-0.02em', lineHeight: 1.1 }}>
          {greeting}, {firstName}.
        </h1>
        <p style={{ fontSize: '14px', color: '#6B6866', marginTop: '8px', lineHeight: 1.6 }}>
          Here&apos;s what needs your attention today.
        </p>
      </div>

      {loadError && (
        <div
          style={{
            marginBottom: '24px',
            background: '#fff7ed',
            border: '1px solid #fdba74',
            borderRadius: '12px',
            padding: '14px 16px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: '16px',
          }}
        >
          <div style={{ display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
            <Icon name="alert-circle" size={16} style={{ color: '#ea580c', marginTop: '1px', flexShrink: 0 }} />
            <p style={{ margin: 0, fontSize: '13px', color: '#9a3412', lineHeight: 1.5 }}>{loadError}</p>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{
              padding: '8px 14px',
              borderRadius: '8px',
              border: '1px solid #fdba74',
              background: '#ffffff',
              color: '#9a3412',
              fontSize: '12px',
              fontWeight: 600,
              cursor: 'pointer',
              whiteSpace: 'nowrap',
            }}
          >
            Retry
          </button>
        </div>
      )}

      {/* Needs Attention */}
      <section style={{ marginBottom: '44px' }}>
        <SectionHeader title="Needs Your Attention" />
        {loading ? (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: '16px' }}>
            {[0, 1].map((i) => (
              <div key={i} style={{ background: '#fff', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.08)', padding: '20px', display: 'flex', flexDirection: 'column', gap: '14px' }}>
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
              background: '#fff7ed',
              border: '1px solid #fdba74',
              borderRadius: '12px',
              padding: '20px 24px',
              display: 'flex',
              alignItems: 'center',
              gap: '12px',
              color: '#9a3412',
              fontSize: '14px',
            }}
          >
            <Icon name="alert-circle" size={18} style={{ color: '#ea580c' }} />
            Parent dashboard actions are unavailable until the latest data loads successfully.
          </div>
        ) : shouldShowCollectionEmptyState(loading, loadError, alerts.length) ? (
          <div
            style={{
              background: '#f0fdf4',
              border: '1px solid #bbf7d0',
              borderRadius: '12px',
              padding: '20px 24px',
              display: 'flex',
              alignItems: 'center',
              gap: '12px',
              color: '#166534',
              fontSize: '14px',
            }}
          >
            <Icon name="check-circle" size={18} style={{ color: '#16a34a' }} />
            You&apos;re all caught up — no pending actions right now.
          </div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: '16px' }}>
            {alerts.map((alert, i) => (
              <div key={alert.id} style={{ animationDelay: `${i * 60}ms`, animation: 'slideUp 0.35s var(--ease-out) both' }}>
                <AlertCard alert={alert} />
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Swimmers */}
      <section style={{ marginBottom: '44px' }}>
        <SectionHeader title="Your Swimmers" action="View All" actionPath="/portal/household" />
        {loading ? (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: '14px' }}>
            {[0, 1].map((i) => (
              <div key={i} style={{ background: '#fff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.08)', padding: '22px', display: 'flex', gap: '16px' }}>
                <Skeleton w={52} h={52} r={26} />
                <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px' }}>
                  <Skeleton w="50%" h={15} />
                  <Skeleton w="70%" h={12} />
                </div>
              </div>
            ))}
          </div>
        ) : loadError ? (
          <div style={{ color: '#9A9693', fontSize: '14px' }}>Household swimmer data is unavailable right now.</div>
        ) : shouldShowCollectionEmptyState(loading, loadError, dashSwimmers.length) ? (
          <div style={{ color: '#9A9693', fontSize: '14px' }}>No swimmers found for your household.</div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: '14px' }}>
            {dashSwimmers.map((swimmer) => (
              <SwimmerCard key={swimmer.id} swimmer={swimmer} />
            ))}
          </div>
        )}
      </section>

      {/* Upcoming meets */}
      <section>
        <SectionHeader title="Upcoming Meets" action="See All Meets" actionPath="/portal/meets" />
        {loading ? (
          <div style={{ background: '#fff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.08)', overflow: 'hidden' }}>
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
          <div style={{ background: '#fff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.08)', padding: '32px', textAlign: 'center', color: '#9A9693', fontSize: '14px' }}>
            Upcoming meet data is unavailable right now.
          </div>
        ) : shouldShowCollectionEmptyState(loading, loadError, dashMeets.length) ? (
          <div style={{ background: '#fff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.08)', padding: '32px', textAlign: 'center', color: '#9A9693', fontSize: '14px' }}>
            No upcoming meets scheduled.
          </div>
        ) : (
          <div style={{ background: '#ffffff', borderRadius: '14px', border: '1px solid rgba(0,0,0,0.08)', overflow: 'hidden', boxShadow: '0 2px 8px rgba(0,0,0,0.05)', animation: 'slideUp 0.45s var(--ease-out) 0.15s both' }}>
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
