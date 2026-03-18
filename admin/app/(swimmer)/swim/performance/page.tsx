'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

// ── Design tokens ──────────────────────────────────────────────────────────
const C = {
  accent:      '#00D4AA',
  accentDim:   'rgba(0,212,170,0.10)',
  accentBorder:'rgba(0,212,170,0.22)',
  text:        '#E4F2FF',
  subtle:      '#7FA8C9',
  muted:       '#4A6B82',
  card:        '#0D2235',
  border:      'rgba(0,212,170,0.10)',
}

const STANDARD_COLOR: Record<string, string> = {
  AAA: '#00D4AA',
  AA:  '#3b82f6',
  A:   '#8b5cf6',
  BB:  '#F59E0B',
  B:   '#9ca3af',
}

// ── Stub data ──────────────────────────────────────────────────────────────
type BestTime = {
  event: string
  scy?: { time: string; date: string; meet: string; standard: string }
  scm?: { time: string; date: string; meet: string; standard: string }
  lcm?: { time: string; date: string; meet: string; standard: string }
}

const BEST_TIMES: BestTime[] = [
  {
    event: '50 Free',
    scy: { time: '25.34', date: 'Feb 14, 2026', meet: 'Winter Invite',    standard: 'A'  },
    scm: { time: '27.01', date: 'Nov 8, 2025',  meet: 'Fall Short Course', standard: 'B'  },
    lcm: { time: '28.44', date: 'Jul 12, 2025', meet: 'Summer Champs',    standard: 'B'  },
  },
  {
    event: '100 Free',
    scy: { time: '55.82', date: 'Feb 14, 2026', meet: 'Winter Invite',    standard: 'A'  },
    scm: { time: '58.91', date: 'Nov 8, 2025',  meet: 'Fall Short Course', standard: 'A'  },
    lcm: { time: '1:02.33', date: 'Jul 12, 2025', meet: 'Summer Champs', standard: 'B' },
  },
  {
    event: '200 Free',
    scy: { time: '1:52.34', date: 'Jan 19, 2026', meet: 'Tri-City Classic',  standard: 'A'  },
    scm: { time: '1:58.12', date: 'Nov 8, 2025',  meet: 'Fall Short Course', standard: 'A'  },
    lcm: { time: '2:08.45', date: 'Jul 12, 2025', meet: 'Summer Champs',    standard: 'B'  },
  },
  {
    event: '100 Back',
    scy: { time: '59.87', date: 'Feb 14, 2026', meet: 'Winter Invite',    standard: 'AA' },
    scm: { time: '1:03.44', date: 'Nov 8, 2025', meet: 'Fall Short Course', standard: 'A' },
    lcm: { time: '1:08.91', date: 'Jul 12, 2025', meet: 'Summer Champs',  standard: 'A' },
  },
  {
    event: '200 Back',
    scy: { time: '2:07.88', date: 'Jan 19, 2026', meet: 'Tri-City Classic',  standard: 'A'  },
    scm: { time: '2:15.21', date: 'Nov 8, 2025',  meet: 'Fall Short Course', standard: 'B'  },
    lcm: undefined,
  },
  {
    event: '100 Breast',
    scy: { time: '1:07.23', date: 'Feb 14, 2026', meet: 'Winter Invite', standard: 'B' },
    scm: undefined,
    lcm: undefined,
  },
  {
    event: '200 IM',
    scy: { time: '2:13.45', date: 'Jan 19, 2026', meet: 'Tri-City Classic', standard: 'A' },
    scm: { time: '2:19.88', date: 'Nov 8, 2025', meet: 'Fall Short Course', standard: 'B' },
    lcm: undefined,
  },
  {
    event: '400 IM',
    scy: { time: '4:42.11', date: 'Feb 14, 2026', meet: 'Winter Invite',    standard: 'B' },
    scm: undefined,
    lcm: { time: '5:01.34', date: 'Jul 12, 2025', meet: 'Summer Champs',    standard: 'B' },
  },
]


