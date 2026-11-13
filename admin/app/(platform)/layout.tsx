import type { ReactNode } from 'react'
import Link from 'next/link'
import { AuthGuard } from '@/components/auth/AuthGuard'
import { PlatformNav } from './PlatformNav'
import './platform.css'

// Operator console shell: deliberately separate from the club admin shell so a
// platform operator never sees (or is mistaken for) a club's own navigation.
export default function PlatformLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGuard requiredRoles={['platform_admin']}>
      <div className="platform-shell">
        <header className="platform-bar">
          <div className="platform-bar-inner">
            <div className="platform-brand">
              <strong className="platform-brand-name">aquify</strong>
              <span className="platform-brand-tag">Operator console</span>
            </div>
            <Link href="/logout" className="btn btn-ghost">Sign out</Link>
          </div>
          <PlatformNav />
        </header>
        <main id="main" className="platform-main">
          <div className="page">{children}</div>
        </main>
      </div>
    </AuthGuard>
  )
}
