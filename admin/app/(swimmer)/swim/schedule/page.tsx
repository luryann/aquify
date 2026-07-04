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
  fetchPractices,
  type MeetRecord,
  type PracticeRecord,
} from '@/lib/adminData'

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

function buildScheduleFromMeets(meets: MeetRecord[]): ScheduleEvent[] {
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
    })

    // Commitment deadline event
    if (m.commitment_deadline) {
      const deadlineDate = new Date(m.commitment_deadline)
      if (deadlineDate >= now) {
        events.push({
          id: `deadline-${m.id}`,
          title: `Commitment Deadline — ${m.name}`,
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

function buildScheduleFromPractices(practices: PracticeRecord[], now: Date): ScheduleEvent[] {
  return practices
    .filter((p) => new Date(p.date + 'T23:59:59') >= now)
    .map((p) => {
      const practiceDate = new Date(p.date + 'T00:00:00')
      return {
        id: `practice-${p.id}`,
        title: p.description ? `Practice — ${p.description.slice(0, 60)}` : 'Practice',
        type: 'practice' as EventType,
        date: practiceDate,
        dateLabel: fmtDayLabel(practiceDate),
        detail: p.yardage_target > 0 ? `${p.yardage_target.toLocaleString()} yds target` : undefined,
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
        borderLeft: `3px solid ${s.leftBorder}`,
        borderRadius: '0 10px 10px 0',
        padding: '14px 16px',
        display: 'flex',
        gap: '14px',
        transition: 'border-color 120ms ease',
      }}
      onMouseEnter={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.borderColor = s.dot
        el.style.borderLeftColor = s.dot
      }}
      onMouseLeave={(e) => {
        const el = e.currentTarget as HTMLDivElement
        el.style.borderColor = s.border
        el.style.borderLeftColor = s.leftBorder
      }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        {/* Title row */}
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '10px' }}>
          <p style={{ fontSize: '14px', fontWeight: 600, color: C.text, lineHeight: 1.3, marginBottom: '6px' }}>
            {ev.title}
          </p>
          <div style={{ display: 'flex', gap: '6px', alignItems: 'center', flexShrink: 0 }}>
            {ev.course && (
              <span style={{ fontSize: '10px', fontFamily: 'var(--font-body)', fontWeight: 600, color: 'var(--bg-surface)', background: C.accent, padding: '2px 6px', borderRadius: '4px', letterSpacing: '0.04em' }}>
                {ev.course}
              </span>
            )}
            <span
              style={{
                fontSize: '10px',
                fontFamily: 'var(--font-body)',
                fontWeight: 600,
                letterSpacing: '0.04em',
                textTransform: 'uppercase',
                color: s.color,
                background: s.bg,
                border: `1px solid ${s.border}`,
                padding: '2px 7px',
                borderRadius: '999px',
              }}
            >
              {s.label}
            </span>
          </div>
        </div>

        {/* Meta row */}
        <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
          {ev.location && (
            <span style={{ fontSize: '12px', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
              <Icon name="map-pin" size={11} />
              {ev.location}
            </span>
          )}
        </div>

        {/* Expandable content */}
        {(ev.detail || ev.meetPath) && (
          <div>
            <button
              onClick={() => setExpanded(!expanded)}
              style={{
                background: 'transparent',
                border: 'none',
                color: s.color,
                fontSize: '12px',
                fontWeight: 500,
                cursor: 'pointer',
                padding: '6px 0 0',
                display: 'flex',
                alignItems: 'center',
                gap: '4px',
              }}
            >
              {expanded ? 'Less' : 'More'}
              <Icon name={expanded ? 'chevron-up' : 'chevron-down'} size={12} />
            </button>

            {expanded && (
              <div style={{ marginTop: '10px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {ev.detail && (
                  <p style={{ fontSize: '12.5px', color: C.muted, lineHeight: 1.55 }}>
                    {ev.detail}
                  </p>
                )}
                {ev.meetPath && (
                  <div>
                    <Link
                      href={ev.meetPath}
                      style={{
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: '5px',
                        fontSize: '12px',
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

  const [schedule, setSchedule] = useState<ScheduleEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [typeFilter, setTypeFilter] = useState<EventType | 'all' | 'practice'>('all')
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId) {
      setLoading(false)
      setLoadError('Swimmer schedule is unavailable because no organization is selected.')
      return
    }
    const now = new Date()
    setLoading(true)
    setLoadError(null)
    Promise.all([
      fetchMeets(api, orgId),
      fetchPractices(api, orgId),
    ])
      .then(([meets, practices]) => {
        const futureMeets = meets.filter(m => {
          if (m.end_date) return new Date(m.end_date) >= now
          if (m.start_date) return new Date(m.start_date) >= now
          return false
        })
        const allEvents = [
          ...buildScheduleFromMeets(futureMeets),
          ...buildScheduleFromPractices(practices, now),
        ].sort((a, b) => a.date.getTime() - b.date.getTime())
        setSchedule(allEvents)
      })
      .catch((error) => {
        setSchedule([])
        setLoadError(reportUIError(
          'swimmer-schedule-load',
          error,
          'Swimmer schedule data is unavailable right now.',
          { orgId },
        ))
      })
      .finally(() => setLoading(false))
  }, [api, orgId, reloadKey])

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
      <div style={{ marginBottom: '28px', animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) both' }}>
        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: 'clamp(26px, 4vw, 34px)',
            color: C.text,
            letterSpacing: '-0.02em',
            lineHeight: 1.1,
            marginBottom: '6px',
          }}
        >
          Schedule
        </h1>
        <p style={{ fontSize: '14px', color: C.subtle }}>
          Upcoming meets and commitment deadlines.
        </p>
      </div>

      {loadError && (
        <div
          style={{
            marginBottom: '24px',
            background: 'var(--warning-bg)',
            border: '1px solid rgba(245,158,11,0.25)',
            borderRadius: '10px',
            padding: '12px 16px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: '12px',
          }}
        >
          <div style={{ display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
            <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
            <p style={{ margin: 0, fontSize: '12.5px', color: C.text, lineHeight: 1.5 }}>{loadError}</p>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{ background: 'transparent', border: '1px solid rgba(245,158,11,0.25)', borderRadius: '8px', cursor: 'pointer', color: C.text, fontSize: '12px', fontWeight: 600, padding: '8px 14px', whiteSpace: 'nowrap' }}
          >
            Retry
          </button>
        </div>
      )}

      {/* Type filter */}
      <div
        style={{
          display: 'flex',
          gap: '8px',
          marginBottom: '28px',
          flexWrap: 'wrap',
          animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.05s both',
        }}
      >
        {(['all', 'meet', 'practice', 'deadline'] as const).map((type) => {
          const active = typeFilter === type
          const s = type !== 'all' ? EVENT_STYLE[type] : null
          return (
            <button
              key={type}
              onClick={() => setTypeFilter(type)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                padding: '5px 14px',
                borderRadius: '999px',
                border: `1px solid ${active ? (s ? s.border : C.accentBorder) : C.border}`,
                background: active ? (s ? s.bg : C.accentDim) : 'transparent',
                color: active ? (s ? s.color : C.accent) : C.subtle,
                fontSize: '12.5px',
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

      {/* Schedule sections */}
      {loading ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
          {[1, 2].map(i => (
            <div key={i} style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <div style={{ height: '18px', width: '140px', background: 'var(--border)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite' }} />
              <div className="card" style={{ padding: '18px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
                <SkeletonLine width="60%" />
                <SkeletonLine width="40%" height="12px" />
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '36px' }}>
          {sections.map((section, si) => (
            <section
              key={section.heading}
              style={{ animation: `slideUp 0.35s cubic-bezier(0.16,1,0.3,1) ${si * 60}ms both` }}
            >
              {/* Section heading */}
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '14px' }}>
                <h2
                  style={{
                    fontFamily: 'var(--font-display)',
                    fontSize: '17px',
                    color: C.text,
                    letterSpacing: '-0.01em',
                    whiteSpace: 'nowrap',
                  }}
                >
                  {section.heading}
                </h2>
                <div style={{ flex: 1, height: '1px', background: C.border }} />
                <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.muted, whiteSpace: 'nowrap' }}>
                  {section.events.length} {section.events.length === 1 ? 'event' : 'events'}
                </span>
              </div>

              {/* Events in section */}
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {section.events.map((ev, i) => (
                  <div
                    key={ev.id}
                    style={{
                      display: 'grid',
                      gridTemplateColumns: '100px 1fr',
                      gap: '12px',
                      alignItems: 'flex-start',
                      animationDelay: `${si * 60 + i * 40}ms`,
                      animation: 'slideUp 0.3s cubic-bezier(0.16,1,0.3,1) both',
                    }}
                  >
                    {/* Date column */}
                    <div style={{ paddingTop: '14px' }}>
                      <p style={{ fontSize: '13px', fontWeight: 600, color: C.text, lineHeight: 1.3 }}>
                        {ev.dateLabel.split(', ')[0]}
                      </p>
                      <p style={{ fontSize: '12px', color: C.subtle }}>
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

          {loadError ? (
            <div className="card"
              style={{
                padding: '60px 20px',
                textAlign: 'center',
              }}
            >
              <Icon name="calendar" size={32} style={{ color: C.muted, marginBottom: '16px' }} />
              <p style={{ fontSize: '15px', color: C.subtle, fontWeight: 500 }}>Schedule data is unavailable right now.</p>
              <p style={{ fontSize: '13px', color: C.muted, marginTop: '4px' }}>
                Reload to see upcoming meets, practices, and deadlines.
              </p>
            </div>
          ) : shouldShowCollectionEmptyState(loading, loadError, sections.length) && (
            <div className="card"
              style={{
                padding: '60px 20px',
                textAlign: 'center',
              }}
            >
              <Icon name="calendar" size={32} style={{ color: C.muted, marginBottom: '16px' }} />
              <p style={{ fontSize: '15px', color: C.subtle, fontWeight: 500 }}>No upcoming events</p>
              <p style={{ fontSize: '13px', color: C.muted, marginTop: '4px' }}>
                {typeFilter !== 'all' ? 'No events match this filter.' : 'No meets or deadlines scheduled yet.'}
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
