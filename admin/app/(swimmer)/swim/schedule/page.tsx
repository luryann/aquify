'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'

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

// ── Types & data ────────────────────────────────────────────────────────────
type EventType = 'practice' | 'meet' | 'optional' | 'deadline'

type ScheduleEvent = {
  id: string
  title: string
  type: EventType
  date: string
  dayLabel: string
  time?: string
  duration?: string
  location?: string
  detail?: string
  myEvents?: string[]
  meetPath?: string
}

const SCHEDULE: ScheduleEvent[] = [
  // This week (Mar 17 week)
  {
    id: 'ev-001',
    title: 'Morning Practice — Senior A',
    type: 'practice',
    date: 'Mon, Mar 17',
    dayLabel: 'Today',
    time: '5:30 AM',
    duration: '90 min',
    location: 'Main Pool, Lane 7–8',
    detail: 'Focus: race pace 200s and turns work.',
  },
  {
    id: 'ev-002',
    title: 'Morning Practice — Senior A',
    type: 'practice',
    date: 'Tue, Mar 18',
    dayLabel: 'Tomorrow',
    time: '5:30 AM',
    duration: '90 min',
    location: 'Main Pool, Lane 7–8',
  },
  {
    id: 'ev-003',
    title: 'Commitment Deadline — Tri-City Classic',
    type: 'deadline',
    date: 'Thu, Mar 19',
    dayLabel: '',
    detail: 'Your parent must respond by 11:59 PM tonight.',
  },
  {
    id: 'ev-004',
    title: 'Morning Practice — Senior A',
    type: 'practice',
    date: 'Fri, Mar 21',
    dayLabel: '',
    time: '5:30 AM',
    duration: '90 min',
    location: 'Main Pool, Lane 7–8',
    detail: 'Pre-meet taper workout.',
  },
  // Meet weekend
  {
    id: 'ev-005',
    title: 'Tri-City Classic — Day 1',
    type: 'meet',
    date: 'Sat, Mar 22',
    dayLabel: '',
    time: '7:00 AM warm-up',
    location: 'Riverside Aquatic Center',
    detail: 'Bring team cap, two suits, water bottle. Arrive 15 min before warm-up.',
    myEvents: ['200 Free (Heat 3)', '100 Back (Heat 5)'],
    meetPath: '/swim/meets/meet-001',
  },
  {
    id: 'ev-006',
    title: 'Tri-City Classic — Day 2',
    type: 'meet',
    date: 'Sun, Mar 23',
    dayLabel: '',
    time: '7:30 AM warm-up',
    location: 'Riverside Aquatic Center',
    myEvents: ['400 IM (Heat 2)'],
    meetPath: '/swim/meets/meet-001',
  },
  // Post meet week
  {
    id: 'ev-007',
    title: 'Morning Practice — Senior A',
    type: 'practice',
    date: 'Mon, Mar 24',
    dayLabel: '',
    time: '5:30 AM',
    duration: '90 min',
    location: 'Main Pool, Lane 7–8',
    detail: 'Post-meet recovery session.',
  },
  {
    id: 'ev-008',
    title: 'Morning Practice — Senior A',
    type: 'practice',
    date: 'Tue, Mar 25',
    dayLabel: '',
    time: '5:30 AM',
    duration: '90 min',
    location: 'Main Pool, Lane 7–8',
  },
  {
    id: 'ev-009',
    title: 'Season Registration Deadline',
    type: 'deadline',
    date: 'Wed, Mar 25',
    dayLabel: '',
    detail: 'Insurance card copy required for Emma. Notify your parent.',
  },
  // Spring break
  {
    id: 'ev-010',
    title: 'Open Swim — Optional',
    type: 'optional',
    date: 'Sat, Apr 4',
    dayLabel: '',
    time: '10:00 AM',
    duration: '2 hrs',
    location: 'Main Pool',
    detail: 'Optional open swim during spring break.',
  },
  // April
  {
    id: 'ev-011',
    title: 'Practice Resumes',
    type: 'practice',
    date: 'Mon, Apr 6',
    dayLabel: '',
    time: '5:30 AM',
    duration: '90 min',
    location: 'Main Pool, Lane 7–8',
    detail: 'Regular schedule resumes after spring break.',
  },
  {
    id: 'ev-012',
    title: 'Regional Spring Invite',
    type: 'meet',
    date: 'Sun, Apr 5',
    dayLabel: '',
    time: '8:00 AM warm-up',
    location: 'University Pool',
    detail: 'Heat sheets released April 3rd. Commitment window still open.',
    meetPath: '/swim/meets/meet-002',
  },
  {
    id: 'ev-013',
    title: 'Morning Practice — Senior A',
    type: 'practice',
    date: 'Tue, Apr 7',
    dayLabel: '',
    time: '5:30 AM',
    duration: '90 min',
    location: 'Main Pool, Lane 7–8',
  },
  {
    id: 'ev-014',
    title: 'Morning Practice — Senior A',
    type: 'practice',
    date: 'Thu, Apr 9',
    dayLabel: '',
    time: '5:30 AM',
    duration: '90 min',
    location: 'Main Pool, Lane 7–8',
  },
  {
    id: 'ev-015',
    title: 'Season Registration Deadline',
    type: 'deadline',
    date: 'Wed, Apr 15',
    dayLabel: '',
    detail: 'Complete 2026–2027 season registration and all waivers.',
  },
]

