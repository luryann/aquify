'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { parseBackendDate } from '@/lib/dates'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
import { UnlinkedSwimmerNotice } from '@/components/swimmer/UnlinkedSwimmerNotice'

const COURSE_LABELS: Record<string, string> = {
  SCY: 'Short Course Yards',
  SCM: 'Short Course Meters',
  LCM: 'Long Course Meters',
}
function courseTitle(course: string): string | undefined {
  return COURSE_LABELS[course.toUpperCase()]
}
import {
  fetchSwimmers,
  fetchMeets,
  fetchMeetCommitments,
  fetchSwimmerBestTimes,
  fetchAnnouncements,
  type SwimmerRecord,
  type AnnouncementRecord,
} from '@/lib/adminData'

// ── Design tokens ──────────────────────────────────────────────────────────
const C = {
  accent:   'var(--accent)',
  accentDim: 'var(--accent-subtle)',
  accentBorder: 'var(--accent-border)',
  text:     'var(--text)',
  subtle:   'var(--text-subtle)',
  muted:    'var(--text-muted)',
  card:     'var(--bg-surface)',
  cardHov:  'var(--bg-elevated)',
  border:   'var(--border)',
  warn:     'var(--warning)',
  warnBg:   'var(--warning-bg)',
  warnBorder: 'var(--warning-border)',
}

// ── Helpers ────────────────────────────────────────────────────────────────
function fmtTime(ms: number): string {
  const s = ms / 1000
  const m = Math.floor(s / 60)
  const rem = s % 60
  return m > 0 ? `${m}:${rem.toFixed(2).padStart(5, '0')}` : rem.toFixed(2)
}

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
  pending:   { label: 'Response Needed', color: 'var(--warning)', bg: 'var(--warning-bg)', border: 'var(--warning-border)' },
  no:        { label: 'Not Going',       color: 'var(--error)', bg: 'var(--error-bg)',  border: 'var(--error-border)' },
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
}

// ── Skeleton components ────────────────────────────────────────────────────
function SkeletonLine({ width = '100%', height = '14px' }: { width?: string; height?: string }) {
  return (
    <div
      style={{
        width,
        height,
        background: 'var(--border)',
        borderRadius: '6px',
        animation: 'pulse 1.5s ease-in-out infinite',
      }}
    />
  )
}

function SkeletonCard() {
  return (
    <div className="card"
      style={{
        padding: '18px 20px',
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
      }}
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
        padding: '18px 20px',
        boxShadow: accent ? `0 0 20px rgba(0,212,170,0.08)` : 'none',
      }}
    >
      <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.muted, letterSpacing: '0.05em', textTransform: 'uppercase', marginBottom: '6px' }}>
        {label}
      </p>
      <p style={{ fontFamily: 'var(--font-display)', fontSize: '28px', color: accent ? C.accent : C.text, letterSpacing: '-0.02em', lineHeight: 1 }}>
        {value}
      </p>
      {sub && (
        <p style={{ fontSize: '12px', color: C.muted, marginTop: '4px' }}>{sub}</p>
      )}
    </div>
  )
}

