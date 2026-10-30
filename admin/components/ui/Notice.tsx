import type { ReactNode } from 'react'

const TONES = {
  ok: { background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)' },
  err: { background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' },
} as const

/** Inline success/error message box. Errors are announced as alerts, successes as status. */
export function Notice({ tone, className = '', children }: { tone: keyof typeof TONES; className?: string; children: ReactNode }) {
  return (
    <div role={tone === 'err' ? 'alert' : 'status'} className={`u-p-8px-12px u-fs-fs-sm ${className}`} style={{ borderRadius: '8px', ...TONES[tone] }}>
      {children}
    </div>
  )
}
