'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useSession } from '@/lib/session'
import './page.inline.css'

type LogoutResponse = {
  status?: string
  logout_url?: string
  redirect_url?: string
  message?: string
  error?: string | { message?: string }
}

export default function LogoutPage() {
  const router = useRouter()
  const { signOut } = useSession()
  const [mounted, setMounted] = useState(false)
  const [stage, setStage] = useState<'revoking' | 'done' | 'error'>('revoking')
  const [error, setError] = useState('')
  const [redirectTarget, setRedirectTarget] = useState('/login')
  const [topLevelRedirect, setTopLevelRedirect] = useState(false)

  useEffect(() => {
    let cancelled = false
    setMounted(true)

    async function revoke() {
      try {
        const res = await fetch('/api/v1/auth/sessions/logout', {
          method: 'POST',
          credentials: 'include',
        })

        if (!res.ok) {
          let message = 'Couldn’t revoke your session. Try again.'
          try {
            const payload = await res.json() as {
              message?: string
              error?: string | { message?: string }
            }
            const errField = payload.error
            message = payload.message
              ?? (typeof errField === 'string' ? errField : errField?.message)
              ?? message
          } catch {
            // Use default message.
          }
          throw new Error(message)
        }

        const payload = await res.json() as LogoutResponse
        const nextRedirect = payload.redirect_url?.trim() || '/login'
        const logoutURL = payload.logout_url?.trim() || ''

        signOut()
        if (!cancelled) {
          setRedirectTarget(logoutURL || nextRedirect)
          setTopLevelRedirect(Boolean(logoutURL))
          setStage('done')
        }
      } catch {
        signOut()
        if (!cancelled) {
          setStage('error')
          setError('Couldn’t sign out cleanly. Try again.')
        }
      }
    }

    revoke()
    return () => {
      cancelled = true
    }
  }, [signOut])

  useEffect(() => {
    if (stage === 'done') {
      const t = setTimeout(() => {
        if (topLevelRedirect) {
          window.location.assign(redirectTarget)
          return
        }
        router.push(redirectTarget)
      }, 1200)
      return () => clearTimeout(t)
    }
  }, [redirectTarget, router, stage, topLevelRedirect])

  return (
    <div
      className="is-44ea83d"
    >
      <style>{`
        @keyframes logoutSpin { to { transform: rotate(360deg); } }
        @keyframes logoutFade { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
        @media (prefers-reduced-motion: reduce) {
          @keyframes logoutFade { from { opacity: 0; } to { opacity: 1; } }
        }
      `}</style>

      <div
        style={{
          textAlign: 'center',
          opacity: mounted ? 1 : 0,
          transform: mounted ? 'translateY(0)' : 'translateY(16px)',
          transition: 'opacity 0.35s ease, transform 0.35s ease',
        }}
      >
        {stage === 'revoking' ? (
          <>
            {/* Spinner */}
            <div className="is-4e4a1e4">
              <div style={{
                position: 'absolute', inset: 0, borderRadius: '50%',
                border: '2px solid var(--border)',
              }} />
              <div style={{
                position: 'absolute', inset: 0, borderRadius: '50%',
                border: '2px solid transparent', borderTopColor: 'var(--accent)',
                animation: 'logoutSpin 0.9s linear infinite',
              }} />
            </div>
            <h2 className="is-46136c0">
              Signing you out…
            </h2>
            <p className="is-6c43e0d">
              Revoking session and clearing credentials
            </p>
          </>
        ) : stage === 'done' ? (
          <div style={{ animation: 'logoutFade 0.4s ease' }}>
            {/* Checkmark */}
            <div className="is-1291541">
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
                stroke="var(--success)" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
                <polyline points="20 6 9 17 4 12" />
              </svg>
            </div>
            <h2 className="is-46136c0">
              Signed out
            </h2>
            <p className="is-6c43e0d">
              Redirecting to sign-in…
            </p>

            <button type="button"
              onClick={() => {
                if (topLevelRedirect) {
                  window.location.assign(redirectTarget)
                  return
                }
                router.push(redirectTarget)
              }}
              className="is-a7494a5"
              onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' }}
              onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
            >
              Sign in again
            </button>
          </div>
        ) : (
          <div style={{ animation: 'logoutFade 0.4s ease' }}>
            <div className="is-c45f24a">
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
                stroke="var(--error)" strokeWidth={2.25} strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="10" />
                <line x1="12" y1="8" x2="12" y2="12" />
                <line x1="12" y1="16" x2="12.01" y2="16" />
              </svg>
            </div>
            <h2 className="is-46136c0">
              Sign-out needs attention
            </h2>
            <p className="is-a32f9ad">
              {error}
            </p>
            <button type="button"
              onClick={() => window.location.reload()}
              className="is-d6e9bd3"
            >
              Retry sign-out
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
