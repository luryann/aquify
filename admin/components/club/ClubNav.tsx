'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'

export function ClubNav({ slug }: { slug: string }) {
  const pathname = usePathname()
  const base = `/club/${slug}`
  const items = [
    { href: base, label: 'Home' },
    { href: `${base}/news`, label: 'News' },
    { href: `${base}/schedule`, label: 'Schedule' },
    { href: `${base}/records`, label: 'Records' },
  ]
  return (
    <nav className="cs-nav" aria-label="Club site">
      {items.map((item) => (
        <Link key={item.href} href={item.href} aria-current={pathname === item.href ? 'page' : undefined}>
          {item.label}
        </Link>
      ))}
      <Link href="/register" className="cs-join">Join the team</Link>
    </nav>
  )
}
