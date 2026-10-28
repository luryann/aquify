'use client'

import type { ReactNode } from 'react'
import { useSession } from '@/lib/session'
import { useRouter } from 'next/navigation'
import { SwimmerNav } from './SwimmerNav'
import { SWIMMER_PORTAL_ENABLED } from '@/lib/featureFlags'

// Shown when the swimmer portal feature flag is off
function PortalDisabled() {
  return (
    <div
      data-portal="swim"
      style={{
        minHeight: '100vh',
        background: 'var(--bg)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '24px',
        fontFamily: 'var(--font-body)',
      }}
    >
      <div
        style={{
          maxWidth: '420px',
          textAlign: 'center',
          animation: 'slideUp 0.4s cubic-bezier(0.16,1,0.3,1) both',
        }}
      >
        {/* Wave icon */}
        <div style={{ marginBottom: '24px' }}>
          <svg width="48" height="32" viewBox="0 0 48 32" fill="none" style={{ margin: '0 auto' }}>
            <path
              d="M2 16C6 10 10 4 17 4C24 4 24 16 31 16C38 16 38 4 44 4"
              stroke="var(--accent)"
              strokeWidth="2.5"
              strokeLinecap="round"
              fill="none"
              opacity="0.6"
            />
            <path
              d="M2 26C6 20 10 14 17 14C24 14 24 26 31 26C38 26 38 14 44 14"
              stroke="var(--accent)"
              strokeWidth="2"
              strokeLinecap="round"
              fill="none"
              opacity="0.25"
            />
          </svg>
        </div>

        <h1 className="page-title">
          Athlete Portal Coming Soon
        </h1>
        <p
          style={{
            fontSize: 'var(--fs-md)',
            color: 'var(--text-subtle)',
            lineHeight: 1.65,
            marginBottom: '24px',
          }}
        >
          The swimmer portal isn&apos;t enabled for your club yet. Meets, billing, and documents are available through your parent&apos;s account.
        </p>
        <a href="/portal/dashboard" className="btn btn-primary" style={{ textDecoration: 'none' }}>
          Go to Parent Portal
        </a>
      </div>
    </div>
  )
}

export function SwimmerShell({ children }: { children: ReactNode }) {
  const { session } = useSession()
  const router = useRouter()

  if (!session) return null

  if (!SWIMMER_PORTAL_ENABLED) {
    return <PortalDisabled />
  }

  const handleSignOut = () => {
    router.push('/logout')
  }

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
            'radial-gradient(ellipse 80% 60% at 50% -10%, var(--accent-subtle) 0%, transparent 70%)',
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
        onSignOut={handleSignOut}
      />

      <main className="portal-frame portal-main">
        {children}
      </main>
    </div>
  )
}
