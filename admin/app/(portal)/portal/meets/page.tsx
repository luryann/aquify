'use client'

import Link from 'next/link'
import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  fetchMeets,
  fetchMeetCommitments,
  fetchSwimmers,
  type MeetRecord,
  type CommitmentRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import { parseBackendDate } from '@/lib/dates'

// ── Types ───────────────────────────────────────────────────────────────────
type CommitmentStatus = 'pending' | 'yes' | 'no' | 'open' | 'closed'

type MeetCard = {
  id: string
  name: string
  dateLabel: string        // "Mar 22"
  monthLabel: string       // "Mar"
  dayLabel: string         // "22"
  deadlineDate: string     // ISO
  course: string
  location: string
  status: string
  daysUntilDeadline: number
  swimmers: Array<{ id: string; name: string; initials: string; gradient: string; commitment: CommitmentStatus }>
}

// ── Helpers ─────────────────────────────────────────────────────────────────
const GRADIENTS = [
  'linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%)',
  'linear-gradient(135deg, #10b981 0%, #059669 100%)',
  'linear-gradient(135deg, #f59e0b 0%, #d97706 100%)',
  'linear-gradient(135deg, #ef4444 0%, #dc2626 100%)',
]

function fmtMonth(iso?: string): string {
  if (!iso) return '—'
  return parseBackendDate(iso).toLocaleDateString('en-US', { month: 'short' })
}

function fmtDay(iso?: string): string {
  if (!iso) return '—'
  return parseBackendDate(iso).toLocaleDateString('en-US', { day: 'numeric' })
}

function fmtDate(iso?: string): string {
  if (!iso) return '—'
  return parseBackendDate(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function daysUntil(iso?: string): number {
  if (!iso) return -1
  return Math.ceil((new Date(iso).getTime() - Date.now()) / 86400000)
}

function mapCommitment(commitments: CommitmentRecord[], swimmerId: string, meetStatus: string): CommitmentStatus {
  const rec = commitments.find((c) => c.swimmer_id === swimmerId)
  if (!rec) {
    if (meetStatus === 'commitment_open') return 'pending'
    if (['commitment_closed', 'entry_finalized', 'completed', 'archived'].includes(meetStatus)) return 'closed'
    return 'open'
  }
  if (rec.status === 'committed_yes') return 'yes'
  if (rec.status === 'committed_no') return 'no'
  if (rec.status === 'invited') return 'pending'
  if (['coach_reviewed', 'locked'].includes(rec.status)) return 'closed'
  return 'open'
}

function buildMeetCard(
  meet: MeetRecord,
  hhSwimmers: SwimmerRecord[],
  commitmentsByMeet: Map<string, CommitmentRecord[]>,
): MeetCard {
  const commitments = commitmentsByMeet.get(meet.id) ?? []
  const swimmers = hhSwimmers.map((sw, idx) => ({
    id: sw.id,
    name: `${sw.first_name} ${sw.last_name}`,
    initials: `${sw.first_name[0] ?? ''}${sw.last_name[0] ?? ''}`.toUpperCase(),
    gradient: GRADIENTS[idx % GRADIENTS.length],
    commitment: mapCommitment(commitments, sw.id, meet.status),
  }))
  return {
    id: meet.id,
    name: meet.name,
    dateLabel: fmtDate(meet.start_date),
    monthLabel: fmtMonth(meet.start_date),
    dayLabel: fmtDay(meet.start_date),
    deadlineDate: meet.commitment_deadline ?? '',
    course: meet.course,
    location: meet.location ?? '—',
    status: meet.status,
    daysUntilDeadline: daysUntil(meet.commitment_deadline),
    swimmers,
  }
}

// ── Helpers ─────────────────────────────────────────────────────────────────
const COMMITMENT_CONFIG = {
  pending: { label: 'Respond',    bg: 'var(--warning-bg)', color: 'var(--warning)', border: 'rgba(217,119,6,0.2)' },
  yes:     { label: 'Going',      bg: 'var(--success-bg)', color: 'var(--success)', border: 'rgba(5,150,105,0.2)' },
  no:      { label: 'Not going',  bg: 'var(--error-bg)', color: 'var(--error)', border: 'rgba(220,38,38,0.2)' },
  open:    { label: 'Opens soon', bg: '#f0f9ff', color: '#0284c7', border: 'rgba(2,132,199,0.2)' },
  closed:  { label: 'Closed',     bg: 'var(--bg-surface)', color: 'var(--text-muted)', border: 'rgba(0,0,0,0.1)' },
}

const MEET_STATUS_LABEL: Record<string, string> = {
  draft:              'Draft',
  published:          'Opens Soon',
  commitment_open:    'Commitment Open',
  commitment_closed:  'Commitment Closed',
  entry_finalized:    'Entries Finalized',
  completed:          'Completed',
  archived:           'Archived',
}

function Skeleton({ w = '100%', h = 16, r = 8 }: { w?: string | number; h?: number; r?: number }) {
  return (
    <div
      style={{
        width: w, height: h, borderRadius: r,
        background: 'linear-gradient(90deg, #f0efee 25%, #e6e5e3 50%, #f0efee 75%)',
        backgroundSize: '200% 100%',
        animation: 'shimmer 1.5s infinite',
      }}
    />
  )
}

function DeadlineCountdown({ days }: { days: number }) {
  if (days < 0) return null
  const urgent  = days <= 3
  const warning = days <= 7

  const color = urgent ? 'var(--error)' : warning ? 'var(--warning)' : 'var(--text-subtle)'
  const bg    = urgent ? 'var(--error-bg)' : warning ? 'var(--warning-bg)' : 'rgba(0,0,0,0.04)'

  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '4px',
        padding: '4px 8px',
        borderRadius: '999px',
        fontSize: 'var(--fs-sm)',
        fontWeight: urgent || warning ? 600 : 400,
        color,
        background: bg,
      }}
    >
      {urgent && <Icon name="alert-circle" size={12} style={{ color }} />}
      {days === 0 ? 'Due today' : days === 1 ? '1 day left' : `${days} days left`}
    </span>
  )
}

