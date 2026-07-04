'use client'

import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { UnlinkedSwimmerNotice } from '@/components/swimmer/UnlinkedSwimmerNotice'
import {
  fetchSwimmerCutQualification,
  fetchSwimmers,
  fetchSwimmerBestTimes,
  fetchTimeStandards,
  type BestTimeRecord,
} from '@/lib/adminData'
import {
  filterApplicableTimeStandards,
  sortCutQualificationCards,
  type SwimmerCutQualificationCard,
} from '@/lib/swimmer-cut-qualifications'
import {
  buildSwimmerCutsViewModel,
  describeCutQualification,
} from '@/lib/swimmer-performance-cuts'

// ── Design tokens ──────────────────────────────────────────────────────────
const C = {
  accent:      'var(--accent)',
  accentDim:   'var(--border)',
  accentBorder:'var(--accent-border)',
  text:        'var(--text)',
  subtle:      'var(--text-subtle)',
  muted:       'var(--text-muted)',
  card:        'var(--bg-surface)',
  border:      'var(--border)',
}

const STANDARD_COLOR: Record<string, string> = {
  AAA: 'var(--accent)',
  AA:  '#3b82f6',
  A:   '#8b5cf6',
  BB:  'var(--warning)',
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

function toneColors(tone: 'success' | 'warning' | 'muted') {
  switch (tone) {
    case 'success':
      return {
        text: C.accent,
        bg: 'var(--border)',
        border: 'var(--accent-border)',
      }
    case 'warning':
      return {
        text: 'var(--warning)',
        bg: 'var(--warning-bg)',
        border: 'var(--warning-border)',
      }
    default:
      return {
        text: C.subtle,
        bg: 'rgba(127,168,201,0.10)',
        border: 'var(--text-subtle)',
      }
  }
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
    <div style={{ width, height, background: 'var(--border)', borderRadius: '6px', animation: 'pulse 1.5s ease-in-out infinite' }} />
  )
}

