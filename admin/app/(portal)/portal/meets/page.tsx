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
import { formatDateOnly, parseBackendDate } from '@/lib/dates'
import './page.inline.css'
import { SWIMMER_AVATAR_COLORS } from '@/lib/avatar-colors'
import { ShimmerBlock } from '@/components/ui/ShimmerBlock'

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
function fmtMonth(iso?: string): string {
  if (!iso) return '—'
  return parseBackendDate(iso).toLocaleDateString('en-US', { month: 'short' })
}

function fmtDay(iso?: string): string {
  if (!iso) return '—'
  return parseBackendDate(iso).toLocaleDateString('en-US', { day: 'numeric' })
}

function daysUntil(iso?: string): number {
  if (!iso) return -1
  return Math.ceil((parseBackendDate(iso).getTime() - Date.now()) / 86400000)
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
    gradient: SWIMMER_AVATAR_COLORS[idx % SWIMMER_AVATAR_COLORS.length],
    commitment: mapCommitment(commitments, sw.id, meet.status),
  }))
  return {
    id: meet.id,
    name: meet.name,
    dateLabel: formatDateOnly(meet.start_date),
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
  pending: { label: 'Respond',    bg: 'var(--warning-bg)', color: 'var(--warning)', border: 'var(--warning-border)' },
  yes:     { label: 'Going',      bg: 'var(--success-bg)', color: 'var(--success)', border: 'var(--success-border)' },
  no:      { label: 'Not going',  bg: 'var(--error-bg)', color: 'var(--error)', border: 'var(--error-border)' },
  open:    { label: 'Opens soon', bg: 'var(--info-bg)', color: 'var(--info)', border: 'var(--info-border)' },
  closed:  { label: 'Closed',     bg: 'var(--bg-surface)', color: 'var(--text-muted)', border: 'var(--border)' },
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

function DeadlineCountdown({ days }: { days: number }) {
  if (days < 0) return null
  const urgent  = days <= 3
  const warning = days <= 7

  const color = urgent ? 'var(--error)' : warning ? 'var(--warning)' : 'var(--text-subtle)'
  const bg    = urgent ? 'var(--error-bg)' : warning ? 'var(--warning-bg)' : 'var(--bg-wash)'

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
      
      {/* Page header */}
      <div className="page-heading">
        <h1 className="page-title">
          Meets
        </h1>
      </div>

      {/* Loading skeletons */}
      {loading && (
        <div className="is-930d87f">
          {[0, 1].map((i) => (
            <div key={i} className="card is-4d0790f" >
              <ShimmerBlock w={52} h={64} r={10} />
              <div className="is-e933947">
                <ShimmerBlock w="45%" h={20} />
                <ShimmerBlock w="65%" h={13} />
                <ShimmerBlock w="40%" h={30} r={8} />
              </div>
            </div>
          ))}
        </div>
      )}

      {!loading && (
        <>
          {loadError && (
            <div className="is-9dc000d">
              <div>
                <div className="is-137a7ce">
                  Meet data is unavailable
                </div>
                <div className="is-de107ca">{loadError}</div>
              </div>
              <button type="button"
                onClick={() => setRetryKey((current) => current + 1)}
                className="is-b2368ab"
              >
                Retry
              </button>
            </div>
          )}

          {/* Action needed banner */}
          {!loadError && pendingRespond > 0 && (
            <div className="is-d9d9712">
              <div style={{ width: '40px', height: '40px', borderRadius: '50%', background: 'rgba(235,93,30,0.12)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                <Icon name="calendar" size={18} style={{ color: 'var(--accent)' }} />
              </div>
              <div style={{ flex: 1 }}>
                <div className="is-fe12638">
                  {pendingRespond} meet{pendingRespond !== 1 ? 's' : ''} need{pendingRespond === 1 ? 's' : ''} your response
                </div>
                <div className="is-023517c">
                  Respond before the deadline to secure spots for your swimmers.
                </div>
              </div>
            </div>
          )}

          {/* Active meets (commitment open) */}
          {!loadError && activeMeets.length > 0 && (
            <section className="is-6a59054">
              <div className="is-a86d472">
                Needs your response
              </div>
              <div className="is-930d87f">
                {activeMeets.map((meet, idx) => {
                  const hasUrgent = meet.swimmers.some((s) => s.commitment === 'pending') && meet.daysUntilDeadline <= 3

                  return (
                    <div
                      key={meet.id}
                      style={{
                        background: 'var(--bg-elevated)',
                        borderRadius: '12px',
                        border: `1px solid ${hasUrgent ? 'rgba(220,38,38,0.15)' : 'var(--border)'}`,
                        overflow: 'hidden',
                        animation: `slideUp ${0.4 + idx * 0.07}s ease both`,
                      }}
                    >
                      <div className="is-86963eb">
                        {/* Date block */}
                        <div style={{ width: '52px', borderRadius: '12px', background: hasUrgent ? 'rgba(220,38,38,0.07)' : 'rgba(235,93,30,0.07)', border: `1px solid ${hasUrgent ? 'rgba(220,38,38,0.15)' : 'rgba(235,93,30,0.15)'}`, display: 'flex', flexDirection: 'column', alignItems: 'center', padding: '8px 4px', flexShrink: 0, textAlign: 'center' }}>
                          <span className="is-53214bb">
                            {meet.monthLabel}
                          </span>
                          <span className="is-a1e6bb6">
                            {meet.dayLabel}
                          </span>
                        </div>

                        {/* Meet info */}
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div className="is-79731f1">
                            <span className="is-fdc9019">
                              {meet.name}
                            </span>
                            <span className="is-11fadb7">
                              {meet.course}
                            </span>
                          </div>
                          <div className="is-7bec0ff">
                            <span>{meet.location}</span>
                            {meet.deadlineDate && (
                              <>
                                <span>·</span>
                                <span title="Respond by this date or your swimmer won't be entered in this meet">Deadline: {formatDateOnly(meet.deadlineDate)}</span>
                                <DeadlineCountdown days={meet.daysUntilDeadline} />
                              </>
                            )}
                          </div>

                          {/* Per-swimmer commitment status */}
                          {meet.swimmers.length > 0 && (
                            <div className="is-7c0bd9a">
                              {meet.swimmers.map((swimmer) => {
                                const cfg = COMMITMENT_CONFIG[swimmer.commitment]
                                return (
                                  <div key={swimmer.id} style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '4px 12px', background: cfg.bg, border: `1px solid ${cfg.border}`, borderRadius: '8px' }}>
                                    <div style={{ width: '20px', height: '20px', borderRadius: '50%', background: swimmer.gradient, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--bg-elevated)', fontSize: 'var(--fs-2xs)', fontWeight: 700, fontFamily: 'var(--font-body)', flexShrink: 0 }}>
                                      {swimmer.initials}
                                    </div>
                                    <span className="is-5341e30">
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
            <section className="is-6a59054">
              <div className="is-483d59c">
                Entries confirmed
              </div>
              <MeetList meets={upcomingMeets} />
            </section>
          )}

          {/* Announced */}
          {!loadError && announcedMeets.length > 0 && (
            <section className="is-6a59054">
              <div className="is-483d59c">
                Announced
              </div>
              <MeetList meets={announcedMeets} />
            </section>
          )}

          {/* Empty state */}
          {!loadError && meets.length === 0 && (
            <div className="card is-21695c0" >
              No meets scheduled yet.
            </div>
          )}

          {/* Past meets */}
          {!loadError && pastMeets.length > 0 && (
            <section>
              <div className="is-483d59c">
                Past
              </div>
              <div className="card" style={{ overflow: 'hidden' }}>
                {pastMeets.map((meet, i) => (
                  <Link
                    key={meet.id}
                    href={`/portal/meets/${meet.id}`}
                    style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 20px', borderBottom: i < pastMeets.length - 1 ? '1px solid var(--border)' : 'none', textDecoration: 'none', transition: 'background 100ms' }}
                    onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--bg-wash)')}
                    onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                  >
                    <div>
                      <span className="is-75a6c45">{meet.name}</span>
                      <span className="is-7db2283">{meet.dateLabel}</span>
                    </div>
                    <div className="is-e2ff0fd">
                      <span className="is-915a8b5">
                        {meet.course}
                      </span>
                      <span
                        title={meet.status === 'entry_finalized' ? "Entries are locked. Contact your coach for any changes." : undefined}
                        className="is-2d20fd4"
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
    <div className="is-355eedd">
      {meets.map((meet) => (
        <Link
          key={meet.id}
          href={`/portal/meets/${meet.id}`}
          className="is-e9d15aa hv-c28f3d"
        >
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="is-79731f1">
              <span className="is-ddcee0b">{meet.name}</span>
              <span className="is-647d1e4">
                {meet.course}
              </span>
            </div>
            <div className="is-dd721e0">
              {meet.dateLabel} · {meet.location}
            </div>
          </div>
          <div className="is-796c8fe">
            <DeadlineCountdown days={meet.daysUntilDeadline} />
            <Icon name="chevron-right" size={15} style={{ color: 'var(--border-strong)' }} />
          </div>
        </Link>
      ))}
    </div>
  )
}
