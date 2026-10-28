'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useState } from 'react'
import { Icon } from '@/components/ui/icons'
import './SwimmerNav.inline.css'

type SwimmerNavProps = {
  swimmerName?: string
  initials?: string
  onSignOut?: () => void
}

const NAV_LINKS = [
  { label: 'Dashboard',   path: '/swim' },
  { label: 'Messages',    path: '/swim/messages' },
  { label: 'Performance', path: '/swim/performance' },
  { label: 'Records',     path: '/swim/records' },
  { label: 'Schedule',    path: '/swim/schedule' },
  { label: 'Check-in',    path: '/swim/wellness' },
  { label: 'Profile',     path: '/swim/profile' },
]

// Swimmer portal colours — resolved from the [data-portal="swim"] token overrides in globals.css
const C = {
  bg:        'var(--bg)',
  surface:   'var(--bg-surface)',
  accent:    'var(--accent)',
  text:      'var(--text)',
  subtle:    'var(--text-subtle)',
  border:    'var(--border)',
  borderHov: 'var(--accent-border)',
}

export function SwimmerNav({ swimmerName, initials, onSignOut }: SwimmerNavProps) {
  const pathname = usePathname()
  const [menuOpen, setMenuOpen] = useState(false)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  const isActive = (path: string) =>
    path === '/swim' ? pathname === '/swim' : pathname.startsWith(path)

  return (
    <>
    <header
      style={{
        position: 'sticky',
        top: 0,
        zIndex: 100,
        background: 'rgba(6,22,35,0.90)',
        backdropFilter: 'blur(14px)',
        WebkitBackdropFilter: 'blur(14px)',
        borderBottom: `1px solid ${C.border}`,
      }}
    >
      <div
        className="portal-frame is-a98e6ad"
        
      >
        {/* Wordmark */}
        <Link
          href="/swim"
          className="is-e285dd0"
        >
          {/* Teal wave icon */}
          <svg width="22" height="14" viewBox="0 0 22 14" fill="none">
            <path
              d="M1 7C3 4 5 1 8 1C11 1 11 7 14 7C17 7 17 1 20 1"
              stroke={C.accent}
              strokeWidth="2"
              strokeLinecap="round"
              fill="none"
            />
            <path
              d="M1 12C3 9 5 6 8 6C11 6 11 12 14 12C17 12 17 6 20 6"
              stroke={C.accent}
              strokeWidth="1.5"
              strokeLinecap="round"
              fill="none"
              opacity="0.4"
            />
          </svg>
          <span
            style={{
              fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)',
              fontSize: 'var(--fs-xl)',
              color: C.text,
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
          className="desktop-nav-links is-c7a0c1e"
          
          aria-label="Athlete navigation"
        >
          {NAV_LINKS.map((link) => {
            const active = isActive(link.path)
            return (
              <Link
                key={link.path}
                href={link.path}
                style={{
                  padding: '6px 12px',
                  borderRadius: '8px',
                  fontSize: 'var(--fs-base)',
                  fontWeight: active ? 600 : 400,
                  color: active ? C.accent : C.subtle,
                  background: active ? 'rgba(0,212,170,0.10)' : 'transparent',
                  textDecoration: 'none',
                  transition: 'background 120ms ease, color 120ms ease',
                  whiteSpace: 'nowrap',
                }}
                onMouseEnter={(e) => {
                  if (!active) {
                    e.currentTarget.style.background = 'rgba(0,212,170,0.06)'
                    e.currentTarget.style.color = C.text
                  }
                }}
                onMouseLeave={(e) => {
                  if (!active) {
                    e.currentTarget.style.background = 'transparent'
                    e.currentTarget.style.color = C.subtle
                  }
                }}
              >
                {link.label}
              </Link>
            )
          })}
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
            onClick={() => setMenuOpen((v) => !v)}
            aria-label="Athlete menu"
            aria-expanded={menuOpen}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '4px 12px 4px 4px',
              background: 'transparent',
              border: `1px solid ${C.border}`,
              borderRadius: '999px',
              cursor: 'pointer',
              transition: 'border-color 120ms ease, background 120ms ease',
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.background = 'rgba(0,212,170,0.06)'
              e.currentTarget.style.borderColor = C.borderHov
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.background = 'transparent'
              e.currentTarget.style.borderColor = C.border
            }}
          >
            {/* Avatar */}
            <div
              className="is-36100be"
            >
              {initials ?? <Icon name="user" size={14} />}
            </div>
            <span
              style={{
                fontSize: 'var(--fs-base)',
                fontWeight: 500,
                color: C.text,
                maxWidth: '120px',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {swimmerName ?? 'Athlete'}
            </span>
            <Icon name="chevron-down" size={13} style={{ color: C.subtle, flexShrink: 0 }} />
          </button>

          {/* Dropdown */}
          {menuOpen && (
            <>
              <div
                style={{ position: 'fixed', inset: 0, zIndex: 99 }}
                onClick={() => setMenuOpen(false)}
              />
              <div
                style={{
                  position: 'absolute',
                  top: 'calc(100% + 6px)',
                  right: 0,
                  background: C.surface,
                  border: `1px solid ${C.border}`,
                  borderRadius: '12px',
                  boxShadow: 'var(--shadow-sm)',
                  minWidth: '180px',
                  padding: '6px',
                  zIndex: 100,
                  animation: 'slideUp 0.18s ease both',
                }}
              >
                <Link
                  href="/swim/profile"
                  onClick={() => setMenuOpen(false)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '8px',
                    padding: '8px 12px',
                    borderRadius: '8px',
                    textDecoration: 'none',
                    color: C.text,
                    fontSize: 'var(--fs-base)',
                    transition: 'background 100ms ease',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,212,170,0.08)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="user" size={14} style={{ color: C.subtle }} />
                  My Profile
                </Link>
                <div style={{ height: '1px', background: C.border, margin: '4px 0' }} />
                <button type="button"
                  onClick={() => { setMenuOpen(false); onSignOut?.() }}
                  className="is-31cee2f"
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--error-bg)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="log-out" size={14} style={{ color: 'var(--error)' }} />
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
          style={{
            width: 'min(280px, 84vw)',
            background: C.surface,
            borderRight: `1px solid ${C.border}`,
            flexDirection: 'column',
            padding: '20px 12px',
          }}
        >
          <div className="is-40001e4">
            <span style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: C.text }}>aquify</span>
            <button type="button"
              className="btn-icon"
              onClick={() => setMobileNavOpen(false)}
              aria-label="Close navigation menu"
            >
              <Icon name="x" size={16} />
            </button>
          </div>
          <nav className="is-32bee05" aria-label="Athlete navigation">
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
                    fontWeight: active ? 600 : 400,
                    color: active ? C.accent : C.subtle,
                    background: active ? 'rgba(0,212,170,0.10)' : 'transparent',
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
