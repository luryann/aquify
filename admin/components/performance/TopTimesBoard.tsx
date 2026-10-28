'use client'

import { useCallback, useEffect, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { parseBackendDate } from '@/lib/dates'
import { fetchSeasons, type SeasonRecord } from '@/lib/adminData'
import {
  AGE_GROUPS, AGE_GROUP_LABELS, COURSES, EVENT_DISTANCES, STROKES, STROKE_LABELS,
  fetchTopTimes, formatSwimTime, genderLabel, type Course, type TopTimesList,
} from '@/lib/performanceData'
import { FIELD_STYLE, FilterRow, LoadErrorBanner, Segmented } from './PerfControls'
import { periodRange, type PeriodPreset } from '@/lib/performance-periods'

/** Top-N board: one best swim per swimmer per event, filterable by course,
 *  gender, age group (age on the swim date), event and time window. */
export function TopTimesBoard({ canUseSeasons = false }: { canUseSeasons?: boolean }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [course, setCourse] = useState<Course>('SCY')
  const [gender, setGender] = useState<'F' | 'M'>('F')
  const [ageGroup, setAgeGroup] = useState('OPEN')
  const [event, setEvent] = useState('') // "" = all events, else "100|free"
  const [period, setPeriod] = useState<PeriodPreset | string>('all')
  const [limit, setLimit] = useState(10)
  const [seasons, setSeasons] = useState<SeasonRecord[]>([])
  const [lists, setLists] = useState<TopTimesList[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!canUseSeasons || !orgId) return
    fetchSeasons(api, orgId).then(setSeasons).catch(() => setSeasons([]))
  }, [api, orgId, canUseSeasons])

  const load = useCallback(async () => {
    if (!orgId) return
    setLoading(true)
    setError(null)
    const season = seasons.find((s) => s.id === period)
    const range = season ? { from: season.start_date, to: season.end_date } : periodRange(period as PeriodPreset, new Date())
    const [distance, stroke] = event ? event.split('|') : ['', '']
    try {
      setLists(await fetchTopTimes(api, orgId, { course, gender, age_group: ageGroup, distance: distance ? Number(distance) : undefined, stroke: stroke || undefined, from: range.from, to: range.to, limit }))
    } catch (err) {
      setError(reportUIError('performance.top-times.load', err, 'Top times could not be loaded.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId, course, gender, ageGroup, event, period, limit, seasons])

  useEffect(() => { void load() }, [load])

  return (
    <div>
      <FilterRow>
        <Segmented label="Course" value={course} onChange={setCourse} options={COURSES.map((c) => ({ value: c, label: c }))} />
        <Segmented label="Gender" value={gender} onChange={setGender} options={[{ value: 'F', label: ageGroup === 'OPEN' ? 'Women' : 'Girls' }, { value: 'M', label: ageGroup === 'OPEN' ? 'Men' : 'Boys' }]} />
        <select aria-label="Age group" value={ageGroup} onChange={(e) => setAgeGroup(e.target.value)} style={FIELD_STYLE}>
          {AGE_GROUPS.map((g) => <option key={g} value={g}>{AGE_GROUP_LABELS[g]}</option>)}
        </select>
        <select aria-label="Event" value={event} onChange={(e) => setEvent(e.target.value)} style={FIELD_STYLE}>
          <option value="">All events</option>
          {STROKES.flatMap((s) => EVENT_DISTANCES[s].map((d) => <option key={`${d}|${s}`} value={`${d}|${s}`}>{d} {STROKE_LABELS[s]}</option>))}
        </select>
        <select aria-label="Time window" value={period} onChange={(e) => setPeriod(e.target.value)} style={FIELD_STYLE}>
          <option value="all">All time</option>
          <option value="12m">Last 12 months</option>
          <option value="ytd">This calendar year</option>
          {seasons.filter((s) => s.start_date && s.end_date).map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
        </select>
        <select aria-label="List length" value={limit} onChange={(e) => setLimit(Number(e.target.value))} style={FIELD_STYLE}>
          {[10, 16, 25, 50].map((n) => <option key={n} value={n}>Top {n}</option>)}
        </select>
      </FilterRow>

      {error ? <LoadErrorBanner message={error} onRetry={() => void load()} /> : null}

      {loading ? (
        <div style={{ color: 'var(--text-muted)', fontSize: 'var(--fs-base)', padding: '24px 0' }}>Loading top times…</div>
      ) : !error && lists.length === 0 ? (
        <EmptyState icon="bar-chart-2" title="No swims match these filters" description="Try another age group, course or time window." />
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 300px), 1fr))', gap: '24px 24px' }}>
          {lists.map((list) => (
            <section key={`${list.distance}-${list.stroke}`} aria-label={list.label}>
              <h2 className="card-title" style={{ margin: '0 0 6px' }}>
                {list.distance} {STROKE_LABELS[list.stroke]} <span style={{ fontWeight: 400, color: 'var(--text-muted)' }}>{genderLabel(list.gender, list.age_group)} {AGE_GROUP_LABELS[list.age_group]}</span>
              </h2>
              <ol style={{ listStyle: 'none', margin: 0, padding: 0, borderTop: '1px solid var(--border)' }}>
                {list.entries.map((e) => (
                  <li key={e.result_id} style={{ display: 'grid', gridTemplateColumns: '24px minmax(0, 1fr) auto', gap: '12px', alignItems: 'baseline', padding: '8px 0', borderBottom: '1px solid var(--border)', fontSize: 'var(--fs-base)' }}>
                    <span style={{ color: e.rank === 1 ? 'var(--accent)' : 'var(--text-muted)', fontWeight: e.rank === 1 ? 600 : 400, fontVariantNumeric: 'tabular-nums' }}>{e.rank}</span>
                    <span style={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={e.meet_name ? `${e.meet_name}, ${e.swim_date}` : e.swim_date}>
                      {e.swimmer_name}{e.age != null ? <span style={{ color: 'var(--text-muted)' }}> {e.age}</span> : null}
                      <span style={{ color: 'var(--text-muted)', fontSize: 'var(--fs-sm)' }}> · {parseBackendDate(e.swim_date)?.toLocaleDateString('en-US', { month: 'short', year: 'numeric' }) ?? e.swim_date}</span>
                    </span>
                    <span style={{ fontVariantNumeric: 'tabular-nums', fontWeight: 600 }}>{formatSwimTime(e.time_ms)}</span>
                  </li>
                ))}
              </ol>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}
