'use client'

import { useState } from 'react'
import { useSession } from '@/lib/session'
import { DEV_SESSIONS } from '@/lib/session'
import { ROLE_LABELS } from '@/lib/roles'
import { DEV_STUB_TOKENS_ENABLED } from '@/lib/featureFlags'

const STUB_ORDER = [
  'stub-admin',
  'stub-coach',
  'stub-parent',
  'stub-swimmer',
] as const

export function DevRoleSwitcher() {
  const { session, setSession } = useSession()
  const [open, setOpen] = useState(false)

  // Only render in development
  if (!DEV_STUB_TOKENS_ENABLED) return null

  const currentStub = session?.token ?? ''
  const currentRole = session?.user.role ?? '—'

  function switchTo(stub: string) {
    const s = DEV_SESSIONS[stub]
    if (s) {
      setSession({ ...s, expiresAt: Date.now() + 86_400_000 * 30 })
      setOpen(false)
    }
  }

  return (
    <div
      className="u-ff-font-body u-fs-fs-xs" style={{ position: 'fixed', bottom: '16px', left: '16px', zIndex: 10000 }}
    >
      {open && (
        <div
          className="u-p-8px u-mb-6px" style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)', borderRadius: 'var(--r-lg)', boxShadow: 'var(--shadow-lg)', minWidth: '160px' }}
        >
          <div className="u-mb-4px" style={{ color: 'var(--text-muted)', padding: '4px 8px 8px', borderBottom: '1px solid var(--border)' }}>
            Dev Role
          </div>
          {STUB_ORDER.map((stub) => {
            const s = DEV_SESSIONS[stub]
            if (!s) return null
            const isActive = currentStub === stub
            return (
              <button type="button"
                key={stub}
                onClick={() => switchTo(stub)}
                className="u-gap-8px u-p-6px-8px u-ff-font-body u-fs-fs-sm" style={{ display: 'flex', alignItems: 'center', width: '100%', background: isActive ? 'var(--accent-subtle)' : 'transparent', border: 'none', borderRadius: 'var(--r-md)', cursor: 'pointer', color: isActive ? 'var(--accent)' : 'var(--text-subtle)', textAlign: 'left', fontWeight: isActive ? 600 : 400 }}
                onMouseEnter={(e) => { if (!isActive) e.currentTarget.style.background = 'rgba(255,255,255,0.04)' }}
                onMouseLeave={(e) => { if (!isActive) e.currentTarget.style.background = 'transparent' }}
              >
                {isActive && (
                  <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--accent)', flexShrink: 0 }} />
                )}
                {!isActive && (
                  <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--border-strong)', flexShrink: 0 }} />
                )}
                {ROLE_LABELS[s.user.role]}
              </button>
            )
          })}
        </div>
      )}

      <button type="button"
        onClick={() => setOpen((v) => !v)}
        className="u-gap-6px u-p-4px-12px u-ff-font-body u-fs-fs-xs u-fw-600 u-ls-0_02em" style={{ display: 'flex', alignItems: 'center', background: 'var(--bg-elevated)', border: '1px solid var(--border-strong)', borderRadius: 'var(--r-md)', cursor: 'pointer', color: 'var(--accent-text, var(--accent))', boxShadow: 'var(--shadow-md)' }}
      >
        <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--accent)' }} />
        DEV · {ROLE_LABELS[currentRole as keyof typeof ROLE_LABELS] ?? currentRole}
      </button>
    </div>
  )
}
