'use client'

import type { ReactNode } from 'react'
import { useSession } from '@/lib/session'
import { useRouter } from 'next/navigation'
import { PortalNav } from './PortalNav'

export function PortalShell({ children }: { children: ReactNode }) {
  const { session } = useSession()
  const router = useRouter()

  const handleSignOut = () => {
    router.push('/logout')
  }

  if (!session) return null

  return (
    <div
      data-theme="light"
      data-portal="parent"
      style={{
        minHeight: '100vh',
        background: 'var(--bg)',
        // Re-resolve text color against this portal's tokens; body's color was
        // computed from the admin (dark) root tokens and would otherwise inherit.
        color: 'var(--text)',
        fontFamily: 'var(--font-body)',
        position: 'relative',
      }}
    >
      {/* Warm gradient wash at top */}
      <div
        aria-hidden="true"
        style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          height: '340px',
          background:
            'linear-gradient(180deg, rgba(235,93,30,0.06) 0%, rgba(247,244,240,0) 100%)',
          pointerEvents: 'none',
          zIndex: 0,
        }}
      />

      <PortalNav
        userName={session.user.name}
        avatarInitials={session.user.avatarInitials ?? session.user.name?.slice(0, 2) ?? '?'}
        onSignOut={handleSignOut}
      />

      <main className="portal-frame portal-main">
        {children}
      </main>
    </div>
  )
}
