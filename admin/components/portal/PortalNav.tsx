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

const NAV_LINKS = [
  { label: 'Home',           path: '/portal/dashboard' },
  { label: 'Meets',          path: '/portal/meets' },
  { label: 'Billing',        path: '/portal/billing' },
  { label: 'Documents',      path: '/portal/documents' },
  { label: 'Communications', path: '/portal/communications' },
  { label: 'Calendar',       path: '/portal/calendar' },
]

export function PortalNav({ userName, avatarInitials, onSignOut }: PortalNavProps) {
  const pathname = usePathname()
  const [userMenuOpen, setUserMenuOpen] = useState(false)

  const isActive = (path: string) => pathname === path || pathname.startsWith(path + '/')

  return (
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
        style={{
          maxWidth: '1100px',
          margin: '0 auto',
          padding: '0 28px',
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
            gap: '7px',
            textDecoration: 'none',
            marginRight: '36px',
            flexShrink: 0,
          }}
        >
          <div
            style={{
              width: '8px',
              height: '8px',
              borderRadius: '50%',
              background: 'var(--accent)',
              boxShadow: '0 0 8px rgba(235,93,30,0.45)',
            }}
          />
          <span
            style={{
              fontFamily: 'var(--font-display)',
              fontSize: '19px',
              color: '#1A1714',
              letterSpacing: '-0.01em',
              lineHeight: 1,
              userSelect: 'none',
            }}
          >
            aquify
          </span>
          <span
            style={{
              fontSize: '10px',
              fontFamily: 'var(--font-mono)',
              color: '#A09D9B',
              letterSpacing: '0.10em',
              textTransform: 'uppercase',
              paddingTop: '2px',
            }}
          >
            portal
          </span>
        </Link>

        {/* Nav links */}
        <nav
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '2px',
            flex: 1,
          }}
          aria-label="Portal navigation"
        >
          {NAV_LINKS.map((link) => {
            const active = isActive(link.path)
            return (
              <Link
                key={link.path}
                href={link.path}
                style={{
                  padding: '6px 12px',
                  borderRadius: '7px',
                  fontSize: '13.5px',
                  fontWeight: active ? 600 : 450,
                  color: active ? '#1A1714' : '#6B6866',
                  background: active ? 'rgba(235,93,30,0.08)' : 'transparent',
                  textDecoration: 'none',
                  transition: 'background 120ms ease, color 120ms ease',
                  whiteSpace: 'nowrap',
                }}
                onMouseEnter={(e) => {
                  if (!active) {
                    e.currentTarget.style.background = 'rgba(0,0,0,0.05)'
                    e.currentTarget.style.color = '#1A1714'
                  }
                }}
                onMouseLeave={(e) => {
                  if (!active) {
                    e.currentTarget.style.background = 'transparent'
                    e.currentTarget.style.color = '#6B6866'
                  }
                }}
              >
                {link.label}
              </Link>
            )
          })}
        </nav>

        {/* User area */}
        <div style={{ position: 'relative', flexShrink: 0 }}>
          <button
            onClick={() => setUserMenuOpen((v) => !v)}
            aria-label="User menu"
            aria-expanded={userMenuOpen}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '4px 10px 4px 4px',
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
                background: 'linear-gradient(135deg, #eb5d1e 0%, #f4823a 100%)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#ffffff',
                fontSize: '11px',
                fontWeight: 700,
                fontFamily: 'var(--font-mono)',
                textTransform: 'uppercase',
                flexShrink: 0,
              }}
            >
              {avatarInitials ?? <Icon name="user" size={14} />}
            </div>
            <span
              style={{
                fontSize: '13px',
                fontWeight: 500,
                color: '#1A1714',
                maxWidth: '120px',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {userName ?? 'My Account'}
            </span>
            <Icon name="chevron-down" size={13} style={{ color: '#9A9693', flexShrink: 0 }} />
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
                style={{
                  position: 'absolute',
                  top: 'calc(100% + 6px)',
                  right: 0,
                  background: '#ffffff',
                  border: '1px solid rgba(0,0,0,0.10)',
                  borderRadius: '10px',
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
                    gap: '9px',
                    padding: '8px 10px',
                    borderRadius: '7px',
                    textDecoration: 'none',
                    color: '#1A1714',
                    fontSize: '13px',
                    transition: 'background 100ms ease',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,0,0,0.04)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="home" size={15} style={{ color: '#6B6866' }} />
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
                    gap: '9px',
                    padding: '8px 10px',
                    borderRadius: '7px',
                    background: 'transparent',
                    border: 'none',
                    color: '#dc2626',
                    fontSize: '13px',
                    cursor: 'pointer',
                    width: '100%',
                    textAlign: 'left',
                    transition: 'background 100ms ease',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(220,38,38,0.06)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="log-out" size={15} style={{ color: '#dc2626' }} />
                  Sign out
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </header>
  )
}
