'use client'

import { useEffect, useRef, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchGroups, type GroupRecord } from '@/lib/adminData'
import { reportUIError } from '@/lib/frontend-telemetry'
import { parseBackendDate } from '@/lib/dates'
import {
  fetchSwimmerWellness, fetchWellnessHeatmap,
  type SwimmerWellness, type WellnessCheckIn, type WellnessFlag, type WellnessHeatmap,
} from '@/lib/engagementData'
import { READINESS_STEP_LABELS, readinessStep } from '@/lib/wellness-scale'
import { READINESS_VIZ_CSS, readinessFill } from '@/components/engagement/vizTokens'
import { ReadinessTrend } from '@/components/engagement/ReadinessTrend'

const dayLabel = (d: string) => parseBackendDate(d).toLocaleDateString('en-US', { weekday: 'narrow' })
const dateLabel = (d: string) => parseBackendDate(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })

function FlagChip({ flag }: { flag: WellnessFlag }) {
  const alert = flag.severity === 'alert'
  return (
    <span title={flag.message} className={alert ? 'badge badge-error' : 'badge badge-warning'} style={{ textTransform: 'none', letterSpacing: 0, gap: 4 }}>
      <Icon name={alert ? 'alert-circle' : 'alert-triangle'} size={11} />
      {flag.message}
    </span>
  )
}

type Tip = { x: number; y: number; name: string; date: string; c: WellnessCheckIn | null }

function Heatmap({ data, onSelect, selected }: { data: WellnessHeatmap; onSelect: (id: string) => void; selected: string | null }) {
  const [tip, setTip] = useState<Tip | null>(null)
  const cols = data.days.length
  // Today is the right-most column; on narrow screens start scrolled to it.
  const scrollerRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = scrollerRef.current
    if (el) el.scrollLeft = el.scrollWidth
  }, [data])
  return (
    <div className="readiness-viz" style={{ position: 'relative' }} onMouseLeave={() => setTip(null)}>
      <style>{READINESS_VIZ_CSS}</style>
      <div ref={scrollerRef} style={{ overflowX: 'auto' }}>
        <div role="grid" aria-label="Readiness by swimmer and day" style={{ display: 'grid', gridTemplateColumns: `minmax(120px, 180px) repeat(${cols}, minmax(18px, 1fr))`, gap: 2, minWidth: 120 + cols * 20 }}>
          <span />
          {data.days.map((d, i) => (
            <span key={d} style={{ fontSize: 'var(--fs-2xs)', color: 'var(--text-muted)', textAlign: 'center', paddingBottom: 4 }} title={dateLabel(d)}>
              {i === cols - 1 ? 'Today' : dayLabel(d)}
            </span>
          ))}
          {data.swimmers.map((row) => (
            <div key={row.swimmer_id} role="row" style={{ display: 'contents' }}>
              <button type="button" onClick={() => onSelect(row.swimmer_id)}
                style={{
                  textAlign: 'left', background: 'none', border: 'none', cursor: 'pointer', padding: '4px 6px 4px 0',
                  fontSize: 'var(--fs-base)', fontWeight: selected === row.swimmer_id ? 700 : 500,
                  color: selected === row.swimmer_id ? 'var(--accent)' : 'var(--text)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
                  display: 'flex', alignItems: 'center', gap: 6,
                }}>
                {row.flags.length > 0 && <Icon name="alert-triangle" size={12} style={{ color: row.flags.some((f) => f.severity === 'alert') ? 'var(--error)' : 'var(--warning)', flexShrink: 0 }} aria-label="Flagged" />}
                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{row.display_name}</span>
              </button>
              {row.cells.map((cell) => (
                <span key={cell.date} role="gridcell"
                  aria-label={`${row.display_name}, ${dateLabel(cell.date)}: ${cell.readiness !== null ? `readiness ${cell.readiness}` : 'no check-in'}`}
                  onMouseEnter={(e) => {
                    const host = (e.currentTarget.closest('.readiness-viz') as HTMLElement).getBoundingClientRect()
                    const r = e.currentTarget.getBoundingClientRect()
                    setTip({ x: r.left - host.left + r.width / 2, y: r.top - host.top, name: row.display_name, date: cell.date, c: cell.checkin })
                  }}
                  onClick={() => onSelect(row.swimmer_id)}
                  style={{
                    height: 24, borderRadius: 4, background: readinessFill(readinessStep(cell.readiness)), cursor: 'pointer',
                    outline: cell.checkin?.note ? '1px dashed var(--text-muted)' : undefined, outlineOffset: -3,
                  }} />
              ))}
            </div>
          ))}
          <span style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', paddingTop: 6 }}>Team average</span>
          {data.daily.map((d) => (
            <span key={d.date} style={{ fontSize: 'var(--fs-2xs)', color: 'var(--text-subtle)', textAlign: 'center', paddingTop: 6 }} title={`${d.count} check-ins`}>
              {d.readiness !== null ? d.readiness.toFixed(1) : '–'}
            </span>
          ))}
        </div>
      </div>
      {tip && (
        <div role="status" style={{
          position: 'absolute', left: tip.x, top: tip.y, transform: 'translate(-50%, calc(-100% - 6px))', pointerEvents: 'none', zIndex: 5,
          background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)', borderRadius: 8, padding: '8px 12px', fontSize: 'var(--fs-sm)',
          color: 'var(--text)', boxShadow: 'var(--shadow)', minWidth: 170, maxWidth: 240,
        }}>
          <div style={{ fontWeight: 600 }}>{tip.name} · {dateLabel(tip.date)}</div>
          {tip.c ? (
            <div style={{ color: 'var(--text-subtle)', lineHeight: 1.5 }}>
              Readiness {tip.c.readiness.toFixed(1)} ({READINESS_STEP_LABELS[readinessStep(tip.c.readiness)!]})<br />
              Sleep {tip.c.sleep_hours}h, quality {tip.c.sleep_quality} · soreness {tip.c.soreness}<br />
              Energy {tip.c.energy} · stress {tip.c.stress} · mood {tip.c.mood}
              {tip.c.note && <><br /><em>“{tip.c.note}”</em></>}
            </div>
          ) : <div style={{ color: 'var(--text-muted)' }}>No check-in</div>}
        </div>
      )}
      <div aria-label="Legend" style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center', marginTop: 12, fontSize: 'var(--fs-xs)', color: 'var(--text-subtle)' }}>
        {[1, 2, 3, 4, 5].map((s) => (
          <span key={s} style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
            <span style={{ width: 14, height: 14, borderRadius: 4, background: readinessFill(s) }} />{READINESS_STEP_LABELS[s as 1]}
          </span>
        ))}
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><span style={{ width: 14, height: 14, borderRadius: 4, background: readinessFill(null), border: '1px solid var(--border)' }} />No check-in</span>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><span style={{ width: 14, height: 14, borderRadius: 4, outline: '1px dashed var(--text-muted)', outlineOffset: -3 }} />Has a note</span>
      </div>
    </div>
  )
}

