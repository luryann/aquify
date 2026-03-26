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
      style={{
        minHeight: '100vh',
        background: '#F7F4F0',
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

      <main
        style={{
          position: 'relative',
          zIndex: 1,
          maxWidth: '1100px',
          margin: '0 auto',
          padding: '40px 28px 80px',
        }}
      >
        {children}
      </main>
    </div>
  )
}
