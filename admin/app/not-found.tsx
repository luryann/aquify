'use client'

import Link from 'next/link'
import { useEffect, useState } from 'react'

export default function NotFound() {
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
        @keyframes notFoundFloat {
          0%, 100% { transform: translateY(0px); }
          50%       { transform: translateY(-8px); }
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
          background: 'radial-gradient(circle, rgba(235,93,30,0.06) 0%, transparent 65%)',
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
        <div
          style={{
            marginBottom: '32px',
            animation: 'notFoundFloat 4s ease-in-out infinite',
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
            404
          </span>
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
          Page not found
        </h1>
        <p
          style={{
            fontSize: '14px',
            color: 'var(--text-subtle)',
            lineHeight: 1.7,
            marginBottom: '36px',
          }}
        >
          The page you&apos;re looking for doesn&apos;t exist or may have moved.
          Check the URL, or head back to the dashboard.
        </p>

        <div style={{ display: 'flex', gap: '12px', justifyContent: 'center', flexWrap: 'wrap' }}>
          <Link
            href="/dashboard"
            style={{
              padding: '10px 22px',
              background: 'var(--accent)',
              border: 'none',
              borderRadius: '8px',
              color: '#fff',
              fontSize: '13px',
              fontWeight: 600,
              fontFamily: 'var(--font-body)',
              textDecoration: 'none',
              display: 'inline-flex',
              alignItems: 'center',
              gap: '7px',
            }}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
              strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <rect x="3" y="3" width="7" height="7" rx="1" /><rect x="14" y="3" width="7" height="7" rx="1" />
              <rect x="3" y="14" width="7" height="7" rx="1" /><rect x="14" y="14" width="7" height="7" rx="1" />
            </svg>
            Dashboard
          </Link>
        </div>
      </div>
    </div>
  )
}