export default function CoachWellnessPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [group, setGroup] = useState('')
  const [data, setData] = useState<WellnessHeatmap | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [asTable, setAsTable] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  const [detail, setDetail] = useState<SwimmerWellness | null>(null)
  const [detailError, setDetailError] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId) return
    fetchGroups(api, orgId).then((g) => setGroups(g.filter((x) => x.is_active))).catch(() => setGroups([]))
  }, [api, orgId])

  useEffect(() => {
    if (!orgId) return
    setError(null)
    setData(null)
    fetchWellnessHeatmap(api, orgId, group, 14)
      .then(setData)
      .catch((err) => setError(reportUIError('coach-wellness-heatmap', err, 'Wellness data could not be loaded.')))
  }, [api, orgId, group, reloadKey])

  useEffect(() => {
    if (!selected || !orgId) { setDetail(null); return }
    setDetail(null)
    setDetailError(null)
    fetchSwimmerWellness(api, orgId, selected, 30)
      .then(setDetail)
      .catch((err) => setDetailError(reportUIError('coach-wellness-swimmer', err, 'This swimmer’s trend could not be loaded.')))
  }, [api, orgId, selected])

  const trendDays = detail ? (() => {
    const out: string[] = []
    const end = parseBackendDate(detail.to)
    for (let i = 29; i >= 0; i--) {
      const d = new Date(end); d.setDate(end.getDate() - i)
      out.push(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`)
    }
    return out
  })() : []

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Wellness</h1>
          <p className="page-subtitle">Last 14 days. Swimmers needing a conversation sort first.</p>
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          <select className="input" value={group} onChange={(e) => { setGroup(e.target.value); setSelected(null) }} aria-label="Group" style={{ minWidth: 160 }}>
            <option value="">All swimmers</option>
            {groups.map((g) => <option key={g.id} value={g.code}>{g.name}</option>)}
          </select>
          <button type="button" className="btn btn-secondary btn-sm" aria-pressed={asTable} onClick={() => setAsTable((v) => !v)}>{asTable ? 'Show heat-map' : 'Show as table'}</button>
        </div>
      </div>

      {error && (
        <div role="alert" className="card" style={{ padding: 16, marginBottom: 16, display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center', color: 'var(--warning)', fontSize: 'var(--fs-base)' }}>
          {error}
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setReloadKey((k) => k + 1)}>Retry</button>
        </div>
      )}

      {!data && !error && <div className="skeleton" style={{ height: 280, borderRadius: 12 }} />}
      {data && (
        <>
          <p style={{ fontSize: 'var(--fs-md)', color: 'var(--text-subtle)', marginBottom: 16 }}>
            <strong style={{ color: 'var(--text)', fontFamily: 'var(--font-display)', fontSize: 'var(--fs-2xl)', fontWeight: 'var(--display-weight)' }}>{data.checked_in_today}</strong> of {data.roster_size} checked in today
            {data.flagged_count > 0 ? <> · <strong style={{ color: 'var(--text)' }}>{data.flagged_count}</strong> flagged</> : ' · nobody flagged'}
          </p>
          {data.roster_size === 0 ? (
            <div className="card" style={{ padding: 24, fontSize: 'var(--fs-md)', color: 'var(--text-subtle)' }}>No swimmers in this group.</div>
          ) : (
            <section className="card" style={{ padding: 16 }}>
              {asTable ? (
                <div style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 'var(--fs-base)' }}>
                    <thead>
                      <tr style={{ textAlign: 'left', color: 'var(--text-muted)', fontSize: 'var(--fs-sm)' }}>
                        <th style={{ padding: 6 }}>Swimmer</th><th style={{ padding: 6 }}>Avg readiness</th><th style={{ padding: 6 }}>Avg sleep</th><th style={{ padding: 6 }}>Check-ins</th><th style={{ padding: 6 }}>Flags</th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.swimmers.map((r) => (
                        <tr key={r.swimmer_id} style={{ borderTop: '1px solid var(--border)', cursor: 'pointer' }} onClick={() => setSelected(r.swimmer_id)}>
                          <td style={{ padding: 6, color: 'var(--text)' }}>{r.display_name}</td>
                          <td style={{ padding: 6 }}>{r.averages.count ? r.averages.readiness.toFixed(1) : '–'}</td>
                          <td style={{ padding: 6 }}>{r.averages.count ? `${r.averages.sleep_hours}h` : '–'}</td>
                          <td style={{ padding: 6 }}>{r.averages.count} / {data.days.length}</td>
                          <td style={{ padding: 6 }}><span style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>{r.flags.map((f) => <FlagChip key={f.code} flag={f} />)}</span></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : <Heatmap data={data} onSelect={setSelected} selected={selected} />}
            </section>
          )}
        </>
      )}

      {selected && (
        <section className="card" style={{ padding: 20, marginTop: 16 }} aria-live="polite">
          {detailError ? <p role="alert" style={{ fontSize: 'var(--fs-base)', color: 'var(--warning)' }}>{detailError}</p> : !detail ? (
            <div className="skeleton" style={{ height: 200, borderRadius: 12 }} />
          ) : (
            <>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', marginBottom: 8 }}>
                <h2 className="section-title">{detail.swimmer.display_name}</h2>
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => setSelected(null)} aria-label="Close swimmer trend"><Icon name="x" size={14} /></button>
              </div>
              {detail.flags.length > 0 && <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 12 }}>{detail.flags.map((f) => <FlagChip key={f.code} flag={f} />)}</div>}
              <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginBottom: 8 }}>
                30-day averages: readiness {detail.averages.readiness.toFixed(1)}, sleep {detail.averages.sleep_hours}h, soreness {detail.averages.soreness}, stress {detail.averages.stress} ({detail.averages.count} check-ins)
              </p>
              <ReadinessTrend checkins={detail.checkins} days={trendDays} />
              {detail.checkins.some((c) => c.note) && (
                <div style={{ marginTop: 16 }}>
                  <h3 className="card-title" style={{ marginBottom: 6 }}>Notes</h3>
                  <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 6 }}>
                    {detail.checkins.filter((c) => c.note).slice(-6).reverse().map((c) => (
                      <li key={c.id} style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }}><span style={{ color: 'var(--text-muted)' }}>{dateLabel(c.date)}:</span> {c.note}</li>
                    ))}
                  </ul>
                </div>
              )}
            </>
          )}
        </section>
      )}
    </div>
  )
}
