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
      <p style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-3xl)', lineHeight: 1, color: 'var(--text)', margin: 0 }}>{value}</p>
      <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginTop: '6px' }}>{label}</p>
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
      <div role="group" aria-label="Date range" style={{ display: 'flex', gap: '4px', marginBottom: '20px', background: 'var(--bg-surface)', padding: '4px', borderRadius: '8px', width: 'fit-content' }}>
        {RANGES.map((r) => (
          <button key={r} type="button" aria-pressed={weeks === r} onClick={() => setWeeks(r)}
            style={{ padding: '6px 16px', fontSize: 'var(--fs-base)', borderRadius: '8px', cursor: 'pointer', border: weeks === r ? '1px solid var(--border)' : '1px solid transparent', background: weeks === r ? 'var(--bg-elevated)' : 'transparent', color: weeks === r ? 'var(--text)' : 'var(--text-muted)', fontWeight: weeks === r ? 600 : 400 }}>
            {r} weeks
          </button>
        ))}
      </div>

      {error && (
        <div role="alert" style={{ marginBottom: '16px', padding: '12px 16px', borderRadius: '8px', border: '1px solid var(--error-border)', background: 'var(--error-bg)', color: 'var(--error)', fontSize: 'var(--fs-base)', display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center' }}>
          <span>{error}</span>
          <button type="button" className="btn btn-secondary" onClick={load} style={{ fontSize: 'var(--fs-sm)', padding: '6px 12px' }}>Retry</button>
        </div>
      )}
      {loading && !data && <p style={{ color: 'var(--text-muted)', fontSize: 'var(--fs-base)' }}>Loading analytics…</p>}

      {data && totals && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '20px', opacity: loading ? 0.6 : 1, transition: 'opacity var(--dur)' }}>
          <div className="card" style={{ padding: '20px', display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))', gap: '20px' }}>
            <Stat value={`${(totals.planned / 1000).toFixed(1)}k yd`} label={`planned since ${shortDate(data.from)}`} />
            <Stat value={`${(totals.swimmer / 1000).toFixed(0)}k yd`} label="swum by swimmers marked present" />
            <Stat value={totals.pct == null ? 'Not taken' : `${totals.pct}%`} label="attendance, excused excluded" />
            <Stat value={`${data.structured_practices} of ${data.total_practices}`} label="practices with a structured workout" />
          </div>

          {data.groups.length === 0 ? (
            <div className="card" style={{ padding: '24px', textAlign: 'center', color: 'var(--text-muted)', fontSize: 'var(--fs-base)' }}>
              No practices in this range. Schedule practices or assign library workouts to see weekly volume.
            </div>
          ) : (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 340px), 1fr))', gap: '16px' }}>
              {data.groups.map((g) => (
                <section key={g.group_id || 'team'} className="card" style={{ padding: '16px 20px' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'baseline', marginBottom: '12px' }}>
                    <h3 className="section-title" style={{ margin: 0 }}>{groupName(g.group_id)} weekly volume</h3>
                    <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', whiteSpace: 'nowrap' }}>
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

          <section className="card" style={{ padding: '16px 20px' }}>
            <h3 className="section-title" style={{ margin: '0 0 4px' }}>Intensity mix</h3>
            {zoneTotal === 0 ? (
              <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>No zoned sets yet. Add EN1–SP3 to sets to track intensity.</p>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {ZONES.map((z) => {
                  const v = data.zone_yards[z] ?? 0
                  const share = v / zoneTotal
                  return (
                    <div key={z} title={`${z} ${ZONE_LABELS[z]}: ${v.toLocaleString('en-US')} yd`} style={{ display: 'grid', gridTemplateColumns: '110px minmax(0,1fr) 64px', gap: '12px', alignItems: 'center', fontSize: 'var(--fs-sm)' }}>
                      <span style={{ color: 'var(--text)' }}><span style={{ fontFamily: 'var(--font-mono)' }}>{z}</span> <span style={{ color: 'var(--text-muted)' }}>{ZONE_LABELS[z]}</span></span>
                      <span style={{ height: 10, background: 'var(--bg-elevated)', borderRadius: 4, overflow: 'hidden' }}>
                        <span style={{ display: 'block', height: '100%', width: `${share * 100}%`, background: zoneColor(z), borderRadius: 4 }} />
                      </span>
                      <span style={{ textAlign: 'right', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>{Math.round(share * 100)}%</span>
                    </div>
                  )
                })}
              </div>
            )}
          </section>

          <section className="card" style={{ padding: 0, overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 'var(--fs-base)', minWidth: 520 }}>
              <caption style={{ textAlign: 'left', padding: '16px 20px 6px', fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: 'var(--text)' }}>Group totals</caption>
              <thead>
                <tr style={{ color: 'var(--text-muted)', fontSize: 'var(--fs-xs)', textAlign: 'right' }}>
                  <th style={{ textAlign: 'left', padding: '8px 20px', fontWeight: 500 }}>Group</th>
                  <th style={{ padding: '8px', fontWeight: 500 }}>Practices</th>
                  <th style={{ padding: '8px', fontWeight: 500 }}>Planned yd</th>
                  <th style={{ padding: '8px', fontWeight: 500 }}>Swimmer yd</th>
                  <th style={{ padding: '8px 20px', fontWeight: 500 }}>Present</th>
                </tr>
              </thead>
              <tbody>
                {data.groups.map((g) => (
                  <tr key={g.group_id || 'team'} style={{ borderTop: '1px solid var(--border)', textAlign: 'right', fontFamily: 'var(--font-mono)', fontSize: 'var(--fs-sm)' }}>
                    <td style={{ textAlign: 'left', padding: '8px 20px', fontFamily: 'var(--font-body)', fontSize: 'var(--fs-base)' }}>{groupName(g.group_id)}</td>
                    <td style={{ padding: '8px 8px' }}>{g.practices}</td>
                    <td style={{ padding: '8px 8px' }}>{g.planned_yards.toLocaleString('en-US')}</td>
                    <td style={{ padding: '8px 8px' }}>{g.swimmer_yards.toLocaleString('en-US')}</td>
                    <td style={{ padding: '8px 20px' }}>{g.attendance_pct == null ? '—' : `${Math.round(g.attendance_pct)}% (${g.present}/${g.present + g.absent})`}</td>
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
