'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
import { fetchMeets, fetchPractices, fetchGroups, type MeetRecord, type PracticeRecord, type GroupRecord } from '@/lib/adminData'

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

function deadlineCountdownText(iso: string): string {
  const days = Math.ceil((new Date(iso).getTime() - Date.now()) / 86400000)
  if (days <= 0) return 'Respond yes/no for each swimmer by tonight.'
  if (days === 1) return 'Respond yes/no for each swimmer — 1 day left.'
  return `Respond yes/no for each swimmer — ${days} days left.`
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
        detail: deadlineCountdownText(meet.commitment_deadline),
        linkPath: `/portal/meets/${meet.id}`,
      })
    }
  }
  return events
}


function practicesToCalEvents(practices: PracticeRecord[], groupNameById: Map<string, string>): CalEvent[] {
  return practices.map((p) => {
    const groupName = p.group_id ? groupNameById.get(p.group_id) : undefined
    const label = groupName ?? 'All groups'
    return {
      id: `practice-${p.id}`,
      title: p.description ? `Practice — ${label} — ${p.description.slice(0, 60)}` : `Practice — ${label}`,
      type: 'practice' as EventType,
      date: p.date,
      detail: p.yardage_target > 0 ? `${p.yardage_target.toLocaleString()} yds target` : undefined,
    }
  })
}

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
        <p style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '2px', lineHeight: 1.3 }}>
          {ev.title}
        </p>
        <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
          {ev.time && (
            <span style={{ fontSize: '11.5px', color: 'var(--text-subtle)', display: 'flex', alignItems: 'center', gap: '3px' }}>
              <Icon name="clock" size={11} />
              {ev.time}
            </span>
          )}
          {ev.location && (
            <span style={{ fontSize: '11.5px', color: 'var(--text-subtle)', display: 'flex', alignItems: 'center', gap: '3px' }}>
              <Icon name="map-pin" size={11} />
              {ev.location}
            </span>
          )}
        </div>
        {ev.detail && (
          <p style={{ fontSize: '11.5px', color: 'var(--text-muted)', marginTop: '4px', lineHeight: 1.4 }}>
            {ev.detail}
          </p>
        )}
      </div>
      <span
        style={{
          fontSize: '10px',
          fontFamily: 'var(--font-body)',
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
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [events, setEvents] = useState<CalEvent[]>([])
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId) {
      setLoadError('Calendar is unavailable because no organization is selected.')
      return
    }
    setLoadError(null)
    Promise.all([
      fetchMeets(api, orgId),
      fetchPractices(api, orgId),
      fetchGroups(api, orgId),
    ])
      .then(([meets, practices, groups]) => {
        const groupNameById = new Map<string, string>(groups.map((g: GroupRecord) => [g.id, g.name]))
        setEvents([...meetsToCalEvents(meets), ...practicesToCalEvents(practices, groupNameById)])
      })
      .catch((error) => {
        setEvents([])
        setLoadError(reportUIError(
          'portal-calendar-load',
          error,
          'Calendar data is unavailable right now.',
          { orgId },
        ))
      })
  }, [api, orgId, reloadKey])

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
  const eventsByDate = events.reduce<Record<string, CalEvent[]>>((acc, ev) => {
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
  const upcomingEvents = events
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
          <h1 className="page-title">
            Calendar
          </h1>
          <p className="page-subtitle">
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
                background: view === v ? 'var(--bg-elevated)' : 'transparent',
                color: view === v ? 'var(--text)' : 'var(--text-subtle)',
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

      {loadError && (
        <div
          style={{
            marginBottom: '20px',
            padding: '12px 16px',
            background: 'var(--warning-bg)',
            border: '1px solid #fdba74',
            borderRadius: '10px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: '12px',
          }}
        >
          <div style={{ display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
            <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
            <span style={{ fontSize: '13px', color: 'var(--warning)', lineHeight: 1.5 }}>{loadError}</span>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{ background: 'var(--bg-elevated)', border: '1px solid #fdba74', borderRadius: '8px', cursor: 'pointer', color: 'var(--warning)', fontSize: '12px', fontWeight: 600, padding: '8px 14px', whiteSpace: 'nowrap' }}
          >
            Retry
          </button>
        </div>
      )}

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
                color: active ? (s ? s.color : 'var(--text)') : 'var(--text-subtle)',
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
          className="card" style={{ overflowX: 'auto', overflowY: 'hidden', boxShadow: '0 2px 8px rgba(0,0,0,0.05)', minWidth: 0 }}
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
                color: 'var(--text-subtle)',
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
                  color: 'var(--text)',
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
                  color: 'var(--text-subtle)',
                  cursor: 'pointer',
                  transition: 'all 120ms ease',
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = 'rgba(0,0,0,0.05)'
                  e.currentTarget.style.color = 'var(--text)'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background = 'transparent'
                  e.currentTarget.style.color = 'var(--text-subtle)'
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
                color: 'var(--text-subtle)',
                transition: 'background 120ms ease',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.05)')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              <Icon name="chevron-right" size={15} />
            </button>
          </div>

          {isMobile ? (
            /* ── MOBILE AGENDA VIEW ─────────────────────────────────── */
            /* The 7-column grid can't reflow to a narrow screen without
               either truncating every cell or forcing horizontal scroll —
               a stacked day-by-day list is the usable mobile equivalent. */
            <div style={{ display: 'flex', flexDirection: 'column' }}>
              {(view === 'month'
                ? Array.from({ length: daysInMonth }, (_, i) => toDateStr(viewYear, viewMonth, i + 1))
                : weekDays.map((wd) => wd.date)
              ).map((dateStr) => {
                const dayEvents = (eventsByDate[dateStr] ?? []).filter(
                  (ev) => typeFilter === 'all' || ev.type === typeFilter
                )
                if (view === 'month' && dayEvents.length === 0) return null
                const isToday = dateStr === todayStr
                const isSelected = dateStr === selectedDate
                const d = new Date(dateStr + 'T12:00:00')

                return (
                  <div
                    key={dateStr}
                    onClick={() => setSelectedDate(dateStr === selectedDate ? null : dateStr)}
                    style={{
                      padding: '12px 16px',
                      borderBottom: '1px solid rgba(0,0,0,0.06)',
                      cursor: 'pointer',
                      background: isSelected ? 'rgba(235,93,30,0.04)' : 'transparent',
                      display: 'flex',
                      gap: '12px',
                      alignItems: 'flex-start',
                    }}
                  >
                    <div
                      style={{
                        width: '34px',
                        height: '34px',
                        borderRadius: '50%',
                        flexShrink: 0,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        fontSize: '13px',
                        fontWeight: isToday ? 700 : 400,
                        color: isToday ? 'var(--bg-elevated)' : isSelected ? 'var(--accent)' : 'var(--text)',
                        background: isToday ? 'var(--accent)' : 'transparent',
                      }}
                    >
                      {d.getDate()}
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '6px' }}>
                        {d.toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' })}
                      </p>
                      {dayEvents.length === 0 ? (
                        <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>No events</span>
                      ) : (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
                          {dayEvents.map((ev) => (
                            <EventChip key={ev.id} ev={ev} />
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          ) : view === 'month' ? (
            <>
              {/* Day-of-week headers */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(7, minmax(110px, 1fr))',
                  borderBottom: '1px solid rgba(0,0,0,0.06)',
                  minWidth: '770px',
                }}
              >
                {DAY_LABELS.map((d) => (
                  <div
                    key={d}
                    style={{
                      padding: '8px 4px',
                      textAlign: 'center',
                      fontSize: '11px',
                      fontFamily: 'var(--font-body)',
                      fontWeight: 600,
                      letterSpacing: '0.05em',
                      textTransform: 'uppercase',
                      color: 'var(--text-muted)',
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
                  minWidth: '770px',
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
                          color: isToday ? 'var(--bg-elevated)' : isSelected ? 'var(--accent)' : 'var(--text)',
                          background: isToday ? 'var(--accent)' : 'transparent',
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
                              color: 'var(--text-muted)',
                              fontFamily: 'var(--font-body)',
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
                  minWidth: '770px',
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
                        fontFamily: 'var(--font-body)',
                        color: 'var(--text-muted)',
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
                        background: wd.isToday ? 'var(--accent)' : 'transparent',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        margin: '0 auto',
                        fontSize: '15px',
                        fontWeight: wd.isToday ? 700 : 400,
                        color: wd.isToday ? 'var(--bg-elevated)' : 'var(--text)',
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
                  minWidth: '770px',
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
              className="card" style={{ overflow: 'hidden', boxShadow: '0 2px 12px rgba(235,93,30,0.08)', animation: 'slideUp 0.2s var(--ease-out) both' }}
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
                    fontFamily: 'var(--font-body)',
                    color: 'var(--accent)',
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
                    color: 'var(--text)',
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
            className="card" style={{ overflow: 'hidden', boxShadow: '0 2px 8px rgba(0,0,0,0.05)' }}
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
                  color: 'var(--text)',
                }}
              >
                Upcoming
              </p>
              <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '2px' }}>
                Next events from today
              </p>
            </div>

            {loadError ? (
              <div style={{ padding: '32px 16px', textAlign: 'center' }}>
                <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>Upcoming calendar events are unavailable right now.</p>
              </div>
            ) : shouldShowCollectionEmptyState(false, loadError, upcomingEvents.length) ? (
              <div style={{ padding: '32px 16px', textAlign: 'center' }}>
                <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>No upcoming events</p>
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
