'use client'

import { useState, useEffect } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
import { fetchMeets, fetchPractices, fetchGroups, type MeetRecord, type PracticeRecord, type GroupRecord } from '@/lib/adminData'
import { fetchClubEvents, type ClubEventRecord } from '@/lib/engagementData'
import { clubEventDateKey, formatClubEventWhen } from '@/lib/club-events'
import { CalendarSubscribeCard } from '@/components/engagement/CalendarSubscribeCard'
import { fetchTrainingSchedule, type Workout } from '@/lib/trainingData'
import { addDaysISO } from '@/lib/workout-format'
import { WorkoutSheet } from '@/components/training/WorkoutSheet'

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
  workout?: Workout
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


function practicesToCalEvents(practices: PracticeRecord[], groupNameById: Map<string, string>, workouts: Map<string, Workout>): CalEvent[] {
  return practices.map((p) => {
    const groupName = p.group_id ? groupNameById.get(p.group_id) : undefined
    const label = groupName ?? 'All groups'
    return {
      id: `practice-${p.id}`,
      title: p.description ? `Practice — ${label} — ${p.description.slice(0, 60)}` : `Practice — ${label}`,
      type: 'practice' as EventType,
      date: p.date,
      detail: p.yardage_target > 0 ? `${p.yardage_target.toLocaleString()} yds target` : undefined,
      workout: workouts.get(p.id),
    }
  })
}

