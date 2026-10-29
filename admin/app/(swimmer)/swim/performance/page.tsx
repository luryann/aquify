'use client'

import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { GoalsPanel } from '@/components/performance/GoalsPanel'
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
import './page.inline.css'
import { fetchSwimmerGoals, formatSwimTime, type Goal } from '@/lib/performanceData'
import { SkeletonLine } from '@/components/ui/SkeletonLine'

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

// ── Helpers ────────────────────────────────────────────────────────────────
function eventName(distance: number, stroke: string): string {
  const cap = stroke.charAt(0).toUpperCase() + stroke.slice(1)
  return `${distance} ${cap}`
}

/** Seconds as a swim-time label ("58.12", "1:02.34"). */
function fmtSec(s: number): string {
  return formatSwimTime(Math.round(s * 1000))
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
      time: formatSwimTime(r.best_time_ms),
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
// ── Components ─────────────────────────────────────────────────────────────
function CourseCell({ entry }: { entry?: PivotEntry }) {
  if (!entry) {
    return (
      <div style={{ textAlign: 'center' }}>
        <span style={{ fontSize: 'var(--fs-base)', color: C.muted, fontFamily: 'var(--font-body)' }}>—</span>
      </div>
    )
  }
  return (
    <div>
      <div className="is-92d0f5a">
        <span style={{ fontSize: 'var(--fs-md)', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.accent }}>
          {entry.time}
        </span>
      </div>
      {entry.date && (
        <p style={{ fontSize: 'var(--fs-2xs)', color: C.muted }}>{entry.date}</p>
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

  const [tab, setTab] = useState<'times' | 'cuts' | 'progression'>('times')
  const [progressEvent, setProgressEvent] = useState('')
  const [goals, setGoals] = useState<Goal[]>([])
  const [viewMode, setViewMode] = useState<'chart' | 'table' | 'monthly'>('chart')
  const [retryKey, setRetryKey] = useState(0)
  const [noLinkedSwimmer, setNoLinkedSwimmer] = useState(false)

  useEffect(() => {
    if (!orgId || !householdId) {
      setLoadError('We could not find your household details. Sign out and back in, or contact your club if this keeps happening.')
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
          setLoadError('We could not load this page. Check your connection and try again.')
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
            'Couldn’t load cut qualifications. Try again.',
            { orgId, householdId, swimmerId: mySwimmer.id },
          ))
        }
      } catch (error) {
        if (cancelled) return
        reportUIError(
          'swimmer.performance.load',
          error,
          'Couldn’t load swimmer performance. Try again.',
          { orgId, householdId },
        )
        setLoadError('Couldn’t load swimmer performance. Try again.')
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

  // Goals are saved through GoalsPanel below; re-read them whenever the
  // Progression tab opens so the goal line reflects changes made there.
  useEffect(() => {
    if (tab !== 'progression' || !orgId || !swimmerId) return
    let cancelled = false
    fetchSwimmerGoals(api, orgId, swimmerId)
      .then((next) => { if (!cancelled) setGoals(next) })
      .catch(() => { if (!cancelled) setGoals([]) })
    return () => { cancelled = true }
  }, [api, orgId, swimmerId, tab])

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

  const activeGoal = goals.find((g) => g.status === 'active' && `${eventName(g.distance, g.stroke)} ${g.course}` === progressEvent)
  const goalSec = activeGoal ? activeGoal.target_time_ms / 1000 : null
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
                <div className="is-bb27019">
          <h1 className="page-title">
            Performance
          </h1>
        </div>
        <UnlinkedSwimmerNotice />
      </div>
    )
  }

  if (!loading && loadError) {
    return (
      <div>
                <div className="is-bb27019">
          <h1 className="page-title">
            Performance
          </h1>
        </div>
        <div className="card is-653b4fd" >
          <p style={{ fontSize: 'var(--fs-xl)', color: C.text, fontWeight: 600, marginBottom: '8px' }}>
            We could not load this page.
          </p>
          <p style={{ fontSize: 'var(--fs-base)', color: C.subtle, lineHeight: 1.6, marginBottom: '16px' }}>
            {loadError}
          </p>
          <button type="button"
            onClick={() => setRetryKey((current) => current + 1)}
            className="is-b241e34"
          >
            Retry
          </button>
        </div>
      </div>
    )
  }

  return (
    <div>
      
      {/* Page header */}
      <div className="is-bb27019">
        <h1 className="page-title"
        >
          Performance
        </h1>
      </div>

      {/* Tabs */}
      <div
        className="tab-strip is-7eb09d9"
        
      >
        {[
          { key: 'times',       label: 'Best times' },
          { key: 'cuts',        label: 'Cut qualification' },
          { key: 'progression', label: 'Progression' },
        ].map(({ key, label }) => (
          <button type="button"
            key={key}
            onClick={() => setTab(key as typeof tab)}
            style={{
              padding: '8px 16px',
              borderRadius: '8px',
              border: 'none',
              whiteSpace: 'nowrap',
              flexShrink: 0,
              background: tab === key ? C.accentDim : 'transparent',
              color: tab === key ? C.accent : C.subtle,
              fontSize: 'var(--fs-base)',
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
          {/* Best times table */}
          <div className="card" style={{ overflow: 'hidden' }}>
            {/* Header */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', padding: '12px 20px', borderBottom: `1px solid ${C.border}` }}>
              {['Event', 'SCY', 'SCM', 'LCM'].map((h) => (
                <span key={h} style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: C.muted, }}>
                  {h}
                </span>
              ))}
            </div>

            {/* Rows */}
            {loading ? (
              <div className="is-824b1f4">
                {[1,2,3,4].map(i => <SkeletonLine key={i} />)}
              </div>
            ) : pivotedRows.length === 0 ? (
              <div className="is-91ef90a">
                <p style={{ fontSize: 'var(--fs-md)', color: C.muted }}>No best times recorded yet.</p>
              </div>
            ) : (
              pivotedRows.map((bt, i) => (
                <div
                  key={bt.event}
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '1fr 1fr 1fr 1fr',
                    padding: '16px 20px',
                    borderBottom: i < pivotedRows.length - 1 ? `1px solid ${C.border}` : 'none',
                    alignItems: 'center',
                    transition: 'background 100ms ease',
                    animationDelay: `${i * 30}ms`,
                    animation: 'slideUp 0.3s cubic-bezier(0.16,1,0.3,1) both',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--border)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <span style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: C.text }}>{bt.event}</span>
                  <CourseCell entry={bt.scy} />
                  <CourseCell entry={bt.scm} />
                  <CourseCell entry={bt.lcm} />
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* ── CUT QUALIFICATION TAB ──────────────────────────────────── */}
      {tab === 'cuts' && (
        <div style={{ animation: 'slideUp 0.25s cubic-bezier(0.16,1,0.3,1) both' }}>
          {(cutsView.state === 'loading' || cutsView.state === 'error' || cutsView.state === 'empty') && (
            <div className="card is-911be78" >
              <Icon name={cutsView.state === 'error' ? 'alert-circle' : 'zap'} size={28} style={{ color: cutsView.state === 'error' ? 'var(--warning)' : C.muted, marginBottom: '12px' }} />
              <p style={{ fontSize: 'var(--fs-md)', color: C.subtle, fontWeight: 500, marginBottom: '6px' }}>{cutsView.title}</p>
              <p style={{ fontSize: 'var(--fs-base)', color: C.muted, lineHeight: 1.5 }}>
                {cutsView.message}
              </p>
            </div>
          )}

          {cutsView.state === 'ready' && (
            <>
              <div className="cols-3 is-7e7f577" >
                {[
                  { label: 'Qualified', value: String(cutsView.qualifiedCount), accent: C.accent },
                  { label: 'Active standards', value: String(cutsView.totalCount), accent: C.text },
                  { label: 'Still chasing', value: String(cutsView.totalCount - cutsView.qualifiedCount), accent: 'var(--warning)' },
                ].map((stat) => (
                  <div key={stat.label} className="card is-e402fee" >
                    <div style={{ fontSize: 'var(--fs-sm)', color: C.muted, fontFamily: 'var(--font-body)', marginBottom: '6px' }}>{stat.label}</div>
                    <div style={{ fontSize: 'var(--fs-xl)', fontFamily: 'var(--font-body)', fontWeight: 700, color: stat.accent }}>{stat.value}</div>
                  </div>
                ))}
              </div>

              <div style={{ marginBottom: '16px', background: 'rgba(0,212,170,0.05)', border: `1px solid ${C.border}`, borderRadius: '12px', padding: '16px 16px' }}>
                <p style={{ fontSize: 'var(--fs-md)', color: C.subtle, fontWeight: 600, marginBottom: '4px' }}>{cutsView.title}</p>
                <p style={{ fontSize: 'var(--fs-sm)', color: C.muted, lineHeight: 1.5 }}>{cutsView.message}</p>
              </div>

              <div className="is-13fb4cd">
                {cutCards.map((card) => {
                  const status = describeCutQualification(card)
                  const colors = toneColors(status.tone)
                  const standard = card.standard
                  const qualification = card.qualification
                  return (
                    <div key={standard.id} style={{ background: C.card, border: `1px solid ${colors.border}`, borderRadius: '12px', padding: '16px 20px' }}>
                      <div className="is-a3c9e1b">
                        <div>
                          <div className="is-ebe0de7">
                            <span style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', fontWeight: 700, color: colors.text, background: colors.bg, border: `1px solid ${colors.border}`, padding: '4px 8px', borderRadius: '999px' }}>
                              {status.badge}
                            </span>
                            <span style={{ fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.subtle }}>
                              {standard.standard_code}
                            </span>
                          </div>
                          <p style={{ fontSize: 'var(--fs-lg)', fontWeight: 700, color: C.text, marginBottom: '4px' }}>
                            {standard.event_distance} {standard.stroke.charAt(0).toUpperCase() + standard.stroke.slice(1)} {standard.course}
                          </p>
                          <p style={{ fontSize: 'var(--fs-sm)', color: C.muted, lineHeight: 1.5 }}>
                            {status.detail}
                          </p>
                        </div>
                        <div style={{ minWidth: '170px', textAlign: 'right' }}>
                          <div style={{ fontSize: 'var(--fs-sm)', color: C.muted, fontFamily: 'var(--font-body)', marginBottom: '4px' }}>Cut time</div>
                          <div style={{ fontSize: 'var(--fs-xl)', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.text, marginBottom: '8px' }}>
                            {formatSwimTime(standard.time_ms)}
                          </div>
                          <div style={{ fontSize: 'var(--fs-sm)', color: C.muted, fontFamily: 'var(--font-body)', marginBottom: '4px' }}>Best time</div>
                          <div style={{ fontSize: 'var(--fs-md)', fontFamily: 'var(--font-body)', fontWeight: 700, color: qualification.best_time_ms != null ? colors.text : C.muted }}>
                            {qualification.best_time_ms != null ? formatSwimTime(qualification.best_time_ms) : 'No time yet'}
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
            <div className="is-930d87f">
              {[1,2,3].map(i => <SkeletonLine key={i} />)}
            </div>
          ) : progressionKeys.length === 0 ? (
            <div className="card is-91ef90a" >
              <p style={{ fontSize: 'var(--fs-md)', color: C.muted }}>No progression data available yet.</p>
            </div>
          ) : (
            <>
              {/* Event picker + view mode */}
              <div className="is-4cb2bc7">
                <div className="is-1a1daae">
                  {progressionKeys.map((ev) => (
                    <button type="button"
                      key={ev}
                      onClick={() => setProgressEvent(ev)}
                      style={{
                        fontSize: 'var(--fs-xs)', padding: '4px 12px', borderRadius: '16px', cursor: 'pointer',
                        background: progressEvent === ev ? C.accent : 'var(--border)',
                        color: progressEvent === ev ? 'var(--bg)' : C.subtle,
                        border: `1px solid ${progressEvent === ev ? C.accent : C.border}`,
                        fontFamily: 'var(--font-body)', fontWeight: 600,
                        transition: 'all 100ms ease',
                      }}
                    >{ev}</button>
                  ))}
                </div>
                <div className="is-d2f87c7">
                  {(['chart', 'table', 'monthly'] as const).map((m) => (
                    <button type="button"
                      key={m}
                      onClick={() => setViewMode(m)}
                      style={{
                        fontSize: 'var(--fs-xs)', padding: '4px 12px', borderRadius: '16px', cursor: 'pointer',
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
              <div className="cols-4 is-7e7f577" >
                {[
                  { label: 'Current best', value: latestTime ? fmtSec(latestTime) : '—', color: C.accent, sub: eventData[eventData.length - 1]?.meet ?? '' },
                  { label: 'Total drop', value: improvement > 0 ? `-${improvement.toFixed(2)}s` : '—', color: improvement > 0 ? C.accent : C.muted, sub: `over ${eventData.length} data points` },
                  { label: 'My goal', value: goalSec ? fmtSec(goalSec) : 'Not set', color: goalSec ? 'var(--warning)' : C.muted, sub: toGoal !== null ? (toGoal > 0 ? `${toGoal.toFixed(2)}s to go` : `Beat by ${Math.abs(toGoal).toFixed(2)}s!`) : 'Set one under Goals below' },
                  { label: 'Data points', value: String(eventData.length), color: C.subtle, sub: 'recorded swims' },
                ].map((s) => (
                  <div key={s.label} className="card is-e402fee" >
                    <div style={{ fontSize: 'var(--fs-sm)', color: C.muted, fontFamily: 'var(--font-body)', marginBottom: '6px' }}>{s.label}</div>
                    <div style={{ fontSize: 'var(--fs-xl)', fontFamily: 'var(--font-body)', fontWeight: 700, color: s.color, marginBottom: '2px' }}>{s.value}</div>
                    <div style={{ fontSize: 'var(--fs-2xs)', color: C.muted }}>{s.sub}</div>
                  </div>
                ))}
              </div>

              {eventData.length === 0 ? (
                <div className="card is-911be78" >
                  <p style={{ fontSize: 'var(--fs-base)', color: C.muted }}>No data points for this event yet.</p>
                </div>
              ) : (
                <>
                  {/* Chart view */}
                  {viewMode === 'chart' && (
                    <div className="card is-bb717d2" >
                      <div className="is-bbd2fae">
                        {goalSec && (
                          <div style={{
                            position: 'absolute',
                            bottom: `${((1 - (goalSec - minTime) / (maxTime - minTime)) * 140)}px`,
                            left: 0, right: 0, height: '1px', borderTop: '1px dashed #F59E0B', zIndex: 2,
                          }}>
                            <span style={{ position: 'absolute', right: 0, top: '-14px', fontSize: 'var(--fs-2xs)', color: 'var(--warning)', fontFamily: 'var(--font-body)', background: C.card, padding: '0 4px' }}>goal</span>
                          </div>
                        )}
                        {eventData.map((d, i) => {
                          const normalized = 1 - (d.time - minTime) / (maxTime - minTime)
                          const barH = Math.max(20, normalized * 130)
                          const isLatest = i === eventData.length - 1
                          return (
                            <div key={`${d.date}-${i}`} className="is-472283d">
                              <p style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: isLatest ? C.accent : C.muted, fontWeight: isLatest ? 700 : 400, whiteSpace: 'nowrap' }}>
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
                            <p style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: C.muted }}>{d.date}</p>
                          </div>
                        ))}
                      </div>
                      {eventData.length >= 3 && improvement > 0 && (
                        <div style={{ marginTop: '16px', padding: '12px 12px', background: 'var(--border)', borderRadius: '8px', border: `1px solid ${C.border}` }}>
                          <p style={{ fontSize: 'var(--fs-xs)', color: C.subtle }}>
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
                      <div style={{ display: 'grid', gridTemplateColumns: '100px 100px 1fr 1fr', padding: '12px 20px', borderBottom: `1px solid ${C.border}` }}>
                        {['Period', 'Time', 'Drop', 'Meet'].map((h) => (
                          <span key={h} style={{ fontSize: 'var(--fs-2xs)', fontFamily: 'var(--font-body)', color: C.muted, }}>{h}</span>
                        ))}
                      </div>
                      {[...eventData].reverse().map((d, i) => {
                        const prev = [...eventData].reverse()[i + 1]
                        const drop = prev ? prev.time - d.time : null
                        const isLatest = i === 0
                        return (
                          <div key={`tbl-${i}`} style={{ display: 'grid', gridTemplateColumns: '100px 100px 1fr 1fr', padding: '12px 20px', borderBottom: i < eventData.length - 1 ? `1px solid ${C.border}` : 'none', background: isLatest ? 'var(--border)' : 'transparent' }}>
                            <span style={{ fontSize: 'var(--fs-sm)', color: isLatest ? C.accent : C.subtle, fontFamily: 'var(--font-body)' }}>{d.date}</span>
                            <span style={{ fontSize: 'var(--fs-md)', fontFamily: 'var(--font-body)', fontWeight: 700, color: isLatest ? C.accent : C.text }}>{fmtSec(d.time)}</span>
                            <span style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: drop !== null && drop > 0 ? C.accent : drop !== null ? 'var(--error)' : C.muted }}>
                              {drop !== null ? (drop > 0 ? `-${drop.toFixed(2)}s` : `+${Math.abs(drop).toFixed(2)}s`) : '—'}
                            </span>
                            <span style={{ fontSize: 'var(--fs-xs)', color: C.muted }}>{d.meet ?? '—'}</span>
                          </div>
                        )
                      })}
                    </div>
                  )}

                  {/* Monthly view */}
                  {viewMode === 'monthly' && (
                    <div className="is-930d87f">
                      {Object.entries(monthMap).map(([month, times]) => {
                        const best = Math.min(...times)
                        const worst = Math.max(...times)
                        return (
                          <div key={month} className="card is-5c576af" >
                            <div style={{ width: '72px', flexShrink: 0 }}>
                              <span style={{ fontSize: 'var(--fs-base)', fontFamily: 'var(--font-body)', fontWeight: 700, color: C.subtle }}>{month}</span>
                            </div>
                            <div style={{ flex: 1 }}>
                              <ProgressionBar value={1 - (best - minTime) / (maxTime - minTime)} max={1} color={C.accent} />
                            </div>
                            <div style={{ textAlign: 'right', flexShrink: 0 }}>
                              <div style={{ fontFamily: 'var(--font-body)', fontWeight: 700, color: C.accent, fontSize: 'var(--fs-md)' }}>{fmtSec(best)}</div>
                              {times.length > 1 && <div style={{ fontSize: 'var(--fs-2xs)', color: C.muted, fontFamily: 'var(--font-body)' }}>range: {fmtSec(best)} – {fmtSec(worst)}</div>}
                              <div style={{ fontSize: 'var(--fs-2xs)', color: C.muted }}>{times.length} swims</div>
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

      {swimmerId ? (
        <div className="is-8dd12b4">
          <GoalsPanel swimmerId={swimmerId} />
        </div>
      ) : null}
    </div>
  )
}
