'use client'

import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchSwimmers,
  fetchSwimmerBestTimes,
  type BestTimeRecord,
} from '@/lib/adminData'

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

function parseGoalTimeInput(value: string): number | null {
  const trimmed = value.trim()
  if (!trimmed) return null
  const secondsOnly = /^\d{1,2}(\.\d{1,2})?$/
  const minutesSeconds = /^\d+:\d{2}(\.\d{1,2})?$/
  if (secondsOnly.test(trimmed)) return Number.parseFloat(trimmed)
  if (minutesSeconds.test(trimmed)) {
    const [minutes, seconds] = trimmed.split(':')
    return Number.parseInt(minutes, 10) * 60 + Number.parseFloat(seconds)
  }
  return Number.NaN
}

function fmtSec(s: number): string {
  if (s < 60) return s.toFixed(2)
  const m = Math.floor(s / 60)
  const rem = (s % 60).toFixed(2).padStart(5, '0')
  return `${m}:${rem}`
}

// ── Types ──────────────────────────────────────────────────────────────────
type Course = 'SCY' | 'SCM' | 'LCM'

type PivotEntry = {
  time: string
  date: string
  ms: number
}

type BestTimeRow = {
  event: string
  scy?: PivotEntry
  scm?: PivotEntry
  lcm?: PivotEntry
}

// Build pivoted table from BestTimeRecord[]
function pivotBestTimes(records: BestTimeRecord[]): BestTimeRow[] {
  const map = new Map<string, BestTimeRow>()
  for (const r of records) {
    if (!r.best_time_ms) continue
    const ev = eventName(r.event_distance, r.stroke)
    if (!map.has(ev)) map.set(ev, { event: ev })
    const row = map.get(ev)!
    const course = r.course?.toUpperCase() as Course
    const entry: PivotEntry = {
      time: fmtTime(r.best_time_ms),
      date: r.recorded_at ? new Date(r.recorded_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : '',
      ms: r.best_time_ms,
    }
    if (course === 'SCY') row.scy = entry
    else if (course === 'SCM') row.scm = entry
    else if (course === 'LCM') row.lcm = entry
  }
  return Array.from(map.values())
}

// Build progression data from BestTimeRecord[] for a given event+course key
function buildProgressionData(records: BestTimeRecord[], eventKey: string): Array<{ date: string; time: number; meet?: string }> {
  const courseMatch = /(SCY|SCM|LCM)$/.exec(eventKey)
  if (!courseMatch) return []
  const course = courseMatch[1].toLowerCase()
  const filteredEvent = eventKey.replace(/ (SCY|SCM|LCM)$/, '')

  return records
    .filter(r => {
      const ev = eventName(r.event_distance, r.stroke)
      const c = r.course?.toUpperCase()
      return ev === filteredEvent && c === course.toUpperCase() && r.best_time_ms
    })
    .sort((a, b) => {
      const da = a.recorded_at ? new Date(a.recorded_at).getTime() : 0
      const db = b.recorded_at ? new Date(b.recorded_at).getTime() : 0
      return da - db
    })
    .map(r => ({
      date: r.recorded_at ? new Date(r.recorded_at).toLocaleDateString('en-US', { month: 'short', year: '2-digit' }) : '',
      time: r.best_time_ms! / 1000,
    }))
}

// ── Skeleton ───────────────────────────────────────────────────────────────
function SkeletonLine({ width = '100%', height = '14px' }: { width?: string; height?: string }) {
  return (
    <div style={{ width, height, background: 'rgba(0,212,170,0.08)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite' }} />
  )
}

// ── Components ─────────────────────────────────────────────────────────────
function CourseCell({ entry }: { entry?: PivotEntry }) {
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
      </div>
      {entry.date && (
        <p style={{ fontSize: '10.5px', color: C.muted }}>{entry.date}</p>
      )}
    </div>
  )
}

