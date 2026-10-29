'use client'

import { COMPLIANCE_LABELS, formatHours, hoursProgress, type ComplianceStatus } from '@/lib/volunteer-state'

/**
 * Capacity as a row of lane markers: filled spots are solid accent bars,
 * open spots are hollow. Collapses to a count past 24 spots.
 */
export function SpotPips({ filled, capacity }: { filled: number; capacity: number }) {
  const label = `${filled} of ${capacity} spots filled`
  if (capacity > 24) {
    return (
      <span title={label} className="u-ff-font-mono u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>
        {filled}/{capacity}
      </span>
    )
  }
  return (
    <span role="img" aria-label={label} title={label} className="u-gap-4px" style={{ display: 'inline-flex', flexWrap: 'wrap', maxWidth: '180px' }}>
      {Array.from({ length: capacity }, (_, i) => (
        <span
          key={i}
          style={{
            width: '6px', height: '14px', borderRadius: '2px',
            background: i < filled ? 'var(--accent)' : 'transparent',
            border: `1.5px solid ${i < filled ? 'var(--accent)' : 'var(--border-strong)'}`,
          }}
        />
      ))}
    </span>
  )
}

/**
 * Hours progress styled as a lane rope: earned hours are solid, scheduled
 * (signed up, not yet worked) hours are striped, the rest is open water.
 */
export function HoursMeter({ required, earned, scheduled, compact = false }: {
  required: number
  earned: number
  scheduled: number
  compact?: boolean
}) {
  const p = hoursProgress(required, earned, scheduled)
  const h = compact ? 8 : 14
  return (
    <div>
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={required}
        aria-valuenow={Math.min(earned, required)}
        aria-label={`${formatHours(earned)} earned of ${formatHours(required)} required`}
        style={{
          position: 'relative', height: `${h}px`, borderRadius: `${h}px`, overflow: 'hidden',
          background: 'var(--bg-elevated)', border: '1px solid var(--border)',
        }}
      >
        <div style={{
          position: 'absolute', inset: 0, background: 'var(--accent)',
          transform: `scaleX(${p.earned})`, transformOrigin: 'left',
          transition: 'transform var(--dur-slow, 400ms) var(--ease-out, ease-out)',
        }} />
        <div style={{
          position: 'absolute', top: 0, bottom: 0, left: `${p.earned * 100}%`, width: `${p.scheduled * 100}%`,
          background: 'repeating-linear-gradient(135deg, var(--accent-subtle) 0 5px, var(--accent-border) 5px 7px)',
        }} />
      </div>
      {!compact ? (
        <div className="u-gap-16px u-mt-8px u-fs-fs-sm" style={{ display: 'flex', flexWrap: 'wrap', color: 'var(--text-muted)' }}>
          <Legend swatch="var(--accent)">{formatHours(earned)} earned</Legend>
          {scheduled > 0 ? <Legend swatch="var(--accent-border)">{formatHours(scheduled)} signed up</Legend> : null}
          <span>{formatHours(required)} required</span>
        </div>
      ) : null}
    </div>
  )
}

function Legend({ swatch, children }: { swatch: string; children: React.ReactNode }) {
  return (
    <span className="u-gap-6px" style={{ display: 'inline-flex', alignItems: 'center' }}>
      <span style={{ width: '10px', height: '10px', borderRadius: '4px', background: swatch }} />
      {children}
    </span>
  )
}

const COMPLIANCE_BADGE: Record<ComplianceStatus, string> = {
  exempt: 'badge badge-default',
  met: 'badge badge-success',
  on_track: 'badge badge-info',
  short: 'badge badge-warning',
  bought_out: 'badge badge-accent',
}

export function ComplianceBadge({ status }: { status: ComplianceStatus }) {
  return <span className={COMPLIANCE_BADGE[status]}>{COMPLIANCE_LABELS[status]}</span>
}

export function InlineNotice({ tone, children }: { tone: 'error' | 'success'; children: React.ReactNode }) {
  return (
    <div role={tone === 'error' ? 'alert' : 'status'} className="u-p-12px-12px u-mb-12px u-fs-fs-base" style={{ borderRadius: 'var(--r-md, 8px)', background: `var(--${tone}-bg)`, border: `1px solid var(--${tone}-border)`, color: `var(--${tone})` }}>
      {children}
    </div>
  )
}
