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
      style={{
        minHeight: '100vh',
        background: '#061623',
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
        <div style={{ marginBottom: '28px' }}>
          <svg width="48" height="32" viewBox="0 0 48 32" fill="none" style={{ margin: '0 auto' }}>
            <path
              d="M2 16C6 10 10 4 17 4C24 4 24 16 31 16C38 16 38 4 44 4"
              stroke="#00D4AA"
              strokeWidth="2.5"
              strokeLinecap="round"
              fill="none"
              opacity="0.6"
            />
            <path
              d="M2 26C6 20 10 14 17 14C24 14 24 26 31 26C38 26 38 14 44 14"
              stroke="#00D4AA"
              strokeWidth="2"
              strokeLinecap="round"
              fill="none"
              opacity="0.25"
            />
          </svg>
        </div>

        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: '26px',
            color: '#E4F2FF',
            letterSpacing: '-0.01em',
            marginBottom: '12px',
          }}
        >
          Athlete Portal Coming Soon
        </h1>
        <p
          style={{
            fontSize: '14px',
            color: '#7FA8C9',
            lineHeight: 1.65,
            marginBottom: '28px',
          }}
        >
          The optional swimmer portal is not yet enabled for your club. All important information —
          meets, registration, billing, and documents — is available through your parent&apos;s account.
        </p>
        <a
          href="/portal/dashboard"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: '8px',
            padding: '10px 22px',
            background: '#00D4AA',
            color: '#061623',
            borderRadius: '9px',
            fontSize: '14px',
            fontWeight: 700,
            textDecoration: 'none',
            transition: 'background 120ms ease',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.background = '#00b896')}
          onMouseLeave={(e) => (e.currentTarget.style.background = '#00D4AA')}
        >
          Go to Parent Portal
        </a>
      </div>
    </div>
  )
}

export function SwimmerShell({ children }: { children: ReactNode }) {
  const { session, signOut } = useSession()
  const router = useRouter()

  if (!session) return null

  if (!SWIMMER_PORTAL_ENABLED) {
    return <PortalDisabled />
  }

  const handleSignOut = () => {
    signOut()
    router.push('/login')
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        background: '#061623',
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
            'radial-gradient(ellipse 80% 60% at 50% -10%, rgba(0,212,170,0.12) 0%, transparent 70%)',
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

      <main
        style={{
          position: 'relative',
          zIndex: 1,
          maxWidth: '1100px',
          margin: '0 auto',
          padding: '40px 28px 80px',
        }}
      >
        {children}
      </main>
    </div>
  )
}
