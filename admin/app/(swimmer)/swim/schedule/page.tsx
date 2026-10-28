'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { parseBackendDate } from '@/lib/dates'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  fetchMeets,
  fetchMeetCommitments,
  type MeetRecord,
} from '@/lib/adminData'
import { fetchTrainingSchedule, type ScheduleItem, type Workout } from '@/lib/trainingData'
import { formatDistance, formatDuration } from '@/lib/workout-format'
import { WorkoutSheet } from '@/components/training/WorkoutSheet'
import { SwimmerClubEvents } from '@/components/engagement/SwimmerClubEvents'
import './page.inline.css'

// Convert raw commitment status to a plain-language answer, mirroring the
// dashboard's own commitment badge so the swimmer sees the same wording
// everywhere they can see their own going/not-going status.
function commitmentAnswer(raw: string | undefined): 'yes' | 'no' | 'waiting' | undefined {
  if (!raw) return undefined
  if (raw === 'committed_yes') return 'yes'
  if (raw === 'committed_no') return 'no'
  if (raw === 'invited') return 'waiting'
  return undefined
}

const COMMITMENT_ANSWER_STYLE: Record<'yes' | 'no' | 'waiting', { label: string; color: string; bg: string; border: string }> = {
  yes:     { label: 'Going',     color: 'var(--accent)',  bg: 'var(--border)',      border: 'var(--accent-border)' },
  no:      { label: 'Not going', color: 'var(--error)',   bg: 'var(--error-bg)',    border: 'var(--error-border)' },
  waiting: { label: 'Waiting for your answer', color: 'var(--warning)', bg: 'var(--warning-bg)', border: 'var(--warning-border)' },
}

// ── Design tokens ──────────────────────────────────────────────────────────
const C = {
  accent:       'var(--accent)',
  accentDim:    'var(--border)',
  accentBorder: 'var(--accent-border)',
  text:         'var(--text)',
  subtle:       'var(--text-subtle)',
  muted:        'var(--text-muted)',
  card:         'var(--bg-surface)',
  border:       'var(--border)',
}

// ── Types ──────────────────────────────────────────────────────────────────
type EventType = 'meet' | 'deadline' | 'practice'

type ScheduleEvent = {
  id: string
  title: string
  type: EventType
  date: Date
  dateLabel: string      // "Sat, Mar 22"
  location?: string
  course?: string
  detail?: string
  meetPath?: string
  commitmentAnswer?: 'yes' | 'no' | 'waiting'
  workout?: Workout
}

const EVENT_STYLE: Record<EventType, { dot: string; bg: string; border: string; color: string; label: string; leftBorder: string }> = {
  meet:     { dot: 'var(--accent)', bg: 'var(--border)',  border: 'rgba(0,212,170,0.20)',   color: 'var(--accent)', label: 'Meet',      leftBorder: 'var(--accent)' },
  deadline: { dot: 'var(--error)', bg: 'var(--error-bg)',  border: 'var(--error-border)',  color: 'var(--error)', label: 'Deadline',  leftBorder: 'var(--error)' },
  practice: { dot: 'var(--text-subtle)', bg: 'rgba(127,168,201,0.06)', border: 'var(--text-subtle)', color: 'var(--text-subtle)', label: 'Practice', leftBorder: 'var(--text-subtle)' },
}

// ── Helpers ────────────────────────────────────────────────────────────────
function fmtDayLabel(d: Date): string {
  return d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
}

