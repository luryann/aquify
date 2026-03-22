'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchMeets, type MeetRecord } from '@/lib/adminData'

// ── Types ───────────────────────────────────────────────────────────────────
type EventType = 'meet' | 'practice' | 'deadline' | 'holiday' | 'event'

type CalEvent = {
  id: string
  title: string
  type: EventType
  date: string          // YYYY-MM-DD
  time?: string
  location?: string
  detail?: string
  linkPath?: string
}

// ── Helpers ─────────────────────────────────────────────────────────────────
function dateOnly(iso?: string): string {
  if (!iso) return ''
  return iso.split('T')[0]
}

function meetsToCalEvents(meets: MeetRecord[]): CalEvent[] {
  const events: CalEvent[] = []
  for (const meet of meets) {
    if (meet.start_date) {
      events.push({
        id: `meet-${meet.id}`,
        title: meet.name,
        type: 'meet',
        date: dateOnly(meet.start_date),
        location: meet.location ?? undefined,
        detail: meet.host_org ? `Hosted by ${meet.host_org}` : undefined,
        linkPath: `/portal/meets/${meet.id}`,
      })
      if (meet.end_date && dateOnly(meet.end_date) !== dateOnly(meet.start_date)) {
        events.push({
          id: `meet-${meet.id}-end`,
          title: `${meet.name} — Day 2`,
          type: 'meet',
          date: dateOnly(meet.end_date),
          location: meet.location ?? undefined,
          linkPath: `/portal/meets/${meet.id}`,
        })
      }
    }
    if (meet.commitment_deadline && meet.status === 'commitment_open') {
      events.push({
        id: `deadline-${meet.id}`,
        title: `Commitment Deadline — ${meet.name}`,
        type: 'deadline',
        date: dateOnly(meet.commitment_deadline),
        detail: 'Respond yes/no for each swimmer by tonight.',
        linkPath: `/portal/meets/${meet.id}`,
      })
    }
  }
  return events
}

