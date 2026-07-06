'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  fetchPerformanceBestTimes,
  fetchPerformanceProgression,
  fetchSwimmers,
  type BestTimeRecord,
  type ProgressionResultRecord,
  type SwimmerRecord,
} from '@/lib/adminData'

function formatSwimTime(milliseconds?: number | null) {
  if (milliseconds == null) return 'NT'
  const totalSeconds = milliseconds / 1000
  if (totalSeconds < 60) return totalSeconds.toFixed(2)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = (totalSeconds % 60).toFixed(2).padStart(5, '0')
  return `${minutes}:${seconds}`
}

function eventKeyFor(record: ProgressionResultRecord): string {
  return `${record.distance} ${record.stroke} ${record.course}`
}

function eventLabelFor(record: ProgressionResultRecord): string {
  const stroke = record.stroke.charAt(0).toUpperCase() + record.stroke.slice(1)
  return `${record.distance} ${stroke} (${record.course})`
}

type SwimmerSnapshot = {
  swimmer: SwimmerRecord
  bestTimes: BestTimeRecord[]
  progression: ProgressionResultRecord[]
}

// A hand-built line chart (no charting dependency) plotting finish time over
// date for a single swimmer/event/course combo. Faster times plot higher —
// an upward line reads as "getting faster" the way a coach would expect.
function ProgressionChart({ results }: { results: ProgressionResultRecord[] }) {
  const points = results
    .filter((r) => r.fin_time_ms != null)
    .map((r) => ({ date: new Date(r.created_at), ms: r.fin_time_ms as number }))
    .sort((a, b) => a.date.getTime() - b.date.getTime())

  if (points.length === 0) {
    return (
      <div style={{ padding: '32px 18px', textAlign: 'center' }}>
        <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>No timed results recorded for this event yet.</p>
      </div>
    )
  }

  const width = 640
  const height = 240
  const padL = 64
  const padR = 20
  const padT = 20
  const padB = 34
  const plotW = width - padL - padR
  const plotH = height - padT - padB

  const minMs = Math.min(...points.map((p) => p.ms))
  const maxMs = Math.max(...points.map((p) => p.ms))
  const msRange = maxMs - minMs || 1
  const minDate = points[0].date.getTime()
  const maxDate = points[points.length - 1].date.getTime()
  const dateRange = maxDate - minDate || 1

  // Faster (lower ms) plots higher on the y-axis.
  const xFor = (t: number) => padL + (points.length === 1 ? plotW / 2 : ((t - minDate) / dateRange) * plotW)
  const yFor = (ms: number) => padT + ((ms - minMs) / msRange) * plotH

  const linePath = points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${xFor(p.date.getTime())} ${yFor(p.ms)}`).join(' ')

  const yTicks = [minMs, minMs + msRange / 2, maxMs]

  return (
    <div style={{ padding: '16px 18px' }}>
      <svg viewBox={`0 0 ${width} ${height}`} style={{ width: '100%', height: 'auto', display: 'block' }} role="img" aria-label="Finish time over time, faster times plotted higher">
        {yTicks.map((ms, i) => (
          <g key={i}>
            <line x1={padL} x2={width - padR} y1={yFor(ms)} y2={yFor(ms)} stroke="var(--border)" strokeWidth={1} />
            <text x={padL - 8} y={yFor(ms)} textAnchor="end" dominantBaseline="middle" fontSize="10.5" fill="var(--text-muted)" fontFamily="var(--font-body)">
              {formatSwimTime(ms)}
            </text>
          </g>
        ))}

        <path d={linePath} fill="none" stroke="var(--accent)" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" />

        {points.map((p, i) => (
          <g key={i}>
            <circle cx={xFor(p.date.getTime())} cy={yFor(p.ms)} r={4} fill="var(--accent)" stroke="var(--bg-surface)" strokeWidth={1.5} />
            <title>{`${p.date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}: ${formatSwimTime(p.ms)}`}</title>
          </g>
        ))}

        <text x={padL} y={height - 8} fontSize="10.5" fill="var(--text-muted)" fontFamily="var(--font-body)">
          {points[0].date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
        </text>
        <text x={width - padR} y={height - 8} textAnchor="end" fontSize="10.5" fill="var(--text-muted)" fontFamily="var(--font-body)">
          {points[points.length - 1].date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
        </text>
      </svg>
      <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '8px' }}>
        {points.length} timed result{points.length !== 1 ? 's' : ''}. Faster times plot higher.
      </p>
    </div>
  )
}

export default function PerformanceVisualizationPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [snapshots, setSnapshots] = useState<SwimmerSnapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [chartSwimmerId, setChartSwimmerId] = useState<string>('')
  const [chartEventKey, setChartEventKey] = useState<string>('')

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId) {
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const [swimmers, bestTimes, progression] = await Promise.all([
          fetchSwimmers(api, session.user.orgId),
          fetchPerformanceBestTimes(api, session.user.orgId),
          fetchPerformanceProgression(api, session.user.orgId),
        ])
        const bestTimesBySwimmer = new Map<string, BestTimeRecord[]>()
        for (const record of bestTimes) {
          const bucket = bestTimesBySwimmer.get(record.swimmer_id) ?? []
          bucket.push(record)
          bestTimesBySwimmer.set(record.swimmer_id, bucket)
        }
        const progressionBySwimmer = new Map<string, ProgressionResultRecord[]>()
        for (const record of progression) {
          const bucket = progressionBySwimmer.get(record.swimmer_id) ?? []
          bucket.push(record)
          progressionBySwimmer.set(record.swimmer_id, bucket)
        }
        const next = swimmers.map((swimmer) => ({
          swimmer,
          bestTimes: bestTimesBySwimmer.get(swimmer.id) ?? [],
          progression: progressionBySwimmer.get(swimmer.id) ?? [],
        }))
        if (!cancelled) setSnapshots(next)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load performance visualization data.')
          setSnapshots([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId])

  const summary = useMemo(() => {
    const swimmerCount = snapshots.length
    const bestTimeCount = snapshots.reduce((sum, snapshot) => sum + snapshot.bestTimes.length, 0)
    const progressionCount = snapshots.reduce((sum, snapshot) => sum + snapshot.progression.length, 0)
    const manualOverrides = snapshots.reduce((sum, snapshot) => sum + snapshot.bestTimes.filter((record) => record.is_manual_override).length, 0)

    const topSwimmers = [...snapshots]
      .map((snapshot) => ({
        swimmer: snapshot.swimmer,
        bestTimeCount: snapshot.bestTimes.filter((record) => record.best_time_ms != null).length,
        lastResultAt: snapshot.progression.at(-1)?.created_at,
      }))
      .sort((a, b) => b.bestTimeCount - a.bestTimeCount)
      .slice(0, 6)

    const eventMap = new Map<string, { count: number; fastest?: number | null }>()
    for (const snapshot of snapshots) {
      for (const bestTime of snapshot.bestTimes) {
        const key = `${bestTime.event_distance} ${bestTime.stroke} ${bestTime.course}`
        const current = eventMap.get(key) ?? { count: 0, fastest: undefined }
        current.count += 1
        if (bestTime.best_time_ms != null) {
          current.fastest = current.fastest == null ? bestTime.best_time_ms : Math.min(current.fastest, bestTime.best_time_ms)
        }
        eventMap.set(key, current)
      }
    }

    const topEvents = [...eventMap.entries()]
      .map(([event, value]) => ({ event, count: value.count, fastest: value.fastest }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 8)

    return {
      swimmerCount,
      bestTimeCount,
      progressionCount,
      manualOverrides,
      topSwimmers,
      topEvents,
    }
  }, [snapshots])

  const chartableSwimmers = useMemo(
    () => snapshots.filter((s) => s.progression.some((r) => r.fin_time_ms != null)),
    [snapshots],
  )

  const chartSwimmer = chartableSwimmers.find((s) => s.swimmer.id === chartSwimmerId) ?? chartableSwimmers[0]

  const chartEventOptions = useMemo(() => {
    if (!chartSwimmer) return []
    const byKey = new Map<string, string>()
    for (const r of chartSwimmer.progression) {
      if (r.fin_time_ms == null) continue
      byKey.set(eventKeyFor(r), eventLabelFor(r))
    }
    return [...byKey.entries()].map(([key, label]) => ({ key, label }))
  }, [chartSwimmer])

  const activeEventKey = chartEventOptions.some((o) => o.key === chartEventKey)
    ? chartEventKey
    : (chartEventOptions[0]?.key ?? '')

  const chartResults = chartSwimmer
    ? chartSwimmer.progression.filter((r) => eventKeyFor(r) === activeEventKey)
    : []

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="bar-chart-2" title="Sign in required" description="Sign in to view performance analytics." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1180, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Performance
        </p>
        <h1 className="page-title">
          Team Coverage
        </h1>
        <p className="page-subtitle">
          Which swimmers compete in the most events, and which events have the most swimmers.
        </p>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <EmptyState icon="bar-chart-2" title="Loading analytics" description="Aggregating performance data across swimmers." />
      ) : snapshots.length === 0 ? (
        <EmptyState icon="bar-chart-2" title="No performance data yet" description="Add swimmers and results to populate this visualization." />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
            <span className="badge badge-default">{summary.swimmerCount} swimmers</span>
            <span className="badge badge-default">{summary.bestTimeCount} best times</span>
            <span className="badge badge-default">{summary.progressionCount} results</span>
            <span className="badge badge-warning">{summary.manualOverrides} times entered by hand</span>
          </div>

          {chartableSwimmers.length > 0 && (
            <section className="card" style={{ overflow: 'hidden' }}>
              <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
                <div>
                  <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Time Progression</h2>
                  <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>Finish time over time for one swimmer and event</p>
                </div>
                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                  <select
                    value={chartSwimmer?.swimmer.id ?? ''}
                    onChange={(e) => { setChartSwimmerId(e.target.value); setChartEventKey('') }}
                    style={{ fontSize: '12px', padding: '6px 10px', borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--bg-surface)', color: 'var(--text)' }}
                  >
                    {chartableSwimmers.map((s) => (
                      <option key={s.swimmer.id} value={s.swimmer.id}>{s.swimmer.first_name} {s.swimmer.last_name}</option>
                    ))}
                  </select>
                  <select
                    value={activeEventKey}
                    onChange={(e) => setChartEventKey(e.target.value)}
                    style={{ fontSize: '12px', padding: '6px 10px', borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--bg-surface)', color: 'var(--text)' }}
                  >
                    {chartEventOptions.map((o) => (
                      <option key={o.key} value={o.key}>{o.label}</option>
                    ))}
                  </select>
                </div>
              </div>
              <ProgressionChart results={chartResults} />
            </section>
          )}

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '20px' }}>
            <section className="card" style={{ overflow: 'hidden' }}>
              <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
                <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Swimmers with the Broadest Coverage</h2>
                <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>By number of events with a recorded best time</p>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', padding: '16px 18px' }}>
                {(() => {
                  const max = Math.max(1, ...summary.topSwimmers.map((e) => e.bestTimeCount))
                  return summary.topSwimmers.map((entry) => (
                    <div key={entry.swimmer.id} title={`${entry.swimmer.first_name} ${entry.swimmer.last_name}: ${entry.bestTimeCount} best times`}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '5px' }}>
                        <span style={{ fontSize: '13px', color: 'var(--text)' }}>{entry.swimmer.first_name} {entry.swimmer.last_name}</span>
                        <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--accent)', flexShrink: 0 }}>{entry.bestTimeCount}</span>
                      </div>
                      <div style={{ height: '7px', borderRadius: '4px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
                        <div style={{ height: '100%', width: `${(entry.bestTimeCount / max) * 100}%`, background: 'var(--accent)', borderRadius: '4px' }} />
                      </div>
                    </div>
                  ))
                })()}
              </div>
            </section>

            <section className="card" style={{ overflow: 'hidden' }}>
              <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
                <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Most Populated Events</h2>
                <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>By number of swimmers with a recorded time</p>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', padding: '16px 18px' }}>
                {(() => {
                  const max = Math.max(1, ...summary.topEvents.map((e) => e.count))
                  return summary.topEvents.map((entry) => (
                    <div key={entry.event} title={`${entry.event}: ${entry.count} swimmers, fastest ${formatSwimTime(entry.fastest)}`}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '5px' }}>
                        <span style={{ fontSize: '13px', color: 'var(--text)' }}>{entry.event}</span>
                        <span style={{ fontFamily: 'var(--font-body)', fontSize: '12px', color: 'var(--accent)', flexShrink: 0 }}>{entry.count}</span>
                      </div>
                      <div style={{ height: '7px', borderRadius: '4px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
                        <div style={{ height: '100%', width: `${(entry.count / max) * 100}%`, background: 'var(--accent)', borderRadius: '4px' }} />
                      </div>
                    </div>
                  ))
                })()}
              </div>
            </section>
          </div>
        </div>
      )}
    </div>
  )
}
