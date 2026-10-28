'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

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
        borderBottom: '1px solid rgba(0,0,0,0.07)',
        boxShadow: '0 1px 0 rgba(0,0,0,0.04)',
      }}
    >
      <div
        className="portal-frame portal-nav-bar"
        style={{
          height: '58px',
          display: 'flex',
          alignItems: 'center',
          gap: '0',
        }}
      >
        {/* Wordmark */}
        <Link
          href="/portal/dashboard"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            textDecoration: 'none',
            marginRight: '40px',
            flexShrink: 0,
          }}
        >
          <span
            style={{
              fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)',
              fontSize: 'var(--fs-xl)',
              color: 'var(--text)',
              letterSpacing: 'var(--display-tracking)',
              lineHeight: 1,
              userSelect: 'none',
            }}
          >
            aquify
          </span>
        </Link>

        {/* Nav links (desktop) */}
        <nav
          className="desktop-nav-links"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '2px',
            flex: 1,
          }}
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
                    e.currentTarget.style.background = 'rgba(0,0,0,0.05)'
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
            <button
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
                  e.currentTarget.style.background = 'rgba(0,0,0,0.05)'
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
                  className="card"
                  style={{
                    position: 'absolute',
                    top: 'calc(100% + 8px)',
                    left: 0,
                    boxShadow: '0 8px 32px rgba(0,0,0,0.12), 0 2px 8px rgba(0,0,0,0.06)',
                    minWidth: '190px',
                    padding: '6px',
                    zIndex: 100,
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '2px',
                    animation: 'slideUp 0.18s ease both',
                  }}
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

        <button
          className="mobile-nav-toggle"
          onClick={() => setMobileNavOpen(true)}
          aria-label="Open navigation menu"
          style={{ marginLeft: 'auto' }}
        >
          <Icon name="menu" size={16} />
        </button>

        {/* User area */}
        <div style={{ position: 'relative', flexShrink: 0, marginLeft: '8px' }}>
          <button
            onClick={() => setUserMenuOpen((v) => !v)}
            aria-label="User menu"
            aria-expanded={userMenuOpen}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '4px 12px 4px 4px',
              background: 'transparent',
              border: '1px solid rgba(0,0,0,0.10)',
              borderRadius: '999px',
              cursor: 'pointer',
              transition: 'background 120ms ease, border-color 120ms ease',
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.background = 'rgba(0,0,0,0.04)'
              e.currentTarget.style.borderColor = 'rgba(0,0,0,0.16)'
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.background = 'transparent'
              e.currentTarget.style.borderColor = 'rgba(0,0,0,0.10)'
            }}
          >
            {/* Avatar */}
            <div
              style={{
                width: '28px',
                height: '28px',
                borderRadius: '50%',
                background: 'linear-gradient(135deg, var(--accent) 0%, var(--accent-hover) 100%)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'var(--bg-elevated)',
                fontSize: 'var(--fs-xs)',
                fontWeight: 700,
                fontFamily: 'var(--font-body)',
                textTransform: 'uppercase',
                flexShrink: 0,
              }}
            >
              {avatarInitials ?? <Icon name="user" size={14} />}
            </div>
            <span
              style={{
                fontSize: 'var(--fs-base)',
                fontWeight: 500,
                color: 'var(--text)',
                maxWidth: '120px',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
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
                className="card"
                style={{
                  position: 'absolute',
                  top: 'calc(100% + 6px)',
                  right: 0,
                  boxShadow: '0 8px 32px rgba(0,0,0,0.12), 0 2px 8px rgba(0,0,0,0.06)',
                  minWidth: '180px',
                  padding: '6px',
                  zIndex: 100,
                  animation: 'slideUp 0.18s ease both',
                }}
              >
                <Link
                  href="/portal/household"
                  onClick={() => setUserMenuOpen(false)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '8px',
                    padding: '8px 12px',
                    borderRadius: '8px',
                    textDecoration: 'none',
                    color: 'var(--text)',
                    fontSize: 'var(--fs-base)',
                    transition: 'background 100ms ease',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.04)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="home" size={15} style={{ color: 'var(--text-subtle)' }} />
                  Household Profile
                </Link>
                <div
                  style={{
                    height: '1px',
                    background: 'rgba(0,0,0,0.07)',
                    margin: '4px 0',
                  }}
                />
                <button
                  onClick={() => { setUserMenuOpen(false); onSignOut?.() }}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '8px',
                    padding: '8px 12px',
                    borderRadius: '8px',
                    background: 'transparent',
                    border: 'none',
                    color: 'var(--error)',
                    fontSize: 'var(--fs-base)',
                    cursor: 'pointer',
                    width: '100%',
                    textAlign: 'left',
                    transition: 'background 100ms ease',
                  }}
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
          className="mobile-nav-drawer"
          data-theme="light"
          style={{
            width: 'min(280px, 84vw)',
            background: 'var(--bg-elevated)',
            borderRight: '1px solid rgba(0,0,0,0.08)',
            flexDirection: 'column',
            padding: '20px 12px',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '0 8px 16px' }}>
            <span style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: 'var(--text)' }}>aquify</span>
            <button
              className="btn-icon"
              onClick={() => setMobileNavOpen(false)}
              aria-label="Close navigation menu"
            >
              <Icon name="x" size={16} />
            </button>
          </div>
          <nav style={{ display: 'flex', flexDirection: 'column', gap: '2px' }} aria-label="Portal navigation">
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
    </>
  )
}
