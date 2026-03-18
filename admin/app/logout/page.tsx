'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'

export default function LogoutPage() {
  const router = useRouter()
  const [mounted, setMounted] = useState(false)
  const [stage, setStage] = useState<'revoking' | 'done'>('revoking')

  useEffect(() => {
    setMounted(true)
    // Simulate session revocation
    const t = setTimeout(() => {
      // In production: POST /api/v1/auth/sessions/revoke, clear localStorage
      if (typeof window !== 'undefined') {
        localStorage.removeItem('dare_session')
      }
      setStage('done')
    }, 1400)
    return () => clearTimeout(t)
  }, [])

  useEffect(() => {
    if (stage === 'done') {
      const t = setTimeout(() => router.push('/login'), 1200)
      return () => clearTimeout(t)
    }
  }, [stage, router])

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
      <style>{`
        @keyframes logoutSpin { to { transform: rotate(360deg); } }
        @keyframes logoutFade { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }
      `}</style>

      {/* Subtle background */}
      <div aria-hidden style={{ position: 'absolute', inset: 0, pointerEvents: 'none' }}>
        <div style={{
          position: 'absolute', top: '30%', left: '50%', transform: 'translateX(-50%)',
          width: '400px', height: '400px', borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(235,93,30,0.04) 0%, transparent 65%)',
        }} />
      </div>

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
            <div style={{ position: 'relative', width: '56px', height: '56px', margin: '0 auto 24px' }}>
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
            <h2 style={{
              fontFamily: 'var(--font-display)', fontSize: '22px', fontWeight: 400,
              color: 'var(--text)', marginBottom: '8px', letterSpacing: '-0.01em',
            }}>
              Signing you out…
            </h2>
            <p style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.04em' }}>
              Revoking session and clearing credentials
            </p>
          </>
        ) : (
          <div style={{ animation: 'logoutFade 0.4s ease' }}>
            {/* Checkmark */}
            <div style={{
              width: '56px', height: '56px', borderRadius: '50%',
              background: 'var(--success-bg)', border: '1px solid var(--success-border)',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
              margin: '0 auto 24px',
            }}>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none"
                stroke="var(--success)" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
                <polyline points="20 6 9 17 4 12" />
              </svg>
            </div>
            <h2 style={{
              fontFamily: 'var(--font-display)', fontSize: '22px', fontWeight: 400,
              color: 'var(--text)', marginBottom: '8px', letterSpacing: '-0.01em',
            }}>
              Signed out
            </h2>
            <p style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.04em' }}>
              Redirecting to sign-in…
            </p>

            <button
              onClick={() => router.push('/login')}
              style={{
                marginTop: '24px', padding: '9px 22px',
                background: 'var(--bg-surface)', border: '1px solid var(--border)',
                borderRadius: '8px', color: 'var(--text-subtle)', fontSize: '12px',
                fontFamily: 'var(--font-body)', cursor: 'pointer',
                transition: 'border-color 0.15s, color 0.15s',
              }}
              onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' }}
              onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
            >
              Sign in again
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
