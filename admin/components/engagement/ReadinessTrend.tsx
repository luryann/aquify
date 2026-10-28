'use client'

import { useState } from 'react'
import type { WellnessCheckIn } from '@/lib/engagementData'
import { parseBackendDate } from '@/lib/dates'
import { READINESS_VIZ_CSS } from './vizTokens'

const W = 640
const H = 180
const PAD = { l: 28, r: 12, t: 12, b: 24 }

function fmt(date: string) {
  return parseBackendDate(date).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

/** Single-series readiness line (1–5) with a hover crosshair + tooltip. */
export function ReadinessTrend({ checkins, days }: { checkins: WellnessCheckIn[]; days: string[] }) {
  const [hover, setHover] = useState<number | null>(null)
  if (days.length === 0) return null
  const byDate = new Map(checkins.map((c) => [c.date, c]))
  const x = (i: number) => PAD.l + (days.length === 1 ? 0 : (i / (days.length - 1)) * (W - PAD.l - PAD.r))
  const y = (v: number) => PAD.t + ((5 - v) / 4) * (H - PAD.t - PAD.b)
  const points = days.map((d, i) => ({ i, d, c: byDate.get(d) })).filter((p) => p.c)
  // Break the line across missing days so gaps read as gaps.
  const segments: string[] = []
  let current = ''
  days.forEach((d, i) => {
    const c = byDate.get(d)
    if (!c) { if (current) segments.push(current); current = ''; return }
    current += `${current ? 'L' : 'M'}${x(i).toFixed(1)},${y(c.readiness).toFixed(1)}`
  })
  if (current) segments.push(current)
  const hovered = hover !== null ? { d: days[hover], c: byDate.get(days[hover]) } : null

  return (
    <div className="readiness-viz" style={{ position: 'relative' }}>
      <style>{READINESS_VIZ_CSS}</style>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        width="100%"
        role="img"
        aria-label={`Readiness over ${days.length} days, ${points.length} check-ins`}
        onMouseLeave={() => setHover(null)}
        onMouseMove={(e) => {
          const rect = e.currentTarget.getBoundingClientRect()
          const px = ((e.clientX - rect.left) / rect.width) * W
          const i = Math.round(((px - PAD.l) / (W - PAD.l - PAD.r)) * (days.length - 1))
          setHover(Math.max(0, Math.min(days.length - 1, i)))
        }}
        style={{ display: 'block', touchAction: 'pan-y' }}
      >
        {[1, 2, 3, 4, 5].map((v) => (
          <g key={v}>
            <line x1={PAD.l} x2={W - PAD.r} y1={y(v)} y2={y(v)} stroke="var(--rd-grid)" strokeWidth={1} />
            <text x={PAD.l - 8} y={y(v) + 4} textAnchor="end" fontSize="11" fill="var(--text-muted)">{v}</text>
          </g>
        ))}
        {[0, days.length - 1].map((i) => (
          <text key={i} x={x(i)} y={H - 6} textAnchor={i === 0 ? 'start' : 'end'} fontSize="11" fill="var(--text-muted)">{fmt(days[i])}</text>
        ))}
        {hover !== null && <line x1={x(hover)} x2={x(hover)} y1={PAD.t} y2={H - PAD.b} stroke="var(--text-muted)" strokeWidth={1} strokeDasharray="3 3" />}
        {segments.map((d) => <path key={d} d={d} fill="none" stroke="var(--rd-line)" strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />)}
        {points.map((p) => (
          <circle key={p.d} cx={x(p.i)} cy={y(p.c!.readiness)} r={hover === p.i ? 5 : 4} fill="var(--rd-line)" stroke="var(--bg-elevated)" strokeWidth={2} />
        ))}
      </svg>
      {hovered && (
        <div
          role="status"
          className="u-p-8px-12px u-fs-fs-sm" style={{ position: 'absolute', top: 0, left: `${(x(hover!) / W) * 100}%`, transform: `translateX(${hover! > days.length / 2 ? '-105%' : '5%'})`, background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)', borderRadius: 8, color: 'var(--text)', boxShadow: 'var(--shadow)', pointerEvents: 'none', minWidth: 140 }}
        >
          <div className="u-fw-600 u-mb-2px" >{fmt(hovered.d)}</div>
          {hovered.c ? (
            <div className="u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>
              Readiness {hovered.c.readiness.toFixed(1)}<br />
              Sleep {hovered.c.sleep_hours}h · energy {hovered.c.energy} · soreness {hovered.c.soreness}
            </div>
          ) : <div style={{ color: 'var(--text-muted)' }}>No check-in</div>}
        </div>
      )}
    </div>
  )
}
