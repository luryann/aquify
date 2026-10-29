'use client'

import { AuthBackdrop } from '@/components/auth/AuthBackdrop'
import { Suspense, useEffect, useState, useCallback } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { useSession, DEV_SESSIONS } from '@/lib/session'
import { destinationForRole } from '@/lib/auth-routing'
import { DEV_STUB_TOKENS_ENABLED } from '@/lib/featureFlags'
import { resolvePublicOrg } from '@/lib/public-org'
import './page.inline.css'

const STUB_ROLES = [
  { id: 'stub-admin', label: 'Admin', color: '#60a5fa', rgb: '96,165,250', desc: 'Full administrative access' },
  { id: 'stub-coach', label: 'Coach', color: '#34d399', rgb: '52,211,153', desc: 'Training & meets' },
  { id: 'stub-parent', label: 'Parent', color: '#a78bfa', rgb: '167,139,250', desc: 'Household portal' },
  { id: 'stub-swimmer', label: 'Swimmer', color: '#22d3ee', rgb: '34,211,238', desc: 'Swimmer portal' },
] as const

export default function LoginPage() {
  return (
    <Suspense fallback={null}>
      <LoginPageContent />
    </Suspense>
  )
}

function LoginPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { session, loading, setSession } = useSession()
  const [email, setEmail] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [devOpen, setDevOpen] = useState(false)
  const [mounted, setMounted] = useState(false)
  const [orgError, setOrgError] = useState('')

  useEffect(() => { setMounted(true) }, [])

  useEffect(() => {
    if (!loading && session) {
      router.replace(destinationForRole(session.user.roles))
    }
  }, [loading, session, router])

  const handleSSO = useCallback(() => {
    const resolved = resolvePublicOrg({
      searchParams,
      host: typeof window === 'undefined' ? '' : window.location.hostname,
    })
    if (!resolved.orgId) {
      setOrgError('We could not determine your club. Retry with your club link or ask your administrator for the correct sign-in URL.')
      return
    }
    setSubmitting(true)
    setOrgError('')
    const q = new URLSearchParams()
    if (email.trim()) q.set('login_hint', email.trim())
    q.set('org_id', resolved.orgId)
    window.location.href = `/api/v1/auth/oidc/authorize?${q}`
  }, [email, searchParams])

  const handleStub = useCallback((id: string) => {
    const s = DEV_SESSIONS[id]
    if (!s) return
    setSession(s)
    router.push(destinationForRole(s.user.roles))
  }, [setSession, router])

  if (!mounted || loading) {
    return (
      <div style={{
        position: 'fixed', inset: 0,
        background: '#06101A',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}>
        <div style={{
          width: 8, height: 8, borderRadius: '50%',
          background: 'var(--accent)',
          
          animation: 'lpDot 1.4s ease-in-out infinite',
        }} />
      </div>
    )
  }

  if (session) return null

  return (
    <>
      <div className="sl-root">
        <AuthBackdrop />

        <div className="sl-card">
          {/* Logo mark */}
          <div className="sl-logo">
            <svg width="32" height="32" viewBox="0 0 34 34" fill="none" aria-hidden="true">
              <rect width="34" height="34" rx="8" fill="rgba(235,93,30,0.14)" />
              <path d="M9 25V9h7c4.4 0 8 3.58 8 8s-3.6 8-8 8H9z" fill="var(--accent)" />
              <path d="M13 21v-8h3c2.2 0 4 1.79 4 4s-1.8 4-4 4h-3z" fill="#06101A" />
            </svg>
            <span className="sl-logo-text">aquify</span>
          </div>

          {/* Heading */}
          <div className="sl-heading">
            <h1 className="sl-title">Sign in</h1>
          </div>

          {/* Form */}
          <div className="sl-fields">
            {orgError && (
              <div className="is-6c79662">
                {orgError}
              </div>
            )}
            <div className="sl-field">
              <label htmlFor="sl-email" className="sl-label">Email address</label>
              <input
                id="sl-email"
                type="email"
                className="sl-input"
                placeholder="you@yourclub.com"
                value={email}
                onChange={e => { setEmail(e.target.value); setOrgError('') }}
                onKeyDown={e => { if (e.key === 'Enter') handleSSO() }}
                autoComplete="email"
                autoFocus
                disabled={submitting}
              />
            </div>

            <button type="button"
              className={`sl-btn${submitting ? ' sl-btn--loading' : ''}`}
              onClick={handleSSO}
              disabled={submitting}
            >
              {submitting ? (
                <>
                  <span className="sl-spinner" />
                  Connecting…
                </>
              ) : (
                <>
                  <svg viewBox="0 0 20 20" fill="none" width="14" height="14" aria-hidden="true">
                    <circle cx="10" cy="10" r="8.5" stroke="currentColor" strokeWidth="1.4" />
                    <path d="M10 1.5c-2.5 0-4 4-4 8.5s1.5 8.5 4 8.5m0-17c2.5 0 4 4 4 8.5s-1.5 8.5-4 8.5M1.5 10h17" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
                  </svg>
                  Continue
                </>
              )}
            </button>

            <p className="sl-hint">{"You'll be sent to your club's sign-in page."}</p>
          </div>

          <div className="sl-sep"><span>or</span></div>

          <div className="sl-access-row">
            <span className="sl-access-text">{"No account?"}</span>
            <a href="/register" className="sl-access-link">Request access →</a>
          </div>

          {/* Dev stub login */}
          {DEV_STUB_TOKENS_ENABLED && (
            <div className="sl-dev">
              <button type="button" className="sl-dev-toggle" onClick={() => setDevOpen(v => !v)}>
                <span className="sl-dev-badge">DEV</span>
                <span className="sl-dev-label">Development stub login</span>
                <svg viewBox="0 0 16 16" fill="none" width="12" height="12" aria-hidden="true"
                  style={{ transform: devOpen ? 'rotate(180deg)' : 'none', transition: 'transform 0.2s', flexShrink: 0, color: 'rgba(237,233,227,0.3)' }}>
                  <path d="M3.5 6l4.5 4 4.5-4" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
                </svg>
              </button>
              {devOpen && (
                <div className="sl-dev-body">
                  <p className="sl-dev-note">Bypasses OIDC. Requires <code>AUTH_ENABLE_DEV_STUB_TOKENS=true</code>.</p>
                  <div className="sl-dev-grid">
                    {STUB_ROLES.map(r => (
                      <button type="button" key={r.id} className="sl-dev-role" onClick={() => handleStub(r.id)} style={{ '--rc': r.rgb } as React.CSSProperties}>
                        <span className="sl-dev-dot" style={{ background: r.color }} />
                        <div className="sl-dev-info">
                          <span className="sl-dev-name">{r.label}</span>
                          <span className="sl-dev-desc">{r.desc}</span>
                        </div>
                        <svg viewBox="0 0 12 12" fill="none" width="9" height="9" aria-hidden="true" style={{ color: 'rgba(237,233,227,0.2)', flexShrink: 0 }}>
                          <path d="M4 2l4 4-4 4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
                        </svg>
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

        </div>
      </div>

    </>
  )
}
