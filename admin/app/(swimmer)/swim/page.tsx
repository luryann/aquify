'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { parseBackendDate } from '@/lib/dates'
import { ApiError, useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
import { UnlinkedSwimmerNotice } from '@/components/swimmer/UnlinkedSwimmerNotice'
import { WellnessPromptCard } from '@/components/engagement/WellnessPromptCard'
import { COURSE_LABELS } from '@/lib/meet-events'
import {
  fetchSwimmers,
  fetchMeets,
  fetchMeetCommitments,
  fetchSwimmerBestTimes,
  fetchAnnouncements,
  type SwimmerRecord,
  type AnnouncementRecord,
} from '@/lib/adminData'
import './page.inline.css'
import { formatSwimTime } from '@/lib/performanceData'
import { SkeletonLine } from '@/components/ui/SkeletonLine'

// ── Design tokens ──────────────────────────────────────────────────────────
const C = {
  accent:   'var(--accent)',
  accentBorder: 'var(--accent-border)',
  text:     'var(--text)',
  subtle:   'var(--text-subtle)',
  muted:    'var(--text-muted)',
  border:   'var(--border)',
  warn:     'var(--warning)',
  warnBg:   'var(--warning-bg)',
  warnBorder: 'var(--warning-border)',
}

function courseTitle(course: string): string | undefined {
  return COURSE_LABELS[course.toUpperCase()]
}

// ── Helpers ────────────────────────────────────────────────────────────────
function eventName(distance: number, stroke: string): string {
  const cap = stroke.charAt(0).toUpperCase() + stroke.slice(1)
  return `${distance} ${cap}`
}

function fmtDate(dateStr?: string): string {
  if (!dateStr) return '—'
  const d = parseBackendDate(dateStr)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

// Convert raw commitment status to display status
function commitStatus(raw: string | undefined): 'committed' | 'pending' | 'no' | 'closed' | 'open' {
  if (!raw) return 'open'
  if (raw === 'committed_yes') return 'committed'
  if (raw === 'invited') return 'pending'
  if (raw === 'committed_no') return 'no'
  if (raw === 'coach_reviewed' || raw === 'locked') return 'closed'
  return 'open'
}

const COMMIT_STATUS = {
  committed: { label: 'Committed',       color: 'var(--accent)', bg: 'var(--border)', border: 'var(--accent-border)' },
  pending:   { label: 'Response needed', color: 'var(--warning)', bg: 'var(--warning-bg)', border: 'var(--warning-border)' },
  no:        { label: 'Not going',       color: 'var(--error)', bg: 'var(--error-bg)',  border: 'var(--error-border)' },
  closed:    { label: 'Closed',          color: 'var(--text-muted)', bg: 'var(--border)', border: 'var(--border-strong)' },
  open:      { label: 'Sign-ups Not Started',    color: 'var(--text-muted)', bg: 'var(--border)', border: 'var(--border-strong)' },
}

// ── Types ──────────────────────────────────────────────────────────────────
type MeetDisplay = {
  id: string
  name: string
  date: string
  course: string
  location: string
  commitmentStatus: 'committed' | 'pending' | 'no' | 'closed' | 'open'
  path: string
}

type BestTimeDisplay = {
  event: string
  course: string
  time: string
  fresh: boolean
}

// ── Skeleton components ────────────────────────────────────────────────────
function SkeletonCard() {
  return (
    <div className="card is-d18a76e"
      
    >
      <SkeletonLine width="60%" height="16px" />
      <SkeletonLine width="40%" height="12px" />
    </div>
  )
}

// ── Components ─────────────────────────────────────────────────────────────
function StatCard({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent?: boolean }) {
  return (
    <div className="card"
      style={{
        padding: '20px 20px',
        boxShadow: accent ? `0 0 20px rgba(0,212,170,0.08)` : 'none',
      }}
    >
      <p style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: C.muted, marginBottom: '6px' }}>
        {label}
      </p>
      <p style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-3xl)', color: accent ? C.accent : C.text, letterSpacing: 'var(--display-tracking)', lineHeight: 1 }}>
        {value}
      </p>
      {sub && (
        <p style={{ fontSize: 'var(--fs-sm)', color: C.muted, marginTop: '4px' }}>{sub}</p>
      )}
    </div>
  )
}

