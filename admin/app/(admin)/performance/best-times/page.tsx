'use client'

import Link from 'next/link'
import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Table, col } from '@/components/ui/Table'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchPerformanceBestTimes, fetchSwimmers, type BestTimeRecord, type SwimmerRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { FIELD_INPUT_STYLE } from '@/lib/form-styles'
import { formatSwimTime } from '@/lib/performanceData'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'

const COURSE_OPTIONS = ['SCY', 'SCM', 'LCM']
const STROKE_OPTIONS = ['free', 'back', 'breast', 'fly', 'im']
const DISTANCE_OPTIONS = [25, 50, 100, 200, 400, 500, 800, 1000, 1500, 1650]

type LeaderboardRow = BestTimeRecord & { swimmerName: string }

export default function BestTimesPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [bestTimes, setBestTimes] = useState<BestTimeRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [courseFilter, setCourseFilter] = useState('SCY')
  const [strokeFilter, setStrokeFilter] = useState('')
  const [distanceFilter, setDistanceFilter] = useState('')
  const [swimmerSearch, setSwimmerSearch] = useState('')

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
        const [nextSwimmers, nextBestTimes] = await Promise.all([
          fetchSwimmers(api, session.user.orgId),
          fetchPerformanceBestTimes(api, session.user.orgId, {
            ...(courseFilter ? { course: courseFilter } : {}),
            ...(strokeFilter ? { stroke: strokeFilter } : {}),
          }),
        ])
        if (cancelled) return
        setSwimmers(nextSwimmers)
        setBestTimes(nextBestTimes)
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load live best times. Try again.'))
          setSwimmers([])
          setBestTimes([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId, courseFilter, strokeFilter])

  const swimmerNameById = useMemo(() => {
    const map = new Map<string, string>()
    for (const s of swimmers) map.set(s.id, `${s.first_name} ${s.last_name}`)
    return map
  }, [swimmers])

  const rows: LeaderboardRow[] = useMemo(() => {
    const q = swimmerSearch.trim().toLowerCase()
    return bestTimes
      .filter((record) => (distanceFilter ? String(record.event_distance) === distanceFilter : true))
      .map((record) => ({ ...record, swimmerName: swimmerNameById.get(record.swimmer_id) ?? 'Unknown swimmer' }))
      .filter((record) => (q ? record.swimmerName.toLowerCase().includes(q) : true))
      .sort((a, b) => (a.best_time_ms ?? Infinity) - (b.best_time_ms ?? Infinity))
  }, [bestTimes, distanceFilter, swimmerSearch, swimmerNameById])

  const manualOverrides = useMemo(() => bestTimes.filter((record) => record.is_manual_override).length, [bestTimes])

  if (!session) {
    return <div className="u-p-32px" ><EmptyState icon="bar-chart-2" title="Sign in required" description="Authenticate to inspect live best times." /></div>
  }

  const columns = [
    col<LeaderboardRow>({
      key: 'rank',
      label: '#',
      sortable: false,
      width: '48px',
      render: (_row, index) => <span className="u-ff-font-body" style={{ color: 'var(--text-muted)' }}>{index + 1}</span>,
    }),
    col<LeaderboardRow>({
      key: 'swimmerName',
      label: 'Swimmer',
      sortable: true,
      render: (row) => (
        <Link href={`/members/swimmers/${row.swimmer_id}`} style={{ color: 'var(--text)', textDecoration: 'none' }}>
          {row.swimmerName}
        </Link>
      ),
    }),
    col<LeaderboardRow>({
      key: 'event_distance',
      label: 'Event',
      sortable: true,
      render: (row) => <span style={{ color: 'var(--text-subtle)' }}>{row.event_distance} {row.stroke} · {row.course}</span>,
    }),
    col<LeaderboardRow>({
      key: 'best_time_ms',
      label: 'Time',
      sortable: true,
      render: (row) => <span className="u-ff-font-body" style={{ color: 'var(--accent-text, var(--accent))' }}>{formatSwimTime(row.best_time_ms)}</span>,
    }),
    col<LeaderboardRow>({
      key: 'recorded_at',
      label: 'Recorded',
      sortable: true,
      render: (row) => (
        <span style={{ color: 'var(--text-subtle)' }}>
          {row.recorded_at ? new Date(row.recorded_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : '—'}
        </span>
      ),
    }),
    col<LeaderboardRow>({
      key: 'is_manual_override',
      label: 'Source',
      sortable: true,
      render: (row) => (
        <span style={{ color: row.is_manual_override ? 'var(--warning)' : 'var(--text-muted)' }}>
          {row.is_manual_override ? `Manual override${row.override_reason ? ` · ${row.override_reason}` : ''}` : 'Imported result'}
        </span>
      ),
    }),
  ]

  return (
    <div className="page">
      <div className="page-heading">
        <h1 className="page-title">Best times</h1>
      </div>

      {error ? <LoadErrorBanner message={error} /> : null}

      <div className="u-gap-12px u-mb-16px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 200px', maxWidth: '280px' }}>
          <span style={{ position: 'absolute', left: '11px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }}>
            <Icon name="search" size={14} />
          </span>
          <input aria-label="Find a swimmer" className="input-search"
            value={swimmerSearch}
            onChange={(e) => setSwimmerSearch(e.target.value)}
            placeholder="Find a swimmer…"
            style={{ ...FIELD_INPUT_STYLE, width: '100%' }}
          />
        </div>
        <select value={distanceFilter} onChange={(e) => setDistanceFilter(e.target.value)} style={{ ...FIELD_INPUT_STYLE }}>
          <option value="">All distances</option>
          {DISTANCE_OPTIONS.map((d) => <option key={d} value={d}>{d}</option>)}
        </select>
        <select value={strokeFilter} onChange={(e) => setStrokeFilter(e.target.value)} style={{ ...FIELD_INPUT_STYLE }}>
          <option value="">All strokes</option>
          {STROKE_OPTIONS.map((s) => <option key={s} value={s}>{s}</option>)}
        </select>
        <div style={{ display: 'flex', border: '1px solid var(--border)', borderRadius: '8px', overflow: 'hidden' }} title="Course: Short Course Yards / Short Course Meters / Long Course Meters">
          {COURSE_OPTIONS.map((c) => (
            <button type="button"
              key={c}
              onClick={() => setCourseFilter(c)}
              className="u-p-8px-12px u-fs-fs-sm" style={{ border: 'none', background: courseFilter === c ? 'var(--accent-subtle)' : 'var(--bg-elevated)', color: courseFilter === c ? 'var(--accent)' : 'var(--text-subtle)', fontWeight: courseFilter === c ? 600 : 400, cursor: 'pointer' }}
            >
              {c}
            </button>
          ))}
        </div>
        <span className="u-ml-auto u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>
          {rows.length} records · {manualOverrides} manual overrides
        </span>
      </div>

      <Table
        columns={columns}
        data={rows}
        keyField="id"
        loading={loading}
        paginated
        pageSize={25}
        emptyTitle={bestTimes.length === 0 ? 'No best times recorded' : 'No matches'}
        emptyDescription={bestTimes.length === 0 ? 'Upload meet results or add a manual override to populate this view.' : 'Try a different filter or search.'}
      />
    </div>
  )
}
