'use client'

import { useRouter } from 'next/navigation'
import { useSession } from '@/lib/session'
import { destinationForRole } from '@/lib/auth-routing'
import './page.inline.css'

export default function UnauthorizedPage() {
  const router = useRouter()
  const { session } = useSession()
  const dashboardDestination = session ? destinationForRole(session.user.roles) : '/dashboard'

  return (
    <div
      className="is-66f6a69"
    >
      <div className="unauth-card">
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
