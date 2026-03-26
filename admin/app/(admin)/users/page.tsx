'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import {
  fetchUsers,
  inviteUser,
  createUser,
  type UserRecord,
} from '@/lib/adminData'
import { ROLE_HIERARCHY, type Role } from '@/lib/roles'

const ROLE_OPTIONS: { value: string; label: string }[] = [
  { value: 'owner', label: 'Owner' },
  { value: 'admin', label: 'Admin' },
  { value: 'coach', label: 'Coach' },
  { value: 'billing', label: 'Billing' },
  { value: 'parent', label: 'Parent' },
  { value: 'swimmer', label: 'Swimmer' },
  { value: 'read_only_staff', label: 'Read-Only' },
]

const ROLE_COLORS: Record<string, { bg: string; color: string }> = {
  owner:           { bg: 'rgba(235,93,30,0.15)',   color: '#eb5d1e' },
  admin:           { bg: 'rgba(251,191,36,0.12)',   color: '#fbbf24' },
  coach:           { bg: 'rgba(74,222,128,0.10)',   color: '#4ade80' },
  billing:         { bg: 'rgba(96,165,250,0.10)',   color: '#60a5fa' },
  parent:          { bg: 'rgba(196,148,255,0.10)',  color: '#c484ff' },
  swimmer:         { bg: 'rgba(248,113,113,0.10)',  color: '#f87171' },
  read_only_staff: { bg: 'rgba(155,155,155,0.10)',  color: '#9a9693' },
  'read-only':     { bg: 'rgba(155,155,155,0.10)',  color: '#9a9693' },
}

function RoleBadge({ role }: { role: string }) {
  const c = ROLE_COLORS[role] ?? { bg: 'var(--bg-elevated)', color: 'var(--text-subtle)' }
  const label = ROLE_OPTIONS.find((r) => r.value === role)?.label ?? role
  return (
    <span style={{
      display: 'inline-block',
      fontSize: '10px',
      fontWeight: 700,
      letterSpacing: '0.04em',
      textTransform: 'uppercase',
      padding: '2px 7px',
      borderRadius: '5px',
      background: c.bg,
      color: c.color,
      fontFamily: 'var(--font-mono)',
    }}>{label}</span>
  )
}

function StatusPill({ enabled }: { enabled: boolean }) {
  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: '5px',
      fontSize: '11px',
      fontWeight: 600,
      padding: '3px 9px',
      borderRadius: '20px',
      background: enabled ? 'var(--success-bg)' : 'var(--bg-elevated)',
      color: enabled ? 'var(--success)' : 'var(--text-muted)',
      fontFamily: 'var(--font-mono)',
    }}>
      <span style={{
        width: '5px', height: '5px', borderRadius: '50%',
        background: enabled ? 'var(--success)' : 'var(--text-disabled)',
        flexShrink: 0,
      }} />
      {enabled ? 'Active' : 'Disabled'}
    </span>
  )
}

const INPUT_STYLE: React.CSSProperties = {
  width: '100%',
  padding: '9px 12px',
  background: 'var(--bg-elevated)',
  border: '1px solid var(--border)',
  borderRadius: 'var(--r-md)',
  fontSize: '13px',
  color: 'var(--text)',
  outline: 'none',
  boxSizing: 'border-box',
}
const LABEL_STYLE: React.CSSProperties = {
  fontSize: '11px',
  fontWeight: 600,
  color: 'var(--text-muted)',
  textTransform: 'uppercase',
  letterSpacing: '0.05em',
  display: 'block',
  marginBottom: '5px',
  fontFamily: 'var(--font-mono)',
}

