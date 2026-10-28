'use client'

import { useRouter } from 'next/navigation'
import { useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { destinationForRole } from '@/lib/auth-routing'
import './page.inline.css'

export default function UnauthorizedPage() {
  const router = useRouter()
  const { session } = useSession()
  const dashboardDestination = session ? destinationForRole(session.user.roles) : '/dashboard'
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setMounted(true)
  }, [])

  return (
    <div
      className="is-66f6a69"
    >
      <style>{`
        @keyframes unauthFade {
          from { opacity: 0; transform: translateY(20px); }
          to   { opacity: 1; transform: translateY(0); }
        }
        @media (prefers-reduced-motion: reduce) {
          @keyframes unauthFade { from { opacity: 0; } to { opacity: 1; } }
        }
      `}</style>

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
        <div aria-hidden className="is-1c1d443">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none"
            stroke="var(--error)" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round">
            <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
          </svg>
        </div>

        <h1
          className="is-1ae45f5"
        >
          Access Denied
        </h1>
        <p
          className="is-cbd6755"
        >
          You don&apos;t have permission to view this page. If this is a mistake, contact your club administrator.
        </p>

        {/* Actions */}
        <div className="is-c2dfd00">
          <button type="button"
            onClick={() => router.back()}
            className="is-2da28a3"
            onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' }}
            onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
              strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <line x1="19" y1="12" x2="5" y2="12" /><polyline points="12 19 5 12 12 5" />
            </svg>
            Go back
          </button>
          <button type="button"
            onClick={() => router.push(dashboardDestination)}
            className="is-2858c5e"
            onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--accent-hover)' }}
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

      </div>
    </div>
  )
}
