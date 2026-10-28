'use client'

import { useEffect, type ReactNode } from 'react'
import { useRouter } from 'next/navigation'
import { useSession } from '@/lib/session'
import type { Role } from '@/lib/roles'
import { canAccess } from '@/lib/roles'
import { roleRequiresMFA, destinationForRole } from '@/lib/auth-routing'

type AuthGuardProps = {
  children: ReactNode
  redirectTo?: string
  requiredRoles?: Role[]
  fallback?: ReactNode
  requireMFA?: boolean
}

// Full-screen loading splash shown during session hydration
function LoadingSplash() {
  return (
    <div
      className="u-gap-16px" style={{ position: 'fixed', inset: 0, background: 'var(--bg)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', zIndex: 9999, animation: 'fadeIn 0.2s ease' }}
    >
      {/* Brand mark */}
      <div className="u-gap-12px u-mb-8px" style={{ display: 'flex', alignItems: 'center' }}>
        <div
          style={{
            width: '9px',
            height: '9px',
            borderRadius: '50%',
            background: 'var(--accent)',
            boxShadow: '0 0 12px var(--accent)',
            animation: 'pulseDot 1.5s ease-in-out infinite',
          }}
        />
        <span
          className="u-ff-font-display u-fw-display-weight u-fs-fs-2xl u-ls-display-tracking" style={{ fontStyle: 'italic', color: 'var(--text)', userSelect: 'none' }}
        >
          DARE
        </span>
      </div>

      <div className="spinner" />

      <p
        className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)', letterSpacing: '0.04em' }}
      >
        authenticating…
      </p>
    </div>
  )
}

// Unauthorized / forbidden screen
function UnauthorizedScreen({ reason }: { reason: 'unauthenticated' | 'forbidden' }) {
  const router = useRouter()
  const { session } = useSession()
  const dashboardDestination = session ? destinationForRole(session.user.roles) : '/dashboard'

  return (
    <div
      className="u-gap-12px u-p-24px" style={{ position: 'fixed', inset: 0, background: 'var(--bg)', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', textAlign: 'center' }}
    >
      <div
        className="u-mb-8px" style={{ width: '56px', height: '56px', borderRadius: '50%', background: 'var(--error-bg)', border: '1px solid var(--error-border)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--error)' }}
      >
        <svg
          viewBox="0 0 24 24"
          width="26"
          height="26"
          fill="none"
          stroke="currentColor"
          strokeWidth={1.75}
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
      </div>

      <p
        className="u-ff-font-display u-fw-display-weight u-fs-fs-xl" style={{ color: 'var(--text)' }}
      >
        {reason === 'forbidden' ? 'Access Denied' : 'Sign In Required'}
      </p>

      <p className="u-fs-fs-base u-lh-1_6" style={{ color: 'var(--text-subtle)', maxWidth: '320px' }}>
        {reason === 'forbidden'
          ? "You don't have permission to access this area. Contact your administrator."
          : 'Sign in to continue.'}
      </p>

      <button
        className="btn btn-secondary u-mt-8px"
        onClick={() => router.push(reason === 'forbidden' ? dashboardDestination : '/login')}
        
      >
        {reason === 'forbidden' ? 'Go to Dashboard' : 'Sign In'}
      </button>
    </div>
  )
}

export function AuthGuard({
  children,
  redirectTo = '/login',
  requiredRoles,
  fallback,
  requireMFA,
}: AuthGuardProps) {
  const { session, loading } = useSession()
  const router = useRouter()

  // Derive whether MFA is needed: explicit prop or role-based inference
  const mfaRequired = requireMFA ?? (session ? roleRequiresMFA(session.user.roles) : false)
  const mfaIncomplete = mfaRequired && session && !session.user.mfaAuthenticated

  // Redirect unauthenticated users
  useEffect(() => {
    if (!loading && !session) {
      router.replace(redirectTo)
    }
  }, [loading, session, router, redirectTo])

  // Redirect to MFA step-up if session exists but MFA is not yet completed
  useEffect(() => {
    if (!loading && session && mfaIncomplete) {
      router.replace('/mfa')
    }
  }, [loading, session, mfaIncomplete, router])

  if (loading) {
    return fallback ?? <LoadingSplash />
  }

  if (!session || mfaIncomplete) {
    // Keep showing splash while useEffect fires the redirect
    return fallback ?? <LoadingSplash />
  }

  // Role-based access check
  if (requiredRoles && requiredRoles.length > 0) {
    if (!canAccess(session.user.roles, requiredRoles)) {
      return <UnauthorizedScreen reason="forbidden" />
    }
  }

  return <>{children}</>
}
