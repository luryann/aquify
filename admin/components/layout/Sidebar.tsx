'use client'

import { usePathname } from 'next/navigation'
import Link from 'next/link'
import { useMemo, useState, useRef, useCallback, useEffect } from 'react'
import { filterNavItems, groupNavItems, NAV_SECTIONS, ROLE_LABELS, type Role } from '@/lib/roles'
import { Icon, type IconName } from '@/components/ui/icons'
import { pendingApprovalsRequestInit, shouldFetchPendingApprovals } from '@/lib/sidebar-badge'
import './Sidebar.inline.css'

type SidebarProps = {
  role: Role
  roles: Role[]
  userName?: string
  onSignOut?: () => void
  orgId?: string
  token?: string
}

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

    const getJson = <T,>(path: string): Promise<T | null> =>
      fetch(path, { ...pendingApprovalsRequestInit(orgId, token), signal: controller.signal }).then((r) => (r.ok ? (r.json() as Promise<T>) : null))

    Promise.all([
      getJson<{ count?: number; users?: unknown[] }>('/api/v1/users?status=pending_approval'),
      getJson<{ requests?: unknown[] }>('/api/v1/users/swimmer-link-requests'),
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
          <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', minWidth: 0, overflow: 'hidden' }}>
            <span
              className="u-ff-font-display u-fw-display-weight u-fs-fs-xl u-ls-display-tracking u-lh-1" style={{ color: 'var(--text)', userSelect: 'none', whiteSpace: 'nowrap' }}
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
          type="button"
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
        className="u-p-8px-0" style={{ flex: 1, overflowY: 'auto', overflowX: 'hidden' }}
        aria-label="Main navigation"
      >
        {NAV_SECTIONS.filter((s) => grouped[s]?.length).map((section) => (
          <div
            key={section}
            className="u-mb-2px" 
          >
            {!collapsed && (
              <div
                className="u-fs-fs-xs u-ff-font-body u-fw-600" style={{ padding: '12px 20px 4px', color: 'var(--text-subtle)', whiteSpace: 'nowrap', overflow: 'hidden' }}
              >
                {section}
              </div>
            )}

            {grouped[section]!.map((item) => {
              const active = isActive(item.path)
              return (
                <Link
                  key={item.id}
                  href={item.path}
                  title={collapsed ? item.label : undefined}
                  className="sidebar-link"
                  data-active={active}
                  data-collapsed={collapsed}
                >
                  <Icon
                    name={item.icon as IconName}
                    size={15}
                    style={{ flexShrink: 0, opacity: active ? 1 : 0.7 }}
                  />
                  {!collapsed && (
                    <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', flex: 1 }}>
                      {item.label}
                    </span>
                  )}
                  {!collapsed && item.id === 'approvals' && pendingCount > 0 && (
                    <span className="u-fs-fs-2xs u-fw-700 u-ff-font-body u-p-0-4px u-lh-1" style={{ flexShrink: 0, minWidth: '18px', height: '18px', borderRadius: '8px', background: 'var(--accent)', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                      {pendingCount > 99 ? '99+' : pendingCount}
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
        className="u-gap-8px" style={{ borderTop: '1px solid var(--border)', padding: collapsed ? '12px 0' : '12px 16px', display: 'flex', alignItems: 'center', flexShrink: 0, justifyContent: collapsed ? 'center' : 'flex-start', overflow: 'hidden' }}
      >
        <div
          className="u-fs-fs-xs u-fw-700 u-ff-font-body" style={{ width: '28px', height: '28px', borderRadius: '50%', background: 'var(--accent-subtle)', border: '1px solid var(--accent-border)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, color: 'var(--accent-text, var(--accent))', textTransform: 'uppercase' }}
        >
          {userName ? userName.slice(0, 2) : <Icon name="user" size={14} />}
        </div>

        {!collapsed && (
          <>
            <div style={{ flex: 1, minWidth: 0 }}>
              <p
                className="truncate u-fs-fs-sm u-fw-600"
                style={{ color: 'var(--text)', lineHeight: 1.3 }}
              >
                {userName ?? 'User'}
              </p>
              <p className="u-fs-fs-2xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
                {ROLE_LABELS[role]}
              </p>
            </div>

            {onSignOut && (
              <button
                type="button"
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
          className="sidebar-resize"
          data-dragging={isDragging}
          title="Drag to resize sidebar"
        />
      )}
    </aside>
  )
}
