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
        <style>{`@keyframes lpDot{0%,100%{opacity:.35;transform:scale(.85)}50%{opacity:1;transform:scale(1.2)}}`}</style>
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

      <style>{`
        .sl-root {
          isolation: isolate;
          min-height: 100vh;
          display: flex;
          align-items: center;
          justify-content: center;
          background: transparent;
          padding: 24px;
          position: relative;
        }

        .sl-card {
          position: relative;
          z-index: 1;
          width: 100%;
          max-width: 420px;
          background: #0c1925;
          border: 1px solid rgba(255,255,255,0.07);
          border-radius: 16px;
          padding: 36px 40px 28px;
          box-shadow: 0 8px 24px var(--border-strong);
          animation: slReveal 0.45s cubic-bezier(0.16,1,0.3,1) both;
        }
        @media (max-width: 640px) {
          .sl-card { padding: 28px 24px 22px; }
        }

        @keyframes slReveal {
          from { opacity: 0; transform: translateY(16px) scale(0.98); }
          to   { opacity: 1; transform: translateY(0) scale(1); }
        }

        /* Logo */
        .sl-logo {
          display: flex;
          align-items: center;
          gap: 9px;
          margin-bottom: 32px;
        }
        .sl-logo-text {
          font-family: var(--font-display); font-weight: var(--display-weight);
          font-style: italic;
          font-size: var(--fs-xl);
          color: #EDE9E3;
          letter-spacing: var(--display-tracking);
        }

        /* Heading */
        .sl-heading {
          margin-bottom: 28px;
        }
        .sl-title {
          font-family: var(--font-display);
          font-size: var(--fs-4xl);
          font-weight: var(--display-weight);
          color: #EDE9E3;
          margin: 0 0 6px;
          letter-spacing: var(--display-tracking);
          line-height: 1.1;
        }
        .sl-subtitle {
          font-family: var(--font-body);
          font-size: var(--fs-base);
          color: rgba(237,233,227,0.35);
          margin: 0;
          line-height: 1.5;
        }

        /* Fields */
        .sl-fields {
          display: flex;
          flex-direction: column;
          gap: 12px;
        }
        .sl-field {
          display: flex;
          flex-direction: column;
          gap: 6px;
        }
        .sl-label {
          font-family: var(--font-body);
          font-size: var(--fs-sm);
          font-weight: 600;
          color: rgba(237,233,227,0.55);
        }
        .sl-input {
          width: 100%;
          background: rgba(255,255,255,0.04);
          border: 1px solid rgba(255,255,255,0.09);
          border-radius: 9px;
          color: #EDE9E3;
          font-family: var(--font-body);
          font-size: var(--fs-md);
          padding: 12px 14px;
          outline: 2px solid transparent;
          transition: border-color 0.18s, background 0.18s, box-shadow 0.18s;
          box-sizing: border-box;
        }
        .sl-input::placeholder { color: rgba(237,233,227,0.18); }
        .sl-input:focus {
          border-color: rgba(235,93,30,0.5);
          background: rgba(235,93,30,0.04);
          box-shadow: 0 0 0 3px rgba(235,93,30,0.08);
          outline-color: var(--accent);
        }
        .sl-input:disabled { opacity: 0.45; cursor: not-allowed; }

        /* Button */
        .sl-btn {
          display: flex;
          align-items: center;
          justify-content: center;
          gap: 8px;
          width: 100%;
          padding: 13px 20px;
          background: var(--accent-fill, #c9460f);
          border: none;
          border-radius: 9px;
          color: #fff;
          font-family: var(--font-body);
          font-size: var(--fs-md);
          font-weight: 600;
          letter-spacing: 0.01em;
          cursor: pointer;
          transition: background 0.18s, transform 0.12s, box-shadow 0.18s;
        }
        .sl-btn:hover:not(:disabled) {
          background: var(--accent-fill-hover, #b53f0d);
          transform: translateY(-1px);
          
        }
        .sl-btn:active:not(:disabled) { transform: translateY(0); box-shadow: none; }
        .sl-btn:disabled { opacity: 0.5; cursor: not-allowed; }
        .sl-btn--loading { pointer-events: none; }

        .sl-spinner {
          width: 14px; height: 14px;
          border: 2px solid rgba(255,255,255,0.25);
          border-top-color: #fff;
          border-radius: 50%;
          animation: slSpin 0.65s linear infinite;
          flex-shrink: 0;
        }
        @keyframes slSpin { to { transform: rotate(360deg); } }

        .sl-hint {
          font-family: var(--font-body);
          font-size: var(--fs-xs);
          color: rgba(237,233,227,0.22);
          text-align: center;
          margin: 0;
          line-height: 1.6;
        }

        /* Separator */
        .sl-sep {
          display: flex;
          align-items: center;
          gap: 10px;
          font-family: var(--font-body);
          font-size: var(--fs-sm);
          color: rgba(237,233,227,0.2);
          margin: 20px 0;
        }
        .sl-sep::before, .sl-sep::after {
          content: '';
          flex: 1;
          height: 1px;
          background: rgba(255,255,255,0.07);
        }

        /* Access row */
        .sl-access-row {
          display: flex;
          align-items: center;
          gap: 7px;
          font-family: var(--font-body);
          font-size: var(--fs-base);
        }
        .sl-access-text { color: rgba(237,233,227,0.32); }
        .sl-access-link {
          display: inline-flex;
          align-items: center;
          min-height: 44px;
          color: var(--accent-text, #eb5d1e);
          text-decoration: none;
          font-weight: 500;
          transition: color 0.18s;
        }
        .sl-access-link:hover { color: #f5764a; }

        /* Dev panel */
        .sl-dev {
          margin-top: 24px;
          border: 1px solid rgba(235,93,30,0.15);
          border-radius: 10px;
          overflow: hidden;
        }
        .sl-dev-toggle {
          display: flex;
          align-items: center;
          gap: 8px;
          width: 100%;
          padding: 10px 13px;
          background: rgba(235,93,30,0.05);
          border: none;
          cursor: pointer;
          text-align: left;
          font-family: var(--font-body);
          transition: background 0.15s;
        }
        .sl-dev-toggle:hover { background: rgba(235,93,30,0.09); }
        .sl-dev-badge {
          font-family: var(--font-body);
          font-size: var(--fs-2xs);
          font-weight: 700;
          padding: 2px 6px;
          background: rgba(235,93,30,0.12);
          border: 1px solid rgba(235,93,30,0.28);
          border-radius: 4px;
          color: var(--accent);
          letter-spacing: 0.1em;
        }
        .sl-dev-label {
          flex: 1;
          font-size: var(--fs-sm);
          color: rgba(237,233,227,0.5);
        }
        .sl-dev-body {
          padding: 12px;
          border-top: 1px solid rgba(255,255,255,0.05);
          background: rgba(0,0,0,0.18);
        }
        .sl-dev-note {
          font-family: var(--font-body);
          font-size: var(--fs-2xs);
          color: rgba(237,233,227,0.25);
          margin: 0 0 10px;
          line-height: 1.6;
        }
        .sl-dev-note code {
          background: rgba(255,255,255,0.07);
          padding: 1px 5px;
          border-radius: 3px;
        }
        .sl-dev-grid {
          display: flex;
          flex-direction: column;
          gap: 4px;
        }
        .sl-dev-role {
          display: flex;
          align-items: center;
          gap: 9px;
          padding: 8px 10px;
          background: rgba(255,255,255,0.025);
          border: 1px solid rgba(255,255,255,0.06);
          border-radius: 7px;
          cursor: pointer;
          text-align: left;
          font-family: var(--font-body);
          transition: background 0.13s, border-color 0.13s;
        }
        .sl-dev-role:hover {
          background: rgba(var(--rc), 0.07);
          border-color: rgba(var(--rc), 0.22);
        }
        .sl-dev-dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }
        .sl-dev-info { flex: 1; display: flex; flex-direction: column; gap: 1px; }
        .sl-dev-name { font-size: var(--fs-sm); font-weight: 600; color: rgba(237,233,227,0.78); }
        .sl-dev-desc { font-size: var(--fs-2xs); color: rgba(237,233,227,0.28); }

        /* Footer */
        .sl-footer {
          display: flex;
          align-items: center;
          gap: 7px;
          margin-top: 24px;
          padding-top: 16px;
          border-top: 1px solid rgba(255,255,255,0.055);
        }
        .sl-foot-link {
          font-family: var(--font-body);
          font-size: var(--fs-xs);
          color: rgba(237,233,227,0.22);
          text-decoration: none;
          transition: color 0.18s;
        }
        .sl-foot-link:hover { color: rgba(237,233,227,0.45); }
        .sl-foot-sep { font-size: var(--fs-xs); color: rgba(237,233,227,0.12); }
      `}</style>
    </>
  )
}