const CUT_QUALIFICATIONS = [
  { name: '2026 Junior Nationals',    standard: 'AA', event: '100 Back SCY', status: 'qualified' as const, cutTime: '59.99', bestTime: '59.87' },
  { name: '2026 Regional Champs',     standard: 'A',  event: '200 Free SCY', status: 'qualified' as const, cutTime: '1:53.00', bestTime: '1:52.34' },
  { name: '2026 State Championship',  standard: 'A',  event: '400 IM SCY',   status: 'close' as const,    cutTime: '4:40.00', bestTime: '4:42.11' },
  { name: '2026 Senior Nationals',    standard: 'AAA',event: '100 Back SCY', status: 'not_qualified' as const, cutTime: '56.49', bestTime: '59.87' },
]

// ── Components ─────────────────────────────────────────────────────────────
type Course = 'SCY' | 'SCM' | 'LCM'

function CourseCell({ entry }: { entry?: { time: string; date: string; meet: string; standard: string } }) {
  if (!entry) {
    return (
      <div style={{ textAlign: 'center' }}>
        <span style={{ fontSize: '13px', color: C.muted, fontFamily: 'var(--font-mono)' }}>—</span>
      </div>
    )
  }
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '7px', marginBottom: '2px' }}>
        <span style={{ fontSize: '15px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: C.accent }}>
          {entry.time}
        </span>
        <span
          style={{
            fontSize: '10px',
            fontFamily: 'var(--font-mono)',
            fontWeight: 700,
            color: STANDARD_COLOR[entry.standard] ?? C.subtle,
            background: `${STANDARD_COLOR[entry.standard] ?? C.subtle}18`,
            padding: '1px 6px',
            borderRadius: '4px',
          }}
        >
          {entry.standard}
        </span>
      </div>
      <p style={{ fontSize: '10.5px', color: C.muted }}>{entry.date}</p>
    </div>
  )
}

function ProgressionBar({ value, max, color }: { value: number; max: number; color: string }) {
  const pct = Math.min(100, (value / max) * 100)
  return (
    <div
      style={{
        height: '6px',
        background: 'rgba(0,212,170,0.10)',
        borderRadius: '999px',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          height: '100%',
          width: `${pct}%`,
          background: color,
          borderRadius: '999px',
          transition: 'width 600ms cubic-bezier(0.16,1,0.3,1)',
        }}
      />
    </div>
  )
}

// Additional progression data per event
const PROGRESSION_BY_EVENT: Record<string, Array<{ date: string; time: number; meet?: string }>> = {
  '100 Back SCY': [
    { date: 'Sep 24', time: 63.22, meet: 'Fall Classic' },
    { date: 'Nov 24', time: 61.44, meet: 'Winter Invite' },
    { date: 'Jan 25', time: 60.88, meet: 'Tri-City Classic' },
    { date: 'Mar 25', time: 60.10, meet: 'Spring Invite' },
    { date: 'Jul 25', time: 61.33, meet: 'Summer Champs (LCM)' },
    { date: 'Sep 25', time: 60.55, meet: 'Fall Classic' },
    { date: 'Nov 25', time: 60.12, meet: 'Holiday Invite' },
    { date: 'Feb 26', time: 59.87, meet: 'Winter Invite' },
  ],
  '200 Free SCY': [
    { date: 'Sep 24', time: 118.01, meet: 'Fall Classic' },
    { date: 'Nov 24', time: 116.32, meet: 'Winter Invite' },
    { date: 'Jan 25', time: 115.14, meet: 'Tri-City Classic' },
    { date: 'Mar 25', time: 114.22, meet: 'Spring Invite' },
    { date: 'Sep 25', time: 113.01, meet: 'Fall Classic' },
    { date: 'Nov 25', time: 112.88, meet: 'Holiday Invite' },
    { date: 'Feb 26', time: 112.34, meet: 'Winter Invite' },
  ],
  '50 Free SCY': [
    { date: 'Jan 25', time: 27.45 },
    { date: 'Mar 25', time: 26.88 },
    { date: 'Jul 25', time: 27.02 },
    { date: 'Sep 25', time: 26.44 },
    { date: 'Feb 26', time: 25.34 },
  ],
  '200 IM SCY': [
    { date: 'Nov 24', time: 136.22 },
    { date: 'Jan 25', time: 134.55 },
    { date: 'Mar 25', time: 133.44 },
    { date: 'Nov 25', time: 133.88 },
    { date: 'Jan 26', time: 133.45 },
  ],
}