// ── Components ─────────────────────────────────────────────────────────────
function CourseCell({ entry }: { entry?: PivotEntry }) {
  if (!entry) {
    return (
      <div style={{ textAlign: 'center' }}>
        <span style={{ fontSize: '13px', color: C.muted, fontFamily: 'var(--font-body)' }}>—</span>
      </div>
    )
  }
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '7px', marginBottom: '2px' }}>
        <span style={{ fontSize: '15px', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.accent }}>
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
    <div style={{ height: '6px', background: 'var(--border)', borderRadius: '999px', overflow: 'hidden' }}>
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
  const swimmerId = session?.user.swimmerId ?? ''

  const [rawRecords, setRawRecords] = useState<BestTimeRecord[]>([])
  const [cutCards, setCutCards] = useState<SwimmerCutQualificationCard[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [cutLoading, setCutLoading] = useState(true)
  const [cutError, setCutError] = useState<string | null>(null)

  const [courseFilter, setCourseFilter] = useState<Course>('SCY')
  const [tab, setTab] = useState<'times' | 'cuts' | 'progression'>('times')
  const [progressEvent, setProgressEvent] = useState('')
  const [goalTime, setGoalTime] = useState<Record<string, string>>({})
  const [goalInput, setGoalInput] = useState<Record<string, string>>({})
  const [editingGoal, setEditingGoal] = useState(false)
  const [goalError, setGoalError] = useState<string | null>(null)
  const [viewMode, setViewMode] = useState<'chart' | 'table' | 'monthly'>('chart')
  const [retryKey, setRetryKey] = useState(0)
  const [noLinkedSwimmer, setNoLinkedSwimmer] = useState(false)

  useEffect(() => {
    if (!orgId || !householdId) {
      setLoadError('Swimmer performance is unavailable because household context is missing.')
      setLoading(false)
      setCutLoading(false)
      return
    }
    if (!swimmerId) {
      setNoLinkedSwimmer(true)
      setLoading(false)
      setCutLoading(false)
      return
    }
    setNoLinkedSwimmer(false)

    let cancelled = false

    async function loadPerformance() {
      setLoading(true)
      setCutLoading(true)
      setLoadError(null)
      setCutError(null)
      setRawRecords([])
      setCutCards([])

      try {
        const swimmers = await fetchSwimmers(api, orgId)
        if (cancelled) return

        const mySwimmer = swimmers.find((swimmer) => swimmer.id === swimmerId) ?? null
        if (!mySwimmer) {
          setLoadError('Swimmer performance is unavailable because your linked swimmer record could not be loaded.')
          return
        }

        const [records, standards] = await Promise.all([
          fetchSwimmerBestTimes(api, orgId, mySwimmer.id),
          fetchTimeStandards(api, orgId, { active_only: 'true' }),
        ])
        if (cancelled) return

        setRawRecords(records)

        const firstSCY = records.find((record) => record.course?.toUpperCase() === 'SCY' && record.best_time_ms)
        if (firstSCY) {
          const key = `${eventName(firstSCY.event_distance, firstSCY.stroke)} SCY`
          setProgressEvent(key)
        }

        const applicableStandards = filterApplicableTimeStandards(standards, mySwimmer)
        if (applicableStandards.length === 0) {
          setCutCards([])
          return
        }

        try {
          const qualifications = await Promise.all(
            applicableStandards.map(async (standard) => ({
              standard,
              qualification: await fetchSwimmerCutQualification(api, orgId, mySwimmer.id, standard.id),
            })),
          )
          if (cancelled) return
          setCutCards(sortCutQualificationCards(qualifications))
        } catch (error) {
          if (cancelled) return
          setCutCards([])
          setCutError(reportUIError(
            'swimmer.performance.cuts.load',
            error,
            'Failed to load cut qualifications.',
            { orgId, householdId, swimmerId: mySwimmer.id },
          ))
        }
      } catch (error) {
        if (cancelled) return
        reportUIError(
          'swimmer.performance.load',
          error,
          'Unable to load swimmer performance right now. Please try again.',
          { orgId, householdId },
        )
        setLoadError('Unable to load swimmer performance right now. Please try again.')
      } finally {
        if (!cancelled) {
          setLoading(false)
          setCutLoading(false)
        }
      }
    }

    void loadPerformance()

    return () => {
      cancelled = true
    }
  }, [api, orgId, householdId, swimmerId, retryKey])

  const pivotedRows = pivotBestTimes(rawRecords)
  const cutsView = buildSwimmerCutsViewModel({
    loading: cutLoading,
    error: cutError,
    cards: cutCards,
  })

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

  if (!loading && noLinkedSwimmer) {
    return (
      <div>
        <style>{`
          @keyframes slideUp { from{opacity:0;transform:translateY(12px)} to{opacity:1;transform:translateY(0)} }
        `}</style>
        <div style={{ marginBottom: '32px', animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) both' }}>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: 'clamp(26px, 4vw, 34px)', color: C.text, letterSpacing: '-0.02em', lineHeight: 1.1, marginBottom: '6px' }}>
            Performance
          </h1>
          <p style={{ fontSize: '14px', color: C.subtle }}>
            Best times, cut qualifications, and progression history.
          </p>
        </div>
        <UnlinkedSwimmerNotice />
      </div>
    )
  }

  if (!loading && loadError) {
    return (
      <div>
        <style>{`
          @keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.45} }
          @keyframes slideUp { from{opacity:0;transform:translateY(12px)} to{opacity:1;transform:translateY(0)} }
        `}</style>
        <div style={{ marginBottom: '32px', animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) both' }}>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: 'clamp(26px, 4vw, 34px)', color: C.text, letterSpacing: '-0.02em', lineHeight: 1.1, marginBottom: '6px' }}>
            Performance
          </h1>
          <p style={{ fontSize: '14px', color: C.subtle }}>
            Best times, cut qualifications, and progression history.
          </p>
        </div>
        <div className="card" style={{ padding: '24px 22px' }}>
          <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--error)', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '8px' }}>
            Swimmer Performance
          </div>
          <p style={{ fontSize: '18px', color: C.text, fontWeight: 600, marginBottom: '8px' }}>
            Performance data is unavailable right now.
          </p>
          <p style={{ fontSize: '13px', color: C.subtle, lineHeight: 1.6, marginBottom: '16px' }}>
            {loadError}
          </p>
          <button
            onClick={() => setRetryKey((current) => current + 1)}
            style={{ padding: '10px 16px', borderRadius: '9px', border: 'none', background: 'var(--error)', color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer' }}
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

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
                  fontFamily: 'var(--font-body)',
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
          <div className="card" style={{ overflow: 'hidden' }}>
            {/* Header */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', padding: '12px 20px', borderBottom: `1px solid ${C.border}` }}>
              {['Event', 'SCY', 'SCM', 'LCM'].map((h) => (
                <span key={h} style={{ fontSize: '10.5px', fontFamily: 'var(--font-body)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
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
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--border)')}
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
                <span style={{ fontSize: '10px', fontFamily: 'var(--font-body)', fontWeight: 700, color, background: `${color}18`, padding: '2px 7px', borderRadius: '4px' }}>
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
          {(cutsView.state === 'loading' || cutsView.state === 'error' || cutsView.state === 'empty') && (
            <div className="card" style={{ padding: '32px 20px', textAlign: 'center' }}>
              <Icon name={cutsView.state === 'error' ? 'alert-circle' : 'zap'} size={28} style={{ color: cutsView.state === 'error' ? 'var(--warning)' : C.muted, marginBottom: '12px' }} />
              <p style={{ fontSize: '15px', color: C.subtle, fontWeight: 500, marginBottom: '6px' }}>{cutsView.title}</p>
              <p style={{ fontSize: '13px', color: C.muted, lineHeight: 1.5 }}>
                {cutsView.message}
              </p>
            </div>
          )}

          {cutsView.state === 'ready' && (
            <>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: '12px', marginBottom: '20px' }}>
                {[
                  { label: 'Qualified', value: String(cutsView.qualifiedCount), accent: C.accent },
                  { label: 'Active Standards', value: String(cutsView.totalCount), accent: C.text },
                  { label: 'Still Chasing', value: String(cutsView.totalCount - cutsView.qualifiedCount), accent: 'var(--warning)' },
                ].map((stat) => (
                  <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
                    <div style={{ fontSize: '10px', textTransform: 'uppercase', letterSpacing: '0.05em', color: C.muted, fontFamily: 'var(--font-body)', marginBottom: '6px' }}>{stat.label}</div>
                    <div style={{ fontSize: '20px', fontFamily: 'var(--font-body)', fontWeight: 700, color: stat.accent }}>{stat.value}</div>
                  </div>
                ))}
              </div>

              <div style={{ marginBottom: '16px', background: 'rgba(0,212,170,0.05)', border: `1px solid ${C.border}`, borderRadius: '12px', padding: '14px 16px' }}>
                <p style={{ fontSize: '14px', color: C.subtle, fontWeight: 600, marginBottom: '4px' }}>{cutsView.title}</p>
                <p style={{ fontSize: '12px', color: C.muted, lineHeight: 1.5 }}>{cutsView.message}</p>
              </div>

              <div style={{ display: 'grid', gap: '12px' }}>
                {cutCards.map((card) => {
                  const status = describeCutQualification(card)
                  const colors = toneColors(status.tone)
                  const standard = card.standard
                  const qualification = card.qualification
                  return (
                    <div key={standard.id} style={{ background: C.card, border: `1px solid ${colors.border}`, borderRadius: '14px', padding: '16px 18px' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'flex-start', flexWrap: 'wrap', marginBottom: '10px' }}>
                        <div>
                          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap', marginBottom: '6px' }}>
                            <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', fontWeight: 700, color: colors.text, background: colors.bg, border: `1px solid ${colors.border}`, padding: '3px 8px', borderRadius: '999px' }}>
                              {status.badge}
                            </span>
                            <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.subtle }}>
                              {standard.standard_code}
                            </span>
                          </div>
                          <p style={{ fontSize: '16px', fontWeight: 700, color: C.text, marginBottom: '4px' }}>
                            {standard.event_distance} {standard.stroke.charAt(0).toUpperCase() + standard.stroke.slice(1)} {standard.course}
                          </p>
                          <p style={{ fontSize: '12px', color: C.muted, lineHeight: 1.5 }}>
                            {status.detail}
                          </p>
                        </div>
                        <div style={{ minWidth: '170px', textAlign: 'right' }}>
                          <div style={{ fontSize: '10px', textTransform: 'uppercase', letterSpacing: '0.05em', color: C.muted, fontFamily: 'var(--font-body)', marginBottom: '4px' }}>Cut Time</div>
                          <div style={{ fontSize: '18px', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.text, marginBottom: '8px' }}>
                            {fmtTime(standard.time_ms)}
                          </div>
                          <div style={{ fontSize: '10px', textTransform: 'uppercase', letterSpacing: '0.05em', color: C.muted, fontFamily: 'var(--font-body)', marginBottom: '4px' }}>Best Time</div>
                          <div style={{ fontSize: '15px', fontFamily: 'var(--font-body)', fontWeight: 700, color: qualification.best_time_ms != null ? colors.text : C.muted }}>
                            {qualification.best_time_ms != null ? fmtTime(qualification.best_time_ms) : 'NT'}
                          </div>
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </>
          )}
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
            <div className="card" style={{ padding: '40px 20px', textAlign: 'center' }}>
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
                        background: progressEvent === ev ? C.accent : 'var(--border)',
                        color: progressEvent === ev ? 'var(--bg)' : C.subtle,
                        border: `1px solid ${progressEvent === ev ? C.accent : C.border}`,
                        fontFamily: 'var(--font-body)', fontWeight: 600,
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
                        background: viewMode === m ? 'var(--accent-subtle)' : 'transparent',
                        color: viewMode === m ? C.accent : C.muted,
                        border: `1px solid ${viewMode === m ? C.accentBorder : C.border}`,
                        fontFamily: 'var(--font-body)', textTransform: 'capitalize',
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
                  { label: 'My Goal', value: goalSec ? fmtSec(goalSec) : 'Not set', color: goalSec ? 'var(--warning)' : C.muted, sub: toGoal !== null ? (toGoal > 0 ? `${toGoal.toFixed(2)}s to go` : `Beat by ${Math.abs(toGoal).toFixed(2)}s!`) : 'Tap to set a goal' },
                  { label: 'Data Points', value: String(eventData.length), color: C.subtle, sub: 'recorded swims' },
                ].map((s) => (
                  <div key={s.label} className="card" style={{ padding: '14px 16px' }}>
                    <div style={{ fontSize: '10px', textTransform: 'uppercase', letterSpacing: '0.05em', color: C.muted, fontFamily: 'var(--font-body)', marginBottom: '6px' }}>{s.label}</div>
                    <div style={{ fontSize: '18px', fontFamily: 'var(--font-body)', fontWeight: 700, color: s.color, marginBottom: '2px' }}>{s.value}</div>
                    <div style={{ fontSize: '10px', color: C.muted }}>{s.sub}</div>
                  </div>
                ))}
              </div>

              {/* Goal editor */}
              <div style={{ background: 'rgba(245,158,11,0.06)', border: '1px solid rgba(245,158,11,0.22)', borderRadius: '12px', padding: '14px 16px', marginBottom: '20px', display: 'flex', alignItems: 'center', gap: '12px' }}>
                <Icon name="zap" size={16} style={{ color: 'var(--warning)', flexShrink: 0 }} />
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
                        style={{ background: 'var(--border)', border: `1px solid ${C.border}`, borderRadius: '8px', padding: '5px 10px', fontSize: '12px', fontFamily: 'var(--font-body)', color: C.text, width: '160px' }}
                      />
                      <button
                        onClick={() => {
                          const parsed = parseGoalTimeInput(goalInput[progressEvent] ?? '')
                          if (parsed === null || Number.isNaN(parsed)) { setGoalError('Enter goal time as SS.ss or M:SS.ss.'); return }
                          setGoalTime({ ...goalTime, [progressEvent]: (goalInput[progressEvent] ?? '').trim() })
                          setEditingGoal(false)
                          setGoalError(null)
                        }}
                        className="btn btn-primary btn-sm"
                      >Save</button>
                    </div>
                  ) : (
                    <p style={{ fontSize: '11px', color: C.muted }}>{goalTime[progressEvent] ? `Target: ${goalTime[progressEvent]}` : 'No goal set for this event.'}</p>
                  )}
                  {goalError && <p style={{ fontSize: '11px', color: 'var(--error)', marginTop: '6px' }}>{goalError}</p>}
                </div>
                {!editingGoal && (
                  <button
                    onClick={() => { setGoalInput({ ...goalInput, [progressEvent]: goalTime[progressEvent] ?? '' }); setGoalError(null); setEditingGoal(true) }}
                    style={{ fontSize: '11px', padding: '5px 12px', background: 'rgba(245,158,11,0.15)', color: 'var(--warning)', border: '1px solid rgba(245,158,11,0.30)', borderRadius: '8px', cursor: 'pointer', fontWeight: 600 }}
                  >
                    {goalTime[progressEvent] ? 'Edit Goal' : 'Set Goal'}
                  </button>
                )}
              </div>

              {eventData.length === 0 ? (
                <div className="card" style={{ padding: '32px 20px', textAlign: 'center' }}>
                  <p style={{ fontSize: '13px', color: C.muted }}>No data points for this event yet.</p>
                </div>
              ) : (
                <>
                  {/* Chart view */}
                  {viewMode === 'chart' && (
                    <div className="card" style={{ padding: '24px', marginBottom: '20px' }}>
                      <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '20px' }}>
                        {progressEvent} — Time Progression
                      </p>
                      <div style={{ display: 'flex', alignItems: 'flex-end', gap: '8px', height: '140px', position: 'relative' }}>
                        {goalSec && (
                          <div style={{
                            position: 'absolute',
                            bottom: `${((1 - (goalSec - minTime) / (maxTime - minTime)) * 140)}px`,
                            left: 0, right: 0, height: '1px', borderTop: '1px dashed #F59E0B', zIndex: 2,
                          }}>
                            <span style={{ position: 'absolute', right: 0, top: '-14px', fontSize: '9px', color: 'var(--warning)', fontFamily: 'var(--font-body)', background: C.card, padding: '0 4px' }}>goal</span>
                          </div>
                        )}
                        {eventData.map((d, i) => {
                          const normalized = 1 - (d.time - minTime) / (maxTime - minTime)
                          const barH = Math.max(20, normalized * 130)
                          const isLatest = i === eventData.length - 1
                          return (
                            <div key={`${d.date}-${i}`} style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '4px' }}>
                              <p style={{ fontSize: '9px', fontFamily: 'var(--font-body)', color: isLatest ? C.accent : C.muted, fontWeight: isLatest ? 700 : 400, whiteSpace: 'nowrap' }}>
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
                            <p style={{ fontSize: '9px', fontFamily: 'var(--font-body)', color: C.muted }}>{d.date}</p>
                          </div>
                        ))}
                      </div>
                      {eventData.length >= 3 && improvement > 0 && (
                        <div style={{ marginTop: '14px', padding: '10px 12px', background: 'var(--border)', borderRadius: '8px', border: `1px solid ${C.border}` }}>
                          <p style={{ fontSize: '11px', color: C.subtle }}>
                            At your current rate of improvement <strong style={{ color: C.accent }}>({(improvement / eventData.length).toFixed(2)}s/period)</strong>,
                            you could reach {goalSec ? <strong style={{ color: 'var(--warning)' }}>{fmtSec(goalSec)} (your goal)</strong> : 'your next milestone'} in approximately {goalSec && toGoal && toGoal > 0 ? Math.ceil(toGoal / (improvement / eventData.length)) : '?'} more period(s).
                          </p>
                        </div>
                      )}
                    </div>
                  )}

                  {/* Table view */}
                  {viewMode === 'table' && (
                    <div className="card" style={{ overflow: 'hidden' }}>
                      <div style={{ display: 'grid', gridTemplateColumns: '100px 100px 1fr 1fr', padding: '10px 18px', borderBottom: `1px solid ${C.border}` }}>
                        {['Period', 'Time', 'Drop', 'Meet'].map((h) => (
                          <span key={h} style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: C.muted, textTransform: 'uppercase', letterSpacing: '0.05em' }}>{h}</span>
                        ))}
                      </div>
                      {[...eventData].reverse().map((d, i) => {
                        const prev = [...eventData].reverse()[i + 1]
                        const drop = prev ? prev.time - d.time : null
                        const isLatest = i === 0
                        return (
                          <div key={`tbl-${i}`} style={{ display: 'grid', gridTemplateColumns: '100px 100px 1fr 1fr', padding: '12px 18px', borderBottom: i < eventData.length - 1 ? `1px solid ${C.border}` : 'none', background: isLatest ? 'var(--border)' : 'transparent' }}>
                            <span style={{ fontSize: '12px', color: isLatest ? C.accent : C.subtle, fontFamily: 'var(--font-body)' }}>{d.date}</span>
                            <span style={{ fontSize: '14px', fontFamily: 'var(--font-body)', fontWeight: 700, color: isLatest ? C.accent : C.text }}>{fmtSec(d.time)}</span>
                            <span style={{ fontSize: '12px', fontFamily: 'var(--font-body)', color: drop !== null && drop > 0 ? C.accent : drop !== null ? 'var(--error)' : C.muted }}>
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
                          <div key={month} className="card" style={{ padding: '16px 20px', display: 'flex', alignItems: 'center', gap: '16px' }}>
                            <div style={{ width: '72px', flexShrink: 0 }}>
                              <span style={{ fontSize: '13px', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.subtle }}>{month}</span>
                            </div>
                            <div style={{ flex: 1 }}>
                              <ProgressionBar value={1 - (best - minTime) / (maxTime - minTime)} max={1} color={C.accent} />
                            </div>
                            <div style={{ textAlign: 'right', flexShrink: 0 }}>
                              <div style={{ fontFamily: 'var(--font-body)', fontWeight: 700, color: C.accent, fontSize: '15px' }}>{fmtSec(best)}</div>
                              {times.length > 1 && <div style={{ fontSize: '10px', color: C.muted, fontFamily: 'var(--font-body)' }}>range: {fmtSec(best)} – {fmtSec(worst)}</div>}
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
