'use client'

import Link from 'next/link'
import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { ageFromDob, formatDateOnly, parseBackendDate } from '@/lib/dates'
import { formatCents } from '@/lib/financeData'
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
import './page.inline.css'
import { SWIMMER_AVATAR_COLORS } from '@/lib/avatar-colors'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { ShimmerBlock } from '@/components/ui/ShimmerBlock'

// ── Helpers ─────────────────────────────────────────────────────────────────
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
  age: string
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
  pending:   { label: 'Response needed', color: 'var(--warning)', bg: 'var(--warning-bg)', border: 'var(--warning-border)' },
  committed: { label: 'Committed',       color: 'var(--success)', bg: 'var(--success-bg)', border: 'var(--success-bg)' },
  open:      { label: 'Not yet open',    color: 'var(--text-muted)', bg: 'var(--bg-surface)', border: 'var(--border)' },
}

// ── Skeleton ─────────────────────────────────────────────────────────────────
// ── Components ─────────────────────────────────────────────────────────────
function AlertCard({ alert }: { alert: DashAlert }) {
  const cfg = URGENCY_CONFIG[alert.urgency]
  const iconName = ALERT_ICON[alert.type] as 'file-text' | 'calendar' | 'clipboard-list'

  return (
    <div
      className="dash-alert"
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
        <p className="is-5750ede">
          {alert.body}
        </p>
        <p className="is-437f3fa">
          {alert.title} · {alert.detail}
        </p>
      </div>

      <Link href={alert.ctaPath} className="is-2e4df72 dash-alert-cta">
        {alert.cta}
        <Icon name="chevron-right" size={13} />
      </Link>
    </div>
  )
}

function SwimmerCard({ swimmer }: { swimmer: DashSwimmer }) {
  return (
    <div className="card is-979d69e dash-swimmer">
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
        <div className="is-92d0f5a">
          <p className="is-34a6cf7">
            {swimmer.name}
          </p>
          <span className="is-052ff37">
            Active
          </span>
        </div>
        <p className="is-029575d">
          Age {swimmer.age}
        </p>

        {swimmer.pendingCommits > 0 ? (
          <div className="is-049b86c">
            <Icon name="alert-circle" size={13} style={{ color: 'var(--warning)' }} />
            {swimmer.pendingCommits} commitment pending
          </div>
        ) : (
          <div className="card is-05e6b1e" >
            <Icon name="calendar" size={13} style={{ color: 'var(--text-muted)' }} />
            {swimmer.nextMeet || 'No upcoming meets'}
          </div>
        )}
      </div>

      <Link
        href={`/portal/household/swimmers/${swimmer.id}`}
        aria-label={`View ${swimmer.name}`}
        className="is-3d8302e dash-view-link"
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
      className="meet-row is-cedd9d7"
    >
      <div>
        <p className="is-f923d95">
          {meet.date}
        </p>
        <p className="is-1657eba">
          {meet.course}
        </p>
      </div>

      <div>
        <p className="is-e05938c">{meet.name}</p>
        <p className="is-4f39ab0">{meet.location}</p>
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
          className="btn btn-primary"
          style={{ textDecoration: 'none' }}
        >
          Respond
        </Link>
      ) : (
        <Link href="/portal/meets" className="is-9192dd1 dash-meet-view">
          View
        </Link>
      )}
    </div>
  )
}

function SectionHeader({ title, action, actionPath }: { title: string; action?: string; actionPath?: string }) {
  return (
    <div className="is-abcbf74">
      <h2 className="section-title">
        {title}
      </h2>
      {action && actionPath && (
        <Link
          href={actionPath}
          className="is-5dd0b6c dash-section-link"
        >
          {action}
          <Icon name="chevron-right" size={14} />
        </Link>
      )}
    </div>
  )
}

