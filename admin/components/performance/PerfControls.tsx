'use client'

import type { CSSProperties, ReactNode } from 'react'

export const FIELD_STYLE: CSSProperties = {
  padding: '8px 12px',
  borderRadius: 'var(--r-sm)',
  border: '1px solid var(--border)',
  background: 'var(--bg-elevated)',
  color: 'var(--text)',
  fontSize: 'var(--fs-base)',
  fontFamily: 'var(--font-body)',
  boxSizing: 'border-box',
  minWidth: 0,
}

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
            style={{
              padding: '8px 12px',
              border: 'none',
              borderRight: '1px solid var(--border)',
              background: active ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
              color: active ? 'var(--accent)' : 'var(--text-subtle)',
              fontSize: 'var(--fs-sm)',
              fontWeight: active ? 600 : 400,
              cursor: 'pointer',
              whiteSpace: 'nowrap',
            }}
          >
            {o.label}
          </button>
        )
      })}
    </div>
  )
}

export function LoadErrorBanner({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div role="alert" style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap', margin: '0 0 16px', padding: '12px 16px', borderRadius: 'var(--r-sm)', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: 'var(--fs-base)' }}>
      <span style={{ flex: '1 1 200px' }}>{message}</span>
      {onRetry ? <button type="button" className="btn btn-sm btn-secondary" onClick={onRetry}>Try again</button> : null}
    </div>
  )
}

export function Notice({ tone = 'info', children }: { tone?: 'info' | 'success' | 'warning'; children: ReactNode }) {
  return (
    <div role="status" style={{ margin: '0 0 16px', padding: '12px 16px', borderRadius: 'var(--r-sm)', background: `var(--${tone}-bg)`, border: `1px solid var(--${tone}-border)`, color: `var(--${tone})`, fontSize: 'var(--fs-base)' }}>
      {children}
    </div>
  )
}

export function FilterRow({ children }: { children: ReactNode }) {
  return <div style={{ display: 'flex', flexWrap: 'wrap', gap: '12px', alignItems: 'center', marginBottom: '20px' }}>{children}</div>
}
