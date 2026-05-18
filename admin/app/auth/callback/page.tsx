'use client'

import { useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Suspense } from 'react'
import { useSession } from '@/lib/session'
import { destinationForRole, roleRequiresMFA } from '@/lib/auth-routing'

// ─── Inner component (uses useSearchParams) ───────────────────────────────────

function CallbackInner() {
  const router = useRouter()
  const params = useSearchParams()
  const { refreshSession } = useSession()
  const [stage, setStage] = useState<'processing' | 'mfa' | 'error'>('processing')
  const [errorMsg, setErrorMsg] = useState('')
  const [dots, setDots] = useState(1)

  useEffect(() => {
    // Animate dots
    const iv = setInterval(() => setDots((d) => (d % 3) + 1), 500)
    return () => clearInterval(iv)
  }, [])

  useEffect(() => {
    let cancelled = false

    async function completeCallback() {
      const error = params.get('error')
      if (error) {
        setStage('error')
        setErrorMsg(params.get('error_description') ?? error)
        return
      }

      const code = params.get('code')?.trim() ?? ''
      const state = params.get('state')?.trim() ?? ''
      if (!code || !state) {
        setStage('error')
        setErrorMsg('Missing authorization code or state from the identity provider.')
        return
      }

      try {
        const res = await fetch('/api/v1/auth/oidc/callback', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          credentials: 'include',
          body: JSON.stringify({ code, state }),
        })

        if (!res.ok) {
          let message = 'Unable to complete sign-in.'
          try {
            const payload = await res.json() as { message?: string; error?: { message?: string; code?: string } | string }
            const errField = payload.error
            message = payload.message
              ?? (typeof errField === 'object' ? errField?.message : errField)
              ?? message
          } catch {
            // Use default message.
          }
          throw new Error(message)
        }

        const next = await refreshSession()
        if (cancelled) return

        if (!next) {
          throw new Error('Sign-in completed but no active session could be loaded.')
        }

        if (roleRequiresMFA(next.user.role) && !next.user.mfaAuthenticated) {
          setStage('mfa')
          setTimeout(() => router.replace('/mfa'), 400)
          return
        }

        router.replace(destinationForRole(next.user.role))
      } catch (error) {
        if (cancelled) return
        setStage('error')
        setErrorMsg(error instanceof Error ? error.message : 'An error occurred during sign-in. Please try again.')
      }
    }

    completeCallback()

    return () => {
      cancelled = true
    }
  }, [params, refreshSession, router])

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        background: 'var(--bg)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontFamily: 'var(--font-body)',
      }}
    >
      {/* Background glow */}
      <div
        aria-hidden
        style={{
          position: 'absolute',
          top: '20%',
          left: '50%',
          transform: 'translateX(-50%)',
          width: '600px',
          height: '600px',
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(235,93,30,0.06) 0%, transparent 65%)',
          pointerEvents: 'none',
        }}
      />

      {stage === 'error' ? (
        /* ── Error state ── */
        <div
          style={{
            maxWidth: '420px',
            width: '100%',
            padding: '0 24px',
            textAlign: 'center',
          }}
        >
          <div
            style={{
              width: '56px',
              height: '56px',
              borderRadius: '50%',
              background: 'var(--error-bg)',
              border: '1px solid var(--error-border)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              margin: '0 auto 24px',
            }}
          >
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
              stroke="var(--error)" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="10" />
              <line x1="15" y1="9" x2="9" y2="15" />
              <line x1="9" y1="9" x2="15" y2="15" />
            </svg>
          </div>
          <h2
            style={{
              fontFamily: 'var(--font-display)',
              fontSize: '24px',
              fontWeight: 400,
              color: 'var(--text)',
              marginBottom: '10px',
              letterSpacing: '-0.01em',
            }}
          >
            Authentication failed
          </h2>
          <p
            style={{
              fontSize: '13px',
              color: 'var(--text-subtle)',
              lineHeight: 1.6,
              marginBottom: '28px',
            }}
          >
            {errorMsg || 'An error occurred during sign-in. Please try again.'}
          </p>
          <button
            onClick={() => router.push('/login')}
            style={{
              padding: '10px 24px',
              background: 'var(--accent)',
              border: 'none',
              borderRadius: '8px',
              color: '#fff',
              fontSize: '13px',
              fontWeight: 600,
              cursor: 'pointer',
              fontFamily: 'var(--font-body)',
              letterSpacing: '-0.01em',
            }}
          >
            Return to Sign In
          </button>
        </div>
      ) : (
        /* ── Processing / redirecting state ── */
        <div style={{ textAlign: 'center', maxWidth: '360px', padding: '0 24px' }}>
          {/* Spinner ring */}
          <div
            style={{
              position: 'relative',
              width: '64px',
              height: '64px',
              margin: '0 auto 32px',
            }}
          >
            <div
              style={{
                position: 'absolute',
                inset: 0,
                borderRadius: '50%',
                border: '2px solid var(--border)',
              }}
            />
            <div
              style={{
                position: 'absolute',
                inset: 0,
                borderRadius: '50%',
                border: '2px solid transparent',
                borderTopColor: 'var(--accent)',
                animation: 'spin 0.9s linear infinite',
              }}
            />
            <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
            {/* Center dot */}
            <div
              style={{
                position: 'absolute',
                top: '50%',
                left: '50%',
                transform: 'translate(-50%, -50%)',
                width: '8px',
                height: '8px',
                borderRadius: '50%',
                background: 'var(--accent)',
                boxShadow: '0 0 12px var(--accent)',
              }}
            />
          </div>

          <h2
            style={{
              fontFamily: 'var(--font-display)',
              fontSize: '22px',
              fontWeight: 400,
              color: 'var(--text)',
              marginBottom: '8px',
              letterSpacing: '-0.01em',
            }}
          >
            {stage === 'mfa' ? 'Redirecting to verification' : 'Completing sign-in'}
            {'...'.slice(0, dots)}
          </h2>
          <p
            style={{
              fontSize: '12px',
              fontFamily: 'var(--font-mono)',
              color: 'var(--text-muted)',
              letterSpacing: '0.04em',
            }}
          >
            {stage === 'mfa'
              ? 'MFA required for your role'
              : 'Exchanging token with Keycloak'}
          </p>

          {/* Progress steps */}
          <div
            style={{
              marginTop: '40px',
              display: 'flex',
              flexDirection: 'column',
              gap: '8px',
              textAlign: 'left',
            }}
          >
            {[
              { label: 'Verifying state token', done: true },
              { label: 'Exchanging authorization code', done: true },
              { label: 'Creating session', done: stage !== 'processing' },
              { label: 'Loading workspace', done: false },
            ].map((step) => (
              <div
                key={step.label}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '10px',
                  opacity: step.done ? 1 : 0.35,
                  transition: 'opacity 0.3s ease',
                }}
              >
                <div
                  style={{
                    width: '16px',
                    height: '16px',
                    borderRadius: '50%',
                    background: step.done ? 'var(--success-bg)' : 'var(--bg-elevated)',
                    border: `1px solid ${step.done ? 'var(--success-border)' : 'var(--border)'}`,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    flexShrink: 0,
                  }}
                >
                  {step.done && (
                    <svg width="8" height="8" viewBox="0 0 12 12" fill="none">
                      <path d="M2 6l3 3 5-5" stroke="var(--success)" strokeWidth={1.8}
                        strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                  )}
                </div>
                <span
                  style={{
                    fontSize: '12px',
                    fontFamily: 'var(--font-mono)',
                    color: step.done ? 'var(--text-subtle)' : 'var(--text-muted)',
                    letterSpacing: '0.02em',
                  }}
                >
                  {step.label}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Page export wrapped in Suspense (required for useSearchParams) ────────────

export default function AuthCallbackPage() {
  return (
    <Suspense
      fallback={
        <div
          style={{
            position: 'fixed',
            inset: 0,
            background: 'var(--bg)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <div style={{ width: '32px', height: '32px', borderRadius: '50%',
            border: '2px solid var(--border)', borderTopColor: 'var(--accent)',
            animation: 'spin 0.9s linear infinite' }} />
        </div>
      }
    >
      <CallbackInner />
    </Suspense>
  )
}
