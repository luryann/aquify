'use client'

import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { useMemo, useState, useRef, useCallback, useEffect, type CSSProperties } from 'react'
import { filterNavItems, groupNavItems, ROLE_LABELS, type Role } from '@/lib/roles'
import { Icon, type IconName } from '@/components/ui/icons'
import { pendingApprovalsRequestInit, shouldFetchPendingApprovals } from '@/lib/sidebar-badge'

type SidebarProps = {
  role: Role
  roles: Role[]
  userName?: string
  onSignOut?: () => void
  orgId?: string
  token?: string
}

const SECTION_ORDER = [
  'Overview',
  'Members',
  'Competition',
  'Training',
  'Performance',
  'Billing',
  'Communications',
  'Compliance',
  'System',
  'Advanced',
]

const MIN_WIDTH = 160
const MAX_WIDTH = 380
const DEFAULT_WIDTH = 224
const COLLAPSED_WIDTH = 52

function loadStoredWidth(): number {
  if (typeof window === 'undefined') return DEFAULT_WIDTH
  try {
    const v = localStorage.getItem('aquify-sidebar-width')
    if (v) {
      const n = parseInt(v, 10)
      if (n >= MIN_WIDTH && n <= MAX_WIDTH) return n
    }
  } catch { /* ignore */ }
  return DEFAULT_WIDTH
}