const GOAL_CUT_TIMES: Record<string, number> = {
  '100 Back SCY': 56.49,  // Senior Nationals AAA
  '200 Free SCY': 109.00, // Junior Nationals AA
  '50 Free SCY':  23.99,
  '200 IM SCY':   131.00,
}

function parseGoalTimeInput(value: string): number | null {
  const trimmed = value.trim()
  if (!trimmed) return null

  const secondsOnly = /^\d{1,2}(\.\d{1,2})?$/
  const minutesSeconds = /^\d+:\d{2}(\.\d{1,2})?$/

  if (secondsOnly.test(trimmed)) {
    return Number.parseFloat(trimmed)
  }

  if (minutesSeconds.test(trimmed)) {
    const [minutes, seconds] = trimmed.split(':')
    return Number.parseInt(minutes, 10) * 60 + Number.parseFloat(seconds)
  }

  return Number.NaN
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function PerformancePage() {
  const [courseFilter, setCourseFilter] = useState<Course>('SCY')
  const [tab, setTab] = useState<'times' | 'cuts' | 'progression'>('times')
  const [progressEvent, setProgressEvent] = useState('100 Back SCY')
  const [goalTime, setGoalTime] = useState<Record<string, string>>({
    '100 Back SCY': '58.50',
    '200 Free SCY': '1:50.00',
    '50 Free SCY': '24.80',
    '200 IM SCY': '2:11.00',
  })
  const [goalInput, setGoalInput] = useState<Record<string, string>>({
    '100 Back SCY': '58.50',
    '200 Free SCY': '1:50.00',
    '50 Free SCY': '24.80',
    '200 IM SCY': '2:11.00',
  })
  const [editingGoal, setEditingGoal] = useState(false)
  const [goalError, setGoalError] = useState<string | null>(null)
  const [viewMode, setViewMode] = useState<'chart' | 'table' | 'monthly'>('chart')

  return (
    <div>
      {/* Page header */}
      <div
        style={{
          marginBottom: '32px',
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
          Performance
        </h1>
        <p style={{ fontSize: '14px', color: C.subtle }}>
          Best times, cut qualifications, and progression history.
        </p>
      </div>

      {/* Tabs */}
      <div
        style={{
          display: 'flex',
          gap: '4px',
          background: 'rgba(0,0,0,0.30)',
          borderRadius: '10px',
          padding: '4px',
          marginBottom: '28px',
          width: 'fit-content',
          animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) 0.05s both',
        }}
      >
        {[
          { key: 'times',       label: 'Best Times' },
          { key: 'cuts',        label: 'Cut Qualification' },
          { key: 'progression', label: 'Progression' },
        ].map(({ key, label }) => (
          <button
            key={key}
            onClick={() => setTab(key as typeof tab)}
            style={{
              padding: '7px 16px',
              borderRadius: '7px',
              border: 'none',
              background: tab === key ? C.accentDim : 'transparent',
              color: tab === key ? C.accent : C.subtle,
              fontSize: '13.5px',
              fontWeight: tab === key ? 600 : 400,
              cursor: 'pointer',
              boxShadow: tab === key ? `0 0 0 1px ${C.accentBorder}` : 'none',
              transition: 'all 150ms ease',
            }}
          >
            {label}
          </button>
        ))}
      </div>

      {/* ── BEST TIMES TAB ─────────────────────────────────────────── */}
      {tab === 'times' && (
        <div style={{ animation: 'slideUp 0.25s cubic-bezier(0.16,1,0.3,1) both' }}>
          {/* Course filter */}
          <div
            style={{
              display: 'flex',
              gap: '8px',
              marginBottom: '20px',
            }}
          >
            {(['SCY', 'SCM', 'LCM'] as Course[]).map((course) => (
              <button
                key={course}
                onClick={() => setCourseFilter(course)}
                style={{
                  padding: '6px 16px',
                  borderRadius: '8px',
                  border: `1px solid ${courseFilter === course ? C.accentBorder : C.border}`,
                  background: courseFilter === course ? C.accentDim : 'transparent',
                  color: courseFilter === course ? C.accent : C.subtle,
                  fontSize: '12.5px',
                  fontWeight: 600,
                  fontFamily: 'var(--font-mono)',
                  cursor: 'pointer',
                  letterSpacing: '0.04em',
                  transition: 'all 120ms ease',
                }}
              >
                {course}
              </button>
            ))}
          </div>

          {/* Best times table */}
          <div
            style={{
              background: C.card,
              border: `1px solid ${C.border}`,
              borderRadius: '14px',
              overflow: 'hidden',
            }}
          >
            {/* Header */}
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr 1fr 1fr 1fr',
                padding: '12px 20px',
                borderBottom: `1px solid ${C.border}`,
              }}
            >
              {['Event', 'SCY', 'SCM', 'LCM'].map((h) => (
                <span
                  key={h}
                  style={{
                    fontSize: '10.5px',
                    fontFamily: 'var(--font-mono)',
                    color: C.muted,
                    textTransform: 'uppercase',
                    letterSpacing: '0.05em',
                  }}
                >
                  {h}
                </span>
              ))}
            </div>

            {/* Rows */}
            {BEST_TIMES.map((bt, i) => (
              <div
                key={bt.event}
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 1fr 1fr 1fr',
                  padding: '14px 20px',
                  borderBottom: i < BEST_TIMES.length - 1 ? `1px solid ${C.border}` : 'none',
                  alignItems: 'center',
                  transition: 'background 100ms ease',
                  animationDelay: `${i * 30}ms`,
                  animation: 'slideUp 0.3s cubic-bezier(0.16,1,0.3,1) both',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,212,170,0.03)')}
                onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
              >
                <span style={{ fontSize: '14px', fontWeight: 600, color: C.text }}>{bt.event}</span>
                <CourseCell entry={bt.scy} />
                <CourseCell entry={bt.scm} />
                <CourseCell entry={bt.lcm} />
              </div>
            ))}
          </div>

          {/* Standard legend */}
          <div
            style={{
              display: 'flex',
              gap: '16px',
              marginTop: '14px',
              flexWrap: 'wrap',
            }}
          >
            {Object.entries(STANDARD_COLOR).map(([std, color]) => (
              <div key={std} style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                <span
                  style={{
                    fontSize: '10px',
                    fontFamily: 'var(--font-mono)',
                    fontWeight: 700,
                    color,
                    background: `${color}18`,
                    padding: '2px 7px',
                    borderRadius: '4px',
                  }}
                >
                  {std}
                </span>
                <span style={{ fontSize: '11px', color: C.muted }}>
                  {std === 'AAA' ? 'National Qualifying' : std === 'AA' ? 'Junior National' : std === 'A' ? 'Regional' : std === 'BB' ? 'Sectional' : 'Divisional'}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── CUT QUALIFICATION TAB ──────────────────────────────────── */}
      {tab === 'cuts' && (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: '12px',
            animation: 'slideUp 0.25s cubic-bezier(0.16,1,0.3,1) both',
          }}
        >
          {CUT_QUALIFICATIONS.map((cut, i) => {
            const isQualified = cut.status === 'qualified'
            const isClose = cut.status === 'close'

            const statusConfig = {
              qualified:     { label: 'Qualified',      color: '#00D4AA', bg: 'rgba(0,212,170,0.10)', border: 'rgba(0,212,170,0.25)' },
              close:         { label: 'Close',          color: '#F59E0B', bg: 'rgba(245,158,11,0.10)', border: 'rgba(245,158,11,0.25)' },
              not_qualified: { label: 'Not Qualified',  color: '#4A6B82', bg: 'rgba(74,107,130,0.10)', border: 'rgba(74,107,130,0.20)' },
            }[cut.status]

            // Calculate proximity (lower is better in swimming — convert times to seconds)
            const parseTime = (t: string) => {
              const parts = t.split(':').map(parseFloat)
              return parts.length === 2 ? parts[0] * 60 + parts[1] : parts[0]
            }
            const best = parseTime(cut.bestTime)
            const cutT = parseTime(cut.cutTime)
            const diff = best - cutT
            const pctToGo = isQualified ? 0 : Math.min(100, (diff / cutT) * 100 * 10)

            return (
              <div
                key={cut.name}
                style={{
                  background: C.card,
                  border: `1px solid ${isQualified ? C.accentBorder : C.border}`,
                  borderRadius: '12px',
                  padding: '18px 20px',
                  boxShadow: isQualified ? '0 0 20px rgba(0,212,170,0.06)' : 'none',
                  animation: `slideUp 0.3s cubic-bezier(0.16,1,0.3,1) ${i * 60}ms both`,
                }}
              >
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '12px', marginBottom: '14px' }}>
                  <div>
                    <p style={{ fontSize: '15px', fontWeight: 600, color: C.text, marginBottom: '3px' }}>
                      {cut.name}
                    </p>
                    <p style={{ fontSize: '12.5px', color: C.subtle }}>
                      {cut.event} · {cut.standard} Standard
                    </p>
                  </div>
                  <span
                    style={{
                      fontSize: '11px',
                      fontWeight: 600,
                      fontFamily: 'var(--font-mono)',
                      color: statusConfig.color,
                      background: statusConfig.bg,
                      border: `1px solid ${statusConfig.border}`,
                      padding: '4px 11px',
                      borderRadius: '999px',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {statusConfig.label}
                  </span>
                </div>

                {/* Times comparison */}
                <div
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '1fr 1fr',
                    gap: '12px',
                    marginBottom: '14px',
                  }}
                >
                  <div>
                    <p style={{ fontSize: '10.5px', fontFamily: 'var(--font-mono)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '4px' }}>
                      My Best Time
                    </p>
                    <p style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: C.accent }}>
                      {cut.bestTime}
                    </p>
                  </div>
                  <div>
                    <p style={{ fontSize: '10.5px', fontFamily: 'var(--font-mono)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '4px' }}>
                      Qualifying Cut
                    </p>
                    <p style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: C.subtle }}>
                      {cut.cutTime}
                    </p>
                  </div>
                </div>

                {/* Progress bar */}
                {!isQualified && (
                  <div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '5px' }}>
                      <span style={{ fontSize: '11px', color: C.muted }}>
                        {isClose ? `Only ${diff.toFixed(2)}s from qualifying` : `${diff.toFixed(2)}s from cut`}
                      </span>
                    </div>
                    <ProgressionBar
                      value={100 - pctToGo}
                      max={100}
                      color={isClose ? '#F59E0B' : C.muted}
                    />
                  </div>
                )}

                {isQualified && (
                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                    <Icon name="check-circle" size={14} style={{ color: C.accent }} />
                    <span style={{ fontSize: '12.5px', color: C.accent, fontWeight: 500 }}>
                      Qualified by {Math.abs(parseTime(cut.bestTime) - parseTime(cut.cutTime)).toFixed(2)}s
                    </span>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {/* ── PROGRESSION TAB ────────────────────────────────────────── */}
      {tab === 'progression' && (() => {
        const eventData = PROGRESSION_BY_EVENT[progressEvent] ?? []
        const allTimes = eventData.map((d) => d.time)
        const minTime = Math.min(...allTimes) * 0.98
        const maxTime = Math.max(...allTimes) * 1.01
        const latestTime = allTimes[allTimes.length - 1] ?? 0
        const improvement = allTimes.length > 1 ? allTimes[0] - latestTime : 0

        const goalSec = (() => {
          const parsed = parseGoalTimeInput(goalTime[progressEvent] ?? '')
          return parsed === null || Number.isNaN(parsed) ? null : parsed
        })()

        const cutSec = GOAL_CUT_TIMES[progressEvent]
        const toGoal = goalSec ? latestTime - goalSec : null
        const toCut = cutSec ? latestTime - cutSec : null

        function fmtSec(s: number): string {
          if (s < 60) return s.toFixed(2)
          const m = Math.floor(s / 60)
          const rem = (s % 60).toFixed(2).padStart(5, '0')
          return `${m}:${rem}`
        }

        // Monthly grouping for monthly view
        const monthMap: Record<string, number[]> = {}
        eventData.forEach((d) => {
          const mo = d.date
          if (!monthMap[mo]) monthMap[mo] = []
          monthMap[mo].push(d.time)
        })

        return (
          <div style={{ animation: 'slideUp 0.25s cubic-bezier(0.16,1,0.3,1) both' }}>
            {/* Event picker + view mode */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px', flexWrap: 'wrap', gap: '10px' }}>
              <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
                {Object.keys(PROGRESSION_BY_EVENT).map((ev) => (
                  <button
                    key={ev}
                    onClick={() => setProgressEvent(ev)}
                    style={{
                      fontSize: '11px', padding: '4px 12px', borderRadius: '20px', cursor: 'pointer',
                      background: progressEvent === ev ? C.accent : 'rgba(0,212,170,0.06)',
                      color: progressEvent === ev ? '#061623' : C.subtle,
                      border: `1px solid ${progressEvent === ev ? C.accent : C.border}`,
                      fontFamily: 'var(--font-mono)', fontWeight: 600,
                      transition: 'all 100ms ease',
                    }}
                  >{ev}</button>
                ))}
              </div>
              <div style={{ display: 'flex', gap: '4px' }}>
                {(['chart', 'table', 'monthly'] as const).map((m) => (
                  <button
                    key={m}
                    onClick={() => setViewMode(m)}
                    style={{
                      fontSize: '11px', padding: '4px 12px', borderRadius: '20px', cursor: 'pointer',
                      background: viewMode === m ? 'rgba(0,212,170,0.12)' : 'transparent',
                      color: viewMode === m ? C.accent : C.muted,
                      border: `1px solid ${viewMode === m ? C.accentBorder : C.border}`,
                      fontFamily: 'var(--font-mono)', textTransform: 'capitalize',
                    }}
                  >{m}</button>
                ))}
              </div>
            </div>

            {/* Goal + stats strip */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', gap: '10px', marginBottom: '20px' }}>
              {[
                { label: 'Current Best', value: fmtSec(latestTime), color: C.accent, sub: eventData[eventData.length - 1]?.meet ?? '' },
                { label: 'Total Drop', value: improvement > 0 ? `-${improvement.toFixed(2)}s` : '—', color: improvement > 0 ? C.accent : C.muted, sub: `over ${eventData.length} data points` },
                { label: 'My Goal', value: goalSec ? fmtSec(goalSec) : 'Not set', color: goalSec ? '#F59E0B' : C.muted, sub: toGoal !== null ? (toGoal > 0 ? `${toGoal.toFixed(2)}s to go` : `Beat by ${Math.abs(toGoal).toFixed(2)}s!`) : 'Tap to set a goal' },
                { label: 'Next Cut', value: cutSec ? fmtSec(cutSec) : '—', color: toCut !== null ? (toCut <= 0 ? C.accent : C.muted) : C.muted, sub: toCut !== null ? (toCut > 0 ? `${toCut.toFixed(2)}s from qualifying` : 'Qualified!') : '' },
              ].map((s) => (
                <div key={s.label} style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '12px', padding: '14px 16px' }}>
                  <div style={{ fontSize: '10px', textTransform: 'uppercase', letterSpacing: '0.05em', color: C.muted, fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
                  <div style={{ fontSize: '18px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: s.color, marginBottom: '2px' }}>{s.value}</div>
                  <div style={{ fontSize: '10px', color: C.muted }}>{s.sub}</div>
                </div>
              ))}
            </div>

            {/* Goal editor */}
            <div style={{ background: 'rgba(245,158,11,0.06)', border: '1px solid rgba(245,158,11,0.22)', borderRadius: '12px', padding: '14px 16px', marginBottom: '20px', display: 'flex', alignItems: 'center', gap: '12px' }}>
              <Icon name="zap" size={16} style={{ color: '#F59E0B', flexShrink: 0 }} />
              <div style={{ flex: 1 }}>
                <p style={{ fontSize: '12px', fontWeight: 600, color: C.text, marginBottom: '2px' }}>Goal for {progressEvent}</p>
                {editingGoal ? (
                  <div style={{ display: 'flex', gap: '8px', alignItems: 'center', marginTop: '6px' }}>
                    <input
                      value={goalInput[progressEvent] ?? ''}
                      onChange={(e) => {
                        setGoalInput({ ...goalInput, [progressEvent]: e.target.value })
                        setGoalError(null)
                      }}
                      placeholder="e.g. 58.50 or 1:58.00"
                      style={{ background: 'rgba(0,212,170,0.06)', border: `1px solid ${C.border}`, borderRadius: '8px', padding: '5px 10px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: C.text, width: '160px' }}
                    />
                    <button
                      onClick={() => {
                        const parsed = parseGoalTimeInput(goalInput[progressEvent] ?? '')
                        if (parsed === null || Number.isNaN(parsed)) {
                          setGoalError('Enter goal time as SS.ss or M:SS.ss.')
                          return
                        }
                        setGoalTime({ ...goalTime, [progressEvent]: (goalInput[progressEvent] ?? '').trim() })
                        setEditingGoal(false)
                        setGoalError(null)
                      }}
                      style={{ fontSize: '11px', padding: '5px 12px', background: C.accent, color: '#061623', border: 'none', borderRadius: '6px', cursor: 'pointer', fontWeight: 700 }}
                    >
                      Save
                    </button>
                  </div>
                ) : (
                  <p style={{ fontSize: '11px', color: C.muted }}>{goalTime[progressEvent] ? `Target: ${goalTime[progressEvent]}` : 'No goal set for this event.'}</p>
                )}
                {goalError && (
                  <p style={{ fontSize: '11px', color: '#fca5a5', marginTop: '6px' }}>{goalError}</p>
                )}
              </div>
              {!editingGoal && (
                <button onClick={() => { setGoalInput({ ...goalInput, [progressEvent]: goalTime[progressEvent] ?? '' }); setGoalError(null); setEditingGoal(true) }} style={{ fontSize: '11px', padding: '5px 12px', background: 'rgba(245,158,11,0.15)', color: '#F59E0B', border: '1px solid rgba(245,158,11,0.30)', borderRadius: '8px', cursor: 'pointer', fontWeight: 600 }}>
                  {goalTime[progressEvent] ? 'Edit Goal' : 'Set Goal'}
                </button>
              )}
            </div>

            {/* Chart view */}
            {viewMode === 'chart' && (
              <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '14px', padding: '24px', marginBottom: '20px' }}>
                <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '20px' }}>
                  {progressEvent} — Time Progression
                </p>
                <div style={{ display: 'flex', alignItems: 'flex-end', gap: '8px', height: '140px', position: 'relative' }}>
                  {/* Goal line */}
                  {goalSec && (
                    <div style={{
                      position: 'absolute', bottom: `${((1 - (goalSec - minTime) / (maxTime - minTime)) * 140)}px`,
                      left: 0, right: 0, height: '1px', background: '#F59E0B',
                      borderTop: '1px dashed #F59E0B', zIndex: 2,
                    }}>
                      <span style={{ position: 'absolute', right: 0, top: '-14px', fontSize: '9px', color: '#F59E0B', fontFamily: 'var(--font-mono)', background: C.card, padding: '0 4px' }}>goal</span>
                    </div>
                  )}
                  {eventData.map((d, i) => {
                    const normalized = 1 - (d.time - minTime) / (maxTime - minTime)
                    const barH = Math.max(20, normalized * 130)
                    const isLatest = i === eventData.length - 1
                    return (
                      <div key={d.date} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '4px' }}>
                        <p style={{ fontSize: '9px', fontFamily: 'var(--font-mono)', color: isLatest ? C.accent : C.muted, fontWeight: isLatest ? 700 : 400, whiteSpace: 'nowrap' }}>
                          {fmtSec(d.time)}
                        </p>
                        <div style={{
                          width: '100%', height: `${barH}px`,
                          background: isLatest ? C.accent : `rgba(0,212,170,${0.15 + (i / eventData.length) * 0.35})`,
                          borderRadius: '4px 4px 0 0',
                          boxShadow: isLatest ? '0 0 16px rgba(0,212,170,0.35)' : 'none',
                          position: 'relative', zIndex: 1,
                          transition: 'height 600ms cubic-bezier(0.16,1,0.3,1)',
                        }} />
                      </div>
                    )
                  })}
                </div>
                <div style={{ display: 'flex', gap: '8px', marginTop: '8px', borderTop: `1px solid ${C.border}`, paddingTop: '8px' }}>
                  {eventData.map((d) => (
                    <div key={d.date} style={{ flex: 1, textAlign: 'center' }}>
                      <p style={{ fontSize: '9px', fontFamily: 'var(--font-mono)', color: C.muted }}>{d.date}</p>
                    </div>
                  ))}
                </div>
                {/* Projection note */}
                {eventData.length >= 3 && improvement > 0 && (
                  <div style={{ marginTop: '14px', padding: '10px 12px', background: 'rgba(0,212,170,0.06)', borderRadius: '8px', border: `1px solid ${C.border}` }}>
                    <p style={{ fontSize: '11px', color: C.subtle }}>
                      At your current rate of improvement <strong style={{ color: C.accent }}>({(improvement / eventData.length).toFixed(2)}s/period)</strong>,
                      you could reach {goalSec ? <strong style={{ color: '#F59E0B' }}>{fmtSec(goalSec)} (your goal)</strong> : 'your next cut'} in approximately {goalSec && toGoal && toGoal > 0 ? Math.ceil(toGoal / (improvement / eventData.length)) : '?'} more period(s).
                    </p>
                  </div>
                )}
              </div>
            )}

            {/* Table view */}
            {viewMode === 'table' && (
              <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '14px', overflow: 'hidden' }}>
                <div style={{ display: 'grid', gridTemplateColumns: '100px 100px 1fr 1fr', padding: '10px 18px', borderBottom: `1px solid ${C.border}` }}>
                  {['Period', 'Time', 'Drop', 'Meet'].map((h) => (
                    <span key={h} style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>{h}</span>
                  ))}
                </div>
                {[...eventData].reverse().map((d, i) => {
                  const prev = [...eventData].reverse()[i + 1]
                  const drop = prev ? prev.time - d.time : null
                  const isLatest = i === 0
                  return (
                    <div key={d.date} style={{ display: 'grid', gridTemplateColumns: '100px 100px 1fr 1fr', padding: '12px 18px', borderBottom: i < eventData.length - 1 ? `1px solid ${C.border}` : 'none', background: isLatest ? 'rgba(0,212,170,0.04)' : 'transparent' }}>
                      <span style={{ fontSize: '12px', color: isLatest ? C.accent : C.subtle, fontFamily: 'var(--font-mono)' }}>{d.date}</span>
                      <span style={{ fontSize: '14px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: isLatest ? C.accent : C.text }}>{fmtSec(d.time)}</span>
                      <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: drop !== null && drop > 0 ? C.accent : drop !== null ? '#ef4444' : C.muted }}>
                        {drop !== null ? (drop > 0 ? `-${drop.toFixed(2)}s` : `+${Math.abs(drop).toFixed(2)}s`) : '—'}
                      </span>
                      <span style={{ fontSize: '11px', color: C.muted, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{d.meet ?? '—'}</span>
                    </div>
                  )
                })}
              </div>
            )}

            {/* Monthly view */}
            {viewMode === 'monthly' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
                {Object.entries(monthMap).map(([month, times]) => {
                  const best = Math.min(...times)
                  const worst = Math.max(...times)
                  return (
                    <div key={month} style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '12px', padding: '16px 20px', display: 'flex', alignItems: 'center', gap: '16px' }}>
                      <div style={{ width: '72px', flexShrink: 0 }}>
                        <span style={{ fontSize: '13px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: C.subtle }}>{month}</span>
                      </div>
                      <div style={{ flex: 1 }}>
                        <ProgressionBar value={1 - (best - minTime) / (maxTime - minTime)} max={1} color={C.accent} />
                      </div>
                      <div style={{ textAlign: 'right', flexShrink: 0 }}>
                        <div style={{ fontFamily: 'var(--font-mono)', fontWeight: 700, color: C.accent, fontSize: '15px' }}>{fmtSec(best)}</div>
                        {times.length > 1 && <div style={{ fontSize: '10px', color: C.muted, fontFamily: 'var(--font-mono)' }}>range: {fmtSec(best)} – {fmtSec(worst)}</div>}
                        <div style={{ fontSize: '10px', color: C.muted }}>{times.length} swims</div>
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        )
      })()}
    </div>
  )
}
