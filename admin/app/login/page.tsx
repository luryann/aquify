'use client'

import { useEffect, useState, useCallback } from 'react'
import { useRouter } from 'next/navigation'
import { useSession, DEV_SESSIONS } from '@/lib/session'

const IS_DEV = process.env.NODE_ENV === 'development'

const STUB_ROLES = [
  { id: 'stub-owner',    label: 'Owner',     color: '#eb5d1e', rgb: '235,93,30',   desc: 'Full system access' },
  { id: 'stub-admin',    label: 'Admin',     color: '#60a5fa', rgb: '96,165,250',  desc: 'Administrative access' },
  { id: 'stub-coach',    label: 'Coach',     color: '#34d399', rgb: '52,211,153',  desc: 'Training & meets' },
  { id: 'stub-parent',   label: 'Parent',    color: '#a78bfa', rgb: '167,139,250', desc: 'Household portal' },
  { id: 'stub-billing',  label: 'Billing',   color: '#fbbf24', rgb: '251,191,36',  desc: 'Financial access' },
  { id: 'stub-readonly', label: 'Read-Only', color: '#9ca3af', rgb: '156,163,175', desc: 'View-only staff' },
  { id: 'stub-swimmer',  label: 'Swimmer',   color: '#22d3ee', rgb: '34,211,238',  desc: 'Swimmer portal' },
] as const

