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

type SwimmerSnapshot = {
  swimmer: SwimmerRecord
  bestTimes: BestTimeRecord[]
  progression: ProgressionResultRecord[]
}

export default function PerformanceVisualizationPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [snapshots, setSnapshots] = useState<SwimmerSnapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

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

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="bar-chart-2" title="Sign in required" description="Authenticate to view live performance analytics." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1180, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Performance
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', fontWeight: 400, color: 'var(--text)', marginBottom: '6px' }}>
          Live Visualization
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
          Snapshot view composed from live swimmers, best times, and progression history.
        </p>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <EmptyState icon="bar-chart-2" title="Loading analytics" description="Aggregating live performance data across swimmers." />
      ) : snapshots.length === 0 ? (
        <EmptyState icon="bar-chart-2" title="No performance data yet" description="Add swimmers and results to populate this visualization." />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px' }}>
            {[
              { label: 'Swimmers', value: String(summary.swimmerCount) },
              { label: 'Best Times', value: String(summary.bestTimeCount) },
              { label: 'Results', value: String(summary.progressionCount) },
              { label: 'Manual Overrides', value: String(summary.manualOverrides) },
            ].map((stat) => (
              <div key={stat.label} className="card-elevated" style={{ padding: '14px 16px' }}>
                <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{stat.label}</div>
                <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{stat.value}</div>
              </div>
            ))}
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '20px' }}>
            <section className="card" style={{ overflow: 'hidden' }}>
              <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
                <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Swimmers with the Broadest Coverage</h2>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column' }}>
                {summary.topSwimmers.map((entry, index) => (
                  <div key={entry.swimmer.id} style={{ padding: '14px 18px', borderBottom: index < summary.topSwimmers.length - 1 ? '1px solid var(--border)' : 'none' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '4px' }}>
                      <span style={{ fontSize: '13px', color: 'var(--text)' }}>{entry.swimmer.first_name} {entry.swimmer.last_name}</span>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--accent)' }}>{entry.bestTimeCount} best times</span>
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                      Latest result: {entry.lastResultAt ? new Date(entry.lastResultAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : 'No official progression rows yet'}
                    </div>
                  </div>
                ))}
              </div>
            </section>

            <section className="card" style={{ overflow: 'hidden' }}>
              <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
                <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Most Populated Events</h2>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column' }}>
                {summary.topEvents.map((entry, index) => (
                  <div key={entry.event} style={{ padding: '14px 18px', borderBottom: index < summary.topEvents.length - 1 ? '1px solid var(--border)' : 'none' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '4px' }}>
                      <span style={{ fontSize: '13px', color: 'var(--text)' }}>{entry.event}</span>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--accent)' }}>{entry.count} swimmers</span>
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                      Fastest recorded time: {formatSwimTime(entry.fastest)}
                    </div>
                  </div>
                ))}
              </div>
            </section>
          </div>
        </div>
      )}
    </div>
  )
}
