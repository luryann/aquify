'use client'

import { useState } from 'react'

export type WeeklyBarDatum = { key: string; label: string; value: number; target?: number; detail?: string }

function compact(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 10000 ? 0 : 1)}k`
  return String(n)
}

/**
 * Weekly volume columns with an optional target step line. One measure, one
 * axis; the hover layer reports the exact week values.
 */
export function WeeklyBars({
  data, max, height = 120, ariaLabel, valueLabel = 'Planned', targetLabel = 'Target', unit = 'yd',
}: {
  data: WeeklyBarDatum[]
  max?: number
  height?: number
  ariaLabel: string
  valueLabel?: string
  targetLabel?: string
  unit?: string
}) {
  const [hover, setHover] = useState<number | null>(null)
  const peak = Math.max(1, max ?? Math.max(...data.map((d) => Math.max(d.value, d.target ?? 0))))
  const n = Math.max(1, data.length)
  const W = 600
  const slot = W / n
  const barW = Math.max(2, slot - 2) // 2px surface gap between adjacent bars
  const y = (v: number) => height - (v / peak) * height
  const hasTarget = data.some((d) => (d.target ?? 0) > 0)
  let path = ''
  data.forEach((d, i) => {
    if (!d.target) return
    const x0 = i * slot
    const x1 = x0 + slot
    const yy = y(d.target)
    const prev = data[i - 1]?.target
    path += prev ? ` L${x0},${yy} L${x1},${yy}` : ` M${x0},${yy} L${x1},${yy}`
  })
  const hovered = hover != null ? data[hover] : null

  return (
    <figure className="u-m-0" style={{ position: 'relative' }}>
      {hasTarget && (
        <div className="u-gap-16px u-fs-fs-xs u-mb-6px" style={{ display: 'flex', color: 'var(--text-subtle)' }}>
          <span className="u-gap-4px" style={{ display: 'inline-flex', alignItems: 'center' }}>
            <span aria-hidden style={{ width: 10, height: 10, borderRadius: 2, background: 'var(--accent)' }} />{valueLabel}
          </span>
          <span className="u-gap-4px" style={{ display: 'inline-flex', alignItems: 'center' }}>
            <span aria-hidden style={{ width: 14, height: 0, borderTop: '2px solid var(--text)' }} />{targetLabel}
          </span>
        </div>
      )}
      <div className="u-gap-6px" style={{ display: 'flex' }}>
        <div aria-hidden style={{ display: 'flex', flexDirection: 'column', justifyContent: 'space-between', height, fontSize: 'var(--fs-2xs)', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', textAlign: 'right', minWidth: 28 }}>
          <span>{compact(peak)}</span>
          <span>0</span>
        </div>
        <svg role="img" aria-label={ariaLabel} viewBox={`0 0 ${W} ${height}`} preserveAspectRatio="none"
          style={{ flex: 1, minWidth: 0, width: '100%', height, display: 'block', overflow: 'visible' }}
          onMouseLeave={() => setHover(null)}>
          <line x1={0} x2={W} y1={height} y2={height} stroke="var(--border)" strokeWidth={1} vectorEffect="non-scaling-stroke" />
          {data.map((d, i) => {
            const h = Math.max(0, height - y(d.value))
            const x = i * slot + 1
            return (
              <g key={d.key}>
                {h > 0 && (
                  <path
                    d={`M${x},${height} L${x},${height - h + Math.min(4, h)} Q${x},${height - h} ${x + Math.min(4, barW / 2)},${height - h} L${x + barW - Math.min(4, barW / 2)},${height - h} Q${x + barW},${height - h} ${x + barW},${height - h + Math.min(4, h)} L${x + barW},${height} Z`}
                    fill="var(--accent)" opacity={hover == null || hover === i ? 1 : 0.55}
                  />
                )}
                <rect x={i * slot} y={0} width={slot} height={height} fill="transparent" onMouseEnter={() => setHover(i)} onFocus={() => setHover(i)} />
              </g>
            )
          })}
          {path && <path d={path.trim()} fill="none" stroke="var(--text)" strokeWidth={2} vectorEffect="non-scaling-stroke" pointerEvents="none" />}
        </svg>
      </div>
      <div aria-hidden className="u-fs-fs-2xs u-mt-4px u-pl-32px" style={{ display: 'flex', justifyContent: 'space-between', color: 'var(--text-muted)' }}>
        <span>{data[0]?.label}</span>
        <span>{data[data.length - 1]?.label}</span>
      </div>
      {hovered && (
        <div role="tooltip" className="u-p-8px-12px u-fs-fs-sm" style={{ position: 'absolute', top: 0, left: `min(calc(34px + ${((hover! + 0.5) / n) * 100}% ), calc(100% - 170px))`, background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', boxShadow: 'var(--shadow-sm)', pointerEvents: 'none', minWidth: 140, zIndex: 2 }}>
          <div className="u-mb-2px" style={{ color: 'var(--text-subtle)' }}>Week of {hovered.label}</div>
          <div>{valueLabel}: {hovered.value.toLocaleString('en-US')} {unit}</div>
          {hovered.target ? <div>{targetLabel}: {hovered.target.toLocaleString('en-US')} {unit}</div> : null}
          {hovered.detail && <div style={{ color: 'var(--text-subtle)' }}>{hovered.detail}</div>}
        </div>
      )}
    </figure>
  )
}
