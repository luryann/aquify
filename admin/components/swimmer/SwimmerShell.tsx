'use client'

import { useEffect, useState, type ReactNode } from 'react'
import { SWIMMER_ACCESS_DISABLED_EVENT } from '@/lib/api'
import { useSession } from '@/lib/session'
import { useRouter } from 'next/navigation'
import { SwimmerNav } from './SwimmerNav'
import './SwimmerShell.inline.css'

export function SwimmerShell({ children }: { children: ReactNode }) {
  const { session } = useSession()
  const router = useRouter()
  const [accessDisabled, setAccessDisabled] = useState(false)

  useEffect(() => {
    const onDisabled = () => setAccessDisabled(true)
    window.addEventListener(SWIMMER_ACCESS_DISABLED_EVENT, onDisabled)
    return () => window.removeEventListener(SWIMMER_ACCESS_DISABLED_EVENT, onDisabled)
  }, [])

  if (!session) return null

  return (
    <div
      data-portal="swim"
      style={{
        minHeight: '100vh',
        background: 'var(--bg)',
        // Re-resolve text color against this portal's tokens; body's color was
        // computed from the admin (dark) root tokens and would otherwise inherit.
        color: 'var(--text)',
        fontFamily: 'var(--font-body)',
        position: 'relative',
      }}
    >
      {/* Subtle teal gradient wash */}
      <div
        aria-hidden="true"
        style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          height: '400px',
          background:
            'radial-gradient(ellipse 80% 60% at 50% -10%, var(--accent-subtle) 0%, transparent 70%), radial-gradient(ellipse 50% 40% at 100% 0, color-mix(in oklab, var(--hot) 10%, transparent) 0%, transparent 70%)',
          pointerEvents: 'none',
          zIndex: 0,
        }}
      />

      {/* Subtle dot grid background */}
      <div
        aria-hidden="true"
        style={{
          position: 'fixed',
          inset: 0,
          backgroundImage: 'radial-gradient(rgba(0,212,170,0.06) 1px, transparent 1px)',
          backgroundSize: '28px 28px',
          pointerEvents: 'none',
          zIndex: 0,
        }}
      />

      <SwimmerNav
        swimmerName={session.user.name}
        initials={session.user.avatarInitials ?? session.user.name?.slice(0, 2)}
        onSignOut={() => router.push('/logout')}
      />

      <main className="portal-frame portal-main">
        {accessDisabled ? (
          <div className="page" role="alert">
            <div className="page-heading">
              <h1 className="page-title">Swimmer sign-in is off</h1>
            </div>
            <p className="prose u-fs-fs-lg" style={{ color: 'var(--text-subtle)' }}>
              Your club has turned off swimmer sign-in. Ask a parent or coach.
            </p>
            <button type="button" className="btn btn-primary u-mt-16px" onClick={() => router.push('/logout')}>
              Sign out
            </button>
          </div>
        ) : children}
      </main>
    </div>
  )
}
