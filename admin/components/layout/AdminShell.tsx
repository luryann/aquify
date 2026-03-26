'use client'

import type { ReactNode } from 'react'
import { useEffect } from 'react'
import { useSession } from '@/lib/session'
import { useRouter } from 'next/navigation'
import { Sidebar } from '@/components/layout/Sidebar'
import { Topbar } from '@/components/layout/Topbar'
import { buildSidebarSessionProps } from '@/lib/sidebar-badge'

export function AdminShell({ children }: { children: ReactNode }) {
  const { session } = useSession()
  const router = useRouter()

  // Restore saved theme preference on mount
  useEffect(() => {
    try {
      const saved = localStorage.getItem('dare-theme')
      if (saved) {
        document.documentElement.setAttribute('data-theme', saved)
      }
    } catch { /* localStorage unavailable in SSR/private mode */ }
  }, [])

  const handleSignOut = () => {
    router.push('/logout')
  }

  if (!session) return null

  const sidebarProps = buildSidebarSessionProps(session)

  return (
    <div className="admin-shell">
      <div className="admin-sidebar-wrap">
        <Sidebar
          {...sidebarProps}
          onSignOut={handleSignOut}
        />
      </div>
      <main className="admin-main">
        <Topbar />
        <div className="admin-content">
          {children}
        </div>
      </main>
    </div>
  )
}
