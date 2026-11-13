'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'

const ITEMS = [
  { href: '/platform', label: 'Clubs', exact: false },
  { href: '/platform/attention', label: 'Needs attention', exact: true },
  { href: '/platform/activity', label: 'Activity', exact: true },
  { href: '/platform/system', label: 'System status', exact: true },
]

export function PlatformNav() {
  const path = usePathname()
  const isActive = (href: string, exact: boolean) => (exact ? path === href : path === href || path.startsWith('/platform/clubs'))
  return (
    <nav className="platform-nav" aria-label="Operator console">
      {ITEMS.map((i) => (
        <Link key={i.href} href={i.href} className="platform-nav-link" aria-current={isActive(i.href, i.exact) ? 'page' : undefined}>{i.label}</Link>
      ))}
    </nav>
  )
}
