'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useState } from 'react'
import { Icon, type IconName } from '@/components/ui/icons'
import './portal.css'
import './PortalNav.inline.css'

type PortalNavProps = {
  userName?: string
  avatarInitials?: string
  onSignOut?: () => void
}

// Primary links are the weekly parent tasks and always sit in the bar; the rest
// live under "More" on desktop. The mobile drawer lists everything in this order.
const NAV_LINKS = [
  { label: 'Home',           path: '/portal/dashboard',      primary: true },
  { label: 'Meets',          path: '/portal/meets',          primary: true },
  { label: 'Calendar',       path: '/portal/calendar',       primary: true },
  { label: 'Registrations',  path: '/portal/registrations',  primary: true },
  { label: 'Billing',        path: '/portal/billing',        primary: true },
  { label: 'Documents',      path: '/portal/documents',      primary: true },
  { label: 'Messages',       path: '/portal/messages',       primary: true },
  { label: 'Events',         path: '/portal/events',         primary: false },
  { label: 'Volunteer',      path: '/portal/volunteer',      primary: false },
  { label: 'Records',        path: '/portal/records',        primary: false },
  { label: 'Wellness',       path: '/portal/wellness',       primary: false },
  { label: 'Communications', path: '/portal/communications', primary: false },
]
const TAB_LINKS: { label: string; path: string; icon: IconName }[] = [
  { label: 'Home', path: '/portal/dashboard', icon: 'home' },
  { label: 'Meets', path: '/portal/meets', icon: 'clipboard-list' },
  { label: 'Calendar', path: '/portal/calendar', icon: 'calendar' },
  { label: 'Billing', path: '/portal/billing', icon: 'credit-card' },
  { label: 'Messages', path: '/portal/messages', icon: 'mail' },
]
const PRIMARY_LINKS = NAV_LINKS.filter((link) => link.primary)
const MORE_LINKS = NAV_LINKS.filter((link) => !link.primary)

