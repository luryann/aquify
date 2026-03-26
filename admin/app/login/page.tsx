'use client'

import { useEffect, useState, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { useSession, DEV_SESSIONS } from '@/lib/session'
import { destinationForRole } from '@/lib/auth-routing'
import { DEV_STUB_TOKENS_ENABLED } from '@/lib/featureFlags'

const STUB_ROLES = [
  { id: 'stub-owner',    label: 'Owner',     color: '#eb5d1e', rgb: '235,93,30',   desc: 'Full system access' },
  { id: 'stub-admin',    label: 'Admin',     color: '#60a5fa', rgb: '96,165,250',  desc: 'Administrative access' },
  { id: 'stub-coach',    label: 'Coach',     color: '#34d399', rgb: '52,211,153',  desc: 'Training & meets' },
  { id: 'stub-parent',   label: 'Parent',    color: '#a78bfa', rgb: '167,139,250', desc: 'Household portal' },
  { id: 'stub-billing',  label: 'Billing',   color: '#fbbf24', rgb: '251,191,36',  desc: 'Financial access' },
  { id: 'stub-readonly', label: 'Read-Only', color: '#9ca3af', rgb: '156,163,175', desc: 'View-only staff' },
  { id: 'stub-swimmer',  label: 'Swimmer',   color: '#22d3ee', rgb: '34,211,238',  desc: 'Swimmer portal' },
] as const

export default function LoginPage() {
  const router = useRouter()
  const { session, loading, setSession } = useSession()
  const [email, setEmail] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [devOpen, setDevOpen] = useState(false)
  const [mounted, setMounted] = useState(false)

  useEffect(() => { setMounted(true) }, [])

  useEffect(() => {
    if (!loading && session) {
      router.replace(destinationForRole(session.user.role))
    }
  }, [loading, session, router])

  const handleSSO = useCallback(() => {
    setSubmitting(true)
    const q = new URLSearchParams()
    if (email.trim()) q.set('login_hint', email.trim())
    q.set('org_id', 'org-demo')
    window.location.href = `/api/v1/auth/oidc/authorize?${q}`
  }, [email])

  const handleStub = useCallback((id: string) => {
    const s = DEV_SESSIONS[id]
    if (!s) return
    setSession(s)
    router.push(destinationForRole(s.user.role))
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
          background: '#eb5d1e',
          boxShadow: '0 0 16px rgba(235,93,30,0.6)',
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
        <div className="sl-bg-grain" />

        <div className="sl-card">
          {/* Logo mark */}
          <div className="sl-logo">
            <svg width="32" height="32" viewBox="0 0 34 34" fill="none" aria-hidden="true">
              <rect width="34" height="34" rx="8" fill="rgba(235,93,30,0.14)" />
              <path d="M9 25V9h7c4.4 0 8 3.58 8 8s-3.6 8-8 8H9z" fill="#eb5d1e" />
              <path d="M13 21v-8h3c2.2 0 4 1.79 4 4s-1.8 4-4 4h-3z" fill="#06101A" />
            </svg>
            <span className="sl-logo-text">aquify</span>
          </div>

          {/* Heading */}
          <div className="sl-heading">
            <h1 className="sl-title">Sign in</h1>
            <p className="sl-subtitle">Enter your email to continue with SSO</p>
          </div>

          {/* Form */}
          <div className="sl-fields">
            <div className="sl-field">
              <label htmlFor="sl-email" className="sl-label">Email address</label>
              <input
                id="sl-email"
                type="email"
                className="sl-input"
                placeholder="you@yourclub.com"
                value={email}
                onChange={e => setEmail(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') handleSSO() }}
                autoComplete="email"
                autoFocus
                disabled={submitting}
              />
            </div>

            <button
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
                  Continue with SSO
                </>
              )}
            </button>

            <p className="sl-hint">{"You'll be redirected to your club's identity provider."}</p>
          </div>

          <div className="sl-sep"><span>or</span></div>

          <div className="sl-access-row">
            <span className="sl-access-text">{"No account?"}</span>
            <a href="/register" className="sl-access-link">Request access →</a>
          </div>

          {/* Dev stub login */}
          {DEV_STUB_TOKENS_ENABLED && (
            <div className="sl-dev">
              <button className="sl-dev-toggle" onClick={() => setDevOpen(v => !v)}>
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
                      <button key={r.id} className="sl-dev-role" onClick={() => handleStub(r.id)} style={{ '--rc': r.rgb } as React.CSSProperties}>
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

          <footer className="sl-footer">
            <a href="#" className="sl-foot-link">Privacy</a>
            <span className="sl-foot-sep">·</span>
            <a href="#" className="sl-foot-link">Terms</a>
            <span className="sl-foot-sep">·</span>
            <a href="#" className="sl-foot-link">Support</a>
          </footer>
        </div>
      </div>

      <style>{`
        .sl-root {
          min-height: 100vh;
          display: flex;
          align-items: center;
          justify-content: center;
          background: #06101A;
          padding: 24px;
          position: relative;
        }

        /* Subtle grain overlay */
        .sl-bg-grain {
          position: fixed;
          inset: 0;
          pointer-events: none;
          background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.025'/%3E%3C/svg%3E");
          background-size: 200px 200px;
          opacity: 0.4;
          z-index: 0;
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
          box-shadow: 0 24px 80px rgba(0,0,0,0.5), 0 0 0 1px rgba(235,93,30,0.04);
          animation: slReveal 0.45s cubic-bezier(0.16,1,0.3,1) both;
        }
        @media (max-width: 480px) {
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
          font-family: var(--font-display);
          font-style: italic;
          font-size: 20px;
          color: #EDE9E3;
          letter-spacing: -0.01em;
        }

        /* Heading */
        .sl-heading {
          margin-bottom: 28px;
        }
        .sl-title {
          font-family: var(--font-display);
          font-size: 32px;
          font-weight: 400;
          color: #EDE9E3;
          margin: 0 0 6px;
          letter-spacing: -0.02em;
          line-height: 1.1;
        }
        .sl-subtitle {
          font-family: var(--font-body);
          font-size: 13.5px;
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
          font-size: 11px;
          font-weight: 600;
          color: rgba(237,233,227,0.42);
          letter-spacing: 0.05em;
          text-transform: uppercase;
        }
        .sl-input {
          width: 100%;
          background: rgba(255,255,255,0.04);
          border: 1px solid rgba(255,255,255,0.09);
          border-radius: 9px;
          color: #EDE9E3;
          font-family: var(--font-body);
          font-size: 14px;
          padding: 12px 14px;
          outline: none;
          transition: border-color 0.18s, background 0.18s, box-shadow 0.18s;
          box-sizing: border-box;
        }
        .sl-input::placeholder { color: rgba(237,233,227,0.18); }
        .sl-input:focus {
          border-color: rgba(235,93,30,0.5);
          background: rgba(235,93,30,0.04);
          box-shadow: 0 0 0 3px rgba(235,93,30,0.08);
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
          background: #eb5d1e;
          border: none;
          border-radius: 9px;
          color: #fff;
          font-family: var(--font-body);
          font-size: 14px;
          font-weight: 600;
          letter-spacing: 0.01em;
          cursor: pointer;
          transition: background 0.18s, transform 0.12s, box-shadow 0.18s;
        }
        .sl-btn:hover:not(:disabled) {
          background: #d44f14;
          transform: translateY(-1px);
          box-shadow: 0 6px 20px rgba(235,93,30,0.3);
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
          font-size: 11.5px;
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
          font-size: 12px;
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
          font-size: 13px;
        }
        .sl-access-text { color: rgba(237,233,227,0.32); }
        .sl-access-link {
          color: #eb5d1e;
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
          font-family: var(--font-mono);
          font-size: 9px;
          font-weight: 700;
          padding: 2px 6px;
          background: rgba(235,93,30,0.12);
          border: 1px solid rgba(235,93,30,0.28);
          border-radius: 4px;
          color: #eb5d1e;
          letter-spacing: 0.1em;
        }
        .sl-dev-label {
          flex: 1;
          font-size: 12px;
          color: rgba(237,233,227,0.5);
        }
        .sl-dev-body {
          padding: 12px;
          border-top: 1px solid rgba(255,255,255,0.05);
          background: rgba(0,0,0,0.18);
        }
        .sl-dev-note {
          font-family: var(--font-mono);
          font-size: 10.5px;
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
        .sl-dev-name { font-size: 12px; font-weight: 600; color: rgba(237,233,227,0.78); }
        .sl-dev-desc { font-size: 10.5px; color: rgba(237,233,227,0.28); }

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
          font-size: 11.5px;
          color: rgba(237,233,227,0.22);
          text-decoration: none;
          transition: color 0.18s;
        }
        .sl-foot-link:hover { color: rgba(237,233,227,0.45); }
        .sl-foot-sep { font-size: 11px; color: rgba(237,233,227,0.12); }
      `}</style>
    </>
  )
}
