'use client'

import { usePathname } from 'next/navigation'
import Link from 'next/link'
import type { ReactNode } from 'react'
import { useState, useEffect } from 'react'
import { Icon } from '@/components/ui/icons'
import { NAV_ITEMS } from '@/lib/roles'
import { OPEN_COMMAND_PALETTE_EVENT } from '@/components/layout/CommandPalette'

type TopbarProps = {
  actions?: ReactNode
  onOpenMobileNav?: () => void
}

// Build breadcrumb from pathname
function useBreadcrumb() {
  const pathname = usePathname()
  const segments = pathname.split('/').filter(Boolean)

  const crumbs: { label: string; path: string }[] = []

  // Sidebar section names (Competition, System, …) are not pages; repeating
  // them as a crumb only adds a kicker above the title, so the trail starts at
  // the first real page.
  // Record IDs are UUIDs when DB-backed, or `<prefix>_<hex>` in in-memory mode.
  const isUUID = (value: string) =>
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value) || /^[a-z]+_[0-9a-f]{12,}$/i.test(value)

  let built = ''
  for (const seg of segments) {
    built += `/${seg}`
    const match = NAV_ITEMS.find((i) => i.path === built)
    if (match) {
      crumbs.push({ label: match.label, path: built })
    } else if (isUUID(seg)) {
      // Dynamic record IDs (e.g. /meets/{id}) aren't human-readable — the page's
      // own heading already shows the record's name, so skip adding a crumb for it.
      continue
    } else {
      const decoded = decodeURIComponent(seg)
      const label = decoded.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
      crumbs.push({ label, path: built })
    }
  }

  const seen = new Set<string>()
  return crumbs.filter((c) => {
    if (seen.has(c.label)) return false
    seen.add(c.label)
    return true
  })
}

function ThemeToggle() {
  const [mounted, setMounted] = useState(false)
  const [isDark, setIsDark] = useState(false)

  useEffect(() => {
    const current = document.documentElement.getAttribute('data-theme')
    setIsDark(current !== 'light')
    setMounted(true)
  }, [])

  function toggle() {
    const next = isDark ? 'light' : 'dark'
    document.documentElement.setAttribute('data-theme', next)
    setIsDark(!isDark)
    try { localStorage.setItem('dare-theme', next) } catch { /* ignore */ }
  }

  return (
    <button type="button"
      onClick={toggle}
      title={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
      style={{
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        width: '30px', height: '30px', borderRadius: 'var(--r-md)',
        background: 'transparent', border: '1px solid var(--border)',
        cursor: 'pointer', color: 'var(--text-muted)',
        transition: 'all var(--dur-fast)',
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.background = 'var(--bg-elevated)'
        e.currentTarget.style.color = 'var(--text)'
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = 'transparent'
        e.currentTarget.style.color = 'var(--text-muted)'
      }}
    >
      {!mounted ? (
        <span style={{ width: '14px', height: '14px', display: 'block' }} />
      ) : isDark ? (
        // Sun icon
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="5" />
          <line x1="12" y1="1" x2="12" y2="3" />
          <line x1="12" y1="21" x2="12" y2="23" />
          <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
          <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
          <line x1="1" y1="12" x2="3" y2="12" />
          <line x1="21" y1="12" x2="23" y2="12" />
          <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
          <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
        </svg>
      ) : (
        // Moon icon
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
        </svg>
      )}
    </button>
  )
}

export function Topbar({ actions, onOpenMobileNav }: TopbarProps) {
  const crumbs = useBreadcrumb()

  return (
    <header
      className="u-gap-12px" style={{ height: 'var(--topbar-h)', borderBottom: '1px solid var(--border)', background: 'var(--bg)', display: 'flex', alignItems: 'center', padding: '0 var(--page-gutter)', flexShrink: 0 }}
    >
      <button
        type="button"
        className="mobile-nav-toggle"
        onClick={onOpenMobileNav}
        aria-label="Open navigation menu"
      >
        <Icon name="menu" size={16} />
      </button>

      <span
        className="mobile-only u-ff-font-display u-fw-display-weight u-fs-fs-lg u-ls-display-tracking"
        style={{ color: 'var(--text)', flexShrink: 0 }}
      >
        aquify
      </span>

      {/* Breadcrumb */}
      <nav
        aria-label="Breadcrumb"
        className="u-gap-4px" style={{ flex: 1, display: 'flex', alignItems: 'center', minWidth: 0 }}
      >
        {crumbs.map((crumb, i) => {
          const isLast = i === crumbs.length - 1
          return (
            <div
              key={`${crumb.path}-${i}`}
              className={isLast ? 'crumb crumb-current' : 'crumb crumb-parent'}
              style={{ display: 'flex', alignItems: 'center', gap: '4px', minWidth: isLast ? 0 : undefined }}
            >
              {i > 0 && (
                <Icon
                  name="chevron-right"
                  size={13}
                  className="crumb-sep"
                  style={{ color: 'var(--text-muted)', flexShrink: 0 }}
                />
              )}
              {isLast ? (
                <span
                  className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                >
                  {crumb.label}
                </span>
              ) : crumb.path === '#' ? (
                <span
                  className="u-fs-fs-base" style={{ color: 'var(--text-muted)', whiteSpace: 'nowrap' }}
                >
                  {crumb.label}
                </span>
              ) : (
                <Link
                  href={crumb.path}
                  className="u-fs-fs-base" style={{ color: 'var(--text-subtle)', whiteSpace: 'nowrap', textDecoration: 'none', transition: 'color var(--dur-fast)' }}
                  onMouseEnter={(e) => (e.currentTarget.style.color = 'var(--text)')}
                  onMouseLeave={(e) => (e.currentTarget.style.color = 'var(--text-subtle)')}
                >
                  {crumb.label}
                </Link>
              )}
            </div>
          )
        })}
      </nav>

      {/* Right-side actions */}
      <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', flexShrink: 0 }}>
        {actions}

        <button
          type="button"
          className="btn btn-secondary btn-sm"
          onClick={() => window.dispatchEvent(new Event(OPEN_COMMAND_PALETTE_EVENT))}
          aria-label="Open command palette"
          aria-keyshortcuts="Control+K Meta+K"
        >
          <Icon name="search" size={14} />
          <span className="mobile-hide">Search</span>
          <span className="kbd-hint topbar-kbd" aria-hidden>Ctrl K</span>
        </button>

        <ThemeToggle />
      </div>
    </header>
  )
}
