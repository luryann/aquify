'use client'

import type { ReactNode } from 'react'
import { useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { usePathname, useRouter } from 'next/navigation'
import { Sidebar } from '@/components/layout/Sidebar'
import { Topbar } from '@/components/layout/Topbar'
import { buildSidebarSessionProps } from '@/lib/sidebar-badge'

export function AdminShell({ children }: { children: ReactNode }) {
  const { session } = useSession()
  const router = useRouter()
  const pathname = usePathname()
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [navigating, setNavigating] = useState(false)

  // Restore saved theme preference on mount
  useEffect(() => {
    try {
      const saved = localStorage.getItem('dare-theme')
      if (saved) {
        document.documentElement.setAttribute('data-theme', saved)
      }
    } catch { /* localStorage unavailable in SSR/private mode */ }
  }, [])

  // Close the mobile drawer whenever navigation happens (link click inside it).
  useEffect(() => {
    setMobileNavOpen(false)
  }, [pathname])

  // Clear the nav-progress indicator once the route has actually changed.
  useEffect(() => {
    setNavigating(false)
  }, [pathname])

  // Safety net: clicking a link to the already-active route (or a link that
  // opens a new tab via modifier keys we couldn't detect) never changes
  // pathname, so drop the bar after a few seconds rather than leaving it stuck.
  useEffect(() => {
    if (!navigating) return
    const timeout = setTimeout(() => setNavigating(false), 4000)
    return () => clearTimeout(timeout)
  }, [navigating])

  // A dev-server route that hasn't been visited yet compiles on demand and
  // can take a couple of seconds with no visual feedback, which reads as a
  // click that "didn't register." Show an immediate progress bar on any
  // in-app link click so the click is visibly acknowledged right away.
  const handleNavClickCapture = (e: React.MouseEvent) => {
    const anchor = (e.target as HTMLElement).closest('a')
    if (!anchor) return
    const href = anchor.getAttribute('href')
    if (!href || href.startsWith('#') || anchor.target === '_blank') return
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    setNavigating(true)
  }

  const handleSignOut = () => {
    router.push('/logout')
  }

  if (!session) return null

  const sidebarProps = buildSidebarSessionProps(session)

  return (
    <div className="admin-shell" onClickCapture={handleNavClickCapture}>
      <a href="#main-content" className="skip-to-content">
        Skip to main content
      </a>
      {navigating && (
        <div
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            height: '2px',
            zIndex: 9998,
            overflow: 'hidden',
            background: 'transparent',
          }}
        >
          <div
            style={{
              height: '100%',
              width: '100%',
              background: 'var(--accent)',
              transformOrigin: 'left center',
              animation: 'navProgressIndeterminate 1s ease-in-out infinite',
            }}
          />
        </div>
      )}
      <div className="admin-sidebar-wrap">
        <Sidebar
          {...sidebarProps}
          onSignOut={handleSignOut}
        />
      </div>
      <main className="admin-main">
        <Topbar onOpenMobileNav={() => setMobileNavOpen(true)} />
        <div className="admin-content" id="main-content" tabIndex={-1}>
          {children}
        </div>
      </main>

      {mobileNavOpen && (
        <>
          <div className="mobile-nav-overlay" onClick={() => setMobileNavOpen(false)} />
          <div className="mobile-nav-drawer">
            <Sidebar
              {...sidebarProps}
              onSignOut={handleSignOut}
            />
          </div>
        </>
      )}
    </div>
  )
}