export function Sidebar({ role, roles, userName, onSignOut, orgId, token }: SidebarProps) {
  const pathname = usePathname()
  const [collapsed, setCollapsed] = useState(false)
  const [sidebarWidth, setSidebarWidth] = useState(DEFAULT_WIDTH)
  const [isDragging, setIsDragging] = useState(false)
  const [pendingCount, setPendingCount] = useState(0)

  const isResizing = useRef(false)
  const startX = useRef(0)
  const startWidth = useRef(0)
  const currentWidthRef = useRef(sidebarWidth)

  // Load stored width on mount (client-only)
  useEffect(() => {
    const w = loadStoredWidth()
    setSidebarWidth(w)
    currentWidthRef.current = w
  }, [])

  // Sync CSS variable whenever width or collapsed state changes
  useEffect(() => {
    const w = collapsed ? COLLAPSED_WIDTH : sidebarWidth
    document.documentElement.style.setProperty('--sidebar-w', `${w}px`)
  }, [sidebarWidth, collapsed])

  // Fetch pending approvals count, plus pending swimmer-link requests
  // (admin only) — both surface on the same "Approvals" nav badge.
  useEffect(() => {
    if (!shouldFetchPendingApprovals(roles, orgId)) return
    const controller = new AbortController()

    Promise.all([
      fetch('/api/v1/users?status=pending_approval', {
        ...pendingApprovalsRequestInit(orgId, token),
        signal: controller.signal,
      }).then((r) => (r.ok ? r.json() : null)) as Promise<{ count?: number; users?: unknown[] } | null>,
      fetch('/api/v1/users/swimmer-link-requests', {
        ...pendingApprovalsRequestInit(orgId, token),
        signal: controller.signal,
      }).then((r) => (r.ok ? r.json() : null)) as Promise<{ requests?: unknown[] } | null>,
    ])
      .then(([approvals, linkRequests]) => {
        const approvalsCount = approvals
          ? (typeof approvals.count === 'number' ? approvals.count : (approvals.users?.length ?? 0))
          : 0
        const linkRequestsCount = linkRequests?.requests?.length ?? 0
        setPendingCount(approvalsCount + linkRequestsCount)
      })
      .catch(() => { /* silent */ })
    return () => controller.abort()
  }, [orgId, roles, token])

  const grouped = useMemo(() => {
    const items = filterNavItems(roles)
    return groupNavItems(items)
  }, [roles])

  const isActive = (path: string) => pathname === path || pathname.startsWith(path + '/')

  const handleResizeMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    isResizing.current = true
    startX.current = e.clientX
    startWidth.current = currentWidthRef.current
    setIsDragging(true)
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'

    const onMove = (ev: MouseEvent) => {
      if (!isResizing.current) return
      const delta = ev.clientX - startX.current
      const newWidth = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, startWidth.current + delta))
      currentWidthRef.current = newWidth
      setSidebarWidth(newWidth)
    }

    const onUp = () => {
      isResizing.current = false
      setIsDragging(false)
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      try {
        localStorage.setItem('aquify-sidebar-width', String(currentWidthRef.current))
      } catch { /* ignore */ }
    }

    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }, [])

  const effectiveWidth = collapsed ? COLLAPSED_WIDTH : sidebarWidth

  return (
    <aside
      style={{
        width: `${effectiveWidth}px`,
        height: '100vh',
        background: 'var(--bg-sidebar)',
        borderRight: '1px solid var(--border)',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        transition: isDragging ? 'none' : 'width 250ms var(--ease-out)',
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
          padding: collapsed ? '0 12px' : '0 16px 0 20px',
          height: 'var(--topbar-h)',
          borderBottom: '1px solid var(--border)',
          flexShrink: 0,
        }}
      >
        {!collapsed && (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0, overflow: 'hidden' }}>
            <span
              style={{
                fontFamily: 'var(--font-display)',
                fontWeight: 'var(--display-weight)',
                fontSize: 'var(--fs-xl)',
                color: 'var(--text)',
                letterSpacing: 'var(--display-tracking)',
                lineHeight: 1,
                userSelect: 'none',
                whiteSpace: 'nowrap',
              }}
            >
              aquify
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
          style={{ marginLeft: collapsed ? 'auto' : '0', marginRight: collapsed ? 'auto' : '0', flexShrink: 0 }}
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
          padding: '8px 0',
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
            {!collapsed && (
              <div
                style={{
                  padding: '12px 20px 4px',
                  fontSize: 'var(--fs-2xs)',
                  fontFamily: 'var(--font-body)',
                  fontWeight: 500,
                  color: 'var(--text-muted)',
                  whiteSpace: 'nowrap',
                  overflow: 'hidden',
                }}
              >
                {section}
              </div>
            )}

            {grouped[section]!.map((item, itemIdx) => {
              const active = isActive(item.path)
              const baseStyle: CSSProperties = {
                display: 'flex',
                alignItems: 'center',
                gap: '8px',
                padding: collapsed ? '8px 0' : '8px 16px 8px 20px',
                justifyContent: collapsed ? 'center' : 'flex-start',
                textDecoration: 'none',
                color: active ? 'var(--text)' : 'var(--text-subtle)',
                background: active ? 'var(--accent-subtle)' : 'transparent',
                borderLeft: active ? '2px solid var(--accent)' : '2px solid transparent',
                borderRadius: collapsed ? '0' : '0 var(--r-sm) var(--r-sm) 0',
                marginRight: collapsed ? '0' : '8px',
                fontSize: 'var(--fs-base)',
                fontWeight: active ? 600 : 400,
                transition:
                  'background var(--dur-fast), color var(--dur-fast), border-color var(--dur-fast)',
                animation: `slideUp 0.3s var(--ease-out) ${(sectionIdx * 3 + itemIdx) * 30 + 50}ms both`,
                whiteSpace: 'nowrap',
                overflow: 'hidden',
              }

              const iconAndLabel = (
                <>
                  <Icon
                    name={item.icon as IconName}
                    size={15}
                    style={{ flexShrink: 0, opacity: active ? 1 : item.locked ? 0.4 : 0.7 }}
                  />
                  {!collapsed && (
                    <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', flex: 1 }}>
                      {item.label}
                    </span>
                  )}
                  {!collapsed && item.id === 'approvals' && pendingCount > 0 && (
                    <span style={{
                      flexShrink: 0,
                      minWidth: '18px',
                      height: '18px',
                      borderRadius: '8px',
                      background: 'var(--accent)',
                      color: '#fff',
                      fontSize: 'var(--fs-2xs)',
                      fontWeight: 700,
                      fontFamily: 'var(--font-body)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      padding: '0 4px',
                      lineHeight: 1,
                    }}>
                      {pendingCount > 99 ? '99+' : pendingCount}
                    </span>
                  )}
                </>
              )

              if (item.locked) {
                return (
                  <div
                    key={item.id}
                    title={collapsed ? `${item.label} — requires a higher permission level` : 'Requires a higher permission level'}
                    aria-disabled="true"
                    style={{
                      ...baseStyle,
                      color: 'var(--text-muted)',
                      opacity: 0.45,
                      cursor: 'not-allowed',
                      pointerEvents: 'auto',
                    }}
                  >
                    {iconAndLabel}
                  </div>
                )
              }

              return (
                <Link
                  key={item.id}
                  href={item.path}
                  title={collapsed ? item.label : undefined}
                  style={baseStyle}
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
                  {iconAndLabel}
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
          padding: collapsed ? '12px 0' : '12px 16px',
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          flexShrink: 0,
          justifyContent: collapsed ? 'center' : 'flex-start',
          overflow: 'hidden',
        }}
      >
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
            fontSize: 'var(--fs-xs)',
            fontWeight: 700,
            fontFamily: 'var(--font-body)',
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
                style={{ fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.3 }}
              >
                {userName ?? 'User'}
              </p>
              <p style={{ fontSize: 'var(--fs-2xs)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
                {ROLE_LABELS[role]}
              </p>
            </div>

            {onSignOut && (
              <button
                onClick={onSignOut}
                className="btn-icon"
                title="Sign out"
                aria-label="Sign out"
                style={{ flexShrink: 0 }}
              >
                <Icon name="log-out" size={14} />
              </button>
            )}
          </>
        )}
      </div>

      {/* Resize handle — only visible when not collapsed */}
      {!collapsed && (
        <div
          onMouseDown={handleResizeMouseDown}
          style={{
            position: 'absolute',
            top: 0,
            right: 0,
            width: '4px',
            height: '100%',
            cursor: 'col-resize',
            zIndex: 20,
            background: isDragging ? 'var(--accent)' : 'transparent',
            transition: 'background 150ms ease',
          }}
          onMouseEnter={(e) => {
            if (!isDragging) e.currentTarget.style.background = 'var(--accent-border)'
          }}
          onMouseLeave={(e) => {
            if (!isDragging) e.currentTarget.style.background = 'transparent'
          }}
          title="Drag to resize sidebar"
        />
      )}
    </aside>
  )
}