const EVENT_STYLE: Record<EventType, { dot: string; bg: string; border: string; color: string; label: string; leftBorder: string }> = {
  practice: { dot: '#3b82f6', bg: 'rgba(59,130,246,0.06)',  border: 'rgba(59,130,246,0.15)',  color: '#93c5fd', label: 'Practice',  leftBorder: '#3b82f6' },
  meet:     { dot: '#00D4AA', bg: 'rgba(0,212,170,0.06)',  border: 'rgba(0,212,170,0.20)',   color: '#00D4AA', label: 'Meet',      leftBorder: '#00D4AA' },
  optional: { dot: '#8b5cf6', bg: 'rgba(139,92,246,0.06)', border: 'rgba(139,92,246,0.18)', color: '#c4b5fd', label: 'Optional',  leftBorder: '#8b5cf6' },
  deadline: { dot: '#ef4444', bg: 'rgba(239,68,68,0.06)',  border: 'rgba(239,68,68,0.18)',  color: '#fca5a5', label: 'Deadline',  leftBorder: '#ef4444' },
}

function parseScheduleDate(dateLabel: string) {
  return new Date(`${dateLabel}, 2026 12:00:00`)
}

function formatSectionHeading(date: Date) {
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
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
      {/* Content */}
      <div style={{ flex: 1, minWidth: 0 }}>
        {/* Title row */}
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '10px' }}>
          <p style={{ fontSize: '14px', fontWeight: 600, color: C.text, lineHeight: 1.3, marginBottom: '6px' }}>
            {ev.title}
          </p>
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
              flexShrink: 0,
            }}
          >
            {s.label}
          </span>
        </div>

        {/* Meta row */}
        <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
          {ev.time && (
            <span style={{ fontSize: '12px', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
              <Icon name="clock" size={11} />
              {ev.time}
              {ev.duration && ` · ${ev.duration}`}
            </span>
          )}
          {ev.location && (
            <span style={{ fontSize: '12px', color: C.subtle, display: 'flex', alignItems: 'center', gap: '4px' }}>
              <Icon name="map-pin" size={11} />
              {ev.location}
            </span>
          )}
        </div>

        {/* Expandable content */}
        {(ev.detail || (ev.myEvents && ev.myEvents.length > 0)) && (
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
                {ev.myEvents && ev.myEvents.length > 0 && (
                  <div>
                    <p style={{ fontSize: '10.5px', fontFamily: 'var(--font-mono)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '6px' }}>
                      My Events
                    </p>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
                      {ev.myEvents.map((e) => (
                        <span
                          key={e}
                          style={{
                            fontSize: '12px',
                            color: C.accent,
                            background: C.accentDim,
                            border: `1px solid ${C.accentBorder}`,
                            padding: '3px 10px',
                            borderRadius: '6px',
                            fontFamily: 'var(--font-mono)',
                          }}
                        >
                          {e}
                        </span>
                      ))}
                    </div>
                  </div>
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

// ── Entries data ────────────────────────────────────────────────────────────
type Entry = {
  id: string; meet: string; date: string; event: string
  seedTime: string; status: 'submitted' | 'approved' | 'exported' | 'draft'
  heat?: number; lane?: number
}

const MY_ENTRIES: Entry[] = [
  { id: 'e1', meet: 'Tri-City Classic',        date: 'Mar 22–23, 2026', event: '200 Free (F Open)',  seedTime: '1:52.34', status: 'approved',  heat: 3, lane: 4 },
  { id: 'e2', meet: 'Tri-City Classic',        date: 'Mar 22–23, 2026', event: '100 Back (F Open)',  seedTime: '59.87',   status: 'approved',  heat: 5, lane: 6 },
  { id: 'e3', meet: 'Tri-City Classic',        date: 'Mar 22–23, 2026', event: '400 IM (F Open)',    seedTime: '4:42.11', status: 'approved',  heat: 2, lane: 3 },
  { id: 'e4', meet: 'Regional Spring Invite',  date: 'Apr 5, 2026',     event: '100 Back (F Open)',  seedTime: '59.87',   status: 'submitted' },
  { id: 'e5', meet: 'Regional Spring Invite',  date: 'Apr 5, 2026',     event: '50 Free (F Open)',   seedTime: '25.34',   status: 'submitted' },
]

const ENTRY_STATUS: Record<Entry['status'], { label: string; color: string; bg: string }> = {
  submitted: { label: 'Submitted',  color: '#60a5fa', bg: 'rgba(96,165,250,0.12)' },
  approved:  { label: 'Approved',   color: '#00D4AA', bg: 'rgba(0,212,170,0.10)' },
  exported:  { label: 'In Heat Sheet', color: '#a78bfa', bg: 'rgba(167,139,250,0.10)' },
  draft:     { label: 'Draft',      color: '#4A6B82', bg: 'rgba(74,107,130,0.10)' },
}

function EntriesView() {
  const meets = [...new Set(MY_ENTRIES.map((e) => e.meet))]
  return (
    <div style={{ animation: 'slideUp 0.3s cubic-bezier(0.16,1,0.3,1) both' }}>
      <p style={{ fontSize: '13px', color: C.subtle, marginBottom: '20px', lineHeight: 1.6 }}>
        Your submitted event entries for upcoming meets. Approved entries will appear in the official heat sheet.
      </p>
      {meets.map((meet) => {
        const entries = MY_ENTRIES.filter((e) => e.meet === meet)
        const meetDate = entries[0]?.date ?? ''
        return (
          <div key={meet} style={{ marginBottom: '24px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '12px' }}>
              <div>
                <div style={{ fontFamily: 'var(--font-display)', fontSize: '16px', fontWeight: 700, color: C.text, marginBottom: '2px' }}>{meet}</div>
                <div style={{ fontSize: '11px', color: C.muted, fontFamily: 'var(--font-mono)' }}>{meetDate}</div>
              </div>
              <div style={{ flex: 1, height: '1px', background: C.border }} />
              <span style={{ fontSize: '11px', color: C.muted, fontFamily: 'var(--font-mono)' }}>{entries.length} {entries.length === 1 ? 'entry' : 'entries'}</span>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {entries.map((e) => {
                const sc = ENTRY_STATUS[e.status]
                return (
                  <div key={e.id} style={{ background: C.card, border: `1px solid ${C.border}`, borderLeft: `3px solid ${sc.color}`, borderRadius: '0 10px 10px 0', padding: '14px 16px', display: 'flex', alignItems: 'center', gap: '16px' }}>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontWeight: 600, fontSize: '14px', color: C.text, marginBottom: '4px' }}>{e.event}</div>
                      <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' }}>
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '14px', fontWeight: 700, color: C.accent }}>{e.seedTime}</span>
                        {e.heat && <span style={{ fontSize: '11px', color: C.muted, fontFamily: 'var(--font-mono)' }}>Heat {e.heat} · Lane {e.lane}</span>}
                      </div>
                    </div>
                    <span style={{ fontSize: '11px', fontWeight: 600, padding: '3px 10px', borderRadius: '20px', background: sc.bg, color: sc.color, fontFamily: 'var(--font-mono)', flexShrink: 0 }}>{sc.label}</span>
                  </div>
                )
              })}
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function SwimmerSchedulePage() {
  const [typeFilter, setTypeFilter] = useState<EventType | 'all'>('all')
  const [view, setView] = useState<'schedule' | 'entries'>('schedule')

  const filtered = SCHEDULE.filter((ev) => typeFilter === 'all' || ev.type === typeFilter)

  const sections = (() => {
    const sorted = [...filtered].sort(
      (a, b) => parseScheduleDate(a.date).getTime() - parseScheduleDate(b.date).getTime(),
    )
    const groups: Array<{ heading: string; events: ScheduleEvent[] }> = []

    sorted.forEach((event) => {
      const eventDate = parseScheduleDate(event.date)
      const lastGroup = groups[groups.length - 1]
      const firstDate = lastGroup ? parseScheduleDate(lastGroup.events[0].date) : null
      const dayDelta = firstDate ? Math.floor((eventDate.getTime() - firstDate.getTime()) / 86400000) : 0

      if (!lastGroup || dayDelta > 6) {
        groups.push({
          heading: `${formatSectionHeading(eventDate)} Week`,
          events: [event],
        })
        return
      }

      lastGroup.events.push(event)
      const lastDate = parseScheduleDate(lastGroup.events[lastGroup.events.length - 1].date)
      lastGroup.heading = `${formatSectionHeading(firstDate!)} – ${formatSectionHeading(lastDate)}`
    })

    return groups
  })()

  return (
    <div>
      {/* Header */}
      <div
        style={{
          marginBottom: '28px',
          animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) both',
        }}
      >
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
          Practices, meets, and upcoming deadlines for Senior A.
        </p>
      </div>

      {/* Tab switcher */}
      <div style={{ display: 'flex', gap: '4px', background: 'rgba(0,212,170,0.05)', borderRadius: '10px', padding: '3px', marginBottom: '24px', width: 'fit-content', border: `1px solid ${C.border}` }}>
        {[{ id: 'schedule', label: 'Schedule' }, { id: 'entries', label: 'My Entries', count: MY_ENTRIES.length }].map((t) => (
          <button
            key={t.id}
            onClick={() => setView(t.id as 'schedule' | 'entries')}
            style={{
              padding: '6px 16px', borderRadius: '8px', border: 'none', cursor: 'pointer',
              background: view === t.id ? C.accent : 'transparent',
              color: view === t.id ? '#061623' : C.subtle,
              fontWeight: view === t.id ? 700 : 400,
              fontSize: '13px', transition: 'all 150ms ease',
              display: 'flex', alignItems: 'center', gap: '6px',
            }}
          >
            {t.label}
            {t.count !== undefined && (
              <span style={{ fontSize: '10px', padding: '1px 6px', borderRadius: '8px', background: view === t.id ? 'rgba(6,22,35,0.25)' : C.accentDim, color: view === t.id ? '#061623' : C.accent, fontFamily: 'var(--font-mono)', fontWeight: 700 }}>{t.count}</span>
            )}
          </button>
        ))}
      </div>

      {view === 'entries' && <EntriesView />}

      {view === 'schedule' && <>
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
        {(['all', 'practice', 'meet', 'optional', 'deadline'] as const).map((type) => {
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
                <span
                  style={{
                    width: '7px',
                    height: '7px',
                    borderRadius: '50%',
                    background: s.dot,
                    flexShrink: 0,
                  }}
                />
              )}
              <span style={{ textTransform: 'capitalize' }}>
                {type === 'all' ? 'All' : s!.label}
              </span>
            </button>
          )
        })}
      </div>

      {/* Schedule sections */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '36px' }}>
        {sections.map((section, si) => (
          <section
            key={section.heading}
            style={{
              animation: `slideUp 0.35s cubic-bezier(0.16,1,0.3,1) ${si * 60}ms both`,
            }}
          >
            {/* Section heading */}
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '12px',
                marginBottom: '14px',
              }}
            >
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
              <span
                style={{
                  fontSize: '11px',
                  fontFamily: 'var(--font-mono)',
                  color: C.muted,
                  whiteSpace: 'nowrap',
                }}
              >
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
                    animationDelay: `${(si * 60 + i * 40)}ms`,
                    animation: 'slideUp 0.3s cubic-bezier(0.16,1,0.3,1) both',
                  }}
                >
                  {/* Date column */}
                  <div style={{ paddingTop: '14px' }}>
                    <p style={{ fontSize: '13px', fontWeight: 600, color: C.text, lineHeight: 1.3 }}>
                      {ev.date.split(', ')[0]}
                    </p>
                    <p style={{ fontSize: '12px', color: C.subtle }}>
                      {ev.date.split(', ')[1]}
                    </p>
                    {ev.dayLabel && (
                      <p
                        style={{
                          fontSize: '10px',
                          fontFamily: 'var(--font-mono)',
                          color: C.accent,
                          fontWeight: 600,
                          textTransform: 'uppercase',
                          letterSpacing: '0.05em',
                          marginTop: '3px',
                        }}
                      >
                        {ev.dayLabel}
                      </p>
                    )}
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
            <p style={{ fontSize: '15px', color: C.subtle, fontWeight: 500 }}>No events</p>
            <p style={{ fontSize: '13px', color: C.muted, marginTop: '4px' }}>
              No events match this filter.
            </p>
          </div>
        )}
      </div>
      </>}
    </div>
  )
}