function getDestination(role: string): string {
  if (role === 'parent') return '/portal/dashboard'
  if (role === 'swimmer') return '/swim'
  return '/dashboard'
}

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
      router.replace(getDestination(session.user.role))
    }
  }, [loading, session, router])

  const handleSSO = useCallback(() => {
    setSubmitting(true)
    const q = new URLSearchParams()
    if (email.trim()) q.set('login_hint', email.trim())
    window.location.href = `/api/v1/auth/oidc/authorize${q.size ? `?${q}` : ''}`
  }, [email])

  const handleStub = useCallback((id: string) => {
    const s = DEV_SESSIONS[id]
    if (!s) return
    setSession(s)
    router.push(getDestination(s.user.role))
  }, [setSession, router])

  // Loading splash
  if (!mounted || loading) {
    return (
      <div style={{
        position: 'fixed', inset: 0, background: '#040C16',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}>
        <div style={{
          width: 8, height: 8, borderRadius: '50%',
          background: '#eb5d1e', boxShadow: '0 0 16px rgba(235,93,30,0.6)',
          animation: 'lpDot 1.4s ease-in-out infinite',
        }} />
        <style>{`@keyframes lpDot{0%,100%{opacity:.35;transform:scale(.85)}50%{opacity:1;transform:scale(1.2)}}`}</style>
      </div>
    )
  }

  // Already authenticated — redirect in effect, show nothing
  if (session) return null

  return (
    <>
      <div className="lp-root">

        {/* ─── Visual panel (desktop only) ─── */}
        <aside className="lp-visual" aria-hidden="true">
          <div className="lp-v-bg" />
          <div className="lp-v-glow-teal" />
          <div className="lp-v-glow-orange" />
          <div className="lp-v-lanes" />

          <div className="lp-v-content">
            <div className="lp-v-eyebrow">Aquatics Operations Platform</div>
            <h1 className="lp-v-wordmark">DARE</h1>
            <p className="lp-v-tagline">
              Swim club management, reimagined.<br />
              Built to replace TeamUnify.
            </p>
            <ul className="lp-v-features">
              {[
                'Member & household management',
                'Meet scheduling and entries',
                'SD3 / CL2 / HY3 file interoperability',
                'Stripe billing & compliance ledger',
                'Performance tracking & analytics',
              ].map(f => (
                <li key={f} className="lp-v-feature">
                  <svg viewBox="0 0 12 12" fill="none" width="11" height="11" style={{ flexShrink: 0 }}>
                    <path d="M1.5 6l3 3 6-6" stroke="#eb5d1e" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                  {f}
                </li>
              ))}
            </ul>
          </div>

          <div className="lp-v-footer">
            © {new Date().getFullYear()} DARE Aquatics · All rights reserved.
          </div>
        </aside>

        {/* ─── Form panel ─── */}
        <main className="lp-panel">
          <div className="lp-form-wrap">

            {/* Brand mark */}
            <div className="lp-brand">
              <svg width="34" height="34" viewBox="0 0 34 34" fill="none" aria-hidden="true">
                <rect width="34" height="34" rx="9" fill="rgba(235,93,30,0.12)" />
                <path d="M9 25V9h7c4.4 0 8 3.58 8 8s-3.6 8-8 8H9z" fill="#eb5d1e" />
                <path d="M13 21v-8h3c2.2 0 4 1.79 4 4s-1.8 4-4 4h-3z" fill="#07121D" />
              </svg>
              <span className="lp-brandname">DARE</span>
            </div>

            {/* Heading */}
            <div className="lp-heading">
              <h2 className="lp-title">Welcome back.</h2>
              <p className="lp-subtitle">
                Sign in to your DARE Aquatics account to continue.
              </p>
            </div>

            {/* Auth form */}
            <div className="lp-fields">
              <div className="lp-field">
                <label htmlFor="lp-email" className="lp-label">Email address</label>
                <input
                  id="lp-email"
                  type="email"
                  className="lp-input"
                  placeholder="coach@dareaquatics.com"
                  value={email}
                  onChange={e => setEmail(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter') handleSSO() }}
                  autoComplete="email"
                  autoFocus
                  disabled={submitting}
                />
              </div>

              <button
                className={`lp-btn${submitting ? ' lp-btn-loading' : ''}`}
                onClick={handleSSO}
                disabled={submitting}
              >
                {submitting ? (
                  <>
                    <span className="lp-spinner" />
                    Connecting to identity provider…
                  </>
                ) : (
                  <>
                    <svg viewBox="0 0 20 20" fill="none" width="15" height="15" aria-hidden="true">
                      <circle cx="10" cy="10" r="8.5" stroke="currentColor" strokeWidth="1.4" />
                      <path
                        d="M10 1.5c-2.5 0-4 4-4 8.5s1.5 8.5 4 8.5m0-17c2.5 0 4 4 4 8.5s-1.5 8.5-4 8.5M1.5 10h17"
                        stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"
                      />
                    </svg>
                    Continue with SSO
                  </>
                )}
              </button>

              <p className="lp-hint">
                {"You'll be redirected to your club's identity provider (Keycloak)."}
              </p>
            </div>

            <div className="lp-sep"><span>or</span></div>

            <div className="lp-access-row">
              <span className="lp-access-text">{"Don't have an account?"}</span>
              <a href="/register" className="lp-access-link">Request access →</a>
            </div>

            {/* Development stub login — hidden in production */}
            {IS_DEV && (
              <div className="lp-dev">
                <button
                  className="lp-dev-toggle"
                  onClick={() => setDevOpen(v => !v)}
                >
                  <span className="lp-dev-badge">DEV</span>
                  <span className="lp-dev-label">Development stub login</span>
                  <svg
                    viewBox="0 0 16 16" fill="none" width="13" height="13"
                    aria-hidden="true"
                    style={{
                      transform: devOpen ? 'rotate(180deg)' : 'none',
                      transition: 'transform 0.2s',
                      flexShrink: 0,
                      color: 'rgba(237,233,227,0.35)',
                    }}
                  >
                    <path d="M3.5 6l4.5 4 4.5-4" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
                  </svg>
                </button>

                {devOpen && (
                  <div className="lp-dev-body">
                    <p className="lp-dev-note">
                      Bypasses OIDC. Requires <code>AUTH_ENABLE_DEV_STUB_TOKENS=true</code> on the server.
                    </p>
                    <div className="lp-dev-grid">
                      {STUB_ROLES.map(r => (
                        <button
                          key={r.id}
                          className="lp-dev-role"
                          onClick={() => handleStub(r.id)}
                          style={{ '--rc': r.rgb } as React.CSSProperties}
                        >
                          <span className="lp-dev-dot" style={{ background: r.color }} />
                          <div className="lp-dev-info">
                            <span className="lp-dev-name">{r.label}</span>
                            <span className="lp-dev-desc">{r.desc}</span>
                          </div>
                          <svg viewBox="0 0 12 12" fill="none" width="10" height="10"
                            aria-hidden="true"
                            style={{ color: 'rgba(237,233,227,0.2)', flexShrink: 0 }}>
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

          <footer className="lp-footer">
            <a href="#" className="lp-foot-link">Privacy</a>
            <span className="lp-foot-sep">·</span>
            <a href="#" className="lp-foot-link">Terms</a>
            <span className="lp-foot-sep">·</span>
            <a href="#" className="lp-foot-link">Support</a>
          </footer>
        </main>
      </div>

      <style>{`
        /* ── Root ── */
        .lp-root {
          display: flex;
          min-height: 100vh;
          background: #040C16;
        }

        /* ── Visual Panel ── */
        .lp-visual {
          display: none;
          position: relative;
          flex: 1;
          overflow: hidden;
          flex-direction: column;
        }
        @media (min-width: 900px) {
          .lp-visual { display: flex; }
        }

        .lp-v-bg {
          position: absolute;
          inset: 0;
          background: #040C16;
        }
        .lp-v-glow-teal {
          position: absolute;
          width: 700px; height: 700px;
          top: -260px; right: -160px;
          background: radial-gradient(circle, rgba(0,190,215,0.16) 0%, transparent 62%);
          filter: blur(64px);
          animation: lpGlow 9s ease-in-out infinite;
          pointer-events: none;
        }
        .lp-v-glow-orange {
          position: absolute;
          width: 560px; height: 560px;
          bottom: -180px; left: -80px;
          background: radial-gradient(circle, rgba(235,93,30,0.12) 0%, transparent 65%);
          filter: blur(56px);
          animation: lpGlow 12s ease-in-out infinite reverse;
          pointer-events: none;
        }
        @keyframes lpGlow {
          0%,100% { transform: translate(0,0) scale(1); opacity: .7; }
          50%      { transform: translate(18px,-22px) scale(1.07); opacity: 1; }
        }
        .lp-v-lanes {
          position: absolute;
          inset: 0;
          background-image: repeating-linear-gradient(
            -55deg,
            transparent 0px,
            transparent 52px,
            rgba(255,255,255,0.016) 52px,
            rgba(255,255,255,0.016) 54px
          );
          pointer-events: none;
        }
        .lp-v-content {
          position: relative;
          z-index: 1;
          flex: 1;
          display: flex;
          flex-direction: column;
          justify-content: center;
          padding: 64px 60px;
        }
        .lp-v-eyebrow {
          display: inline-flex;
          align-items: center;
          padding: 5px 14px;
          border-radius: 20px;
          border: 1px solid rgba(235,93,30,0.28);
          background: rgba(235,93,30,0.08);
          color: #eb5d1e;
          font-family: var(--font-body);
          font-size: 10.5px;
          font-weight: 600;
          letter-spacing: .09em;
          text-transform: uppercase;
          width: fit-content;
          margin-bottom: 22px;
          animation: lpReveal .6s var(--ease-out) both;
        }
        .lp-v-wordmark {
          font-family: var(--font-display);
          font-style: italic;
          font-size: clamp(108px, 15vw, 186px);
          line-height: .88;
          color: #EDE9E3;
          margin: 0;
          letter-spacing: -.025em;
          text-shadow:
            0 2px 60px rgba(235,93,30,0.08),
            0 30px 80px rgba(0,0,0,0.55);
          animation: lpReveal .7s var(--ease-out) .07s both;
        }
        .lp-v-tagline {
          font-family: var(--font-body);
          font-size: 14px;
          color: rgba(237,233,227,0.4);
          line-height: 1.75;
          margin: 22px 0 44px;
          animation: lpReveal .7s var(--ease-out) .14s both;
        }
        .lp-v-features {
          list-style: none;
          margin: 0;
          padding: 0;
          display: flex;
          flex-direction: column;
          gap: 13px;
          animation: lpReveal .7s var(--ease-out) .2s both;
        }
        .lp-v-feature {
          display: flex;
          align-items: center;
          gap: 11px;
          font-family: var(--font-body);
          font-size: 13px;
          font-weight: 500;
          color: rgba(237,233,227,0.5);
        }
        .lp-v-footer {
          position: absolute;
          bottom: 28px; left: 60px; right: 60px;
          font-family: var(--font-body);
          font-size: 11px;
          color: rgba(237,233,227,0.18);
          letter-spacing: .02em;
          z-index: 1;
        }

        /* ── Form Panel ── */
        .lp-panel {
          width: 100%;
          background: #07121D;
          border-left: 1px solid rgba(255,255,255,0.055);
          display: flex;
          flex-direction: column;
        }
        @media (min-width: 900px) {
          .lp-panel { width: 472px; flex-shrink: 0; }
        }
        @media (max-width: 899px) {
          .lp-panel {
            background: #040C16;
            border-left: none;
            min-height: 100vh;
          }
        }

        .lp-form-wrap {
          flex: 1;
          display: flex;
          flex-direction: column;
          justify-content: center;
          padding: 56px 52px;
        }
        @media (max-width: 540px) {
          .lp-form-wrap { padding: 48px 28px; }
        }

        /* Brand */
        .lp-brand {
          display: flex;
          align-items: center;
          gap: 10px;
          margin-bottom: 48px;
          animation: lpReveal .5s var(--ease-out) both;
        }
        .lp-brandname {
          font-family: var(--font-display);
          font-style: italic;
          font-size: 22px;
          color: #EDE9E3;
          letter-spacing: -.01em;
        }

        /* Heading */
        .lp-heading {
          margin-bottom: 36px;
          animation: lpReveal .55s var(--ease-out) .07s both;
        }
        .lp-title {
          font-family: var(--font-display);
          font-size: 38px;
          font-weight: 400;
          color: #EDE9E3;
          margin: 0 0 8px;
          letter-spacing: -.022em;
          line-height: 1.12;
        }
        .lp-subtitle {
          font-family: var(--font-body);
          font-size: 14px;
          color: rgba(237,233,227,0.38);
          margin: 0;
          line-height: 1.65;
        }

        /* Fields */
        .lp-fields {
          display: flex;
          flex-direction: column;
          gap: 14px;
          animation: lpReveal .6s var(--ease-out) .14s both;
        }
        .lp-field {
          display: flex;
          flex-direction: column;
          gap: 7px;
        }
        .lp-label {
          font-family: var(--font-body);
          font-size: 11px;
          font-weight: 600;
          color: rgba(237,233,227,0.45);
          letter-spacing: .055em;
          text-transform: uppercase;
        }
        .lp-input {
          width: 100%;
          background: rgba(255,255,255,0.042);
          border: 1px solid rgba(255,255,255,0.09);
          border-radius: 10px;
          color: #EDE9E3;
          font-family: var(--font-body);
          font-size: 14px;
          padding: 13px 16px;
          outline: none;
          transition: border-color .2s, background .2s, box-shadow .2s;
          -webkit-font-smoothing: antialiased;
        }
        .lp-input::placeholder { color: rgba(237,233,227,0.2); }
        .lp-input:focus {
          border-color: rgba(235,93,30,0.55);
          background: rgba(235,93,30,0.04);
          box-shadow: 0 0 0 3px rgba(235,93,30,0.09);
        }
        .lp-input:disabled { opacity: .45; cursor: not-allowed; }

        /* Primary button */
        .lp-btn {
          display: flex;
          align-items: center;
          justify-content: center;
          gap: 9px;
          width: 100%;
          padding: 14px 20px;
          background: #eb5d1e;
          border: none;
          border-radius: 10px;
          color: #fff;
          font-family: var(--font-body);
          font-size: 14px;
          font-weight: 600;
          letter-spacing: .01em;
          cursor: pointer;
          position: relative;
          overflow: hidden;
          transition: background .2s, transform .15s, box-shadow .2s;
        }
        .lp-btn::after {
          content: '';
          position: absolute;
          inset: 0;
          background: linear-gradient(90deg, transparent 0%, rgba(255,255,255,0.1) 50%, transparent 100%);
          transform: translateX(-100%);
          transition: transform .55s;
        }
        .lp-btn:hover:not(:disabled)::after { transform: translateX(100%); }
        .lp-btn:hover:not(:disabled) {
          background: #d44f14;
          box-shadow: 0 8px 28px rgba(235,93,30,0.32);
          transform: translateY(-1px);
        }
        .lp-btn:active:not(:disabled) {
          transform: translateY(0);
          box-shadow: none;
        }
        .lp-btn:disabled { opacity: .55; cursor: not-allowed; }
        .lp-btn-loading { pointer-events: none; }

        .lp-spinner {
          width: 15px; height: 15px;
          border: 2px solid rgba(255,255,255,0.3);
          border-top-color: #fff;
          border-radius: 50%;
          animation: lpSpin .7s linear infinite;
          flex-shrink: 0;
        }
        @keyframes lpSpin { to { transform: rotate(360deg); } }

        .lp-hint {
          font-family: var(--font-body);
          font-size: 11.5px;
          color: rgba(237,233,227,0.26);
          text-align: center;
          margin: 0;
          line-height: 1.6;
        }

        /* Separator */
        .lp-sep {
          display: flex;
          align-items: center;
          gap: 12px;
          font-family: var(--font-body);
          font-size: 12px;
          color: rgba(237,233,227,0.25);
          margin: 24px 0;
          animation: lpReveal .6s var(--ease-out) .2s both;
        }
        .lp-sep::before, .lp-sep::after {
          content: '';
          flex: 1;
          height: 1px;
          background: rgba(255,255,255,0.07);
        }

        /* Access row */
        .lp-access-row {
          display: flex;
          align-items: center;
          gap: 8px;
          font-family: var(--font-body);
          font-size: 13px;
          animation: lpReveal .6s var(--ease-out) .23s both;
        }
        .lp-access-text { color: rgba(237,233,227,0.36); }
        .lp-access-link {
          color: #eb5d1e;
          text-decoration: none;
          font-weight: 500;
          transition: color .2s;
        }
        .lp-access-link:hover { color: #f5764a; }

        /* Dev panel */
        .lp-dev {
          margin-top: 28px;
          border: 1px solid rgba(235,93,30,0.18);
          border-radius: 10px;
          overflow: hidden;
          animation: lpReveal .6s var(--ease-out) .28s both;
        }
        .lp-dev-toggle {
          display: flex;
          align-items: center;
          gap: 9px;
          width: 100%;
          padding: 11px 14px;
          background: rgba(235,93,30,0.055);
          border: none;
          cursor: pointer;
          text-align: left;
          font-family: var(--font-body);
          transition: background .15s;
        }
        .lp-dev-toggle:hover { background: rgba(235,93,30,0.09); }
        .lp-dev-badge {
          font-family: var(--font-mono);
          font-size: 9px;
          font-weight: 700;
          padding: 2px 7px;
          background: rgba(235,93,30,0.15);
          border: 1px solid rgba(235,93,30,0.3);
          border-radius: 4px;
          color: #eb5d1e;
          letter-spacing: .1em;
        }
        .lp-dev-label {
          flex: 1;
          font-size: 12.5px;
          color: rgba(237,233,227,0.58);
        }
        .lp-dev-body {
          padding: 12px;
          border-top: 1px solid rgba(255,255,255,0.05);
          background: rgba(0,0,0,0.2);
        }
        .lp-dev-note {
          font-family: var(--font-mono);
          font-size: 11px;
          color: rgba(237,233,227,0.28);
          margin: 0 0 10px;
          line-height: 1.6;
        }
        .lp-dev-note code {
          font-family: var(--font-mono);
          background: rgba(255,255,255,0.07);
          padding: 1px 5px;
          border-radius: 3px;
        }
        .lp-dev-grid {
          display: flex;
          flex-direction: column;
          gap: 5px;
        }
        .lp-dev-role {
          display: flex;
          align-items: center;
          gap: 10px;
          padding: 9px 11px;
          background: rgba(255,255,255,0.03);
          border: 1px solid rgba(255,255,255,0.06);
          border-radius: 7px;
          cursor: pointer;
          text-align: left;
          font-family: var(--font-body);
          transition: background .15s, border-color .15s;
        }
        .lp-dev-role:hover {
          background: rgba(var(--rc), 0.07);
          border-color: rgba(var(--rc), 0.25);
        }
        .lp-dev-dot {
          width: 8px; height: 8px;
          border-radius: 50%;
          flex-shrink: 0;
        }
        .lp-dev-info {
          flex: 1;
          display: flex;
          flex-direction: column;
          gap: 1px;
        }
        .lp-dev-name {
          font-size: 12.5px;
          font-weight: 600;
          color: rgba(237,233,227,0.82);
        }
        .lp-dev-desc {
          font-size: 11px;
          color: rgba(237,233,227,0.3);
        }

        /* Footer */
        .lp-footer {
          padding: 18px 52px;
          display: flex;
          align-items: center;
          gap: 8px;
          border-top: 1px solid rgba(255,255,255,0.055);
        }
        @media (max-width: 540px) { .lp-footer { padding: 18px 28px; } }
        .lp-foot-link {
          font-family: var(--font-body);
          font-size: 11.5px;
          color: rgba(237,233,227,0.26);
          text-decoration: none;
          transition: color .2s;
        }
        .lp-foot-link:hover { color: rgba(237,233,227,0.5); }
        .lp-foot-sep { font-size: 11.5px; color: rgba(237,233,227,0.14); }

        /* Reveal animation */
        @keyframes lpReveal {
          from { opacity: 0; transform: translateY(14px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </>
  )
}