function MeetCard({ meet }: { meet: MeetDisplay }) {
  const status = COMMIT_STATUS[meet.commitmentStatus]
  return (
    <div className="card is-5a15081"
      
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.borderColor = C.accentBorder
        el.style.boxShadow = '0 0 24px rgba(0,212,170,0.06)'
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.borderColor = C.border
        el.style.boxShadow = 'none'
      }}
    >
      {/* Header */}
      <div className="is-99561b3">
        <div>
          <p style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: C.text, marginBottom: '4px' }}>
            {meet.name}
          </p>
          <div className="is-18d4ed1">
            <span style={{ fontSize: 'var(--fs-sm)', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
              <Icon name="calendar" size={11} />
              {meet.date}
            </span>
            {meet.location && (
              <span style={{ fontSize: 'var(--fs-sm)', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
                <Icon name="map-pin" size={11} />
                {meet.location}
              </span>
            )}
          </div>
        </div>
        <div className="is-eba923c">
          {meet.course && (
            <span title={courseTitle(meet.course)} style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', fontWeight: 600, color: 'var(--bg-surface)', background: C.accent, padding: '2px 8px', borderRadius: '4px', }}>
              {meet.course}
            </span>
          )}
          <span
            style={{
              fontSize: 'var(--fs-xs)',
              fontWeight: 500,
              color: status.color,
              background: status.bg,
              border: `1px solid ${status.border}`,
              padding: '4px 8px',
              borderRadius: '999px',
              whiteSpace: 'nowrap',
            }}
          >
            {status.label}
          </span>
        </div>
      </div>

      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <Link
          href={meet.path}
          style={{
            fontSize: 'var(--fs-sm)',
            fontWeight: 600,
            color: C.accent,
            textDecoration: 'none',
            display: 'inline-flex',
            alignItems: 'center',
            gap: '4px',
          }}
        >
          Meet details
          <Icon name="chevron-right" size={13} />
        </Link>
      </div>
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function SwimmerDashboardPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''
  const swimmerId = session?.user.swimmerId ?? ''

  const [swimmer, setSwimmer] = useState<SwimmerRecord | null>(null)
  const [meets, setMeets] = useState<MeetDisplay[]>([])
  const [bestTimes, setBestTimes] = useState<BestTimeDisplay[]>([])
  const [announcements, setAnnouncements] = useState<AnnouncementRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [noLinkedSwimmer, setNoLinkedSwimmer] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)

  const now = new Date()
  const hour = now.getHours()
  const greeting = hour < 12 ? 'Good morning' : hour < 17 ? 'Good afternoon' : 'Good evening'

  useEffect(() => {
    if (!orgId) {
      setLoading(false)
      setLoadError('We could not find your club. Sign out and back in to try again.')
      return
    }

    let cancelled = false
    setLoading(true)
    setLoadError(null)

    Promise.all([
      fetchSwimmers(api, orgId),
      fetchMeets(api, orgId),
      fetchAnnouncements(api, orgId, 'published'),
    ]).then(async ([swimmers, allMeets, anns]) => {
      if (cancelled) return
      // Resolve "which swimmer is me" by the stable swimmer_id carried on the
      // session, never by guessing a household roster entry — a household can
      // have multiple swimmers, and only swimmerId identifies which one this
      // login belongs to.
      const mySwimmer = swimmerId
        ? (swimmers.find(s => s.id === swimmerId) ?? null)
        : null
      setSwimmer(mySwimmer)
      setNoLinkedSwimmer(!swimmerId)
      setAnnouncements(anns.slice(0, 3))

      // Filter meets to those with commitment_open or later
      const ACTIVE_STATUSES = ['commitment_open', 'commitment_closed', 'entry_finalized', 'completed']
      const activeMeets = allMeets.filter(m => ACTIVE_STATUSES.includes(m.status))

      if (mySwimmer) {
        // Fetch best times
        const rawBT = await fetchSwimmerBestTimes(api, orgId, mySwimmer.id)
        if (cancelled) return
        const btDisplay: BestTimeDisplay[] = rawBT
          .filter(bt => bt.best_time_ms != null)
          .slice(0, 3)
          .map(bt => ({
            event: eventName(bt.event_distance, bt.stroke),
            course: bt.course.toUpperCase(),
            time: formatSwimTime(bt.best_time_ms!),
            // A best set in the last 30 days still counts as news.
            fresh: bt.recorded_at ? Date.now() - new Date(bt.recorded_at).getTime() < 30 * 86_400_000 : false,
          }))
        setBestTimes(btDisplay)

        // Fetch commitments for each active meet
        const meetDisplays: MeetDisplay[] = await Promise.all(
          activeMeets.slice(0, 5).map(async (m) => {
            let cs: 'committed' | 'pending' | 'no' | 'closed' | 'open' = 'open'
            try {
              const commitments = await fetchMeetCommitments(api, orgId, m.id)
              const myCommitment = commitments.find(c => c.swimmer_id === mySwimmer.id)
              cs = commitStatus(myCommitment?.status)
            } catch (error) {
              // Only a missing commitment list means "open"; any other failure
              // must reach the page-level banner instead of reading as a status.
              if (!(error instanceof ApiError && error.status === 404)) throw error
            }
            return {
              id: m.id,
              name: m.name,
              date: fmtDate(m.start_date),
              course: m.course?.toUpperCase() ?? '',
              location: m.location ?? '',
              commitmentStatus: cs,
              path: `/swim/meets/${m.id}`,
            }
          })
        )
        if (cancelled) return
        setMeets(meetDisplays)
      } else {
        // No swimmer resolved — still show meets without commitment status
        const meetDisplays: MeetDisplay[] = activeMeets.slice(0, 5).map(m => ({
          id: m.id,
          name: m.name,
          date: fmtDate(m.start_date),
          course: m.course?.toUpperCase() ?? '',
          location: m.location ?? '',
          commitmentStatus: 'open' as const,
          path: `/swim/meets/${m.id}`,
        }))
        setMeets(meetDisplays)
      }
    }).catch((error) => {
      // A re-run (e.g. the session hydrating swimmer_id) supersedes this load.
      if (cancelled) return
      setSwimmer(null)
      setMeets([])
      setBestTimes([])
      setAnnouncements([])
      setLoadError(reportUIError(
        'swimmer-dashboard-load',
        error,
        'We could not load this page. Check your connection and try again.',
        { orgId, householdId },
      ))
    }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [api, orgId, householdId, swimmerId, reloadKey])

  const firstName = swimmer
    ? (swimmer.preferred_name || swimmer.first_name)
    : (session?.user.name?.split(' ')[0] ?? 'Swimmer')

  const initials = swimmer
    ? `${swimmer.first_name[0]}${swimmer.last_name[0]}`.toUpperCase()
    : (session?.user.avatarInitials ?? 'SW')

  // Stats
  const nextMeet = meets[0]
  const topBT = bestTimes[0]

  return (
    <div>
      
      {/* Hero greeting */}
      <div
        className="is-31a2ed9"
      >
        {/* Swimmer avatar + name */}
        <div className="is-3e1d09b">
          <div
            className="is-5ee2994"
          >
            {loading ? '…' : initials}
          </div>
          <div>
            <h1 className="page-title"
            >
              {loading ? '…' : `${greeting}, ${firstName}.`}
            </h1>
          </div>
        </div>
      </div>

      <WellnessPromptCard api={api} orgId={orgId} swimmerId={swimmerId} />

      {/* Stats strip */}
      <div
        className="cols-4 is-7d699cf"
        
      >
        {loading ? (
          <>
            <SkeletonCard />
            <SkeletonCard />
            <SkeletonCard />
            <SkeletonCard />
          </>
        ) : (
          <>
            <StatCard
              label="Upcoming meets"
              value={noLinkedSwimmer ? '—' : String(meets.length)}
              sub={noLinkedSwimmer ? 'Link your account to see this' : 'Scheduled this season'}
              accent
            />
            <StatCard
              label="Next meet"
              value={noLinkedSwimmer ? '—' : nextMeet ? (meets[0]?.date ?? '—') : '—'}
              sub={noLinkedSwimmer ? 'Link your account to see this' : (nextMeet?.name ?? 'No meets scheduled')}
            />
            <StatCard
              label={topBT ? `Best (${topBT.event})` : 'Best Time'}
              value={topBT?.time ?? '—'}
              sub={topBT ? `${topBT.course}` : 'No times recorded'}
            />
            <StatCard
              label="Registration"
              value={swimmer ? (swimmer.registration_status === 'active' ? 'Active' : swimmer.registration_status) : '—'}
              sub="Current season"
            />
          </>
        )}
      </div>

      {loadError && (
        <div
          style={{
            marginBottom: '24px',
            background: C.warnBg,
            border: `1px solid ${C.warnBorder}`,
            borderRadius: '12px',
            padding: '16px 16px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: '16px',
          }}
        >
          <div className="is-493767e">
            <Icon name="alert-circle" size={16} style={{ color: C.warn, marginTop: '1px', flexShrink: 0 }} />
            <p style={{ margin: 0, fontSize: 'var(--fs-base)', color: C.text, lineHeight: 1.5 }}>{loadError}</p>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{
              padding: '8px 16px',
              borderRadius: '8px',
              border: `1px solid ${C.warnBorder}`,
              background: 'transparent',
              color: C.text,
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

      {!loading && !loadError && noLinkedSwimmer && <UnlinkedSwimmerNotice />}

      <div
        className="split-aside is-5345b20"
        
      >
        {/* Left column */}
        <div className="is-141baf8">
          {/* Upcoming meets */}
          <section>
            <div className="is-abcbf74">
              <h2 className="section-title"
              >
                Upcoming Meets
              </h2>
              <Link
                href="/swim/schedule"
                style={{
                  fontSize: 'var(--fs-base)',
                  fontWeight: 500,
                  color: C.accent,
                  textDecoration: 'none',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  transition: 'opacity 120ms ease',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.opacity = '0.7')}
                onMouseLeave={(e) => (e.currentTarget.style.opacity = '1')}
              >
                Full schedule
                <Icon name="chevron-right" size={14} />
              </Link>
            </div>
            <div className="is-930d87f">
              {loading ? (
                <>
                  <SkeletonCard />
                  <SkeletonCard />
                </>
              ) : loadError ? null : shouldShowCollectionEmptyState(loading, loadError, meets.length) ? (
                <div className="card is-911be78"
                  
                >
                  <p style={{ fontSize: 'var(--fs-md)', color: C.subtle }}>No upcoming meets scheduled.</p>
                </div>
              ) : (
                meets.map((meet, i) => (
                  <div
                    key={meet.id}
                    style={{ animation: `slideUp 0.4s cubic-bezier(0.16,1,0.3,1) ${0.1 + i * 0.07}s both` }}
                  >
                    <MeetCard meet={meet} />
                  </div>
                ))
              )}
            </div>
          </section>

          {/* Best times preview */}
          <section>
            <div className="is-abcbf74">
              <h2 className="section-title"
              >
                My Best Times
              </h2>
              <Link
                href="/swim/performance"
                style={{
                  fontSize: 'var(--fs-base)',
                  fontWeight: 500,
                  color: C.accent,
                  textDecoration: 'none',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '4px',
                  transition: 'opacity 120ms ease',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.opacity = '0.7')}
                onMouseLeave={(e) => (e.currentTarget.style.opacity = '1')}
              >
                Full performance
                <Icon name="chevron-right" size={14} />
              </Link>
            </div>
            <div className="card"
              style={{
                overflow: 'hidden',
                animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.18s both',
              }}
            >
              {/* Table header */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 80px 100px',
                  padding: '12px 20px',
                  borderBottom: `1px solid ${C.border}`,
                }}
              >
                {['Event', 'Course', 'Best Time'].map((h) => (
                  <span
                    key={h}
                    style={{
                      fontSize: 'var(--fs-2xs)',
                      fontFamily: 'var(--font-body)',
                      color: C.muted,
                    }}
                  >
                    {h}
                  </span>
                ))}
              </div>
              {loading ? (
                <div className="is-6455ae6">
                  <SkeletonLine />
                  <SkeletonLine />
                  <SkeletonLine />
                </div>
              ) : loadError ? null : shouldShowCollectionEmptyState(loading, loadError, bestTimes.length) ? (
                <div className="is-337a55f">
                  <p style={{ fontSize: 'var(--fs-base)', color: C.muted }}>No times on the board yet. Your first race lands here.</p>
                </div>
              ) : (
                bestTimes.map((bt, i) => (
                  <div
                    key={`${bt.event}-${bt.course}`}
                    style={{
                      display: 'grid',
                      gridTemplateColumns: '1fr 80px 100px',
                      padding: '12px 20px',
                      borderBottom: i < bestTimes.length - 1 ? `1px solid ${C.border}` : 'none',
                      alignItems: 'center',
                      transition: 'background 100ms ease',
                    }}
                    onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--border)')}
                    onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                  >
                    <span style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: C.text }}>{bt.event}{bt.fresh && <> <span className="pb-chip">New best</span></>}</span>
                    <span title={courseTitle(bt.course)} style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: C.subtle }}>{bt.course}</span>
                    <span style={{ fontSize: 'var(--fs-md)', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.accent }}>{bt.time}</span>
                  </div>
                ))
              )}
            </div>
          </section>
        </div>

        {/* Right sidebar — announcements */}
        <div className="card"
          style={{
            overflow: 'hidden',
            animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.12s both',
          }}
        >
          <div
            style={{
              padding: '16px 20px 16px',
              borderBottom: `1px solid ${C.border}`,
            }}
          >
            <p
              style={{
                fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)',
                fontSize: 'var(--fs-lg)',
                color: C.text,
              }}
            >
              Announcements
            </p>
          </div>

          {loading ? (
            <div className="is-6455ae6">
              <SkeletonLine />
              <SkeletonLine width="80%" />
              <SkeletonLine width="90%" />
            </div>
          ) : loadError ? null : shouldShowCollectionEmptyState(loading, loadError, announcements.length) ? (
            <div className="is-337a55f">
              <p style={{ fontSize: 'var(--fs-base)', color: C.muted }}>No announcements.</p>
            </div>
          ) : (
            announcements.map((ann, i) => (
              <div
                key={ann.id}
                style={{
                  padding: '16px 20px',
                  borderBottom: i < announcements.length - 1 ? `1px solid ${C.border}` : 'none',
                  display: 'flex',
                  gap: '12px',
                  alignItems: 'flex-start',
                  transition: 'background 100ms ease',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--border)')}
                onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
              >
                {/* Dot */}
                <div
                  style={{
                    width: '7px',
                    height: '7px',
                    borderRadius: '50%',
                    background: C.accent,
                    flexShrink: 0,
                    marginTop: '4px',
                  }}
                />
                <div>
                  <p style={{ fontSize: 'var(--fs-base)', fontWeight: 500, color: C.text, lineHeight: 1.4, marginBottom: '4px' }}>
                    {ann.title}
                  </p>
                  <p style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: C.muted }}>
                    {ann.published_at ? new Date(ann.published_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : ''}
                  </p>
                </div>
              </div>
            ))
          )}

          <div
            style={{
              padding: '12px 20px',
              borderTop: `1px solid ${C.border}`,
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
            }}
          >
            <Icon name="megaphone" size={12} style={{ color: C.muted }} />
            <span style={{ fontSize: 'var(--fs-xs)', color: C.muted }}>
              Ask your parent to check the full inbox for more
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
