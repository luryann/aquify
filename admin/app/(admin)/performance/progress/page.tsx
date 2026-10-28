'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { fetchGroups, fetchSeasons, type GroupRecord, type SeasonRecord } from '@/lib/adminData'
import { COURSES, STROKE_LABELS, fetchProgressReport, formatDrop, formatSwimTime, type ProgressReport, type SwimmerProgress } from '@/lib/performanceData'
import { defaultComparison } from '@/lib/performance-periods'
import { FIELD_STYLE, FilterRow, LoadErrorBanner } from '@/components/performance/PerfControls'
import { EmptyState } from '@/components/ui/EmptyState'

type Range = { from_a: string; to_a: string; from_b: string; to_b: string }

export default function ProgressReportPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [range, setRange] = useState<Range>(() => defaultComparison(new Date()))
  const [group, setGroup] = useState('')
  const [course, setCourse] = useState('')
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [seasons, setSeasons] = useState<SeasonRecord[]>([])
  const [report, setReport] = useState<ProgressReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId) return
    fetchGroups(api, orgId).then(setGroups).catch(() => setGroups([]))
    fetchSeasons(api, orgId).then(setSeasons).catch(() => setSeasons([]))
  }, [api, orgId])

  const load = useCallback(async () => {
    if (!orgId) return
    setLoading(true)
    setError(null)
    try {
      setReport(await fetchProgressReport(api, orgId, { ...range, group_code: group || undefined, course: course || undefined, limit: 10 }))
    } catch (err) {
      setError(reportUIError('performance.progress.load', err, 'The progress report could not be loaded.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId, range, group, course])
  useEffect(() => { void load() }, [load])

  const datedSeasons = seasons.filter((s) => s.start_date && s.end_date).sort((a, b) => a.start_date.localeCompare(b.start_date))
  const seasonPairs = datedSeasons.slice(1).map((s, i) => ({ a: datedSeasons[i], b: s }))

  const maxPercent = useMemo(() => Math.max(1, ...(report?.most_improved ?? []).map((s) => s.avg_percent)), [report])
  const swimmers = useMemo(() => [...(report?.swimmers ?? [])].filter((s) => s.events_compared > 0).sort((a, b) => b.avg_percent - a.avg_percent), [report])

  const dateInput = (key: keyof Range, label: string) => (
    <label className="u-gap-4px u-fs-fs-sm" style={{ display: 'grid', color: 'var(--text-subtle)' }}>{label}
      <input type="date" value={range[key]} onChange={(e) => setRange({ ...range, [key]: e.target.value })} style={FIELD_STYLE} />
    </label>
  )

  return (
    <div className="page">
      <h1 className="page-title">Progress report</h1>
      <p className="page-subtitle u-mb-24px" >Average percent drop in best times between two periods.</p>

      <FilterRow>
        {seasonPairs.length > 0 ? (
          <select aria-label="Compare seasons" defaultValue="" onChange={(e) => {
            const pair = seasonPairs[Number(e.target.value)]
            if (pair) setRange({ from_a: pair.a.start_date, to_a: pair.a.end_date, from_b: pair.b.start_date, to_b: pair.b.end_date })
          }} style={FIELD_STYLE}>
            <option value="" disabled>Season over season…</option>
            {seasonPairs.map((p, i) => <option key={p.b.id} value={i}>{p.a.name} → {p.b.name}</option>)}
          </select>
        ) : null}
        <select aria-label="Training group" value={group} onChange={(e) => setGroup(e.target.value)} style={FIELD_STYLE}>
          <option value="">All groups</option>
          {groups.map((g) => <option key={g.id} value={g.code}>{g.name}</option>)}
        </select>
        <select aria-label="Course" value={course} onChange={(e) => setCourse(e.target.value)} style={FIELD_STYLE}>
          <option value="">All courses</option>
          {COURSES.map((c) => <option key={c}>{c}</option>)}
        </select>
      </FilterRow>
      <div className="u-mb-24px" style={{ display: 'flex', gap: '12px 20px', flexWrap: 'wrap' }}>
        <fieldset className="u-p-0 u-m-0 u-gap-8px" style={{ border: 'none', display: 'flex', flexWrap: 'wrap' }}>
          <legend className="u-fs-fs-sm u-fw-600 u-mb-4px" >Baseline</legend>
          {dateInput('from_a', 'From')}{dateInput('to_a', 'To')}
        </fieldset>
        <fieldset className="u-p-0 u-m-0 u-gap-8px" style={{ border: 'none', display: 'flex', flexWrap: 'wrap' }}>
          <legend className="u-fs-fs-sm u-fw-600 u-mb-4px" >Compared with</legend>
          {dateInput('from_b', 'From')}{dateInput('to_b', 'To')}
        </fieldset>
      </div>

      {error ? <LoadErrorBanner message={error} onRetry={() => void load()} /> : null}
      {loading ? <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>Building report…</p> : null}

      {!loading && report && !error ? (
        swimmers.length === 0 ? (
          <EmptyState icon="activity" title="No swimmer swam the same event in both periods" description="Widen the date ranges or pick a different group." />
        ) : (
          <>
            <section aria-labelledby="most-improved" className="u-mb-space-8" >
              <h2 id="most-improved" className="u-ff-font-display u-fw-display-weight u-fs-fs-2xl u-m-0-0-12px" >Most improved</h2>
              <ol className="u-m-0 u-p-0 u-gap-8px" style={{ listStyle: 'none', display: 'grid' }}>
                {report.most_improved.map((s, i) => (
                  <li key={s.swimmer_id} className="u-gap-12px u-fs-fs-base" style={{ display: 'grid', gridTemplateColumns: '22px minmax(96px, 200px) minmax(0, 1fr) 64px', alignItems: 'center' }}>
                    <span style={{ color: 'var(--text-muted)', fontVariantNumeric: 'tabular-nums' }}>{i + 1}</span>
                    <Link href={`/members/swimmers/${s.swimmer_id}`} style={{ color: 'var(--text)', textDecoration: 'none', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.swimmer_name}</Link>
                    <span title={`${s.avg_percent.toFixed(2)}% average drop across ${s.events_compared} event${s.events_compared === 1 ? '' : 's'}`} style={{ height: '10px', display: 'block' }}>
                      <span style={{ display: 'block', height: '100%', width: `${(s.avg_percent / maxPercent) * 100}%`, minWidth: '4px', background: 'var(--accent)', borderRadius: '0 4px 4px 0' }} />
                    </span>
                    <span className="u-fw-600" style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{s.avg_percent.toFixed(1)}%</span>
                  </li>
                ))}
              </ol>
            </section>

            <section aria-labelledby="all-swimmers">
              <h2 id="all-swimmers" className="u-ff-font-display u-fw-display-weight u-fs-fs-2xl u-m-0-0-12px" >Every swimmer with comparable swims</h2>
              <div className="table-scroll" style={{ overflowX: 'auto' }}>
                <table className="u-fs-fs-base" style={{ width: '100%', borderCollapse: 'collapse', minWidth: '520px' }}>
                  <thead>
                    <tr className="u-fs-fs-sm" style={{ textAlign: 'left', color: 'var(--text-muted)' }}>
                      <th className="u-p-8px-6px u-fw-500" >Swimmer</th>
                      <th className="u-p-8px-6px u-fw-500" >Events</th>
                      <th className="u-p-8px-6px u-fw-500" >Faster in</th>
                      <th className="u-p-8px-6px u-fw-500" style={{ textAlign: 'right' }}>Avg drop</th>
                      <th className="u-p-8px-6px u-fw-500" style={{ textAlign: 'right' }}>Total time</th>
                    </tr>
                  </thead>
                  <tbody>
                    {swimmers.map((s) => <SwimmerRows key={s.swimmer_id} s={s} open={expanded === s.swimmer_id} onToggle={() => setExpanded(expanded === s.swimmer_id ? null : s.swimmer_id)} />)}
                  </tbody>
                </table>
              </div>
            </section>
          </>
        )
      ) : null}
    </div>
  )
}

function SwimmerRows({ s, open, onToggle }: { s: SwimmerProgress; open: boolean; onToggle: () => void }) {
  const tone = (v: number) => (v > 0 ? 'var(--success)' : v < 0 ? 'var(--error)' : 'var(--text-muted)')
  return (
    <>
      <tr style={{ borderTop: '1px solid var(--border)' }}>
        <td className="u-p-8px-6px" >
          <button type="button" onClick={onToggle} aria-expanded={open} className="u-p-0" style={{ background: 'none', border: 'none', color: 'var(--text)', cursor: 'pointer', font: 'inherit', textAlign: 'left' }}>
            {open ? '▾' : '▸'} {s.swimmer_name}
          </button>
        </td>
        <td className="u-p-8px-6px" >{s.events_compared}{s.new_events ? <span style={{ color: 'var(--text-muted)' }}> +{s.new_events} new</span> : null}</td>
        <td className="u-p-8px-6px" >{s.events_improved} of {s.events_compared}</td>
        <td className="u-p-8px-6px" style={{ textAlign: 'right', color: tone(s.avg_percent), fontVariantNumeric: 'tabular-nums' }}>{s.avg_percent.toFixed(1)}%</td>
        <td className="u-p-8px-6px" style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{formatDrop(s.total_drop_ms)}</td>
      </tr>
      {open ? s.events.map((e) => (
        <tr key={`${e.distance}-${e.stroke}-${e.course}`} className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
          <td style={{ padding: '4px 6px 4px 24px' }}>{e.distance} {STROKE_LABELS[e.stroke] ?? e.stroke} {e.course}</td>
          <td colSpan={2} className="u-p-4px-6px" style={{ fontVariantNumeric: 'tabular-nums' }}>{formatSwimTime(e.before_ms)} → {formatSwimTime(e.after_ms)}</td>
          <td className="u-p-4px-6px" style={{ textAlign: 'right', color: tone(e.percent), fontVariantNumeric: 'tabular-nums' }}>{e.percent.toFixed(1)}%</td>
          <td className="u-p-4px-6px" style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{formatDrop(e.drop_ms)}</td>
        </tr>
      )) : null}
    </>
  )
}
