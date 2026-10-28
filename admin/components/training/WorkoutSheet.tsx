'use client'

import type { Workout, WorkoutSet } from '@/lib/trainingData'
import {
  SECTION_LABELS, ZONE_LABELS, formatDistance, formatDuration, setDetails, setDistance, setHeadline,
  zoneColor, zoneNeedsLightText, zoneShares,
} from '@/lib/workout-format'

type Variant = 'compact' | 'deck'

const SIZES: Record<Variant, { head: number; detail: number; section: number; gap: number; swatch: number }> = {
  compact: { head: 14, detail: 13, section: 17, gap: 6, swatch: 3 },
  deck: { head: 34, detail: 22, section: 40, gap: 16, swatch: 8 },
}

function ZoneTag({ zone, variant }: { zone?: string; variant: Variant }) {
  if (!zone) return null
  const big = variant === 'deck'
  return (
    <span
      title={ZONE_LABELS[zone as keyof typeof ZONE_LABELS] ?? zone}
      className="u-ff-font-mono u-fw-600" style={{ fontSize: big ? 'var(--fs-xl)' : 'var(--fs-2xs)', padding: big ? '4px 12px' : '1px 6px', borderRadius: 'var(--r-xs)', background: zoneColor(zone), color: zoneNeedsLightText(zone) ? 'var(--text-on-accent)' : 'var(--text)', flexShrink: 0 }}
    >
      {zone}
    </span>
  )
}

function SetRow({ set, variant, depth, inheritedZone }: { set: WorkoutSet; variant: Variant; depth: number; inheritedZone?: string }) {
  const sz = SIZES[variant]
  const zone = set.zone || inheritedZone
  const details = setDetails(set)
  const isRound = !!set.sets?.length
  return (
    <li style={{ listStyle: 'none', display: 'flex', gap: sz.gap * 1.5, alignItems: 'stretch' }}>
      <span aria-hidden style={{ width: sz.swatch, borderRadius: sz.swatch, background: zoneColor(zone), flexShrink: 0 }} />
      <div style={{ flex: 1, minWidth: 0, padding: `${sz.gap / 2}px 0` }}>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: sz.gap * 1.5, flexWrap: 'wrap' }}>
          <span className="u-ff-font-mono u-fw-600" style={{ fontSize: sz.head, color: 'var(--text)', letterSpacing: '-0.01em' }}>
            {setHeadline(set)}
          </span>
          {details.length > 0 && (
            <span className="u-ff-font-body" style={{ fontSize: sz.detail, color: 'var(--text-subtle)' }}>
              {details.join('  ')}
            </span>
          )}
          {set.zone && <ZoneTag zone={set.zone} variant={variant} />}
          {isRound && (
            <span className="u-ml-auto u-ff-font-mono" style={{ fontSize: sz.detail, color: 'var(--text-muted)' }}>
              {setDistance(set).toLocaleString('en-US')}
            </span>
          )}
        </div>
        {isRound && (
          <ul className="u-p-0" style={{ margin: `${sz.gap}px 0 0`, display: 'flex', flexDirection: 'column', gap: sz.gap }}>
            {set.sets!.map((child, i) => (
              <SetRow key={i} set={child} variant={variant} depth={depth + 1} inheritedZone={zone} />
            ))}
          </ul>
        )}
      </div>
    </li>
  )
}

export function ZoneBar({ byZone, height = 10 }: { byZone: Record<string, number>; height?: number }) {
  const shares = zoneShares(byZone)
  if (shares.length === 0) return null
  return (
    <div>
      <div role="img" aria-label={shares.map((s) => `${s.zone} ${Math.round(s.share * 100)}%`).join(', ')}
        style={{ display: 'flex', gap: 2, height, borderRadius: 4, overflow: 'hidden' }}>
        {shares.map((s) => (
          <span key={s.zone} title={`${s.zone}: ${s.distance.toLocaleString('en-US')} (${Math.round(s.share * 100)}%)`}
            style={{ flexGrow: s.share, flexBasis: 0, minWidth: 3, background: zoneColor(s.zone === 'unassigned' ? undefined : s.zone) }} />
        ))}
      </div>
      <div className="u-mt-6px" style={{ display: 'flex', flexWrap: 'wrap', gap: '4px 12px' }}>
        {shares.map((s) => (
          <span key={s.zone} className="u-gap-4px u-fs-fs-xs" style={{ display: 'inline-flex', alignItems: 'center', color: 'var(--text-subtle)' }}>
            <span aria-hidden style={{ width: 8, height: 8, borderRadius: 2, background: zoneColor(s.zone === 'unassigned' ? undefined : s.zone) }} />
            {s.zone === 'unassigned' ? 'No zone' : s.zone} {Math.round(s.share * 100)}%
          </span>
        ))}
      </div>
    </div>
  )
}

export function WorkoutSheet({ workout, variant = 'compact' }: { workout: Pick<Workout, 'sections' | 'totals' | 'course'>; variant?: Variant }) {
  const sz = SIZES[variant]
  const unit = workout.totals.unit
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: sz.gap * 3 }}>
      {workout.sections.map((section, i) => {
        const total = workout.totals.by_section[i]
        return (
          <section key={i} style={{ breakInside: 'avoid' }}>
            <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', borderBottom: '1px solid var(--border)', paddingBottom: sz.gap, marginBottom: sz.gap * 1.5 }}>
              <h3 className="u-ff-font-display u-fw-display-weight u-m-0" style={{ fontSize: sz.section, color: 'var(--text)', lineHeight: 1.1 }}>
                {section.kind === 'custom' && section.title ? section.title : SECTION_LABELS[section.kind]}
              </h3>
              {total && (
                <span className="u-ff-font-mono" style={{ fontSize: sz.detail, color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>
                  {formatDistance(total.distance, unit)}{variant === 'compact' ? ` · ${formatDuration(total.estimated_seconds)}` : ''}
                </span>
              )}
            </div>
            {section.notes && <p style={{ fontSize: sz.detail, color: 'var(--text-subtle)', margin: `0 0 ${sz.gap}px` }}>{section.notes}</p>}
            <ul className="u-m-0 u-p-0" style={{ display: 'flex', flexDirection: 'column', gap: sz.gap }}>
              {section.sets.map((set, j) => <SetRow key={j} set={set} variant={variant} depth={0} />)}
            </ul>
          </section>
        )
      })}
    </div>
  )
}
