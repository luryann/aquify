'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

type SwimmerNavProps = {
  swimmerName?: string
  initials?: string
  onSignOut?: () => void
}

const NAV_LINKS = [
  { label: 'Dashboard',   path: '/swim' },
  { label: 'Performance', path: '/swim/performance' },
  { label: 'Schedule',    path: '/swim/schedule' },
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

  const isActive = (path: string) =>
    path === '/swim' ? pathname === '/swim' : pathname.startsWith(path)

  return (
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
          href="/swim"
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            textDecoration: 'none',
            marginRight: '36px',
            flexShrink: 0,
          }}
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
              fontFamily: 'var(--font-display)',
              fontSize: '19px',
              color: C.text,
              letterSpacing: '-0.01em',
              lineHeight: 1,
              userSelect: 'none',
            }}
          >
            aquify
          </span>
          <span
            style={{
              fontSize: '9.5px',
              fontFamily: 'var(--font-body)',
              color: C.accent,
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              paddingTop: '2px',
              opacity: 0.85,
            }}
          >
            athlete
          </span>
        </Link>

        {/* Nav links */}
        <nav
          style={{ display: 'flex', alignItems: 'center', gap: '2px', flex: 1 }}
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
                  borderRadius: '7px',
                  fontSize: '13.5px',
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

        {/* User area */}
        <div style={{ position: 'relative', flexShrink: 0 }}>
          <button
            onClick={() => setMenuOpen((v) => !v)}
            aria-label="Athlete menu"
            aria-expanded={menuOpen}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '4px 10px 4px 4px',
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
              style={{
                width: '28px',
                height: '28px',
                borderRadius: '50%',
                background: 'linear-gradient(135deg, var(--accent) 0%, var(--accent-2) 100%)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'var(--text-on-accent)',
                fontSize: '11px',
                fontWeight: 700,
                fontFamily: 'var(--font-body)',
                flexShrink: 0,
              }}
            >
              {initials ?? <Icon name="user" size={14} />}
            </div>
            <span
              style={{
                fontSize: '13px',
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
                  borderRadius: '10px',
                  boxShadow: '0 8px 32px rgba(0,0,0,0.40)',
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
                    gap: '9px',
                    padding: '8px 10px',
                    borderRadius: '7px',
                    textDecoration: 'none',
                    color: C.text,
                    fontSize: '13px',
                    transition: 'background 100ms ease',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(0,212,170,0.08)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <Icon name="user" size={14} style={{ color: C.subtle }} />
                  My Profile
                </Link>
                <div style={{ height: '1px', background: C.border, margin: '4px 0' }} />
                <button
                  onClick={() => { setMenuOpen(false); onSignOut?.() }}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '9px',
                    padding: '8px 10px',
                    borderRadius: '7px',
                    background: 'transparent',
                    border: 'none',
                    color: 'var(--error)',
                    fontSize: '13px',
                    cursor: 'pointer',
                    width: '100%',
                    textAlign: 'left',
                    transition: 'background 100ms ease',
                  }}
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
  )
}
