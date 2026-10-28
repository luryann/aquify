'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ApiClient } from '@/lib/api'
import type { GroupRecord } from '@/lib/adminData'
import { parseBackendDate } from '@/lib/dates'
import { fetchTrainingAnalytics, type TrainingAnalytics } from '@/lib/trainingData'
import { ZONES, ZONE_LABELS, addDaysISO, weekStartOf, zoneColor } from '@/lib/workout-format'
import { WeeklyBars } from '@/components/training/WeeklyBars'
import { errorText } from './WorkoutComposer'

const RANGES = [4, 8, 12, 26] as const

const shortDate = (iso: string) => parseBackendDate(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })

function Stat({ value, label }: { value: string; label: string }) {
  return (
    <div style={{ minWidth: 0 }}>
      <p className="u-ff-font-display u-fw-display-weight u-fs-fs-3xl u-lh-1 u-m-0" style={{ color: 'var(--text)' }}>{value}</p>
      <p className="u-fs-fs-sm u-mt-6px" style={{ color: 'var(--text-muted)' }}>{label}</p>
    </div>
  )
}

export function TrainingAnalyticsPanel({ api, orgId, groups }: { api: ApiClient; orgId: string; groups: GroupRecord[] }) {
  const [weeks, setWeeks] = useState<(typeof RANGES)[number]>(8)
  const [data, setData] = useState<TrainingAnalytics | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    const today = new Date().toISOString().slice(0, 10)
    const to = addDaysISO(weekStartOf(today), 6)
    const from = addDaysISO(weekStartOf(today), -7 * (weeks - 1))
    try {
      setData(await fetchTrainingAnalytics(api, orgId, from, to))
    } catch (err) {
      setError(errorText(err, 'Couldn’t load training analytics. Try again.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId, weeks])

  useEffect(() => { if (orgId) void load() }, [load, orgId])

  const groupName = useCallback((id: string) => {
    if (!id) return 'Whole team'
    return groups.find((g) => g.id === id)?.name ?? 'Other group'
  }, [groups])

  const totals = useMemo(() => {
    if (!data) return null
    const planned = data.groups.reduce((s, g) => s + g.planned_yards, 0)
    const swimmer = data.groups.reduce((s, g) => s + g.swimmer_yards, 0)
    const present = data.groups.reduce((s, g) => s + g.present, 0)
    const absent = data.groups.reduce((s, g) => s + g.absent, 0)
    return { planned, swimmer, pct: present + absent > 0 ? Math.round((present / (present + absent)) * 100) : null }
  }, [data])

  const peak = useMemo(() => Math.max(1, ...(data?.weeks.map((w) => w.planned_yards) ?? [0])), [data])
  const zoneTotal = data ? ZONES.reduce((s, z) => s + (data.zone_yards[z] ?? 0), 0) : 0

  return (
    <div>
      <div role="group" aria-label="Date range" className="u-gap-4px u-mb-20px u-p-4px" style={{ display: 'flex', background: 'var(--bg-surface)', borderRadius: '8px', width: 'fit-content' }}>
        {RANGES.map((r) => (
          <button key={r} type="button" aria-pressed={weeks === r} onClick={() => setWeeks(r)}
            className="u-p-6px-16px u-fs-fs-base" style={{ borderRadius: '8px', cursor: 'pointer', border: weeks === r ? '1px solid var(--border)' : '1px solid transparent', background: weeks === r ? 'var(--bg-elevated)' : 'transparent', color: weeks === r ? 'var(--text)' : 'var(--text-muted)', fontWeight: weeks === r ? 600 : 400 }}>
            {r} weeks
          </button>
        ))}
      </div>

      {error && (
        <div role="alert" className="u-mb-16px u-p-12px-16px u-fs-fs-base u-gap-12px" style={{ borderRadius: '8px', border: '1px solid var(--error-border)', background: 'var(--error-bg)', color: 'var(--error)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{error}</span>
          <button type="button" className="btn btn-secondary u-fs-fs-sm u-p-6px-12px" onClick={load} >Retry</button>
        </div>
      )}
      {loading && !data && <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>Loading analytics…</p>}

      {data && totals && (
        <div className="u-gap-20px" style={{ display: 'flex', flexDirection: 'column', opacity: loading ? 0.6 : 1, transition: 'opacity var(--dur)' }}>
          <div className="card u-p-20px u-gap-20px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))' }}>
            <Stat value={`${(totals.planned / 1000).toFixed(1)}k yd`} label={`planned since ${shortDate(data.from)}`} />
            <Stat value={`${(totals.swimmer / 1000).toFixed(0)}k yd`} label="swum by swimmers marked present" />
            <Stat value={totals.pct == null ? 'Not taken' : `${totals.pct}%`} label="attendance, excused excluded" />
            <Stat value={`${data.structured_practices} of ${data.total_practices}`} label="practices with a structured workout" />
          </div>

          {data.groups.length === 0 ? (
            <div className="card u-p-24px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
              No practices in this range. Schedule practices or assign library workouts to see weekly volume.
            </div>
          ) : (
            <div className="u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 340px), 1fr))' }}>
              {data.groups.map((g) => (
                <section key={g.group_id || 'team'} className="card u-p-16px-20px" >
                  <div className="u-gap-12px u-mb-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
                    <h3 className="section-title u-m-0" >{groupName(g.group_id)} weekly volume</h3>
                    <span className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)', whiteSpace: 'nowrap' }}>
                      {g.attendance_pct == null ? 'no attendance' : `${Math.round(g.attendance_pct)}% present`}
                    </span>
                  </div>
                  <WeeklyBars
                    ariaLabel={`${groupName(g.group_id)} planned yards per week`}
                    max={peak}
                    data={data.weeks.filter((w) => w.group_id === g.group_id).map((w) => ({
                      key: w.week_start, label: shortDate(w.week_start), value: w.planned_yards,
                      detail: `${w.practices} practices, ${w.swimmer_yards.toLocaleString('en-US')} swimmer-yd${w.attendance_pct == null ? '' : `, ${Math.round(w.attendance_pct)}% present`}`,
                    }))}
                  />
                </section>
              ))}
            </div>
          )}

          <section className="card u-p-16px-20px" >
            <h3 className="section-title u-m-0-0-4px" >Intensity mix</h3>
            {zoneTotal === 0 ? (
              <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>No zoned sets yet. Add EN1–SP3 to sets to track intensity.</p>
            ) : (
              <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
                {ZONES.map((z) => {
                  const v = data.zone_yards[z] ?? 0
                  const share = v / zoneTotal
                  return (
                    <div key={z} title={`${z} ${ZONE_LABELS[z]}: ${v.toLocaleString('en-US')} yd`} className="u-gap-12px u-fs-fs-sm" style={{ display: 'grid', gridTemplateColumns: '110px minmax(0,1fr) 64px', alignItems: 'center' }}>
                      <span style={{ color: 'var(--text)' }}><span className="u-ff-font-mono" >{z}</span> <span style={{ color: 'var(--text-muted)' }}>{ZONE_LABELS[z]}</span></span>
                      <span style={{ height: 10, background: 'var(--bg-elevated)', borderRadius: 4, overflow: 'hidden' }}>
                        <span style={{ display: 'block', height: '100%', width: `${share * 100}%`, background: zoneColor(z), borderRadius: 4 }} />
                      </span>
                      <span className="u-ff-font-mono" style={{ textAlign: 'right', color: 'var(--text-subtle)' }}>{Math.round(share * 100)}%</span>
                    </div>
                  )
                })}
              </div>
            )}
          </section>

          <section className="card table-scroll u-p-0" style={{ overflowX: 'auto' }}>
            <table className="u-fs-fs-base" style={{ width: '100%', borderCollapse: 'collapse', minWidth: 520 }}>
              <caption className="u-ff-font-display u-fw-display-weight u-fs-fs-xl" style={{ textAlign: 'left', padding: '16px 20px 6px', color: 'var(--text)' }}>Group totals</caption>
              <thead>
                <tr className="u-fs-fs-xs" style={{ color: 'var(--text-muted)', textAlign: 'right' }}>
                  <th className="u-p-8px-20px u-fw-500" style={{ textAlign: 'left' }}>Group</th>
                  <th className="u-p-8px u-fw-500" >Practices</th>
                  <th className="u-p-8px u-fw-500" >Planned yd</th>
                  <th className="u-p-8px u-fw-500" >Swimmer yd</th>
                  <th className="u-p-8px-20px u-fw-500" >Present</th>
                </tr>
              </thead>
              <tbody>
                {data.groups.map((g) => (
                  <tr key={g.group_id || 'team'} className="u-ff-font-mono u-fs-fs-sm" style={{ borderTop: '1px solid var(--border)', textAlign: 'right' }}>
                    <td className="u-p-8px-20px u-ff-font-body u-fs-fs-base" style={{ textAlign: 'left' }}>{groupName(g.group_id)}</td>
                    <td className="u-p-8px-8px" >{g.practices}</td>
                    <td className="u-p-8px-8px" >{g.planned_yards.toLocaleString('en-US')}</td>
                    <td className="u-p-8px-8px" >{g.swimmer_yards.toLocaleString('en-US')}</td>
                    <td className="u-p-8px-20px" >{g.attendance_pct == null ? '—' : `${Math.round(g.attendance_pct)}% (${g.present}/${g.present + g.absent})`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        </div>
      )}
    </div>
  )
}