function ProgressionBar({ value, max, color }: { value: number; max: number; color: string }) {
  const pct = Math.min(100, (value / max) * 100)
  return (
    <div style={{ height: '6px', background: 'rgba(0,212,170,0.10)', borderRadius: '999px', overflow: 'hidden' }}>
      <div style={{ height: '100%', width: `${pct}%`, background: color, borderRadius: '999px', transition: 'width 600ms cubic-bezier(0.16,1,0.3,1)' }} />
    </div>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────
export default function PerformancePage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [rawRecords, setRawRecords] = useState<BestTimeRecord[]>([])
  const [loading, setLoading] = useState(true)

  const [courseFilter, setCourseFilter] = useState<Course>('SCY')
  const [tab, setTab] = useState<'times' | 'cuts' | 'progression'>('times')
  const [progressEvent, setProgressEvent] = useState('')
  const [goalTime, setGoalTime] = useState<Record<string, string>>({})
  const [goalInput, setGoalInput] = useState<Record<string, string>>({})
  const [editingGoal, setEditingGoal] = useState(false)
  const [goalError, setGoalError] = useState<string | null>(null)
  const [viewMode, setViewMode] = useState<'chart' | 'table' | 'monthly'>('chart')

  useEffect(() => {
    if (!orgId) { setLoading(false); return }
    fetchSwimmers(api, orgId).then(async (swimmers) => {
      const mySwimmer = householdId
        ? (swimmers.find(s => s.household_id === householdId) ?? null)
        : null
      if (!mySwimmer) { setLoading(false); return }
      const records = await fetchSwimmerBestTimes(api, orgId, mySwimmer.id)
      setRawRecords(records)
      // Set default progression event to first SCY record
      const firstSCY = records.find(r => r.course?.toUpperCase() === 'SCY' && r.best_time_ms)
      if (firstSCY) {
        const key = `${eventName(firstSCY.event_distance, firstSCY.stroke)} SCY`
        setProgressEvent(key)
      }
    }).catch(console.error).finally(() => setLoading(false))
  }, [api, orgId, householdId])

  const pivotedRows = pivotBestTimes(rawRecords)

  // Build progression event keys (unique event+course combos with data)
  const progressionKeys: string[] = Array.from(
    new Set(
      rawRecords
        .filter(r => r.best_time_ms)
        .map(r => `${eventName(r.event_distance, r.stroke)} ${r.course?.toUpperCase()}`)
    )
  )

  const eventData = progressEvent ? buildProgressionData(rawRecords, progressEvent) : []
  const allTimes = eventData.map(d => d.time)
  const minTime = allTimes.length ? Math.min(...allTimes) * 0.98 : 0
  const maxTime = allTimes.length ? Math.max(...allTimes) * 1.01 : 1
  const latestTime = allTimes[allTimes.length - 1] ?? 0
  const improvement = allTimes.length > 1 ? allTimes[0] - latestTime : 0

  const goalSec = (() => {
    const parsed = parseGoalTimeInput(goalTime[progressEvent] ?? '')
    return parsed === null || Number.isNaN(parsed) ? null : parsed
  })()
  const toGoal = goalSec ? latestTime - goalSec : null

  // Monthly grouping
  const monthMap: Record<string, number[]> = {}
  eventData.forEach(d => {
    if (!monthMap[d.date]) monthMap[d.date] = []
    monthMap[d.date].push(d.time)
  })

  return (
    <div>
      <style>{`
        @keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.45} }
        @keyframes slideUp { from{opacity:0;transform:translateY(12px)} to{opacity:1;transform:translateY(0)} }
      `}</style>

      {/* Page header */}
      <div style={{ marginBottom: '32px', animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) both' }}>
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
          <div style={{ display: 'flex', gap: '8px', marginBottom: '20px' }}>
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
          <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '14px', overflow: 'hidden' }}>
            {/* Header */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', padding: '12px 20px', borderBottom: `1px solid ${C.border}` }}>
              {['Event', 'SCY', 'SCM', 'LCM'].map((h) => (
                <span key={h} style={{ fontSize: '10.5px', fontFamily: 'var(--font-mono)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  {h}
                </span>
              ))}
            </div>

            {/* Rows */}
            {loading ? (
              <div style={{ padding: '20px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
                {[1,2,3,4].map(i => <SkeletonLine key={i} />)}
              </div>
            ) : pivotedRows.length === 0 ? (
              <div style={{ padding: '40px 20px', textAlign: 'center' }}>
                <p style={{ fontSize: '14px', color: C.muted }}>No best times recorded yet.</p>
              </div>
            ) : (
              pivotedRows.map((bt, i) => (
                <div
                  key={bt.event}
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '1fr 1fr 1fr 1fr',
                    padding: '14px 20px',
                    borderBottom: i < pivotedRows.length - 1 ? `1px solid ${C.border}` : 'none',
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
              ))
            )}
          </div>

          {/* Standard legend */}
          <div style={{ display: 'flex', gap: '16px', marginTop: '14px', flexWrap: 'wrap' }}>
            {Object.entries(STANDARD_COLOR).map(([std, color]) => (
              <div key={std} style={{ display: 'flex', alignItems: 'center', gap: '5px' }}>
                <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', fontWeight: 700, color, background: `${color}18`, padding: '2px 7px', borderRadius: '4px' }}>
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
        <div style={{ animation: 'slideUp 0.25s cubic-bezier(0.16,1,0.3,1) both' }}>
          <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '12px', padding: '32px 20px', textAlign: 'center' }}>
            <Icon name="zap" size={28} style={{ color: C.muted, marginBottom: '12px' }} />
            <p style={{ fontSize: '15px', color: C.subtle, fontWeight: 500, marginBottom: '6px' }}>Cut qualifications coming soon</p>
            <p style={{ fontSize: '13px', color: C.muted, lineHeight: 1.5 }}>
              Time standard tables are managed by your coach. Check back after your next meet.
            </p>
          </div>
        </div>
      )}

      {/* ── PROGRESSION TAB ────────────────────────────────────────── */}
      {tab === 'progression' && (
        <div style={{ animation: 'slideUp 0.25s cubic-bezier(0.16,1,0.3,1) both' }}>
          {loading ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {[1,2,3].map(i => <SkeletonLine key={i} />)}
            </div>
          ) : progressionKeys.length === 0 ? (
            <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '12px', padding: '40px 20px', textAlign: 'center' }}>
              <p style={{ fontSize: '14px', color: C.muted }}>No progression data available yet.</p>
            </div>
          ) : (
            <>
              {/* Event picker + view mode */}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px', flexWrap: 'wrap', gap: '10px' }}>
                <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
                  {progressionKeys.map((ev) => (
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

              {/* Stats strip */}
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', gap: '10px', marginBottom: '20px' }}>
                {[
                  { label: 'Current Best', value: latestTime ? fmtSec(latestTime) : '—', color: C.accent, sub: eventData[eventData.length - 1]?.meet ?? '' },
                  { label: 'Total Drop', value: improvement > 0 ? `-${improvement.toFixed(2)}s` : '—', color: improvement > 0 ? C.accent : C.muted, sub: `over ${eventData.length} data points` },
                  { label: 'My Goal', value: goalSec ? fmtSec(goalSec) : 'Not set', color: goalSec ? '#F59E0B' : C.muted, sub: toGoal !== null ? (toGoal > 0 ? `${toGoal.toFixed(2)}s to go` : `Beat by ${Math.abs(toGoal).toFixed(2)}s!`) : 'Tap to set a goal' },
                  { label: 'Data Points', value: String(eventData.length), color: C.subtle, sub: 'recorded swims' },
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
                          if (parsed === null || Number.isNaN(parsed)) { setGoalError('Enter goal time as SS.ss or M:SS.ss.'); return }
                          setGoalTime({ ...goalTime, [progressEvent]: (goalInput[progressEvent] ?? '').trim() })
                          setEditingGoal(false)
                          setGoalError(null)
                        }}
                        style={{ fontSize: '11px', padding: '5px 12px', background: C.accent, color: '#061623', border: 'none', borderRadius: '6px', cursor: 'pointer', fontWeight: 700 }}
                      >Save</button>
                    </div>
                  ) : (
                    <p style={{ fontSize: '11px', color: C.muted }}>{goalTime[progressEvent] ? `Target: ${goalTime[progressEvent]}` : 'No goal set for this event.'}</p>
                  )}
                  {goalError && <p style={{ fontSize: '11px', color: '#fca5a5', marginTop: '6px' }}>{goalError}</p>}
                </div>
                {!editingGoal && (
                  <button
                    onClick={() => { setGoalInput({ ...goalInput, [progressEvent]: goalTime[progressEvent] ?? '' }); setGoalError(null); setEditingGoal(true) }}
                    style={{ fontSize: '11px', padding: '5px 12px', background: 'rgba(245,158,11,0.15)', color: '#F59E0B', border: '1px solid rgba(245,158,11,0.30)', borderRadius: '8px', cursor: 'pointer', fontWeight: 600 }}
                  >
                    {goalTime[progressEvent] ? 'Edit Goal' : 'Set Goal'}
                  </button>
                )}
              </div>

              {eventData.length === 0 ? (
                <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '12px', padding: '32px 20px', textAlign: 'center' }}>
                  <p style={{ fontSize: '13px', color: C.muted }}>No data points for this event yet.</p>
                </div>
              ) : (
                <>
                  {/* Chart view */}
                  {viewMode === 'chart' && (
                    <div style={{ background: C.card, border: `1px solid ${C.border}`, borderRadius: '14px', padding: '24px', marginBottom: '20px' }}>
                      <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '20px' }}>
                        {progressEvent} — Time Progression
                      </p>
                      <div style={{ display: 'flex', alignItems: 'flex-end', gap: '8px', height: '140px', position: 'relative' }}>
                        {goalSec && (
                          <div style={{
                            position: 'absolute',
                            bottom: `${((1 - (goalSec - minTime) / (maxTime - minTime)) * 140)}px`,
                            left: 0, right: 0, height: '1px', borderTop: '1px dashed #F59E0B', zIndex: 2,
                          }}>
                            <span style={{ position: 'absolute', right: 0, top: '-14px', fontSize: '9px', color: '#F59E0B', fontFamily: 'var(--font-mono)', background: C.card, padding: '0 4px' }}>goal</span>
                          </div>
                        )}
                        {eventData.map((d, i) => {
                          const normalized = 1 - (d.time - minTime) / (maxTime - minTime)
                          const barH = Math.max(20, normalized * 130)
                          const isLatest = i === eventData.length - 1
                          return (
                            <div key={`${d.date}-${i}`} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '4px' }}>
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
                        {eventData.map((d, i) => (
                          <div key={`label-${i}`} style={{ flex: 1, textAlign: 'center' }}>
                            <p style={{ fontSize: '9px', fontFamily: 'var(--font-mono)', color: C.muted }}>{d.date}</p>
                          </div>
                        ))}
                      </div>
                      {eventData.length >= 3 && improvement > 0 && (
                        <div style={{ marginTop: '14px', padding: '10px 12px', background: 'rgba(0,212,170,0.06)', borderRadius: '8px', border: `1px solid ${C.border}` }}>
                          <p style={{ fontSize: '11px', color: C.subtle }}>
                            At your current rate of improvement <strong style={{ color: C.accent }}>({(improvement / eventData.length).toFixed(2)}s/period)</strong>,
                            you could reach {goalSec ? <strong style={{ color: '#F59E0B' }}>{fmtSec(goalSec)} (your goal)</strong> : 'your next milestone'} in approximately {goalSec && toGoal && toGoal > 0 ? Math.ceil(toGoal / (improvement / eventData.length)) : '?'} more period(s).
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
                          <div key={`tbl-${i}`} style={{ display: 'grid', gridTemplateColumns: '100px 100px 1fr 1fr', padding: '12px 18px', borderBottom: i < eventData.length - 1 ? `1px solid ${C.border}` : 'none', background: isLatest ? 'rgba(0,212,170,0.04)' : 'transparent' }}>
                            <span style={{ fontSize: '12px', color: isLatest ? C.accent : C.subtle, fontFamily: 'var(--font-mono)' }}>{d.date}</span>
                            <span style={{ fontSize: '14px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: isLatest ? C.accent : C.text }}>{fmtSec(d.time)}</span>
                            <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: drop !== null && drop > 0 ? C.accent : drop !== null ? '#ef4444' : C.muted }}>
                              {drop !== null ? (drop > 0 ? `-${drop.toFixed(2)}s` : `+${Math.abs(drop).toFixed(2)}s`) : '—'}
                            </span>
                            <span style={{ fontSize: '11px', color: C.muted }}>{d.meet ?? '—'}</span>
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
                </>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
