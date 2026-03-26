'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchMeets,
  fetchPractices,
  type MeetRecord,
  type PracticeRecord,
} from '@/lib/adminData'

// ── Design tokens ──────────────────────────────────────────────────────────
const C = {
  accent:       '#00D4AA',
  accentDim:    'rgba(0,212,170,0.10)',
  accentBorder: 'rgba(0,212,170,0.22)',
  text:         '#E4F2FF',
  subtle:       '#7FA8C9',
  muted:        '#4A6B82',
  card:         '#0D2235',
  border:       'rgba(0,212,170,0.10)',
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
  meet:     { dot: '#00D4AA', bg: 'rgba(0,212,170,0.06)',  border: 'rgba(0,212,170,0.20)',   color: '#00D4AA', label: 'Meet',      leftBorder: '#00D4AA' },
  deadline: { dot: '#ef4444', bg: 'rgba(239,68,68,0.06)',  border: 'rgba(239,68,68,0.18)',  color: '#fca5a5', label: 'Deadline',  leftBorder: '#ef4444' },
  practice: { dot: '#7FA8C9', bg: 'rgba(127,168,201,0.06)', border: 'rgba(127,168,201,0.18)', color: '#7FA8C9', label: 'Practice', leftBorder: '#7FA8C9' },
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
    const meetDate = new Date(m.start_date)
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
        ? `Meet runs ${new Date(m.start_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })} – ${new Date(m.end_date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}.`
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
    <div style={{ width, height, background: 'rgba(0,212,170,0.08)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite' }} />
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
              <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', fontWeight: 600, color: '#0D2235', background: C.accent, padding: '2px 6px', borderRadius: '4px', letterSpacing: '0.04em' }}>
                {ev.course}
              </span>
            )}
            <span
              style={{
                fontSize: '10px',
                fontFamily: 'var(--font-mono)',
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

  useEffect(() => {
    if (!orgId) { setLoading(false); return }
    const now = new Date()
    Promise.all([
      fetchMeets(api, orgId),
      fetchPractices(api, orgId).catch(() => [] as PracticeRecord[]),
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
      .catch(console.error)
      .finally(() => setLoading(false))
  }, [api, orgId])

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
              <div style={{ height: '18px', width: '140px', background: 'rgba(0,212,170,0.08)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite' }} />
              <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '12px', padding: '18px', display: 'flex', flexDirection: 'column', gap: '10px' }}>
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
                <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.muted, whiteSpace: 'nowrap' }}>
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

          {sections.length === 0 && (
            <div
              style={{
                background: C.card,
                border: `1px solid ${C.border}`,
                borderRadius: '14px',
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
