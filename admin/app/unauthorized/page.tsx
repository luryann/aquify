'use client'

import { useRouter } from 'next/navigation'
import { useEffect, useState } from 'react'

export default function UnauthorizedPage() {
  const router = useRouter()
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setMounted(true)
  }, [])

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
        overflow: 'hidden',
      }}
    >
      <style>{`
        @keyframes unauthFloat {
          0%, 100% { transform: translateY(0px); }
          50%       { transform: translateY(-8px); }
        }
        @keyframes unauthFade {
          from { opacity: 0; transform: translateY(20px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>

      {/* Background elements */}
      <div aria-hidden style={{ position: 'absolute', inset: 0, pointerEvents: 'none' }}>
        <div style={{
          position: 'absolute', inset: 0,
          backgroundImage: 'linear-gradient(rgba(255,255,255,0.015) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.015) 1px, transparent 1px)',
          backgroundSize: '56px 56px',
        }} />
        <div style={{
          position: 'absolute', top: '15%', right: '10%',
          width: '360px', height: '360px', borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(248,113,113,0.05) 0%, transparent 65%)',
        }} />
        <div style={{
          position: 'absolute', bottom: '20%', left: '5%',
          width: '280px', height: '280px', borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(235,93,30,0.04) 0%, transparent 65%)',
        }} />
      </div>

      <div
        style={{
          position: 'relative',
          textAlign: 'center',
          maxWidth: '480px',
          padding: '0 24px',
          opacity: mounted ? 1 : 0,
          transform: mounted ? 'translateY(0)' : 'translateY(24px)',
          transition: 'opacity 0.5s var(--ease-out), transform 0.5s var(--ease-out)',
        }}
      >
        {/* 403 typography */}
        <div
          style={{
            position: 'relative',
            marginBottom: '32px',
            animation: 'unauthFloat 4s ease-in-out infinite',
          }}
        >
          <span
            style={{
              fontFamily: 'var(--font-display)',
              fontStyle: 'italic',
              fontSize: 'clamp(96px, 16vw, 144px)',
              fontWeight: 400,
              color: 'transparent',
              WebkitTextStroke: '1px rgba(255,255,255,0.06)',
              letterSpacing: '-0.04em',
              lineHeight: 1,
              userSelect: 'none',
              display: 'block',
            }}
          >
            403
          </span>
          {/* Overlay lock icon */}
          <div
            style={{
              position: 'absolute',
              top: '50%',
              left: '50%',
              transform: 'translate(-50%, -50%)',
              width: '64px',
              height: '64px',
              borderRadius: '16px',
              background: 'var(--error-bg)',
              border: '1px solid var(--error-border)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              boxShadow: '0 0 40px rgba(248,113,113,0.12)',
            }}
          >
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none"
              stroke="var(--error)" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
              <path d="M7 11V7a5 5 0 0 1 10 0v4" />
            </svg>
          </div>
        </div>

        <h1
          style={{
            fontFamily: 'var(--font-display)',
            fontSize: '28px',
            fontWeight: 400,
            color: 'var(--text)',
            marginBottom: '12px',
            letterSpacing: '-0.02em',
          }}
        >
          Access Denied
        </h1>
        <p
          style={{
            fontSize: '14px',
            color: 'var(--text-subtle)',
            lineHeight: 1.7,
            marginBottom: '36px',
          }}
        >
          You don&apos;t have permission to view this page. This area is restricted to
          authorized roles. If you believe this is a mistake, contact your club
          administrator.
        </p>

        {/* Actions */}
        <div style={{ display: 'flex', gap: '12px', justifyContent: 'center', flexWrap: 'wrap' }}>
          <button
            onClick={() => router.back()}
            style={{
              padding: '10px 22px',
              background: 'var(--bg-surface)',
              border: '1px solid var(--border)',
              borderRadius: '8px',
              color: 'var(--text-subtle)',
              fontSize: '13px',
              fontWeight: 500,
              fontFamily: 'var(--font-body)',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '7px',
              transition: 'all 0.15s ease',
            }}
            onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' }}
            onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
              strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <line x1="19" y1="12" x2="5" y2="12" /><polyline points="12 19 5 12 12 5" />
            </svg>
            Go back
          </button>
          <button
            onClick={() => router.push('/dashboard')}
            style={{
              padding: '10px 22px',
              background: 'var(--accent)',
              border: 'none',
              borderRadius: '8px',
              color: '#fff',
              fontSize: '13px',
              fontWeight: 600,
              fontFamily: 'var(--font-body)',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '7px',
              transition: 'background 0.15s ease',
            }}
            onMouseEnter={(e) => { e.currentTarget.style.background = '#d44f14' }}
            onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--accent)' }}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
              strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <rect x="3" y="3" width="7" height="7" rx="1" /><rect x="14" y="3" width="7" height="7" rx="1" />
              <rect x="3" y="14" width="7" height="7" rx="1" /><rect x="14" y="14" width="7" height="7" rx="1" />
            </svg>
            Dashboard
          </button>
        </div>

        {/* Role hint */}
        <div
          style={{
            marginTop: '40px',
            padding: '12px 16px',
            background: 'var(--bg-surface)',
            border: '1px solid var(--border)',
            borderRadius: '10px',
            display: 'inline-flex',
            alignItems: 'center',
            gap: '10px',
          }}
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none"
            stroke="var(--text-muted)" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" />
            <line x1="12" y1="16" x2="12.01" y2="16" />
          </svg>
          <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', letterSpacing: '0.02em' }}>
            HTTP 403 · Insufficient permissions for this resource
          </span>
        </div>
      </div>
    </div>
  )
}