// ── Legacy stub (empty — replaced by live API data below) ─────────────────
const EVENTS_STUB: CalEvent[] = [
  // March
  { id: 'e-001', type: 'practice', title: 'Morning Practice — Senior A', date: '2026-03-17', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-002', type: 'practice', title: 'Afternoon Practice — Junior B', date: '2026-03-17', time: '4:00 PM', location: 'Main Pool' },
  { id: 'e-003', type: 'deadline', title: 'Commitment Deadline — Tri-City Classic', date: '2026-03-19', detail: 'Respond yes/no for each swimmer by tonight.', linkPath: '/portal/meets/meet-001' },
  { id: 'e-004', type: 'practice', title: 'Morning Practice — Senior A', date: '2026-03-18', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-005', type: 'practice', title: 'Morning Practice', date: '2026-03-19', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-006', type: 'practice', title: 'Morning Practice', date: '2026-03-20', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-007', type: 'meet', title: 'Tri-City Classic', date: '2026-03-22', time: '7:00 AM', location: 'Riverside Aquatic Center', detail: 'Warm-up begins 7 AM. Bring team cap + two suits.', linkPath: '/portal/meets/meet-001' },
  { id: 'e-008', type: 'meet', title: 'Tri-City Classic — Day 2', date: '2026-03-23', time: '7:30 AM', location: 'Riverside Aquatic Center', linkPath: '/portal/meets/meet-001' },
  { id: 'e-009', type: 'practice', title: 'Morning Practice', date: '2026-03-24', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-010', type: 'practice', title: 'Morning Practice', date: '2026-03-25', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-011', type: 'deadline', title: 'Document Due — Insurance Card (Emma)', date: '2026-03-25', detail: 'Insurance card copy required.', linkPath: '/portal/documents' },
  { id: 'e-012', type: 'holiday', title: 'Spring Break — Pool Closed', date: '2026-03-28', detail: 'Facility closed Mar 28 – Apr 5.' },
  { id: 'e-013', type: 'holiday', title: 'Spring Break — Pool Closed', date: '2026-03-29', detail: 'Facility closed.' },
  { id: 'e-014', type: 'holiday', title: 'Spring Break — Pool Closed', date: '2026-03-30', detail: 'Facility closed.' },
  { id: 'e-015', type: 'holiday', title: 'Spring Break — Pool Closed', date: '2026-03-31', detail: 'Facility closed.' },
  // April
  { id: 'e-016', type: 'holiday', title: 'Spring Break — Pool Closed', date: '2026-04-01', detail: 'Facility closed.' },
  { id: 'e-017', type: 'holiday', title: 'Spring Break — Pool Closed', date: '2026-04-02', detail: 'Facility closed.' },
  { id: 'e-018', type: 'holiday', title: 'Spring Break — Pool Closed', date: '2026-04-03', detail: 'Facility closed.' },
  { id: 'e-019', type: 'event', title: 'Open Swim — Optional', date: '2026-04-04', time: '10:00 AM', location: 'Main Pool', detail: 'Optional open swim during spring break.' },
  { id: 'e-020', type: 'holiday', title: 'Spring Break — Pool Closed', date: '2026-04-05', detail: 'Facility closed.' },
  { id: 'e-021', type: 'practice', title: 'Practice Resumes', date: '2026-04-06', time: '5:30 AM', location: 'Main Pool', detail: 'Regular schedule resumes.' },
  { id: 'e-022', type: 'meet', title: 'Regional Spring Invite', date: '2026-04-05', time: '8:00 AM', location: 'University Pool', detail: 'All-day meet. Heat sheets released April 3.', linkPath: '/portal/meets/meet-002' },
  { id: 'e-023', type: 'deadline', title: 'Commitment Deadline — Spring Invite', date: '2026-03-29', detail: 'Respond to Spring Invite commitment request.', linkPath: '/portal/meets/meet-002' },
  { id: 'e-024', type: 'practice', title: 'Morning Practice', date: '2026-04-07', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-025', type: 'practice', title: 'Morning Practice', date: '2026-04-08', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-026', type: 'practice', title: 'Morning Practice', date: '2026-04-09', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-027', type: 'practice', title: 'Morning Practice', date: '2026-04-13', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-028', type: 'practice', title: 'Morning Practice', date: '2026-04-14', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-029', type: 'practice', title: 'Morning Practice', date: '2026-04-15', time: '5:30 AM', location: 'Main Pool' },
  { id: 'e-030', type: 'deadline', title: 'Season Registration Deadline', date: '2026-04-15', detail: 'Complete 2026–2027 season registration and waivers.', linkPath: '/portal/registrations' },
]

// ── Config ─────────────────────────────────────────────────────────────────
const EVENT_STYLE: Record<EventType, { dot: string; bg: string; border: string; color: string; label: string }> = {
  meet:     { dot: 'var(--accent)', bg: 'var(--accent-subtle)', border: 'var(--accent-border)', color: 'var(--accent)', label: 'Meet' },
  practice: { dot: 'var(--info)', bg: 'var(--info-bg)', border: 'var(--info-border)', color: 'var(--info)', label: 'Practice' },
  deadline: { dot: 'var(--error)', bg: 'var(--error-bg)', border: 'var(--error-border)', color: 'var(--error)', label: 'Deadline' },
  holiday:  { dot: 'var(--text-muted)', bg: 'var(--bg-surface)', border: 'var(--border)', color: 'var(--text-muted)', label: 'Holiday' },
  event:    { dot: 'var(--warning)', bg: 'var(--warning-bg)', border: 'var(--warning-border)', color: 'var(--warning)', label: 'Event' },
}

// ── Calendar helpers ────────────────────────────────────────────────────────
function getDaysInMonth(year: number, month: number) {
  return new Date(year, month + 1, 0).getDate()
}

function getFirstDayOfWeek(year: number, month: number) {
  return new Date(year, month, 1).getDay() // 0=Sun
}

function toDateStr(year: number, month: number, day: number) {
  const m = String(month + 1).padStart(2, '0')
  const d = String(day).padStart(2, '0')
  return `${year}-${m}-${d}`
}

const MONTH_NAMES = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
]
const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

// ── Sub-components ──────────────────────────────────────────────────────────
function EventChip({ ev }: { ev: CalEvent }) {
  const s = EVENT_STYLE[ev.type]
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '5px',
        padding: '3px 8px',
        borderRadius: '5px',
        background: s.bg,
        border: `1px solid ${s.border}`,
        fontSize: '11px',
        fontWeight: 500,
        color: s.color,
        overflow: 'hidden',
        whiteSpace: 'nowrap',
        textOverflow: 'ellipsis',
        cursor: 'default',
      }}
    >
      <span
        style={{
          width: '6px',
          height: '6px',
          borderRadius: '50%',
          background: s.dot,
          flexShrink: 0,
        }}
      />
      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
        {ev.title}
      </span>
    </div>
  )
}