function clubEventsToCalEvents(list: ClubEventRecord[]): CalEvent[] {
  return list.map((e) => ({
    id: `club-${e.id}`,
    title: e.status === 'cancelled' ? `Cancelled: ${e.title}` : e.title,
    type: 'event' as EventType,
    date: clubEventDateKey(e),
    time: formatClubEventWhen(e).split(', ').slice(2).join(', ') || undefined,
    location: e.location || undefined,
    detail: e.rsvp_enabled && e.status !== 'cancelled' ? (e.my_rsvps?.length ? 'You have replied. Change it on the Events page.' : 'RSVP on the Events page.') : undefined,
    linkPath: `/portal/events#event-${e.id}`,
  }))
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
        gap: '4px',
        padding: '4px 8px',
        borderRadius: '6px',
        background: s.bg,
        border: `1px solid ${s.border}`,
        fontSize: 'var(--fs-xs)',
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

const UPCOMING_PREVIEW = 5
const UPCOMING_MAX = 15

// Rows stay two lines at most: long practice titles clamp instead of stacking
// one word per line in the narrow side column.
const clampLines = (lines: number): React.CSSProperties => ({
  display: '-webkit-box',
  WebkitLineClamp: lines,
  WebkitBoxOrient: 'vertical',
  overflow: 'hidden',
})

function upcomingDateLabel(date: string): { day: string; weekday: string } {
  const [y, m, d] = date.split('-').map(Number)
  const local = new Date(y, (m ?? 1) - 1, d ?? 1)
  return {
    day: local.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
    weekday: local.toLocaleDateString('en-US', { weekday: 'short' }),
  }
}

function UpcomingEventRow({ ev }: { ev: CalEvent }) {
  const s = EVENT_STYLE[ev.type]
  const when = upcomingDateLabel(ev.date)
  const meta = [ev.time, ev.location].filter(Boolean).join(' · ')
  const content = (
    <div
      title={ev.title}
      style={{
        display: 'grid',
        gridTemplateColumns: '52px minmax(0, 1fr)',
        gap: '12px',
        alignItems: 'start',
        padding: '12px 16px',
        borderBottom: '1px solid var(--border)',
        transition: 'background 120ms ease',
      }}
      onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.02)')}
      onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
    >
      <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.25 }}>
        <span style={{ fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text)' }}>{when.day}</span>
        <span style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)' }}>{when.weekday}</span>
      </div>
      <div style={{ minWidth: 0 }}>
        <p style={{ display: 'flex', gap: '8px', alignItems: 'baseline', fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.3 }}>
          <span aria-label={s.label} style={{ width: '8px', height: '8px', borderRadius: '50%', background: s.dot, flexShrink: 0, transform: 'translateY(-1px)' }} />
          <span style={clampLines(2)}>{ev.title}</span>
        </p>
        {meta && (
          <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-subtle)', marginTop: '2px', ...clampLines(1) }}>{meta}</p>
        )}
        {ev.detail && (
          <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginTop: '2px', ...clampLines(1) }}>{ev.detail}</p>
        )}
      </div>
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
  const [clubCalEvents, setClubCalEvents] = useState<CalEvent[]>([])
  const [clubError, setClubError] = useState<string | null>(null)

  // Club events load independently so a failure there never blanks meets/practices.
  useEffect(() => {
    if (!orgId) return
    setClubError(null)
    fetchClubEvents(api, orgId)
      .then((list) => setClubCalEvents(clubEventsToCalEvents(list)))
      .catch((error) => {
        setClubCalEvents([])
        setClubError(reportUIError('portal-calendar-club-events', error, 'Club events could not be loaded.', { orgId }))
      })
  }, [api, orgId, reloadKey])

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
      // Structured sets for nearby practices; the calendar still renders if unavailable.
      fetchTrainingSchedule(api, orgId, addDaysISO(new Date().toISOString().slice(0, 10), -30), addDaysISO(new Date().toISOString().slice(0, 10), 90)).catch(() => []),
    ])
      .then(([meets, practices, groups, scheduleItems]) => {
        const groupNameById = new Map<string, string>(groups.map((g: GroupRecord) => [g.id, g.name]))
        const workoutByPractice = new Map<string, Workout>()
        for (const item of scheduleItems) if (item.workout) workoutByPractice.set(item.practice.id, item.workout)
        setEvents([...meetsToCalEvents(meets), ...practicesToCalEvents(practices, groupNameById, workoutByPractice)])
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
  const [showAllUpcoming, setShowAllUpcoming] = useState(false)
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
  const allEvents = [...events, ...clubCalEvents]
  const eventsByDate = allEvents.reduce<Record<string, CalEvent[]>>((acc, ev) => {
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
  const upcomingEvents = allEvents
    .filter((ev) => ev.date >= todayStr)
    .filter((ev) => typeFilter === 'all' || ev.type === typeFilter)
    .sort((a, b) => a.date.localeCompare(b.date))
    .slice(0, UPCOMING_MAX)
  const visibleUpcoming = showAllUpcoming ? upcomingEvents : upcomingEvents.slice(0, UPCOMING_PREVIEW)

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
          marginBottom: '24px',
          flexWrap: 'wrap',
          gap: '12px',
          animation: 'slideUp 0.4s var(--ease-out) both',
        }}
      >
        <div>
          <h1 className="page-title">
            Calendar
          </h1>
        </div>

        {/* View toggle */}
        <div
          style={{
            display: 'flex',
            gap: '4px',
            background: 'rgba(0,0,0,0.05)',
            borderRadius: '8px',
            padding: '4px',
          }}
        >
          {(['month', 'week'] as const).map((v) => (
            <button
              key={v}
              onClick={() => setView(v)}
              style={{
                padding: '4px 16px',
                borderRadius: '6px',
                border: 'none',
                background: view === v ? 'var(--bg-elevated)' : 'transparent',
                color: view === v ? 'var(--text)' : 'var(--text-subtle)',
                fontSize: 'var(--fs-sm)',
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

      {clubError && !loadError && (
        <p role="alert" style={{ marginBottom: '12px', fontSize: 'var(--fs-sm)', color: 'var(--warning)' }}>
          {clubError}{' '}
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setReloadKey((key) => key + 1)}>Retry</button>
        </p>
      )}

      {loadError && (
        <div
          style={{
            marginBottom: '20px',
            padding: '12px 16px',
            background: 'var(--warning-bg)',
            border: '1px solid #fdba74',
            borderRadius: '12px',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            gap: '12px',
          }}
        >
          <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-start' }}>
            <Icon name="alert-circle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
            <span style={{ fontSize: 'var(--fs-base)', color: 'var(--warning)', lineHeight: 1.5 }}>{loadError}</span>
          </div>
          <button
            type="button"
            onClick={() => setReloadKey((key) => key + 1)}
            style={{ background: 'var(--bg-elevated)', border: '1px solid #fdba74', borderRadius: '8px', cursor: 'pointer', color: 'var(--warning)', fontSize: 'var(--fs-sm)', fontWeight: 600, padding: '8px 16px', whiteSpace: 'nowrap' }}
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
                padding: '4px 12px',
                borderRadius: '999px',
                border: `1px solid ${active ? (s ? s.border : 'rgba(0,0,0,0.22)') : 'rgba(0,0,0,0.10)'}`,
                background: active ? (s ? s.bg : 'rgba(0,0,0,0.06)') : 'transparent',
                color: active ? (s ? s.color : 'var(--text)') : 'var(--text-subtle)',
                fontSize: 'var(--fs-sm)',
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
          className="card" style={{ overflowX: 'auto', overflowY: 'hidden', minWidth: 0 }}
        >
          {/* Month nav */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '20px 20px 16px',
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
              <h2 className="section-title"
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
                  fontSize: 'var(--fs-sm)',
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
                        fontSize: 'var(--fs-base)',
                        fontWeight: isToday ? 700 : 400,
                        color: isToday ? 'var(--bg-elevated)' : isSelected ? 'var(--accent)' : 'var(--text)',
                        background: isToday ? 'var(--accent)' : 'transparent',
                      }}
                    >
                      {d.getDate()}
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <p style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', fontWeight: 600, color: 'var(--text-subtle)', marginBottom: '6px' }}>
                        {d.toLocaleDateString('en-US', { weekday: 'long', month: 'short', day: 'numeric' })}
                      </p>
                      {dayEvents.length === 0 ? (
                        <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>No events</span>
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
                  gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
                  borderBottom: '1px solid rgba(0,0,0,0.06)',
                  minWidth: '560px',
                }}
              >
                {DAY_LABELS.map((d) => (
                  <div
                    key={d}
                    style={{
                      padding: '8px 4px',
                      textAlign: 'center',
                      fontSize: 'var(--fs-xs)',
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
                  gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
                  minWidth: '560px',
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
                        padding: '12px 8px 8px',
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
                          fontSize: 'var(--fs-base)',
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
                              fontSize: 'var(--fs-2xs)',
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
                  gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
                  borderBottom: '1px solid rgba(0,0,0,0.06)',
                  minWidth: '560px',
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
                        fontSize: 'var(--fs-xs)',
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
                        fontSize: 'var(--fs-md)',
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
                  gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
                  minHeight: '340px',
                  minWidth: '560px',
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
                        padding: '12px 6px',
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
                        <span style={{ fontSize: 'var(--fs-xs)', color: '#D0CCC9', textAlign: 'center', paddingTop: '12px' }}>
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
          <CalendarSubscribeCard api={api} orgId={orgId} />
          {/* Selected day detail */}
          {selectedDate && selectedEvents.length > 0 && (
            <div
              className="card" style={{ overflow: 'hidden', animation: 'slideUp 0.2s var(--ease-out) both' }}
            >
              <div
                style={{
                  padding: '16px 16px 12px',
                  borderBottom: '1px solid rgba(0,0,0,0.07)',
                }}
              >
                <p
                  style={{
                    fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)',
                    fontSize: 'var(--fs-xl)',
                    color: 'var(--text)',
                  }}
                >
                  {new Date(selectedDate + 'T12:00:00').toLocaleDateString('en-US', {
                    weekday: 'long', month: 'long', day: 'numeric',
                  })}
                </p>
              </div>
              {selectedEvents.map((ev) => (
                <div key={ev.id}>
                  <UpcomingEventRow ev={ev} />
                  {ev.workout && (
                    <div style={{ padding: '4px 16px 16px' }}>
                      <WorkoutSheet workout={ev.workout} />
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Upcoming events */}
          <div
            className="card" style={{ padding: 0, overflow: 'hidden' }}
          >
            <div style={{ padding: '16px 16px 12px', borderBottom: '1px solid var(--border)' }}>
              <h2 className="card-title">Upcoming</h2>
            </div>

            {loadError ? (
              <div style={{ padding: '32px 16px', textAlign: 'center' }}>
                <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>Upcoming calendar events are unavailable right now.</p>
              </div>
            ) : shouldShowCollectionEmptyState(false, loadError, upcomingEvents.length) ? (
              <div style={{ padding: '32px 16px', textAlign: 'center' }}>
                <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>No upcoming events</p>
              </div>
            ) : (
              <>
                {visibleUpcoming.map((ev) => (
                  <UpcomingEventRow key={ev.id} ev={ev} />
                ))}
                {upcomingEvents.length > UPCOMING_PREVIEW && (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => setShowAllUpcoming((v) => !v)}
                    style={{ width: '100%', justifyContent: 'center', borderRadius: 0, minHeight: '40px' }}
                  >
                    {showAllUpcoming ? 'Show fewer' : `Show ${upcomingEvents.length - UPCOMING_PREVIEW} more`}
                  </button>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
