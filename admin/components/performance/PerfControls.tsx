'use client'

import type { CSSProperties, ReactNode } from 'react'
import { AGE_GROUPS, AGE_GROUP_LABELS } from '@/lib/performanceData'

export const FIELD_STYLE: CSSProperties = { width: '100%' }

type Option<T extends string> = { value: T; label: string }

/** Compact segmented control used for course / gender / age-group filters. */
export function Segmented<T extends string>({
  value, options, onChange, label,
}: { value: T; options: Option<T>[]; onChange: (v: T) => void; label: string }) {
  return (
    <div role="radiogroup" aria-label={label} style={{ display: 'inline-flex', border: '1px solid var(--border)', borderRadius: 'var(--r-sm)', overflow: 'hidden', maxWidth: '100%', overflowX: 'auto' }}>
      {options.map((o) => {
        const active = o.value === value
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(o.value)}
            className="u-p-8px-12px u-fs-fs-sm" style={{ border: 'none', borderRight: '1px solid var(--border)', background: active ? 'var(--accent-subtle)' : 'var(--bg-elevated)', color: active ? 'var(--accent)' : 'var(--text-subtle)', fontWeight: active ? 600 : 400, cursor: 'pointer', whiteSpace: 'nowrap' }}
          >
            {o.label}
          </button>
        )
      })}
    </div>
  )
}

export function Notice({ tone = 'info', children }: { tone?: 'info' | 'success' | 'warning'; children: ReactNode }) {
  return (
    <div role="status" className="u-m-0-0-16px u-p-12px-16px u-fs-fs-base" style={{ borderRadius: 'var(--r-sm)', background: `var(--${tone}-bg)`, border: `1px solid var(--${tone}-border)`, color: `var(--${tone})` }}>
      {children}
    </div>
  )
}

export function FilterRow({ children }: { children: ReactNode }) {
  return <div className="u-gap-12px u-mb-20px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>{children}</div>
}

/** Small caption above a form control. */
export function LabeledField({ label, children }: { label: string; children: ReactNode }) {
  return <label className="u-gap-4px u-fs-fs-sm" style={{ display: 'grid', color: 'var(--text-subtle)' }}>{label}{children}</label>
}

/** Women/Men at the Open age group, Girls/Boys for the age-group boards. */
export function GenderSegmented({ value, onChange, ageGroup }: { value: 'F' | 'M'; onChange: (v: 'F' | 'M') => void; ageGroup: string }) {
  const open = ageGroup === 'OPEN'
  return <Segmented label="Gender" value={value} onChange={onChange} options={[{ value: 'F', label: open ? 'Women' : 'Girls' }, { value: 'M', label: open ? 'Men' : 'Boys' }]} />
}

export function AgeGroupSelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <select aria-label="Age group" value={value} onChange={(e) => onChange(e.target.value)} style={FIELD_STYLE}>
      {AGE_GROUPS.map((g) => <option key={g} value={g}>{AGE_GROUP_LABELS[g]}</option>)}
    </select>
  )
}