// ── Component ────────────────────────────────────────────────────────────────
export default function MeetsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [meets, setMeets] = useState<MeetCard[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [retryKey, setRetryKey] = useState(0)

  useEffect(() => {
    if (!orgId || !householdId) {
      setLoadError('Your portal session is missing household context. Refresh and sign in again.')
      setLoading(false)
      return
    }
    setLoading(true)
    setLoadError(null)
    Promise.all([
      fetchMeets(api, orgId),
      fetchSwimmers(api, orgId),
    ])
      .then(async ([allMeets, allSwimmers]) => {
        const hhSwimmers = allSwimmers.filter((s) => s.household_id === householdId)

        // Fetch commitments for all actionable meets in parallel
        const actionableMeets = allMeets.filter((m) =>
          ['commitment_open', 'commitment_closed', 'entry_finalized'].includes(m.status)
        )
        const commitmentResults = await Promise.all(
          actionableMeets.map((m) =>
            fetchMeetCommitments(api, orgId, m.id)
              .then((cs): [string, CommitmentRecord[]] => [m.id, cs])
          )
        )
        const commitmentsByMeet = new Map<string, CommitmentRecord[]>(commitmentResults)

        const cards = allMeets.map((m) => buildMeetCard(m, hhSwimmers, commitmentsByMeet))
        setMeets(cards)
      })
      .catch((error) => {
        reportUIError('portal.meets.load', error, 'Couldn’t load meet schedules. Try again.', {
          orgId,
          householdId,
        })
        setLoadError('Couldn’t load meet schedules. Try again.')
      })
      .finally(() => setLoading(false))
  }, [api, orgId, householdId, retryKey])

  const activeMeets   = meets.filter((m) => m.status === 'commitment_open')
  const upcomingMeets = meets.filter((m) => ['entry_finalized', 'commitment_closed'].includes(m.status))
  const announcedMeets = meets.filter((m) => m.status === 'published')
  const pastMeets     = meets.filter((m) => ['completed', 'archived'].includes(m.status))

  const pendingRespond = activeMeets.filter((m) =>
    m.swimmers.some((s) => s.commitment === 'pending')
  ).length

  return (
    <div>
      <style>{`
        @keyframes slideUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @keyframes shimmer { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
      `}</style>

      {/* Page header */}
      <div className="page-heading">
        <h1 className="page-title">
          Meets
        </h1>
      </div>

      {/* Loading skeletons */}
      {loading && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          {[0, 1].map((i) => (
            <div key={i} className="card" style={{ padding: '20px 24px', display: 'flex', gap: '16px' }}>
              <Skeleton w={52} h={64} r={10} />
              <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '12px' }}>
                <Skeleton w="45%" h={20} />
                <Skeleton w="65%" h={13} />
                <Skeleton w="40%" h={30} r={8} />
              </div>
            </div>
          ))}
        </div>
      )}

      {!loading && (
        <>
          {loadError && (
            <div style={{ marginBottom: '20px', background: 'var(--error-bg)', border: '1px solid rgba(220,38,38,0.18)', borderRadius: '12px', padding: '20px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
              <div>
                <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--error)', marginBottom: '4px' }}>
                  Meet data is unavailable
                </div>
                <div style={{ fontSize: 'var(--fs-base)', color: 'var(--error)' }}>{loadError}</div>
              </div>
              <button
                onClick={() => setRetryKey((current) => current + 1)}
                style={{ padding: '8px 16px', borderRadius: '8px', border: 'none', background: 'var(--error)', color: 'var(--bg-elevated)', fontSize: 'var(--fs-base)', fontWeight: 600, cursor: 'pointer', flexShrink: 0 }}
              >
                Retry
              </button>
            </div>
          )}

          {/* Action needed banner */}
          {!loadError && pendingRespond > 0 && (
            <div style={{ background: 'var(--accent-subtle)', border: '1px solid rgba(235,93,30,0.2)', borderRadius: '12px', padding: '16px 20px', marginBottom: '24px', display: 'flex', alignItems: 'center', gap: '16px', animation: 'slideUp 0.35s ease both' }}>
              <div style={{ width: '40px', height: '40px', borderRadius: '50%', background: 'rgba(235,93,30,0.12)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                <Icon name="calendar" size={18} style={{ color: 'var(--accent)' }} />
              </div>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: '#8B3312' }}>
                  {pendingRespond} meet{pendingRespond !== 1 ? 's' : ''} need{pendingRespond === 1 ? 's' : ''} your response
                </div>
                <div style={{ fontSize: 'var(--fs-sm)', color: '#b8541e', marginTop: '2px' }}>
                  Respond before the deadline to secure spots for your swimmers.
                </div>
              </div>
            </div>
          )}

          {/* Active meets (commitment open) */}
          {!loadError && activeMeets.length > 0 && (
            <section style={{ marginBottom: 'var(--space-8)' }}>
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '16px', animation: 'slideUp 0.4s ease both' }}>
                Needs your response
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {activeMeets.map((meet, idx) => {
                  const hasUrgent = meet.swimmers.some((s) => s.commitment === 'pending') && meet.daysUntilDeadline <= 3

                  return (
                    <div
                      key={meet.id}
                      style={{
                        background: 'var(--bg-elevated)',
                        borderRadius: '12px',
                        border: `1px solid ${hasUrgent ? 'rgba(220,38,38,0.15)' : 'rgba(0,0,0,0.07)'}`,
                        overflow: 'hidden',
                        animation: `slideUp ${0.4 + idx * 0.07}s ease both`,
                      }}
                    >
                      <div style={{ padding: '20px 24px', display: 'flex', alignItems: 'flex-start', gap: '16px' }}>
                        {/* Date block */}
                        <div style={{ width: '52px', borderRadius: '12px', background: hasUrgent ? 'rgba(220,38,38,0.07)' : 'rgba(235,93,30,0.07)', border: `1px solid ${hasUrgent ? 'rgba(220,38,38,0.15)' : 'rgba(235,93,30,0.15)'}`, display: 'flex', flexDirection: 'column', alignItems: 'center', padding: '8px 4px', flexShrink: 0, textAlign: 'center' }}>
                          <span style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: 'var(--accent)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                            {meet.monthLabel}
                          </span>
                          <span style={{ fontSize: 'var(--fs-2xl)', fontWeight: 'var(--display-weight)', color: 'var(--text)', lineHeight: 1.1, fontFamily: 'var(--font-display)' }}>
                            {meet.dayLabel}
                          </span>
                        </div>

                        {/* Meet info */}
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                            <span style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: 'var(--text)', lineHeight: 1.1 }}>
                              {meet.name}
                            </span>
                            <span style={{ padding: '2px 8px', background: 'rgba(0,0,0,0.05)', borderRadius: '4px', fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: 'var(--text-subtle)' }}>
                              {meet.course}
                            </span>
                          </div>
                          <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '4px', display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                            <span>{meet.location}</span>
                            {meet.deadlineDate && (
                              <>
                                <span>·</span>
                                <span title="Respond by this date or your swimmer won't be entered in this meet">Deadline: {fmtDate(meet.deadlineDate)}</span>
                                <DeadlineCountdown days={meet.daysUntilDeadline} />
                              </>
                            )}
                          </div>

                          {/* Per-swimmer commitment status */}
                          {meet.swimmers.length > 0 && (
                            <div style={{ display: 'flex', gap: '8px', marginTop: '12px', flexWrap: 'wrap' }}>
                              {meet.swimmers.map((swimmer) => {
                                const cfg = COMMITMENT_CONFIG[swimmer.commitment]
                                return (
                                  <div key={swimmer.id} style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '4px 12px', background: cfg.bg, border: `1px solid ${cfg.border}`, borderRadius: '8px' }}>
                                    <div style={{ width: '20px', height: '20px', borderRadius: '50%', background: swimmer.gradient, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--bg-elevated)', fontSize: 'var(--fs-2xs)', fontWeight: 700, fontFamily: 'var(--font-body)', flexShrink: 0 }}>
                                      {swimmer.initials}
                                    </div>
                                    <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text)', fontWeight: 500 }}>
                                      {swimmer.name.split(' ')[0]}
                                    </span>
                                    <span style={{ fontSize: 'var(--fs-xs)', color: cfg.color, fontWeight: 600 }}>
                                      {cfg.label}
                                    </span>
                                  </div>
                                )
                              })}
                            </div>
                          )}
                        </div>

                        {/* CTA */}
                        <Link
                          href={`/portal/meets/${meet.id}`}
                          style={{ padding: '8px 20px', background: hasUrgent ? 'var(--error)' : 'var(--accent)', color: 'var(--bg-elevated)', borderRadius: '8px', fontSize: 'var(--fs-base)', fontWeight: 600, textDecoration: 'none', flexShrink: 0, transition: 'opacity 120ms' }}
                        >
                          {meet.swimmers.some((s) => s.commitment === 'pending') ? 'Respond' : 'View Meet'}
                        </Link>
                      </div>
                    </div>
                  )
                })}
              </div>
            </section>
          )}

          {/* Entries confirmed */}
          {!loadError && upcomingMeets.length > 0 && (
            <section style={{ marginBottom: 'var(--space-8)' }}>
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '16px' }}>
                Entries confirmed
              </div>
              <MeetList meets={upcomingMeets} />
            </section>
          )}

          {/* Announced */}
          {!loadError && announcedMeets.length > 0 && (
            <section style={{ marginBottom: 'var(--space-8)' }}>
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '16px' }}>
                Announced
              </div>
              <MeetList meets={announcedMeets} />
            </section>
          )}

          {/* Empty state */}
          {!loadError && meets.length === 0 && (
            <div className="card" style={{ padding: '40px', textAlign: 'center', color: 'var(--text-muted)', fontSize: 'var(--fs-md)' }}>
              No meets scheduled yet.
            </div>
          )}

          {/* Past meets */}
          {!loadError && pastMeets.length > 0 && (
            <section>
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, fontFamily: 'var(--font-body)', color: 'var(--text)', marginBottom: '16px' }}>
                Past
              </div>
              <div className="card" style={{ overflow: 'hidden' }}>
                {pastMeets.map((meet, i) => (
                  <Link
                    key={meet.id}
                    href={`/portal/meets/${meet.id}`}
                    style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 20px', borderBottom: i < pastMeets.length - 1 ? '1px solid rgba(0,0,0,0.05)' : 'none', textDecoration: 'none', transition: 'background 100ms' }}
                    onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.02)')}
                    onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                  >
                    <div>
                      <span style={{ fontSize: 'var(--fs-md)', color: 'var(--text-subtle)', fontWeight: 500 }}>{meet.name}</span>
                      <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', marginLeft: '8px' }}>{meet.dateLabel}</span>
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                      <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
                        {meet.course}
                      </span>
                      <span
                        title={meet.status === 'entry_finalized' ? "Entries are locked. Contact your coach for any changes." : undefined}
                        style={{ padding: '2px 8px', background: 'rgba(0,0,0,0.05)', borderRadius: '6px', fontSize: 'var(--fs-xs)', color: 'var(--text-muted)' }}
                      >
                        {MEET_STATUS_LABEL[meet.status] ?? meet.status}
                      </span>
                      <Icon name="chevron-right" size={14} style={{ color: 'var(--border-strong)' }} />
                    </div>
                  </Link>
                ))}
              </div>
            </section>
          )}
        </>
      )}
    </div>
  )
}