function MeetCard({ meet }: { meet: MeetDisplay }) {
  const status = COMMIT_STATUS[meet.commitmentStatus]
  return (
    <div className="card"
      style={{
        padding: '18px 20px',
        display: 'flex',
        flexDirection: 'column',
        gap: '14px',
        transition: 'border-color 150ms ease, box-shadow 150ms ease',
      }}
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
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '12px' }}>
        <div>
          <p style={{ fontSize: '15px', fontWeight: 600, color: C.text, marginBottom: '3px' }}>
            {meet.name}
          </p>
          <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
            <span style={{ fontSize: '12px', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
              <Icon name="calendar" size={11} />
              {meet.date}
            </span>
            {meet.location && (
              <span style={{ fontSize: '12px', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
                <Icon name="map-pin" size={11} />
                {meet.location}
              </span>
            )}
          </div>
        </div>
        <div style={{ display: 'flex', gap: '7px', alignItems: 'center' }}>
          {meet.course && (
            <span title={courseTitle(meet.course)} style={{ fontSize: '10px', fontFamily: 'var(--font-body)', fontWeight: 600, color: 'var(--bg-surface)', background: C.accent, padding: '2px 7px', borderRadius: '4px', letterSpacing: '0.04em' }}>
              {meet.course}
            </span>
          )}
          <span
            style={{
              fontSize: '11px',
              fontWeight: 500,
              color: status.color,
              background: status.bg,
              border: `1px solid ${status.border}`,
              padding: '3px 9px',
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
            fontSize: '12px',
            fontWeight: 600,
            color: C.accent,
            textDecoration: 'none',
            display: 'inline-flex',
            alignItems: 'center',
            gap: '5px',
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
      setLoadError('Swimmer portal is unavailable because no organization is selected.')
      return
    }

    setLoading(true)
    setLoadError(null)

    Promise.all([
      fetchSwimmers(api, orgId),
      fetchMeets(api, orgId),
      fetchAnnouncements(api, orgId, 'published'),
    ]).then(async ([swimmers, allMeets, anns]) => {
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
        const btDisplay: BestTimeDisplay[] = rawBT
          .filter(bt => bt.best_time_ms != null)
          .slice(0, 3)
          .map(bt => ({
            event: eventName(bt.event_distance, bt.stroke),
            course: bt.course.toUpperCase(),
            time: fmtTime(bt.best_time_ms!),
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
            } catch {
              // no commitment record yet → open
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
      setSwimmer(null)
      setMeets([])
      setBestTimes([])
      setAnnouncements([])
      setLoadError(reportUIError(
        'swimmer-dashboard-load',
        error,
        'Swimmer dashboard data is unavailable right now.',
        { orgId, householdId },
      ))
    }).finally(() => setLoading(false))
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
      <style>{`
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.45; }
        }
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(12px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>

      {/* Hero greeting */}
      <div
        style={{
          marginBottom: '40px',
          animation: 'slideUp 0.45s cubic-bezier(0.16,1,0.3,1) both',
        }}
      >
        {/* Swimmer avatar + name */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '18px', marginBottom: '8px' }}>
          <div
            style={{
              width: '56px',
              height: '56px',
              borderRadius: '50%',
              background: 'linear-gradient(135deg, var(--accent) 0%, var(--accent-2) 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: 'var(--bg)',
              fontSize: '18px',
              fontWeight: 700,
              fontFamily: 'var(--font-body)',
              boxShadow: '0 0 24px rgba(0,212,170,0.30)',
              flexShrink: 0,
            }}
          >
            {loading ? '…' : initials}
          </div>
          <div>
            <p
              style={{
                fontSize: '12px',
                fontFamily: 'var(--font-body)',
                color: C.muted,
                letterSpacing: '0.05em',
                marginBottom: '4px',
              }}
            >
              {greeting}
            </p>
            <h1
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: 'clamp(26px, 4vw, 36px)',
                color: C.text,
                letterSpacing: '-0.02em',
                lineHeight: 1.1,
              }}
            >
              {loading ? '…' : `${firstName}.`}
            </h1>
          </div>
        </div>
        <p style={{ fontSize: '14px', color: C.subtle, marginTop: '8px', marginLeft: '74px' }}>
          {loading
            ? ''
            : swimmer
              ? `${swimmer.registration_status === 'active' ? 'Active' : swimmer.registration_status} member`
              : 'Swimmer portal'}
        </p>
      </div>

      {/* Stats strip */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(170px, 1fr))',
          gap: '12px',
          marginBottom: '40px',
          animation: 'slideUp 0.45s cubic-bezier(0.16,1,0.3,1) 0.06s both',
        }}
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
              label="Upcoming Meets"
              value={noLinkedSwimmer ? '—' : String(meets.length)}
              sub={noLinkedSwimmer ? 'Link your account to see this' : 'Scheduled this season'}
              accent
            />
            <StatCard
              label="Next Meet"
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
            padding: '14px 16px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: '16px',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'flex-start', gap: '10px' }}>
            <Icon name="alert-circle" size={16} style={{ color: C.warn, marginTop: '1px', flexShrink: 0 }} />
            <p style={{ margin: 0, fontSize: '13px', color: C.text, lineHeight: 1.5 }}>{loadError}</p>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{
              padding: '8px 14px',
              borderRadius: '8px',
              border: `1px solid ${C.warnBorder}`,
              background: 'transparent',
              color: C.text,
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

      {!loading && !loadError && noLinkedSwimmer && <UnlinkedSwimmerNotice />}

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'minmax(0,1fr) 340px',
          gap: '24px',
          alignItems: 'start',
        }}
      >
        {/* Left column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '32px' }}>
          {/* Upcoming meets */}
          <section>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
              <h2
                style={{
                  fontFamily: 'var(--font-display)',
                  fontSize: '20px',
                  color: C.text,
                  letterSpacing: '-0.01em',
                }}
              >
                Upcoming Meets
              </h2>
              <Link
                href="/swim/schedule"
                style={{
                  fontSize: '13px',
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
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {loading ? (
                <>
                  <SkeletonCard />
                  <SkeletonCard />
                </>
              ) : loadError ? (
                <div className="card"
                  style={{
                    padding: '32px 20px',
                    textAlign: 'center',
                  }}
                >
                  <p style={{ fontSize: '14px', color: C.subtle }}>Meet data is unavailable right now.</p>
                </div>
              ) : shouldShowCollectionEmptyState(loading, loadError, meets.length) ? (
                <div className="card"
                  style={{
                    padding: '32px 20px',
                    textAlign: 'center',
                  }}
                >
                  <p style={{ fontSize: '14px', color: C.subtle }}>No upcoming meets scheduled.</p>
                </div>
              ) : (
                meets.map((meet, i) => (
                  <div
                    key={meet.id}
                    style={{ animationDelay: `${0.1 + i * 0.07}s`, animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) both' }}
                  >
                    <MeetCard meet={meet} />
                  </div>
                ))
              )}
            </div>
          </section>

          {/* Best times preview */}
          <section>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
              <h2
                style={{
                  fontFamily: 'var(--font-display)',
                  fontSize: '20px',
                  color: C.text,
                  letterSpacing: '-0.01em',
                }}
              >
                My Best Times
              </h2>
              <Link
                href="/swim/performance"
                style={{
                  fontSize: '13px',
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
                  padding: '10px 18px',
                  borderBottom: `1px solid ${C.border}`,
                }}
              >
                {['Event', 'Course', 'Best Time'].map((h) => (
                  <span
                    key={h}
                    style={{
                      fontSize: '10.5px',
                      fontFamily: 'var(--font-body)',
                      color: C.muted,
                      textTransform: 'uppercase',
                      letterSpacing: '0.05em',
                    }}
                  >
                    {h}
                  </span>
                ))}
              </div>
              {loading ? (
                <div style={{ padding: '16px 18px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
                  <SkeletonLine />
                  <SkeletonLine />
                  <SkeletonLine />
                </div>
              ) : loadError ? (
                <div style={{ padding: '24px 18px', textAlign: 'center' }}>
                  <p style={{ fontSize: '13px', color: C.muted }}>Best-time data is unavailable right now.</p>
                </div>
              ) : shouldShowCollectionEmptyState(loading, loadError, bestTimes.length) ? (
                <div style={{ padding: '24px 18px', textAlign: 'center' }}>
                  <p style={{ fontSize: '13px', color: C.muted }}>No best times recorded yet.</p>
                </div>
              ) : (
                bestTimes.map((bt, i) => (
                  <div
                    key={`${bt.event}-${bt.course}`}
                    style={{
                      display: 'grid',
                      gridTemplateColumns: '1fr 80px 100px',
                      padding: '12px 18px',
                      borderBottom: i < bestTimes.length - 1 ? `1px solid ${C.border}` : 'none',
                      alignItems: 'center',
                      transition: 'background 100ms ease',
                    }}
                    onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--border)')}
                    onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                  >
                    <span style={{ fontSize: '13.5px', fontWeight: 600, color: C.text }}>{bt.event}</span>
                    <span title={courseTitle(bt.course)} style={{ fontSize: '12px', fontFamily: 'var(--font-body)', color: C.subtle }}>{bt.course}</span>
                    <span style={{ fontSize: '14px', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.accent }}>{bt.time}</span>
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
              padding: '16px 18px 14px',
              borderBottom: `1px solid ${C.border}`,
            }}
          >
            <p
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '17px',
                color: C.text,
                marginBottom: '2px',
              }}
            >
              Announcements
            </p>
            <p style={{ fontSize: '12px', color: C.muted }}>
              Latest from the club
            </p>
          </div>

          {loading ? (
            <div style={{ padding: '14px 18px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <SkeletonLine />
              <SkeletonLine width="80%" />
              <SkeletonLine width="90%" />
            </div>
          ) : loadError ? (
            <div style={{ padding: '24px 18px', textAlign: 'center' }}>
              <p style={{ fontSize: '13px', color: C.muted }}>Announcements are unavailable right now.</p>
            </div>
          ) : shouldShowCollectionEmptyState(loading, loadError, announcements.length) ? (
            <div style={{ padding: '24px 18px', textAlign: 'center' }}>
              <p style={{ fontSize: '13px', color: C.muted }}>No announcements.</p>
            </div>
          ) : (
            announcements.map((ann, i) => (
              <div
                key={ann.id}
                style={{
                  padding: '14px 18px',
                  borderBottom: i < announcements.length - 1 ? `1px solid ${C.border}` : 'none',
                  display: 'flex',
                  gap: '10px',
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
                    marginTop: '5px',
                  }}
                />
                <div>
                  <p style={{ fontSize: '13px', fontWeight: 500, color: C.text, lineHeight: 1.4, marginBottom: '3px' }}>
                    {ann.title}
                  </p>
                  <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.muted }}>
                    {ann.published_at ? new Date(ann.published_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : ''}
                  </p>
                </div>
              </div>
            ))
          )}

          <div
            style={{
              padding: '10px 18px',
              borderTop: `1px solid ${C.border}`,
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
            }}
          >
            <Icon name="megaphone" size={12} style={{ color: C.muted }} />
            <span style={{ fontSize: '11.5px', color: C.muted }}>
              Ask your parent to check the full inbox for more
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