function fmtSectionHeading(d: Date): string {
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

function buildScheduleFromMeets(meets: MeetRecord[], commitmentAnswers: Record<string, 'yes' | 'no' | 'waiting'>): ScheduleEvent[] {
  const now = new Date()
  const events: ScheduleEvent[] = []

  for (const m of meets) {
    if (!m.start_date) continue
    const meetDate = parseBackendDate(m.start_date)
    if (meetDate < now && m.status === 'completed') continue

    // Meet event (start date)
    events.push({
      id: `meet-${m.id}`,
      title: m.name,
      type: 'meet',
      date: meetDate,
      dateLabel: fmtDayLabel(meetDate),
      location: m.location ?? undefined,
      course: m.course?.toUpperCase() ?? undefined,
      detail: m.end_date && m.end_date !== m.start_date
        ? `Meet runs ${parseBackendDate(m.start_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })} – ${parseBackendDate(m.end_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}.`
        : undefined,
      meetPath: `/swim/meets/${m.id}`,
      commitmentAnswer: commitmentAnswers[m.id],
    })

    // Commitment deadline event
    if (m.commitment_deadline) {
      const deadlineDate = parseBackendDate(m.commitment_deadline)
      if (deadlineDate >= now) {
        events.push({
          id: `deadline-${m.id}`,
          title: `Parent RSVP due: ${m.name}`,
          type: 'deadline',
          date: deadlineDate,
          dateLabel: fmtDayLabel(deadlineDate),
          detail: 'Your parent must respond by 11:59 PM on this date.',
        })
      }
    }
  }

  return events.sort((a, b) => a.date.getTime() - b.date.getTime())
}

function buildScheduleFromPractices(items: ScheduleItem[], now: Date): ScheduleEvent[] {
  return items
    .filter(({ practice: p }) => new Date(p.date + 'T23:59:59') >= now)
    .map(({ practice: p, workout }) => {
      const practiceDate = parseBackendDate(p.date)
      return {
        id: `practice-${p.id}`,
        title: p.description ? `Practice — ${p.description.slice(0, 60)}` : 'Practice',
        type: 'practice' as EventType,
        date: practiceDate,
        dateLabel: fmtDayLabel(practiceDate),
        detail: workout
          ? `${formatDistance(workout.totals.distance, workout.totals.unit)} ${workout.course}, about ${formatDuration(workout.totals.estimated_seconds)}.`
          : p.yardage_target > 0 ? `${p.yardage_target.toLocaleString()} yds target` : undefined,
        workout: workout ?? undefined,
      }
    })
}

// Group sorted events into weekly sections
function groupIntoSections(events: ScheduleEvent[]): Array<{ heading: string; events: ScheduleEvent[] }> {
  const groups: Array<{ heading: string; events: ScheduleEvent[]; firstDate: Date }> = []

  for (const ev of events) {
    const lastGroup = groups[groups.length - 1]
    const dayDelta = lastGroup
      ? Math.floor((ev.date.getTime() - lastGroup.firstDate.getTime()) / 86400000)
      : 0

    if (!lastGroup || dayDelta > 6) {
      groups.push({ heading: `${fmtSectionHeading(ev.date)} Week`, events: [ev], firstDate: ev.date })
    } else {
      lastGroup.events.push(ev)
      const lastDate = lastGroup.events[lastGroup.events.length - 1].date
      lastGroup.heading = `${fmtSectionHeading(lastGroup.firstDate)} – ${fmtSectionHeading(lastDate)}`
    }
  }

  return groups.map(({ heading, events }) => ({ heading, events }))
}

// ── Skeleton ───────────────────────────────────────────────────────────────
function SkeletonLine({ width = '100%', height = '14px' }: { width?: string; height?: string }) {
  return (
    <div style={{ width, height, background: 'var(--border)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite' }} />
  )
}

// ── Components ─────────────────────────────────────────────────────────────
function EventCard({ ev }: { ev: ScheduleEvent }) {
  const [expanded, setExpanded] = useState(ev.type === 'meet')
  const s = EVENT_STYLE[ev.type]

  return (
    <div
      style={{
        background: C.card,
        border: `1px solid ${s.border}`,
        borderRadius: '12px',
        padding: '16px 16px',
        display: 'flex',
        gap: '16px',
        transition: 'border-color 120ms ease',
      }}
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.borderColor = s.dot
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.borderColor = s.border
      }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        {/* Title row */}
        <div className="is-ece61a4">
          <p style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: C.text, lineHeight: 1.3, marginBottom: '6px' }}>
            {ev.title}
          </p>
          <div className="is-99659b5">
            {ev.course && (
              <span style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', fontWeight: 600, color: 'var(--bg-surface)', background: C.accent, padding: '2px 6px', borderRadius: '4px', }}>
                {ev.course}
              </span>
            )}
            <span
              style={{
                fontSize: 'var(--fs-2xs)',
                fontFamily: 'var(--font-body)',
                fontWeight: 600,
                color: s.color,
                background: s.bg,
                border: `1px solid ${s.border}`,
                padding: '2px 8px',
                borderRadius: '999px',
              }}
            >
              {s.label}
            </span>
          </div>
        </div>

        {/* Meta row */}
        <div className="is-b7f7d3c">
          {ev.location && (
            <span style={{ fontSize: 'var(--fs-sm)', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
              <Icon name="map-pin" size={11} />
              {ev.location}
            </span>
          )}
          {ev.commitmentAnswer && (
            <span style={{
              fontSize: 'var(--fs-2xs)', fontWeight: 600, padding: '2px 8px', borderRadius: '999px',
              color: COMMITMENT_ANSWER_STYLE[ev.commitmentAnswer].color,
              background: COMMITMENT_ANSWER_STYLE[ev.commitmentAnswer].bg,
              border: `1px solid ${COMMITMENT_ANSWER_STYLE[ev.commitmentAnswer].border}`,
            }}>
              {COMMITMENT_ANSWER_STYLE[ev.commitmentAnswer].label}
            </span>
          )}
        </div>

        {/* Expandable content */}
        {(ev.detail || ev.meetPath) && (
          <div>
            <button type="button"
              onClick={() => setExpanded(!expanded)}
              style={{
                background: 'transparent',
                border: 'none',
                color: s.color,
                fontSize: 'var(--fs-sm)',
                fontWeight: 500,
                cursor: 'pointer',
                padding: '6px 0 0',
                display: 'flex',
                alignItems: 'center',
                gap: '4px',
              }}
            >
              {expanded ? 'Less' : ev.workout ? 'Show sets' : 'More'}
              <Icon name={expanded ? 'chevron-up' : 'chevron-down'} size={12} />
            </button>

            {expanded && (
              <div className="is-5231bff">
                {ev.detail && (
                  <p style={{ fontSize: 'var(--fs-sm)', color: C.muted, lineHeight: 1.55 }}>
                    {ev.detail}
                  </p>
                )}
                {ev.workout && <WorkoutSheet workout={ev.workout} />}
                {ev.meetPath && (
                  <div>
                    <Link
                      href={ev.meetPath}
                      style={{
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: '4px',
                        fontSize: 'var(--fs-sm)',
                        fontWeight: 600,
                        color: C.accent,
                        textDecoration: 'none',
                      }}
                    >
                      Open meet details
                      <Icon name="chevron-right" size={12} />
                    </Link>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function SwimmerSchedulePage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const mySwimmerId = session?.user.swimmerId ?? ''

  const [schedule, setSchedule] = useState<ScheduleEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [typeFilter, setTypeFilter] = useState<EventType | 'all' | 'practice'>('all')
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId) {
      setLoading(false)
      setLoadError('We could not find your club. Sign out and back in to try again.')
      return
    }
    const now = new Date()
    setLoading(true)
    setLoadError(null)
    Promise.all([
      fetchMeets(api, orgId),
      fetchTrainingSchedule(api, orgId, now.toISOString().slice(0, 10), new Date(now.getTime() + 56 * 86400000).toISOString().slice(0, 10)),
    ])
      .then(async ([meets, practices]) => {
        const futureMeets = meets.filter(m => {
          if (m.end_date) return parseBackendDate(m.end_date) >= now
          if (m.start_date) return parseBackendDate(m.start_date) >= now
          return false
        })

        const commitmentAnswers: Record<string, 'yes' | 'no' | 'waiting'> = {}
        if (mySwimmerId) {
          await Promise.all(futureMeets.map(async (m) => {
            try {
              const commitments = await fetchMeetCommitments(api, orgId, m.id)
              const mine = commitments.find((c) => c.swimmer_id === mySwimmerId)
              const answer = commitmentAnswer(mine?.status)
              if (answer) commitmentAnswers[m.id] = answer
            } catch {
              // Sign-ups may not be open yet for this meet — leave the badge off.
            }
          }))
        }

        const allEvents = [
          ...buildScheduleFromMeets(futureMeets, commitmentAnswers),
          ...buildScheduleFromPractices(practices, now),
        ].sort((a, b) => a.date.getTime() - b.date.getTime())
        setSchedule(allEvents)
      })
      .catch((error) => {
        setSchedule([])
        setLoadError(reportUIError(
          'swimmer-schedule-load',
          error,
          'We could not load this page. Check your connection and try again.',
          { orgId },
        ))
      })
      .finally(() => setLoading(false))
  }, [api, orgId, mySwimmerId, reloadKey])

  const filtered = typeFilter === 'all'
    ? schedule
    : schedule.filter(ev => ev.type === typeFilter)

  const sections = groupIntoSections(filtered)

  return (
    <div>
      <style>{`
        @keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.45} }
        @keyframes slideUp { from{opacity:0;transform:translateY(12px)} to{opacity:1;transform:translateY(0)} }
      `}</style>

      {/* Header */}
      <div className="is-88ea830">
        <h1 className="page-title"
        >
          Schedule
        </h1>
      </div>

      {loadError && (
        <div
          className="is-ee77e17"
        >
          <div className="is-730b6a7">
            <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
            <p style={{ margin: 0, fontSize: 'var(--fs-sm)', color: C.text, lineHeight: 1.5 }}>{loadError}</p>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{ background: 'transparent', border: '1px solid rgba(245,158,11,0.25)', borderRadius: '8px', cursor: 'pointer', color: C.text, fontSize: 'var(--fs-sm)', fontWeight: 600, padding: '8px 16px', whiteSpace: 'nowrap' }}
          >
            Retry
          </button>
        </div>
      )}

      {/* Type filter */}
      <div
        className="is-bb0f973"
      >
        {(['all', 'meet', 'practice', 'deadline'] as const).map((type) => {
          const active = typeFilter === type
          const s = type !== 'all' ? EVENT_STYLE[type] : null
          return (
            <button type="button"
              key={type}
              onClick={() => setTypeFilter(type)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                padding: '4px 16px',
                borderRadius: '999px',
                border: `1px solid ${active ? (s ? s.border : C.accentBorder) : C.border}`,
                background: active ? (s ? s.bg : C.accentDim) : 'transparent',
                color: active ? (s ? s.color : C.accent) : C.subtle,
                fontSize: 'var(--fs-sm)',
                fontWeight: 500,
                cursor: 'pointer',
                transition: 'all 120ms ease',
              }}
            >
              {s && (
                <span style={{ width: '7px', height: '7px', borderRadius: '50%', background: s.dot, flexShrink: 0 }} />
              )}
              <span style={{ textTransform: 'capitalize' }}>
                {type === 'all' ? 'All' : s!.label}
              </span>
            </button>
          )
        })}
      </div>

      <SwimmerClubEvents api={api} orgId={orgId} swimmerId={mySwimmerId} />

      {/* Schedule sections */}
      {loading ? (
        <div className="is-efd85b9">
          {[1, 2].map(i => (
            <div key={i} className="is-930d87f">
              <div style={{ height: '18px', width: '140px', background: 'var(--border)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite' }} />
              <div className="card is-824b1f4" >
                <SkeletonLine width="60%" />
                <SkeletonLine width="40%" height="12px" />
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="is-8ceab81">
          {sections.map((section, si) => (
            <section
              key={section.heading}
              style={{ animation: `slideUp 0.35s cubic-bezier(0.16,1,0.3,1) ${si * 60}ms both` }}
            >
              {/* Section heading */}
              <div className="is-7f15173">
                <h2 className="section-title" style={{ whiteSpace: 'nowrap' }}
                >
                  {section.heading}
                </h2>
                <div style={{ flex: 1, height: '1px', background: C.border }} />
                <span style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', color: C.muted, whiteSpace: 'nowrap' }}>
                  {section.events.length} {section.events.length === 1 ? 'event' : 'events'}
                </span>
              </div>

              {/* Events in section */}
              <div className="is-355eedd">
                {section.events.map((ev, i) => (
                  <div
                    key={ev.id}
                    style={{
                      display: 'grid',
                      gridTemplateColumns: 'clamp(52px, 16vw, 100px) minmax(0, 1fr)',
                      gap: '12px',
                      alignItems: 'flex-start',
                      animation: `slideUp 0.3s cubic-bezier(0.16,1,0.3,1) ${si * 60 + i * 40}ms both`,
                    }}
                  >
                    {/* Date column */}
                    <div className="is-36493dc">
                      <p style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: C.text, lineHeight: 1.3 }}>
                        {ev.dateLabel.split(', ')[0]}
                      </p>
                      <p style={{ fontSize: 'var(--fs-sm)', color: C.subtle }}>
                        {ev.dateLabel.split(', ')[1]}
                      </p>
                    </div>

                    {/* Event card */}
                    <EventCard ev={ev} />
                  </div>
                ))}
              </div>
            </section>
          ))}

          {loadError ? null : shouldShowCollectionEmptyState(loading, loadError, sections.length) && (
            <div className="card is-16a78cd"
              
            >
              <Icon name="calendar" size={32} style={{ color: C.muted, marginBottom: '16px' }} />
              <p style={{ fontSize: 'var(--fs-md)', color: C.subtle, fontWeight: 500 }}>No upcoming events</p>
              <p style={{ fontSize: 'var(--fs-base)', color: C.muted, marginTop: '4px' }}>
                {typeFilter !== 'all' ? 'No events match this filter.' : 'No meets or deadlines scheduled yet.'}
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