// ── Simple meet list for non-urgent meets ────────────────────────────────────
function MeetList({ meets }: { meets: MeetCard[] }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
      {meets.map((meet) => (
        <Link
          key={meet.id}
          href={`/portal/meets/${meet.id}`}
          style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', background: 'var(--bg-elevated)', borderRadius: '12px', border: '1px solid rgba(0,0,0,0.07)', padding: '16px 20px', textDecoration: 'none', gap: '16px', transition: 'box-shadow 120ms ease' }}
          onMouseEnter={(e) => (e.currentTarget.style.boxShadow = '0 4px 16px rgba(0,0,0,0.08)')}
          onMouseLeave={(e) => (e.currentTarget.style.boxShadow = 'none')}
        >
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
              <span style={{ fontSize: 'var(--fs-md)', color: 'var(--text)', fontWeight: 500 }}>{meet.name}</span>
              <span style={{ padding: '1px 8px', background: 'rgba(0,0,0,0.05)', borderRadius: '4px', fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: 'var(--text-subtle)' }}>
                {meet.course}
              </span>
            </div>
            <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '4px' }}>
              {meet.dateLabel} · {meet.location}
            </div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexShrink: 0 }}>
            <DeadlineCountdown days={meet.daysUntilDeadline} />
            <Icon name="chevron-right" size={15} style={{ color: 'var(--border-strong)' }} />
          </div>
        </Link>
      ))}
    </div>
  )
}
