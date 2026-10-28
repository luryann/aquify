'use client'

import type { CSSProperties, ReactNode } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { formatCents } from '@/lib/financeData'

export function useFinanceContext() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  return { api, token: session?.token, orgId: session?.user.orgId ?? '' }
}


export const sectionTitle: CSSProperties = {
  fontFamily: 'var(--font-display)',
  fontSize: 'var(--fs-3xl)',
  fontWeight: 'var(--display-weight)',
  margin: '0 0 4px',
  color: 'var(--text)',
}

export const tableStyle: CSSProperties = { width: '100%', borderCollapse: 'collapse', fontSize: 'var(--fs-base)' }
export const thStyle: CSSProperties = {
  textAlign: 'left',
  padding: '12px 12px',
  color: 'var(--text-subtle)',
  fontWeight: 600,
  borderBottom: '1px solid var(--border-strong)',
  whiteSpace: 'nowrap',
}
export const tdStyle: CSSProperties = { padding: '12px 12px', borderBottom: '1px solid var(--border)', color: 'var(--text)', verticalAlign: 'top' }
export const numStyle: CSSProperties = { ...tdStyle, textAlign: 'right', fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' }

export function Money({ cents, strong, muted }: { cents: number; strong?: boolean; muted?: boolean }) {
  return (
    <span style={{ fontVariantNumeric: 'tabular-nums', fontWeight: strong ? 700 : 500, color: muted ? 'var(--text-subtle)' : cents < 0 ? 'var(--success)' : 'var(--text)' }}>
      {formatCents(cents)}
    </span>
  )
}

export function Banner({ tone, children, onRetry }: { tone: 'error' | 'success' | 'info'; children: ReactNode; onRetry?: () => void }) {
  return (
    <div
      role={tone === 'error' ? 'alert' : 'status'}
      style={{
        display: 'flex',
        gap: 12,
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '12px 16px',
        borderRadius: 12,
        margin: '12px 0',
        fontSize: 'var(--fs-base)',
        background: `var(--${tone}-bg)`,
        border: `1px solid var(--${tone}-border)`,
        color: 'var(--text)',
      }}
    >
      <span>{children}</span>
      {onRetry ? (
        <button type="button" className="btn btn-ghost btn-sm" onClick={onRetry}>
          Retry
        </button>
      ) : null}
    </div>
  )
}

export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <label className="form-field" style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 0 }}>
      <span className="form-label">{label}</span>
      {children}
      {hint ? <span className="form-hint">{hint}</span> : null}
    </label>
  )
}

export const formGrid: CSSProperties = { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 16 }

export function ScrollTable({ children }: { children: ReactNode }) {
  return <div style={{ overflowX: 'auto', WebkitOverflowScrolling: 'touch' }}>{children}</div>
}
