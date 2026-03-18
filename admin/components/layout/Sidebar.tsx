'use client'

import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { useMemo, useState } from 'react'
import { filterNavItems, groupNavItems, ROLE_LABELS, type Role } from '@/lib/roles'
import { Icon, type IconName } from '@/components/ui/icons'

type SidebarProps = {
  role: Role
  userName?: string
  onSignOut?: () => void
}

const SECTION_ORDER = [
  'Overview',
  'Members',
  'Competition',
  'Performance',
  'Files',
  'Billing',
  'Communications',
  'Compliance',
  'System',
]

export function Sidebar({ role, userName, onSignOut }: SidebarProps) {
  const pathname = usePathname()
  const [collapsed, setCollapsed] = useState(false)

  const grouped = useMemo(() => {
    const items = filterNavItems(role)
    return groupNavItems(items)
  }, [role])

  const isActive = (path: string) => pathname === path || pathname.startsWith(path + '/')

  return (
    <aside
      style={{
        width: collapsed ? '52px' : 'var(--sidebar-w)',
        height: '100vh',
        background: 'var(--bg-sidebar)',
        borderRight: '1px solid var(--border)',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        transition: 'width 250ms var(--ease-out)',
        flexShrink: 0,
        position: 'relative',
        zIndex: 10,
      }}
    >
      {/* Brand */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: collapsed ? 'center' : 'space-between',
          padding: collapsed ? '0 12px' : '0 14px 0 18px',
          height: 'var(--topbar-h)',
          borderBottom: '1px solid var(--border)',
          flexShrink: 0,
        }}
      >
        {!collapsed && (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            {/* Accent dot */}
            <div
              style={{
                width: '7px',
                height: '7px',
                borderRadius: '50%',
                background: 'var(--accent)',
                boxShadow: '0 0 8px var(--accent)',
                flexShrink: 0,
              }}
            />
            {/* Wordmark */}
            <span
              style={{
                fontFamily: 'var(--font-display)',
                fontStyle: 'bold',
                fontSize: '18px',
                color: 'var(--text)',
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
                color: 'var(--text-muted)',
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                paddingTop: '2px',
              }}
            >
              admin
            </span>
          </div>
        )}

        {collapsed && (
          <div
            style={{
              width: '7px',
              height: '7px',
              borderRadius: '50%',
              background: 'var(--accent)',
              boxShadow: '0 0 8px var(--accent)',
            }}
          />
        )}

        <button
          onClick={() => setCollapsed((c) => !c)}
          className="btn-icon"
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          style={{ marginLeft: collapsed ? 'auto' : '0', marginRight: collapsed ? 'auto' : '0' }}
        >
          <Icon name={collapsed ? 'chevron-right' : 'chevron-left'} size={14} />
        </button>
      </div>

      {/* Nav */}
      <nav
        style={{
          flex: 1,
          overflowY: 'auto',
          overflowX: 'hidden',
          padding: collapsed ? '8px 0' : '8px 0',
        }}
        aria-label="Main navigation"
      >
        {SECTION_ORDER.filter((s) => grouped[s]?.length).map((section, sectionIdx) => (
          <div
            key={section}
            style={{
              marginBottom: '2px',
              animation: `slideUp 0.35s var(--ease-out) ${sectionIdx * 40}ms both`,
            }}
          >
            {/* Section label */}
            {!collapsed && (
              <div
                style={{
                  padding: '10px 18px 4px',
                  fontSize: '10px',
                  fontFamily: 'var(--font-mono)',
                  fontWeight: 500,
                  color: 'var(--text-muted)',
                  letterSpacing: '0.10em',
                  textTransform: 'uppercase',
                }}
              >
                {section}
              </div>
            )}

            {/* Items */}
            {grouped[section]!.map((item, itemIdx) => {
              const active = isActive(item.path)
              return (
                <Link
                  key={item.id}
                  href={item.path}
                  title={collapsed ? item.label : undefined}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '9px',
                    padding: collapsed ? '8px 0' : '7px 14px 7px 18px',
                    justifyContent: collapsed ? 'center' : 'flex-start',
                    textDecoration: 'none',
                    color: active ? 'var(--text)' : 'var(--text-subtle)',
                    background: active ? 'var(--accent-subtle)' : 'transparent',
                    borderLeft: active ? '2px solid var(--accent)' : '2px solid transparent',
                    borderRadius: collapsed ? '0' : '0 var(--r-sm) var(--r-sm) 0',
                    marginRight: collapsed ? '0' : '8px',
                    fontSize: '13px',
                    fontWeight: active ? 600 : 400,
                    transition:
                      'background var(--dur-fast), color var(--dur-fast), border-color var(--dur-fast)',
                    animation: `slideUp 0.3s var(--ease-out) ${(sectionIdx * 3 + itemIdx) * 30 + 50}ms both`,
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                  }}
                  onMouseEnter={(e) => {
                    if (!active) {
                      e.currentTarget.style.background = 'rgba(255,255,255,0.04)'
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
                  <Icon
                    name={item.icon as IconName}
                    size={15}
                    style={{ flexShrink: 0, opacity: active ? 1 : 0.7 }}
                  />
                  {!collapsed && (
                    <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {item.label}
                    </span>
                  )}
                </Link>
              )
            })}
          </div>
        ))}
      </nav>

      {/* Bottom user strip */}
      <div
        style={{
          borderTop: '1px solid var(--border)',
          padding: collapsed ? '10px 0' : '10px 14px',
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          flexShrink: 0,
          justifyContent: collapsed ? 'center' : 'flex-start',
        }}
      >
        {/* Avatar */}
        <div
          style={{
            width: '28px',
            height: '28px',
            borderRadius: '50%',
            background: 'var(--accent-subtle)',
            border: '1px solid var(--accent-border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
            color: 'var(--accent)',
            fontSize: '11px',
            fontWeight: 700,
            fontFamily: 'var(--font-mono)',
            textTransform: 'uppercase',
          }}
        >
          {userName ? userName.slice(0, 2) : <Icon name="user" size={14} />}
        </div>

        {!collapsed && (
          <>
            <div style={{ flex: 1, minWidth: 0 }}>
              <p
                className="truncate"
                style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)', lineHeight: 1.3 }}
              >
                {userName ?? 'User'}
              </p>
              <p style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                {ROLE_LABELS[role]}
              </p>
            </div>

            {onSignOut && (
              <button
                onClick={onSignOut}
                className="btn-icon"
                title="Sign out"
                aria-label="Sign out"
              >
                <Icon name="log-out" size={14} />
              </button>
            )}
          </>
        )}
      </div>
    </aside>
  )
}