function UpcomingEventRow({ ev }: { ev: CalEvent }) {
  const s = EVENT_STYLE[ev.type]
  const content = (
    <div
      style={{
        display: 'flex',
        gap: '12px',
        alignItems: 'flex-start',
        padding: '12px 16px',
        borderBottom: '1px solid rgba(0,0,0,0.06)',
        transition: 'background 120ms ease',
      }}
      onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.02)')}
      onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
    >
      {/* Type dot */}
      <div
        style={{
          width: '10px',
          height: '10px',
          borderRadius: '50%',
          background: s.dot,
          flexShrink: 0,
          marginTop: '4px',
        }}
      />
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ fontSize: '13px', fontWeight: 600, color: '#1A1714', marginBottom: '2px', lineHeight: 1.3 }}>
          {ev.title}
        </p>
        <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
          {ev.time && (
            <span style={{ fontSize: '11.5px', color: '#6B6866', display: 'flex', alignItems: 'center', gap: '3px' }}>
              <Icon name="clock" size={11} />
              {ev.time}
            </span>
          )}
          {ev.location && (
            <span style={{ fontSize: '11.5px', color: '#6B6866', display: 'flex', alignItems: 'center', gap: '3px' }}>
              <Icon name="map-pin" size={11} />
              {ev.location}
            </span>
          )}
        </div>
        {ev.detail && (
          <p style={{ fontSize: '11.5px', color: '#9A9693', marginTop: '4px', lineHeight: 1.4 }}>
            {ev.detail}
          </p>
        )}
      </div>
      <span
        style={{
          fontSize: '10px',
          fontFamily: 'var(--font-mono)',
          fontWeight: 600,
          textTransform: 'uppercase',
          letterSpacing: '0.04em',
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
  )

  if (ev.linkPath) {
    return <Link href={ev.linkPath} style={{ textDecoration: 'none', display: 'block' }}>{content}</Link>
  }
  return content
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function CalendarPage() {
  const today = new Date()
  const [viewYear, setViewYear] = useState(today.getFullYear())
  const [viewMonth, setViewMonth] = useState(today.getMonth())
  const [selectedDate, setSelectedDate] = useState<string | null>(null)
  const [view, setView] = useState<'month' | 'week'>('month')
  const [typeFilter, setTypeFilter] = useState<EventType | 'all'>('all')

  // Responsive: track viewport width to reflow sidebar below calendar on mobile.
  // Start with desktop-default (1024) to avoid SSR/hydration mismatch; update
  // after mount. A brief layout switch on first paint is acceptable and expected.
  const [windowWidth, setWindowWidth] = useState<number>(1024)
  useEffect(() => {
    setWindowWidth(window.innerWidth)
    const handleResize = () => setWindowWidth(window.innerWidth)
    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])
  const isMobile = windowWidth < 720

  const daysInMonth = getDaysInMonth(viewYear, viewMonth)
  const firstDow = getFirstDayOfWeek(viewYear, viewMonth)

  // Events indexed by date string
  const eventsByDate = EVENTS.reduce<Record<string, CalEvent[]>>((acc, ev) => {
    if (!acc[ev.date]) acc[ev.date] = []
    acc[ev.date].push(ev)
    return acc
  }, {})

  const goToPrev = () => {
    if (viewMonth === 0) { setViewYear(y => y - 1); setViewMonth(11) }
    else setViewMonth(m => m - 1)
  }
  const goToNext = () => {
    if (viewMonth === 11) { setViewYear(y => y + 1); setViewMonth(0) }
    else setViewMonth(m => m + 1)
  }
  const goToToday = () => {
    setViewYear(today.getFullYear())
    setViewMonth(today.getMonth())
    setSelectedDate(toDateStr(today.getFullYear(), today.getMonth(), today.getDate()))
  }

  const todayStr = toDateStr(today.getFullYear(), today.getMonth(), today.getDate())

  // Upcoming events from today
  const upcomingEvents = EVENTS
    .filter((ev) => ev.date >= todayStr)
    .filter((ev) => typeFilter === 'all' || ev.type === typeFilter)
    .sort((a, b) => a.date.localeCompare(b.date))
    .slice(0, 15)

  // Selected date events
  const selectedEvents = selectedDate
    ? (eventsByDate[selectedDate] ?? []).filter((ev) => typeFilter === 'all' || ev.type === typeFilter)
    : []

  // Week view helpers — derive weekStart from selectedDate if available,
  // otherwise anchor to the first week of the currently viewed month.
  // This ensures month navigation + week view stay in sync.
  const weekStart = (() => {
    if (selectedDate) {
      const d = new Date(selectedDate + 'T12:00:00')
      d.setDate(d.getDate() - d.getDay())
      return d
    }
    // Default: Sunday of the week containing the 1st of viewMonth
    const d = new Date(viewYear, viewMonth, 1)
    d.setDate(d.getDate() - d.getDay())
    return d
  })()

  const weekDays = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(weekStart)
    d.setDate(weekStart.getDate() + i)
    return {
      date: toDateStr(d.getFullYear(), d.getMonth(), d.getDate()),
      label: DAY_LABELS[d.getDay()],
      num: d.getDate(),
      isToday: toDateStr(d.getFullYear(), d.getMonth(), d.getDate()) === todayStr,
    }
  })

  return (
    <div>
      {/* Page header */}
      <div
        style={{
          display: 'flex',
          alignItems: 'flex-end',
          justifyContent: 'space-between',
          marginBottom: '28px',
          flexWrap: 'wrap',
          gap: '12px',
          animation: 'slideUp 0.4s var(--ease-out) both',
        }}
      >
        <div>
          <h1
            style={{
              fontFamily: 'var(--font-display)',
              fontSize: 'clamp(26px, 4vw, 34px)',
              color: '#1A1714',
              letterSpacing: '-0.02em',
              lineHeight: 1.1,
              marginBottom: '6px',
            }}
          >
            Calendar
          </h1>
          <p style={{ fontSize: '14px', color: '#6B6866' }}>
            Meets, practices, and important deadlines.
          </p>
        </div>

        {/* View toggle */}
        <div
          style={{
            display: 'flex',
            gap: '4px',
            background: 'rgba(0,0,0,0.05)',
            borderRadius: '8px',
            padding: '3px',
          }}
        >
          {(['month', 'week'] as const).map((v) => (
            <button
              key={v}
              onClick={() => setView(v)}
              style={{
                padding: '5px 14px',
                borderRadius: '6px',
                border: 'none',
                background: view === v ? '#ffffff' : 'transparent',
                color: view === v ? '#1A1714' : '#6B6866',
                fontSize: '12.5px',
                fontWeight: view === v ? 600 : 400,
                cursor: 'pointer',
                boxShadow: view === v ? '0 1px 3px rgba(0,0,0,0.08)' : 'none',
                transition: 'all 120ms ease',
                textTransform: 'capitalize',
              }}
            >
              {v}
            </button>
          ))}
        </div>
      </div>

      {/* Legend + type filter */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          marginBottom: '20px',
          flexWrap: 'wrap',
          animation: 'slideUp 0.4s var(--ease-out) 0.05s both',
        }}
      >
        {(['all', 'meet', 'practice', 'deadline', 'event', 'holiday'] as const).map((type) => {
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
                padding: '5px 12px',
                borderRadius: '999px',
                border: `1px solid ${active ? (s ? s.border : 'rgba(0,0,0,0.22)') : 'rgba(0,0,0,0.10)'}`,
                background: active ? (s ? s.bg : 'rgba(0,0,0,0.06)') : 'transparent',
                color: active ? (s ? s.color : '#1A1714') : '#6B6866',
                fontSize: '12px',
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
                {type === 'all' ? 'All types' : EVENT_STYLE[type as EventType].label}
              </span>
            </button>
          )
        })}
      </div>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: isMobile ? '1fr' : 'minmax(0, 1fr) minmax(240px, 280px)',
          gap: '20px',
          animation: 'slideUp 0.4s var(--ease-out) 0.1s both',
          alignItems: 'start',
        }}
      >
        {/* ── MAIN CALENDAR ─────────────────────────────────────────── */}
        <div
          style={{
            background: '#ffffff',
            borderRadius: '16px',
            border: '1px solid rgba(0,0,0,0.08)',
            overflowX: 'auto',
            overflowY: 'hidden',
            boxShadow: '0 2px 8px rgba(0,0,0,0.05)',
            minWidth: 0,
          }}
        >
          {/* Month nav */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '18px 20px 16px',
              borderBottom: '1px solid rgba(0,0,0,0.07)',
            }}
          >
            <button
              onClick={goToPrev}
              aria-label="Previous month"
              style={{
                width: '32px',
                height: '32px',
                borderRadius: '8px',
                border: '1px solid rgba(0,0,0,0.10)',
                background: 'transparent',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#6B6866',
                transition: 'background 120ms ease',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.05)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              <Icon name="chevron-left" size={15} />
            </button>

            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <h2
                style={{
                  fontFamily: 'var(--font-display)',
                  fontSize: '20px',
                  color: '#1A1714',
                  letterSpacing: '-0.01em',
                }}
              >
                {MONTH_NAMES[viewMonth]} {viewYear}
              </h2>
              <button
                onClick={goToToday}
                style={{
                  padding: '4px 12px',
                  borderRadius: '6px',
                  border: '1px solid rgba(0,0,0,0.12)',
                  background: 'transparent',
                  fontSize: '12px',
                  fontWeight: 500,
                  color: '#6B6866',
                  cursor: 'pointer',
                  transition: 'all 120ms ease',
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = 'rgba(0,0,0,0.05)'
                  e.currentTarget.style.color = '#1A1714'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background = 'transparent'
                  e.currentTarget.style.color = '#6B6866'
                }}
              >
                Today
              </button>
            </div>

            <button
              onClick={goToNext}
              aria-label="Next month"
              style={{
                width: '32px',
                height: '32px',
                borderRadius: '8px',
                border: '1px solid rgba(0,0,0,0.10)',
                background: 'transparent',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#6B6866',
                transition: 'background 120ms ease',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.05)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              <Icon name="chevron-right" size={15} />
            </button>
          </div>

          {view === 'month' ? (
            <>
              {/* Day-of-week headers */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, minmax(110px, 1fr))',
                  borderBottom: '1px solid rgba(0,0,0,0.06)',
                  minWidth: isMobile ? '100%' : '770px',
                }}
              >
                {DAY_LABELS.map((d) => (
                  <div
                    key={d}
                    style={{
                      padding: '8px 4px',
                      textAlign: 'center',
                      fontSize: '11px',
                      fontFamily: 'var(--font-mono)',
                      fontWeight: 600,
                      letterSpacing: '0.05em',
                      textTransform: 'uppercase',
                      color: '#A09D9B',
                    }}
                  >
                    {d}
                  </div>
                ))}
              </div>

              {/* Day grid */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, minmax(110px, 1fr))',
                  minWidth: isMobile ? '100%' : '770px',
                }}
              >
                {/* Empty cells before first day */}
                {Array.from({ length: firstDow }, (_, i) => (
                  <div
                    key={`empty-${i}`}
                    style={{
                      minHeight: '172px',
                      borderRight: '1px solid rgba(0,0,0,0.05)',
                      borderBottom: '1px solid rgba(0,0,0,0.05)',
                      background: 'rgba(0,0,0,0.01)',
                    }}
                  />
                ))}

                {/* Day cells */}
                {Array.from({ length: daysInMonth }, (_, i) => {
                  const day = i + 1
                  const dateStr = toDateStr(viewYear, viewMonth, day)
                  const dayEvents = (eventsByDate[dateStr] ?? []).filter(
                    (ev) => typeFilter === 'all' || ev.type === typeFilter
                  )
                  const isToday = dateStr === todayStr
                  const isSelected = dateStr === selectedDate
                  const colIndex = (firstDow + i) % 7

                  return (
                    <div
                      key={day}
                      onClick={() => setSelectedDate(dateStr === selectedDate ? null : dateStr)}
                      style={{
                        minHeight: '172px',
                        borderRight: colIndex < 6 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                        borderBottom: '1px solid rgba(0,0,0,0.05)',
                        padding: '10px 8px 8px',
                        cursor: 'pointer',
                        background: isSelected ? 'rgba(235,93,30,0.04)' : 'transparent',
                        transition: 'background 100ms ease',
                        position: 'relative',
                      }}
                      onMouseEnter={(e) => {
                        if (!isSelected) (e.currentTarget as HTMLDivElement).style.background = 'rgba(0,0,0,0.02)'
                      }}
                      onMouseLeave={(e) => {
                        if (!isSelected) (e.currentTarget as HTMLDivElement).style.background = 'transparent'
                      }}
                    >
                      {/* Day number */}
                      <div
                        style={{
                          width: '26px',
                          height: '26px',
                          borderRadius: '50%',
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          fontSize: '13px',
                          fontWeight: isToday ? 700 : 400,
                          color: isToday ? '#ffffff' : isSelected ? '#eb5d1e' : '#1A1714',
                          background: isToday ? '#eb5d1e' : 'transparent',
                          marginBottom: '6px',
                        }}
                      >
                        {day}
                      </div>

                      {/* Event dots / chips */}
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                        {dayEvents.slice(0, 3).map((ev) => (
                          <EventChip key={ev.id} ev={ev} />
                        ))}
                        {dayEvents.length > 3 && (
                          <span
                            style={{
                              fontSize: '10px',
                              color: '#A09D9B',
                              fontFamily: 'var(--font-mono)',
                              padding: '1px 4px',
                            }}
                          >
                            +{dayEvents.length - 3} more
                          </span>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            </>
          ) : (
            /* ── WEEK VIEW ──────────────────────────────────────────── */
            <>
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, minmax(110px, 1fr))',
                  borderBottom: '1px solid rgba(0,0,0,0.06)',
                  minWidth: isMobile ? '100%' : '770px',
                }}
              >
                {weekDays.map((wd) => (
                  <div
                    key={wd.date}
                    style={{
                      padding: '12px 8px',
                      textAlign: 'center',
                      borderRight: '1px solid rgba(0,0,0,0.05)',
                    }}
                  >
                    <p
                      style={{
                        fontSize: '11px',
                        fontFamily: 'var(--font-mono)',
                        color: '#A09D9B',
                        textTransform: 'uppercase',
                        letterSpacing: '0.05em',
                        marginBottom: '4px',
                      }}
                    >
                      {wd.label}
                    </p>
                    <div
                      style={{
                        width: '30px',
                        height: '30px',
                        borderRadius: '50%',
                        background: wd.isToday ? '#eb5d1e' : 'transparent',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        margin: '0 auto',
                        fontSize: '15px',
                        fontWeight: wd.isToday ? 700 : 400,
                        color: wd.isToday ? '#ffffff' : '#1A1714',
                      }}
                    >
                      {wd.num}
                    </div>
                  </div>
                ))}
              </div>

              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, minmax(110px, 1fr))',
                  minHeight: '340px',
                  minWidth: isMobile ? '100%' : '770px',
                }}
              >
                {weekDays.map((wd, idx) => {
                  const dayEvents = (eventsByDate[wd.date] ?? []).filter(
                    (ev) => typeFilter === 'all' || ev.type === typeFilter
                  )
                  return (
                    <div
                      key={wd.date}
                      style={{
                        padding: '10px 6px',
                        borderRight: idx < 6 ? '1px solid rgba(0,0,0,0.05)' : 'none',
                        display: 'flex',
                        flexDirection: 'column',
                        gap: '4px',
                        background: wd.isToday ? 'rgba(235,93,30,0.03)' : 'transparent',
                      }}
                    >
                      {dayEvents.map((ev) => (
                        <EventChip key={ev.id} ev={ev} />
                      ))}
                      {dayEvents.length === 0 && (
                        <span style={{ fontSize: '11px', color: '#D0CCC9', textAlign: 'center', paddingTop: '12px' }}>
                          —
                        </span>
                      )}
                    </div>
                  )
                })}
              </div>
            </>
          )}
        </div>

        {/* ── SIDEBAR ──────────────────────────────────────────────── */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Selected day detail */}
          {selectedDate && selectedEvents.length > 0 && (
            <div
              style={{
                background: '#ffffff',
                borderRadius: '14px',
                border: '1px solid rgba(235,93,30,0.25)',
                overflow: 'hidden',
                boxShadow: '0 2px 12px rgba(235,93,30,0.08)',
                animation: 'slideUp 0.2s var(--ease-out) both',
              }}
            >
              <div
                style={{
                  padding: '14px 16px 12px',
                  borderBottom: '1px solid rgba(0,0,0,0.07)',
                }}
              >
                <p
                  style={{
                    fontSize: '11px',
                    fontFamily: 'var(--font-mono)',
                    color: '#eb5d1e',
                    textTransform: 'uppercase',
                    letterSpacing: '0.05em',
                    marginBottom: '2px',
                  }}
                >
                  Selected
                </p>
                <p
                  style={{
                    fontFamily: 'var(--font-display)',
                    fontSize: '18px',
                    color: '#1A1714',
                  }}
                >
                  {new Date(selectedDate + 'T12:00:00').toLocaleDateString('en-US', {
                    weekday: 'long', month: 'long', day: 'numeric',
                  })}
                </p>
              </div>
              {selectedEvents.map((ev) => (
                <UpcomingEventRow key={ev.id} ev={ev} />
              ))}
            </div>
          )}

          {/* Upcoming events */}
          <div
            style={{
              background: '#ffffff',
              borderRadius: '14px',
              border: '1px solid rgba(0,0,0,0.08)',
              overflow: 'hidden',
              boxShadow: '0 2px 8px rgba(0,0,0,0.05)',
            }}
          >
            <div
              style={{
                padding: '14px 16px 12px',
                borderBottom: '1px solid rgba(0,0,0,0.07)',
              }}
            >
              <p
                style={{
                  fontFamily: 'var(--font-display)',
                  fontSize: '17px',
                  color: '#1A1714',
                }}
              >
                Upcoming
              </p>
              <p style={{ fontSize: '12px', color: '#9A9693', marginTop: '2px' }}>
                Next events from today
              </p>
            </div>

            {upcomingEvents.length === 0 ? (
              <div style={{ padding: '32px 16px', textAlign: 'center' }}>
                <p style={{ fontSize: '13px', color: '#A09D9B' }}>No upcoming events</p>
              </div>
            ) : (
              upcomingEvents.map((ev) => (
                <UpcomingEventRow key={ev.id} ev={ev} />
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
