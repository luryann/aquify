import type { ReactNode } from 'react'
import Link from 'next/link'
import { AuthGuard } from '@/components/auth/AuthGuard'

// Operator console shell: deliberately separate from the club admin shell so a
// platform operator never sees (or is mistaken for) a club's own navigation.
export default function PlatformLayout({ children }: { children: ReactNode }) {
  return (
    <AuthGuard requiredRoles={['platform_admin']}>
      <header className="platform-bar" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px', padding: '12px 24px', borderBottom: '1px solid var(--border)', background: 'var(--bg-elevated)' }}>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: '12px' }}>
          <strong className="u-ff-font-display u-fw-display-weight u-fs-fs-xl">aquify</strong>
          <span className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>Operator console</span>
        </div>
        <Link href="/logout" className="btn btn-ghost">Sign out</Link>
      </header>
      <main id="main" className="page" style={{ paddingTop: 'var(--section-gap)' }}>{children}</main>
    </AuthGuard>
  )
}