export default function UsersPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)

  const [users, setUsers] = useState<UserRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [search, setSearch] = useState('')
  const [roleFilter, setRoleFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState('all')

  // Invite modal
  const [showInvite, setShowInvite] = useState(false)
  const [inviting, setInviting] = useState(false)
  const [inviteError, setInviteError] = useState<string | null>(null)
  const [inviteSuccess, setInviteSuccess] = useState<string | null>(null)
  const [inviteForm, setInviteForm] = useState({
    first_name: '',
    last_name: '',
    email: '',
    role: 'parent',
    create_household: false,
  })

  // Create user modal (admin sets password directly)
  const [showCreate, setShowCreate] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [createSuccess, setCreateSuccess] = useState<string | null>(null)
  const [showCreatePwd, setShowCreatePwd] = useState(false)
  const [createForm, setCreateForm] = useState({
    first_name: '',
    last_name: '',
    email: '',
    password: '',
    role: 'parent',
    temporary_password: false,
    create_household: false,
  })

  const actorRole = session?.user.role as Role | undefined
  const actorTier = actorRole ? ROLE_HIERARCHY[actorRole] : 0

  // Roles actor can assign (at or below their tier)
  const assignableRoles = ROLE_OPTIONS.filter((r) => {
    const tier = ROLE_HIERARCHY[r.value as Role] ?? 1
    return tier <= actorTier
  })

  function loadUsers(roleOverride?: string) {
    if (!session?.user.orgId) return
    const role = (roleOverride ?? roleFilter) !== 'all' ? (roleOverride ?? roleFilter) : undefined
    setLoading(true)
    setError(null)
    fetchUsers(api, session.user.orgId, { limit: 200, role })
      .then(({ users: fetched }) => setUsers(fetched))
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load users.'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    loadUsers()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session?.user.orgId, roleFilter])

  const filtered = useMemo(() => {
    return users.filter((u) => {
      const name = `${u.firstName} ${u.lastName}`.toLowerCase()
      const q = search.toLowerCase()
      const matchSearch = !search || name.includes(q) || u.email.toLowerCase().includes(q)
      const matchStatus =
        statusFilter === 'all' ||
        (statusFilter === 'active' && u.enabled) ||
        (statusFilter === 'disabled' && !u.enabled)
      return matchSearch && matchStatus
    })
  }, [users, search, statusFilter])

  const stats = useMemo(() => ({
    total: users.length,
    active: users.filter((u) => u.enabled).length,
    disabled: users.filter((u) => !u.enabled).length,
  }), [users])

  async function handleInvite(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    setInviting(true)
    setInviteError(null)
    setInviteSuccess(null)
    try {
      await inviteUser(api, session.user.orgId, inviteForm)
      setInviteSuccess(`Invitation sent to ${inviteForm.email}. A welcome email has been dispatched.`)
      setInviteForm({ first_name: '', last_name: '', email: '', role: 'parent', create_household: false })
      loadUsers()
    } catch (err) {
      setInviteError(err instanceof ApiError ? err.message : 'Failed to send invitation.')
    } finally {
      setInviting(false)
    }
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    setCreating(true)
    setCreateError(null)
    setCreateSuccess(null)
    try {
      await createUser(api, session.user.orgId, createForm)
      setCreateSuccess(`Account created for ${createForm.email}.${createForm.temporary_password ? ' User must change password on first login.' : ''}`)
      setCreateForm({ first_name: '', last_name: '', email: '', password: '', role: 'parent', temporary_password: false, create_household: false })
      loadUsers()
    } catch (err) {
      setCreateError(err instanceof ApiError ? err.message : 'Failed to create user.')
    } finally {
      setCreating(false)
    }
  }

  const selectStyle: React.CSSProperties = {
    padding: '8px 12px',
    background: 'var(--bg-elevated)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--r-md)',
    fontSize: '13px',
    color: 'var(--text)',
    outline: 'none',
    cursor: 'pointer',
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1100px' }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '24px' }}>
        <div>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>
            Users
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
            Manage Keycloak identities, roles, and account status for this organization.
          </p>
        </div>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button
            className="btn"
            style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
            onClick={() => { setShowInvite(true); setInviteError(null); setInviteSuccess(null) }}
          >
            <Icon name="mail" size={14} />
            Invite User
          </button>
          <button
            className="btn btn-primary"
            style={{ display: 'flex', alignItems: 'center', gap: '6px' }}
            onClick={() => { setShowCreate(true); setCreateError(null); setCreateSuccess(null) }}
          >
            <Icon name="user-plus" size={14} />
            Create User
          </button>
        </div>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '20px' }}>
        {[
          { label: 'Total Users', value: stats.total, color: 'var(--text)' },
          { label: 'Active', value: stats.active, color: 'var(--success)' },
          { label: 'Disabled', value: stats.disabled, color: 'var(--text-muted)' },
        ].map((stat) => (
          <div key={stat.label} className="card" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', flexWrap: 'wrap', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: '200px', maxWidth: '320px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input
            type="text"
            placeholder="Search name or email…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }}
          />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Statuses</option>
          <option value="active">Active</option>
          <option value="disabled">Disabled</option>
        </select>
        <select value={roleFilter} onChange={(e) => setRoleFilter(e.target.value)} style={selectStyle}>
          <option value="all">All Roles</option>
          {ROLE_OPTIONS.map((r) => (
            <option key={r.value} value={r.value}>{r.label}</option>
          ))}
        </select>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>
          {filtered.length} of {users.length}
        </span>
      </div>

      {/* Table */}
      {loading ? (
        <div className="card" style={{ padding: '48px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
          <Icon name="loader" size={20} style={{ marginBottom: '10px', opacity: 0.5 }} />
          <div>Loading users…</div>
        </div>
      ) : error ? (
        <div className="card" style={{ padding: '32px', textAlign: 'center', color: 'var(--error)', fontSize: '13px' }}>
          <Icon name="alert-circle" size={18} style={{ marginBottom: '8px' }} />
          <div>{error}</div>
        </div>
      ) : filtered.length === 0 ? (
        <div className="card" style={{ padding: '48px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
          <Icon name="users" size={24} style={{ marginBottom: '10px', opacity: 0.3 }} />
          <div style={{ fontWeight: 500, marginBottom: '4px' }}>No users found</div>
          <div style={{ fontSize: '12px' }}>Adjust your filters or invite a new user.</div>
        </div>
      ) : (
        <div className="card" style={{ overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                {['User', 'Email', 'Roles', 'Status', 'Last Login', ''].map((col) => (
                  <th key={col} style={{
                    padding: '10px 16px',
                    fontSize: '10px',
                    fontWeight: 700,
                    color: 'var(--text-muted)',
                    textTransform: 'uppercase',
                    letterSpacing: '0.06em',
                    textAlign: 'left',
                    fontFamily: 'var(--font-mono)',
                    background: 'var(--bg-elevated)',
                  }}>{col}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filtered.map((user, i) => (
                <tr
                  key={user.id}
                  style={{
                    borderBottom: i < filtered.length - 1 ? '1px solid var(--border)' : 'none',
                    transition: 'background var(--dur-fast)',
                  }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--bg-elevated)')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = '')}
                >
                  <td style={{ padding: '14px 16px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                      <div style={{
                        width: '32px', height: '32px', borderRadius: '50%',
                        background: 'var(--accent-subtle)',
                        border: '1px solid var(--accent-border)',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        fontSize: '12px', fontWeight: 700, color: 'var(--accent)',
                        fontFamily: 'var(--font-mono)', flexShrink: 0,
                      }}>
                        {((user.firstName?.[0] ?? '') + (user.lastName?.[0] ?? '')).toUpperCase() || user.email[0].toUpperCase()}
                      </div>
                      <div>
                        <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>
                          {user.firstName || user.lastName
                            ? `${user.firstName} ${user.lastName}`.trim()
                            : <span style={{ color: 'var(--text-muted)' }}>No name</span>}
                        </div>
                        <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '1px' }}>
                          {user.id.slice(0, 8)}…
                        </div>
                      </div>
                    </div>
                  </td>
                  <td style={{ padding: '14px 16px', fontSize: '13px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>
                    {user.email}
                  </td>
                  <td style={{ padding: '14px 16px' }}>
                    {user.roles && user.roles.length > 0 ? (
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px' }}>
                        {user.roles.map((r) => <RoleBadge key={r} role={r} />)}
                      </div>
                    ) : (
                      <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>—</span>
                    )}
                  </td>
                  <td style={{ padding: '14px 16px' }}>
                    <StatusPill enabled={user.enabled} />
                  </td>
                  <td style={{ padding: '14px 16px', fontSize: '12px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>
                    {user.lastLoginAt
                      ? new Date(user.lastLoginAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
                      : <span style={{ color: 'var(--text-muted)' }}>—</span>}
                  </td>
                  <td style={{ padding: '14px 16px', textAlign: 'right' }}>
                    <Link
                      href={`/users/${user.id}`}
                      style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none', display: 'inline-flex', alignItems: 'center', gap: '3px', fontWeight: 500 }}
                    >
                      Manage <Icon name="chevron-right" size={12} />
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create User Modal */}
      {showCreate && (
        <div
          style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.65)' }}
          onClick={(e) => { if (e.target === e.currentTarget) setShowCreate(false) }}
        >
          <div style={{
            background: 'var(--bg-card)',
            border: '1px solid var(--border-strong)',
            borderRadius: '14px',
            padding: '28px',
            width: '100%',
            maxWidth: '480px',
            boxShadow: 'var(--shadow-lg)',
          }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <div>
                <h2 style={{ fontSize: '18px', fontFamily: 'var(--font-display)', color: 'var(--text)', margin: 0 }}>Create User</h2>
                <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '4px' }}>
                  Creates a Keycloak account with a directly-set password. No email sent.
                </p>
              </div>
              <button onClick={() => setShowCreate(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: '4px' }}>
                <Icon name="x" size={18} />
              </button>
            </div>

            {createSuccess && (
              <div style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>
                {createSuccess}
              </div>
            )}
            {createError && (
              <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>
                {createError}
              </div>
            )}

            <form onSubmit={handleCreate} style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
                <div>
                  <label style={LABEL_STYLE}>First Name</label>
                  <input style={INPUT_STYLE} value={createForm.first_name} onChange={(e) => setCreateForm((f) => ({ ...f, first_name: e.target.value }))} placeholder="Jane" />
                </div>
                <div>
                  <label style={LABEL_STYLE}>Last Name</label>
                  <input style={INPUT_STYLE} value={createForm.last_name} onChange={(e) => setCreateForm((f) => ({ ...f, last_name: e.target.value }))} placeholder="Smith" />
                </div>
              </div>
              <div>
                <label style={LABEL_STYLE}>Email *</label>
                <input required type="email" style={INPUT_STYLE} value={createForm.email} onChange={(e) => setCreateForm((f) => ({ ...f, email: e.target.value }))} placeholder="jane@example.com" />
              </div>
              <div>
                <label style={LABEL_STYLE}>Password *</label>
                <div style={{ position: 'relative' }}>
                  <input
                    required
                    type={showCreatePwd ? 'text' : 'password'}
                    style={{ ...INPUT_STYLE, paddingRight: '40px' }}
                    value={createForm.password}
                    onChange={(e) => setCreateForm((f) => ({ ...f, password: e.target.value }))}
                    placeholder="Min. 8 characters"
                    minLength={8}
                  />
                  <button
                    type="button"
                    onClick={() => setShowCreatePwd((v) => !v)}
                    style={{ position: 'absolute', right: '10px', top: '50%', transform: 'translateY(-50%)', background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: '2px' }}
                    aria-label={showCreatePwd ? 'Hide password' : 'Show password'}
                  >
                    {showCreatePwd ? (
                      <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                        <path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/>
                        <line x1="1" y1="1" x2="23" y2="23"/>
                      </svg>
                    ) : (
                      <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
                        <circle cx="12" cy="12" r="3"/>
                      </svg>
                    )}
                  </button>
                </div>
              </div>
              <div>
                <label style={LABEL_STYLE}>Role *</label>
                <select required style={INPUT_STYLE} value={createForm.role} onChange={(e) => setCreateForm((f) => ({ ...f, role: e.target.value }))}>
                  {assignableRoles.map((r) => (
                    <option key={r.value} value={r.value}>{r.label}</option>
                  ))}
                </select>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)' }}>
                  <input type="checkbox" id="create-temp-pwd" checked={createForm.temporary_password} onChange={(e) => setCreateForm((f) => ({ ...f, temporary_password: e.target.checked }))} style={{ width: '14px', height: '14px', accentColor: 'var(--accent)', cursor: 'pointer' }} />
                  <label htmlFor="create-temp-pwd" style={{ fontSize: '13px', color: 'var(--text)', cursor: 'pointer', flex: 1 }}>Require password change on first login</label>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)' }}>
                  <input type="checkbox" id="create-household-2" checked={createForm.create_household} onChange={(e) => setCreateForm((f) => ({ ...f, create_household: e.target.checked }))} style={{ width: '14px', height: '14px', accentColor: 'var(--accent)', cursor: 'pointer' }} />
                  <label htmlFor="create-household-2" style={{ fontSize: '13px', color: 'var(--text)', cursor: 'pointer', flex: 1 }}>Create linked household</label>
                  <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Optional</span>
                </div>
              </div>
              <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '4px' }}>
                <button type="button" className="btn" onClick={() => setShowCreate(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={creating}>
                  {creating ? 'Creating…' : 'Create Account'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Invite Modal */}
      {showInvite && (
        <div
          style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.65)' }}
          onClick={(e) => { if (e.target === e.currentTarget) setShowInvite(false) }}
        >
          <div style={{
            background: 'var(--bg-card)',
            border: '1px solid var(--border-strong)',
            borderRadius: '14px',
            padding: '28px',
            width: '100%',
            maxWidth: '480px',
            boxShadow: 'var(--shadow-lg)',
          }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <div>
                <h2 style={{ fontSize: '18px', fontFamily: 'var(--font-display)', color: 'var(--text)', margin: 0 }}>Invite User</h2>
                <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '4px' }}>
                  Provisions a Keycloak account and sends a welcome email.
                </p>
              </div>
              <button onClick={() => setShowInvite(false)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: '4px' }}>
                <Icon name="x" size={18} />
              </button>
            </div>

            {inviteSuccess && (
              <div style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>
                {inviteSuccess}
              </div>
            )}
            {inviteError && (
              <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>
                {inviteError}
              </div>
            )}

            <form onSubmit={handleInvite} style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
                <div>
                  <label style={LABEL_STYLE}>First Name</label>
                  <input
                    style={INPUT_STYLE}
                    value={inviteForm.first_name}
                    onChange={(e) => setInviteForm((f) => ({ ...f, first_name: e.target.value }))}
                    placeholder="Jane"
                  />
                </div>
                <div>
                  <label style={LABEL_STYLE}>Last Name</label>
                  <input
                    style={INPUT_STYLE}
                    value={inviteForm.last_name}
                    onChange={(e) => setInviteForm((f) => ({ ...f, last_name: e.target.value }))}
                    placeholder="Smith"
                  />
                </div>
              </div>
              <div>
                <label style={LABEL_STYLE}>Email *</label>
                <input
                  required
                  type="email"
                  style={INPUT_STYLE}
                  value={inviteForm.email}
                  onChange={(e) => setInviteForm((f) => ({ ...f, email: e.target.value }))}
                  placeholder="jane@example.com"
                />
              </div>
              <div>
                <label style={LABEL_STYLE}>Role *</label>
                <select
                  required
                  style={INPUT_STYLE}
                  value={inviteForm.role}
                  onChange={(e) => setInviteForm((f) => ({ ...f, role: e.target.value }))}
                >
                  {assignableRoles.map((r) => (
                    <option key={r.value} value={r.value}>{r.label}</option>
                  ))}
                </select>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '10px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)' }}>
                <input
                  type="checkbox"
                  id="create-household"
                  checked={inviteForm.create_household}
                  onChange={(e) => setInviteForm((f) => ({ ...f, create_household: e.target.checked }))}
                  style={{ width: '14px', height: '14px', accentColor: 'var(--accent)', cursor: 'pointer' }}
                />
                <label htmlFor="create-household" style={{ fontSize: '13px', color: 'var(--text)', cursor: 'pointer', flex: 1 }}>
                  Create linked household
                </label>
                <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Optional</span>
              </div>

              <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end', marginTop: '4px' }}>
                <button type="button" className="btn" onClick={() => setShowInvite(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={inviting}>
                  {inviting ? 'Sending…' : 'Send Invitation'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  )
}