// ── Page ─────────────────────────────────────────────────────────────────────
function FirstRunSteps() {
  const steps = [
    { n: 1, title: 'Add your swimmer', body: 'Register a swimmer so the club can place them in a group.', href: '/portal/registrations/new', cta: 'Add a swimmer' },
    { n: 2, title: 'Sign the waivers', body: 'Required forms must be on file before practice or meets.', href: '/portal/documents', cta: 'Open documents' },
    { n: 3, title: 'Commit to a meet', body: 'Tell the coaches which meets your swimmer will attend.', href: '/portal/meets', cta: 'See meets' },
  ]
  return (
    <section className="card is-df974f5" >
      <h2 className="section-title is-c312289" >Get started in three steps</h2>
      <ol className="is-3fb5e8e">
        {steps.map((st) => (
          <li key={st.n} className="is-c8c998c">
            <span className="is-77c4c01">{st.n}</span>
            <strong className="is-14207b8">{st.title}</strong>
            <span className="is-fb4f5c1">{st.body}</span>
            <Link href={st.href} className="portal-tap is-4becda5" >{st.cta}</Link>
          </li>
        ))}
      </ol>
    </section>
  )
}

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
      setLoadError('We could not find a household linked to your account. Contact your club to get it linked, then reload this page.')
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
          return {
            id: sw.id,
            name: `${sw.first_name} ${sw.last_name}`,
            age: ageFromDob(sw.date_of_birth),
            gradient: SWIMMER_AVATAR_COLORS[idx % SWIMMER_AVATAR_COLORS.length],
            initials: `${sw.first_name[0] ?? ''}${sw.last_name[0] ?? ''}`.toUpperCase(),
            pendingCommits: swCommits.length,
            nextMeet: upcoming[0]?.name ?? '',
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
            date: formatDateOnly(m.start_date),
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
            body: `${formatCents(inv.amount_due_cents)} outstanding`,
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
          'We could not load your dashboard. Check your connection and try again.',
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
      {/* Page greeting */}
      <div className="page-heading">
        <h1 className="page-title">
          {greeting}, {firstName}.
        </h1>
      </div>

      {loadError && (
        <LoadErrorBanner message={loadError} onRetry={() => setReloadKey((key) => key + 1)} />
      )}

      {!loadError && <VolunteerHoursStrip />}

      {!loadError && (<>
      {/* First run */}
      {!loading && dashSwimmers.length === 0 && <FirstRunSteps />}

      {/* Needs Attention */}
      <section className="is-6a59054">
        <SectionHeader title="Needs your attention" />
        {loading ? (
          <div className="is-e680c18">
            {[0, 1].map((i) => (
              <div key={i} className="card is-8ef476f" >
                <ShimmerBlock w={38} h={38} r={10} />
                <div className="is-355eedd">
                  <ShimmerBlock w="40%" h={12} />
                  <ShimmerBlock w="70%" h={20} />
                  <ShimmerBlock w="60%" h={13} />
                </div>
                <ShimmerBlock w={90} h={34} r={8} />
              </div>
            ))}
          </div>
        ) : shouldShowCollectionEmptyState(loading, loadError, alerts.length) ? (
          <div
            className="is-0c7e963"
          >
            <Icon name="check-circle" size={18} style={{ color: 'var(--success)' }} />
            You&apos;re all caught up — no pending actions right now.
          </div>
        ) : (
          <div className="is-930d87f">
            {alerts.map((alert, i) => (
              <div key={alert.id} style={{ animationDelay: `${i * 60}ms`, animation: 'slideUp 0.35s var(--ease-out) both' }}>
                <AlertCard alert={alert} />
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Swimmers */}
      <section className="is-6a59054">
        <SectionHeader title="Your swimmers" action="View all" actionPath="/portal/household" />
        {loading ? (
          <div className="is-50fcb73">
            {[0, 1].map((i) => (
              <div key={i} className="card is-9487582" >
                <ShimmerBlock w={52} h={52} r={26} />
                <div className="is-20f80e0">
                  <ShimmerBlock w="50%" h={15} />
                  <ShimmerBlock w="70%" h={12} />
                </div>
              </div>
            ))}
          </div>
        ) : shouldShowCollectionEmptyState(loading, loadError, dashSwimmers.length) ? (
          <div className="is-e3e0bc5">No swimmers found for your household.</div>
        ) : (
          <div className="is-50fcb73">
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
              <div key={i} className="is-2e1633b">
                <ShimmerBlock w={80} h={14} />
                <div className="is-9808a21">
                  <ShimmerBlock w="60%" h={14} />
                  <ShimmerBlock w="40%" h={12} />
                </div>
                <ShimmerBlock w={90} h={26} r={999} />
                <ShimmerBlock w={70} h={30} r={7} />
              </div>
            ))}
          </div>
        ) : shouldShowCollectionEmptyState(loading, loadError, dashMeets.length) ? (
          <div className="card is-3f7bfac" >
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
      </>)}
    </div>
  )
}
