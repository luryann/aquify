'use client'

import { useState } from 'react'
import { useSession } from '@/lib/session'
import { DEV_SESSIONS } from '@/lib/session'
import { ROLE_LABELS } from '@/lib/roles'

const STUB_ORDER = [
  'stub-owner',
  'stub-admin',
  'stub-coach',
  'stub-billing',
  'stub-readonly',
  'stub-parent',
  'stub-swimmer',
] as const

export function DevRoleSwitcher() {
  const { session, setSession } = useSession()
  const [open, setOpen] = useState(false)

  // Only render in development
  if (process.env.NODE_ENV !== 'development') return null

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
      style={{
        position: 'fixed',
        bottom: '16px',
        left: '16px',
        zIndex: 10000,
        fontFamily: 'var(--font-mono)',
        fontSize: '11px',
      }}
    >
      {open && (
        <div
          style={{
            background: 'var(--bg-elevated)',
            border: '1px solid var(--border-strong)',
            borderRadius: 'var(--r-lg)',
            padding: '8px',
            marginBottom: '6px',
            boxShadow: 'var(--shadow-lg)',
            minWidth: '160px',
          }}
        >
          <div style={{ color: 'var(--text-muted)', padding: '4px 8px 8px', letterSpacing: '0.06em', textTransform: 'uppercase', borderBottom: '1px solid var(--border)', marginBottom: '4px' }}>
            Dev Role
          </div>
          {STUB_ORDER.map((stub) => {
            const s = DEV_SESSIONS[stub]
            if (!s) return null
            const isActive = currentStub === stub
            return (
              <button
                key={stub}
                onClick={() => switchTo(stub)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  width: '100%',
                  padding: '6px 8px',
                  background: isActive ? 'var(--accent-subtle)' : 'transparent',
                  border: 'none',
                  borderRadius: 'var(--r-md)',
                  cursor: 'pointer',
                  color: isActive ? 'var(--accent)' : 'var(--text-subtle)',
                  textAlign: 'left',
                  fontFamily: 'var(--font-mono)',
                  fontSize: '12px',
                  fontWeight: isActive ? 600 : 400,
                }}
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

      <button
        onClick={() => setOpen((v) => !v)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          padding: '5px 10px',
          background: 'var(--bg-elevated)',
          border: '1px solid var(--border-strong)',
          borderRadius: 'var(--r-md)',
          cursor: 'pointer',
          color: 'var(--accent)',
          fontFamily: 'var(--font-mono)',
          fontSize: '11px',
          fontWeight: 600,
          boxShadow: 'var(--shadow-md)',
          letterSpacing: '0.02em',
        }}
      >
        <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: 'var(--accent)' }} />
        DEV · {ROLE_LABELS[currentRole as keyof typeof ROLE_LABELS] ?? currentRole}
      </button>
    </div>
  )
}