export function PortalNav({ userName, avatarInitials, onSignOut }: PortalNavProps) {
  const pathname = usePathname()
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [moreOpen, setMoreOpen] = useState(false)

  const isActive = (path: string) => pathname === path || pathname.startsWith(path + '/')
  const activeMore = MORE_LINKS.find((link) => isActive(link.path))

  return (
    <>
    <header
      data-theme="light"
      style={{
        position: 'sticky',
        top: 0,
        zIndex: 100,
        background: 'rgba(255, 255, 255, 0.88)',
        backdropFilter: 'blur(12px)',
        WebkitBackdropFilter: 'blur(12px)',
        borderBottom: '1px solid var(--border)',
        boxShadow: 'var(--shadow-sm)',
      }}
    >
      <div
        className="portal-frame portal-nav-bar is-a98e6ad"
        
      >
        {/* Wordmark */}
        <Link
          href="/portal/dashboard"
          className="is-e285dd0"
        >
          <span
            className="is-90da9be"
          >
            aquify
          </span>
        </Link>

        {/* Nav links (desktop) */}
        <nav
          className="desktop-nav-links is-c7a0c1e"
          
          aria-label="Portal navigation"
        >
          {PRIMARY_LINKS.map((link) => {
            const active = isActive(link.path)
            return (
              <Link
                key={link.path}
                href={link.path}
                style={{
                  padding: '6px 12px',
                  borderRadius: '8px',
                  fontSize: 'var(--fs-base)',
                  fontWeight: active ? 600 : 450,
                  color: active ? 'var(--text)' : 'var(--text-subtle)',
                  background: active ? 'rgba(235,93,30,0.08)' : 'transparent',
                  textDecoration: 'none',
                  transition: 'background 120ms ease, color 120ms ease',
                  whiteSpace: 'nowrap',
                }}
                onMouseEnter={(e) => {
                  if (!active) {
                    e.currentTarget.style.background = 'var(--bg-wash)'
                    e.currentTarget.style.color = 'var(--text)'
                  }
                }}
                onMouseLeave={(e) => {
                  if (!active) {
                    e.currentTarget.style.background = 'transparent'
                    e.currentTarget.style.color = 'var(--text-subtle)'
                  }
                }}
              >
                {link.label}
              </Link>
            )
          })}

          <div style={{ position: 'relative' }}>
            <button type="button"
              onClick={() => setMoreOpen((v) => !v)}
              aria-expanded={moreOpen}
              aria-haspopup="menu"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '4px',
                padding: '6px 12px 6px 12px',
                borderRadius: '8px',
                border: 'none',
                fontSize: 'var(--fs-base)',
                fontWeight: activeMore ? 600 : 450,
                color: activeMore ? 'var(--text)' : 'var(--text-subtle)',
                background: activeMore ? 'rgba(235,93,30,0.08)' : 'transparent',
                whiteSpace: 'nowrap',
                transition: 'background 120ms ease, color 120ms ease',
              }}
              onMouseEnter={(e) => {
                if (!activeMore) {
                  e.currentTarget.style.background = 'var(--bg-wash)'
                  e.currentTarget.style.color = 'var(--text)'
                }
              }}
              onMouseLeave={(e) => {
                if (!activeMore) {
                  e.currentTarget.style.background = 'transparent'
                  e.currentTarget.style.color = 'var(--text-subtle)'
                }
              }}
            >
              {activeMore?.label ?? 'More'}
              <Icon name="chevron-down" size={13} style={{ color: 'var(--text-muted)' }} />
            </button>
            {moreOpen && (
              <>
                <div style={{ position: 'fixed', inset: 0, zIndex: 99 }} onClick={() => setMoreOpen(false)} />
                <div
                  role="menu"
                  className="card is-88ca7fc"
                  
                >
                  {MORE_LINKS.map((link) => {
                    const active = isActive(link.path)
                    return (
                      <Link
                        key={link.path}
                        href={link.path}
                        role="menuitem"
                        onClick={() => setMoreOpen(false)}
                        style={{
                          padding: '8px 12px',
                          borderRadius: '8px',
                          fontSize: 'var(--fs-base)',
                          fontWeight: active ? 600 : 450,
                          color: active ? 'var(--text)' : 'var(--text-subtle)',
                          background: active ? 'rgba(235,93,30,0.08)' : 'transparent',
                          textDecoration: 'none',
                        }}
                      >
                        {link.label}
                      </Link>
                    )
                  })}
                </div>
              </>
            )}
          </div>
        </nav>

        <button type="button"
          className="mobile-nav-toggle is-5387648"
          onClick={() => setMobileNavOpen(true)}
          aria-label="Open navigation menu"
          
        >
          <Icon name="menu" size={16} />
        </button>

        {/* User area */}
        <div className="is-fed490f">
          <button type="button"
            onClick={() => setUserMenuOpen((v) => !v)}
            aria-label="User menu"
            aria-expanded={userMenuOpen}
            className="is-8d90a6f"
            onMouseEnter={(e) => {
              e.currentTarget.style.background = 'var(--bg-wash)'
              e.currentTarget.style.borderColor = 'var(--border-strong)'
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.background = 'transparent'
              e.currentTarget.style.borderColor = 'var(--border)'
            }}
          >
            {/* Avatar */}
            <div
              className="is-78ae422"
            >
              {avatarInitials ?? <Icon name="user" size={14} />}
            </div>
            <span
              className="is-117f5a5"
            >
              {userName ?? 'My Account'}
            </span>
            <Icon name="chevron-down" size={13} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
          </button>

          {/* Dropdown */}
          {userMenuOpen && (
            <>
              {/* Backdrop */}
              <div
                style={{ position: 'fixed', inset: 0, zIndex: 99 }}
                onClick={() => setUserMenuOpen(false)}
              />
              <div
                className="card is-bb3cf93"
                
              >
                <Link
                  href="/portal/household"
                  onClick={() => setUserMenuOpen(false)}
                  className="is-0561fd1"
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--bg-wash)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="home" size={15} style={{ color: 'var(--text-subtle)' }} />
                  Household Profile
                </Link>
                <div
                  className="is-aa59499"
                />
                <button type="button"
                  onClick={() => { setUserMenuOpen(false); onSignOut?.() }}
                  className="is-31cee2f"
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(220,38,38,0.06)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="log-out" size={15} style={{ color: 'var(--error)' }} />
                  Sign out
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </header>

    {mobileNavOpen && (
      <>
        <div className="mobile-nav-overlay" onClick={() => setMobileNavOpen(false)} />
        <div
          className="mobile-nav-drawer is-5b82ca5"
          data-theme="light"
          
        >
          <div className="is-40001e4">
            <span className="is-f3c1003">aquify</span>
            <button type="button"
              className="btn-icon"
              onClick={() => setMobileNavOpen(false)}
              aria-label="Close navigation menu"
            >
              <Icon name="x" size={16} />
            </button>
          </div>
          <nav className="is-32bee05" aria-label="Portal navigation">
            {NAV_LINKS.map((link) => {
              const active = isActive(link.path)
              return (
                <Link
                  key={link.path}
                  href={link.path}
                  onClick={() => setMobileNavOpen(false)}
                  style={{
                    padding: '12px 12px',
                    borderRadius: '8px',
                    fontSize: 'var(--fs-md)',
                    fontWeight: active ? 600 : 450,
                    color: active ? 'var(--text)' : 'var(--text-subtle)',
                    background: active ? 'rgba(235,93,30,0.08)' : 'transparent',
                    textDecoration: 'none',
                  }}
                >
                  {link.label}
                </Link>
              )
            })}
          </nav>
        </div>
      </>
    )}
    <nav className="portal-tabbar" aria-label="Main">
      {TAB_LINKS.map((t) => (
        <Link key={t.path} href={t.path} className="portal-tabbar-link" aria-current={isActive(t.path) ? 'page' : undefined}>
          <Icon name={t.icon} size={20} />
          <span>{t.label}</span>
        </Link>
      ))}
    </nav>
    </>
  )
}
