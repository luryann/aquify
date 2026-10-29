'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Modal } from '@/components/ui/Modal'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchUsers,
  inviteUser,
  createUser,
  type UserRecord,
  type UserListSource,
} from '@/lib/adminData'
import { ROLE_HIERARCHY, type Role } from '@/lib/roles'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { ROLE_COLORS, ROLE_OPTIONS } from '@/lib/user-roles'

function RoleBadge({ role }: { role: string }) {
  const c = ROLE_COLORS[role] ?? { bg: 'var(--bg-elevated)', color: 'var(--text-subtle)' }
  const label = ROLE_OPTIONS.find((r) => r.value === role)?.label ?? role
  return (
    <span className="u-fs-fs-2xs u-fw-700 u-p-2px-8px u-ff-font-body" style={{ display: 'inline-block', borderRadius: '6px', background: c.bg, color: c.color }}>{label}</span>
  )
}

function StatusPill({ enabled }: { enabled: boolean }) {
  return (
    <span className="u-gap-4px u-fs-fs-xs u-fw-600 u-p-4px-8px u-ff-font-body" style={{ display: 'inline-flex', alignItems: 'center', borderRadius: '16px', background: enabled ? 'var(--success-bg)' : 'var(--bg-elevated)', color: enabled ? 'var(--success)' : 'var(--text-muted)' }}>
      <span style={{
        width: '5px', height: '5px', borderRadius: '50%',
        background: enabled ? 'var(--success)' : 'var(--text-disabled)',
        flexShrink: 0,
      }} />
      {enabled ? 'Active' : 'Disabled'}
    </span>
  )
}

const INPUT_STYLE: React.CSSProperties = { width: '100%' }

export function UsersPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)

  const [users, setUsers] = useState<UserRecord[]>([])
  const [userSource, setUserSource] = useState<UserListSource>('keycloak')
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

  const loadRequestIdRef = useRef(0)

  function loadUsers() {
    if (!session?.user.orgId) {
      setUsers([])
      setUserSource('keycloak')
      setError('Your account isn’t linked to a club yet. Sign out and back in. If this keeps happening, ask your platform administrator to check your account’s club assignment.')
      setLoading(false)
      return
    }
    const requestId = ++loadRequestIdRef.current
    const role = roleFilter !== 'all' ? roleFilter : undefined
    setLoading(true)
    setError(null)
    fetchUsers(api, session.user.orgId, { limit: 200, role })
      .then(({ users: fetched, source }) => {
        if (loadRequestIdRef.current !== requestId) return
        setUsers(fetched)
        setUserSource(source)
      })
      .catch((err) => {
        if (loadRequestIdRef.current !== requestId) return
        setError(uiErrorMessage(err, 'Couldn’t load users. Try again.'))
      })
      .finally(() => {
        if (loadRequestIdRef.current === requestId) setLoading(false)
      })
  }

  useEffect(() => {
    loadUsers()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session?.user.orgId, roleFilter, api])

  const filtered = useMemo(() => {
    return users.filter((u) => {
      const name = `${u.firstName} ${u.lastName}`.toLowerCase()
      const q = search.toLowerCase()
      const matchSearch = !search || name.includes(q) || (u.email ?? '').toLowerCase().includes(q)
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
      setInviteError(uiErrorMessage(err, 'Couldn’t send invitation. Try again.'))
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
      setCreateError(uiErrorMessage(err, 'Couldn’t create user. Try again.'))
    } finally {
      setCreating(false)
    }
  }


  return (
    <div>
      <div className="u-gap-8px u-mb-16px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <button type="button"
          className="btn u-gap-6px"
          style={{ display: 'flex', alignItems: 'center' }}
          onClick={() => { setShowInvite(true); setInviteError(null); setInviteSuccess(null) }}
        >
          <Icon name="mail" size={14} />
          Invite user
        </button>
        <button type="button"
          className="btn btn-primary u-gap-6px"
          style={{ display: 'flex', alignItems: 'center' }}
          onClick={() => { setShowCreate(true); setCreateError(null); setCreateSuccess(null) }}
        >
          <Icon name="user-plus" size={14} />
          Create user
        </button>
      </div>

      {userSource === 'stub' && (
        <div
          className="card u-p-16px-16px u-mb-16px"
          style={{ background: 'rgba(251,191,36,0.10)', border: '1px solid rgba(251,191,36,0.24)' }}
        >
          <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'flex-start' }}>
            <Icon name="alert-circle" size={16} style={{ color: '#fbbf24', flexShrink: 0, marginTop: '1px' }} />
            <div>
              <div className="u-fs-fs-base u-fw-600 u-mb-4px" style={{ color: 'var(--text)' }}>
                Development stub data
              </div>
              <div className="u-fs-fs-sm u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>
                This list is coming from seeded local development accounts, not a live Keycloak admin realm.
              </div>
            </div>
          </div>
        </div>
      )}

      <div
        className="card u-p-16px-16px u-mb-20px"
        style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)' }}
      >
        <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'flex-start' }}>
          <Icon name="info" size={16} style={{ color: 'var(--accent-text, var(--accent))', flexShrink: 0, marginTop: '1px' }} />
          <div>
            <div className="u-fs-fs-base u-fw-600 u-mb-4px" style={{ color: 'var(--text)' }}>
              Swimmers and users are different records
            </div>
            <div className="u-fs-fs-sm u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>
              Add swimmers from Members to attach them to a parent account&apos;s household. Create a user here only when that person needs an actual Aquify login.
            </div>
          </div>
        </div>
      </div>

      {/* Stats */}
      <div className="cols-3 u-gap-12px u-mb-20px" >
        {[
          { label: 'Total users', value: stats.total, color: 'var(--text)' },
          { label: 'Active', value: stats.active, color: 'var(--success)' },
          { label: 'Disabled', value: stats.disabled, color: 'var(--text-muted)' },
        ].map((stat) => (
          <div key={stat.label} className="card u-p-16px-16px" >
            <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
            <div className="u-fs-fs-2xl u-ff-font-body u-fw-700" style={{ color: stat.color }}>{stat.value}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div className="u-gap-12px u-mb-16px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: '200px', maxWidth: '320px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input aria-label="Search name or email"
            type="text"
            placeholder="Search name or email…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }}
          />
        </div>
        <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
          <option value="all">All statuses</option>
          <option value="active">Active</option>
          <option value="disabled">Disabled</option>
        </select>
        <select value={roleFilter} onChange={(e) => setRoleFilter(e.target.value)}>
          <option value="all">All roles</option>
          {ROLE_OPTIONS.map((r) => (
            <option key={r.value} value={r.value}>{r.label}</option>
          ))}
        </select>
        <span className="u-fs-fs-sm u-ff-font-body u-ml-auto" style={{ color: 'var(--text-muted)' }}>
          {filtered.length} of {users.length}
        </span>
      </div>

      {/* Table */}
      {loading ? (
        <div className="card u-p-48px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
          <Icon name="loader" size={20} style={{ marginBottom: '12px', opacity: 0.5 }} />
          <div>Loading users…</div>
        </div>
      ) : error ? (
        <div className="card u-p-32px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--error)' }}>
          <Icon name="alert-circle" size={18} style={{ marginBottom: '8px' }} />
          <div>{error}</div>
        </div>
      ) : filtered.length === 0 ? (
        <div className="card u-p-48px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
          <Icon name="users" size={24} style={{ marginBottom: '12px', opacity: 0.3 }} />
          <div className="u-fw-500 u-mb-4px" >No users found</div>
          <div className="u-fs-fs-sm" >Adjust your filters or create a login account. Household swimmers appear in Members, not here.</div>
        </div>
      ) : (
        <div className="card" style={{ overflow: 'hidden' }}>
          <div className="table-scroll" style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)' }}>
                  {['User', 'Email', 'Roles', 'Status', 'Last Login', ''].map((col) => (
                    <th key={col} className="col-head u-p-12px-16px" style={{ textAlign: 'left', background: 'var(--bg-elevated)' }}>{col}</th>
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
                    <td className="u-p-16px-16px" >
                      <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center' }}>
                        <div className="u-fs-fs-sm u-fw-700 u-ff-font-body" style={{ width: '32px', height: '32px', borderRadius: '50%', background: 'var(--accent-subtle)', border: '1px solid var(--accent-border)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--accent-text, var(--accent))', flexShrink: 0 }}>
                          {((user.firstName?.[0] ?? '') + (user.lastName?.[0] ?? '')).toUpperCase() || (user.email?.[0] ?? '?').toUpperCase()}
                        </div>
                        <div>
                          <div className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)' }}>
                            {user.firstName || user.lastName
                              ? `${user.firstName} ${user.lastName}`.trim()
                              : <span style={{ color: 'var(--text-muted)' }}>No name</span>}
                          </div>
                          <div className="u-fs-fs-xs u-ff-font-body u-mt-1px" style={{ color: 'var(--text-muted)' }}>
                            {user.id.slice(0, 8)}…
                          </div>
                        </div>
                      </div>
                    </td>
                    <td className="u-p-16px-16px u-fs-fs-base u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>
                      {user.email}
                    </td>
                    <td className="u-p-16px-16px" >
                      {user.roles && user.roles.length > 0 ? (
                        <div className="u-gap-4px" style={{ display: 'flex', flexWrap: 'wrap' }}>
                          {user.roles.map((r) => <RoleBadge key={r} role={r} />)}
                        </div>
                      ) : (
                        <span className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>—</span>
                      )}
                    </td>
                    <td className="u-p-16px-16px" >
                      <StatusPill enabled={user.enabled} />
                    </td>
                    <td className="u-p-16px-16px u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>
                      {user.lastLoginAt
                        ? new Date(user.lastLoginAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
                        : <span style={{ color: 'var(--text-muted)' }}>—</span>}
                    </td>
                    <td className="u-p-16px-16px" style={{ textAlign: 'right' }}>
                      <Link
                        href={`/users/${user.id}`}
                        className="u-fs-fs-sm u-gap-4px u-fw-500" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none', display: 'inline-flex', alignItems: 'center' }}
                      >
                        Manage <Icon name="chevron-right" size={12} />
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Create User Modal */}
      <Modal open={Boolean(showCreate)} onClose={() => setShowCreate(false)} title="Create user" description="Creates a Keycloak account with a directly-set password. No email sent." width={480}>
{createSuccess && (
              <div className="u-p-12px-16px u-mb-16px u-fs-fs-base" style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px' }}>
                {createSuccess}
              </div>
            )}
{createError && (
              <div className="u-p-12px-16px u-mb-16px u-fs-fs-base" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px' }}>
                {createError}
              </div>
            )}
<form onSubmit={handleCreate} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div className="cols-2 u-gap-12px" >
                <div>
                  <label className="form-label">First name</label>
                  <input aria-label="Jane" style={INPUT_STYLE} value={createForm.first_name} onChange={(e) => setCreateForm((f) => ({ ...f, first_name: e.target.value }))} placeholder="Jane" />
                </div>
                <div>
                  <label className="form-label">Last name</label>
                  <input aria-label="Smith" style={INPUT_STYLE} value={createForm.last_name} onChange={(e) => setCreateForm((f) => ({ ...f, last_name: e.target.value }))} placeholder="Smith" />
                </div>
              </div>
              <div>
                <label className="form-label form-label-required">Email</label>
                <input aria-label="jane@example.com" required type="email" style={INPUT_STYLE} value={createForm.email} onChange={(e) => setCreateForm((f) => ({ ...f, email: e.target.value }))} placeholder="jane@example.com" />
              </div>
              <div>
                <label className="form-label form-label-required">Password</label>
                <div style={{ position: 'relative' }}>
                  <input aria-label="Min. 8 characters"
                    required
                    type={showCreatePwd ? 'text' : 'password'}
                    style={{ ...INPUT_STYLE }}
                    value={createForm.password}
                    onChange={(e) => setCreateForm((f) => ({ ...f, password: e.target.value }))}
                    placeholder="Min. 8 characters"
                    minLength={8}
                  />
                  <button
                    type="button"
                    onClick={() => setShowCreatePwd((v) => !v)}
                    className="u-p-2px" style={{ position: 'absolute', right: '10px', top: '50%', transform: 'translateY(-50%)', background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)' }}
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
                <label className="form-label form-label-required">Role</label>
                <select required style={INPUT_STYLE} value={createForm.role} onChange={(e) => setCreateForm((f) => ({ ...f, role: e.target.value }))}>
                  {assignableRoles.map((r) => (
                    <option key={r.value} value={r.value}>{r.label}</option>
                  ))}
                </select>
              </div>
              <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
                <div className="u-gap-12px u-p-12px-12px" style={{ display: 'flex', alignItems: 'center', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)' }}>
                  <input type="checkbox" id="create-temp-pwd" checked={createForm.temporary_password} onChange={(e) => setCreateForm((f) => ({ ...f, temporary_password: e.target.checked }))} style={{ width: '14px', height: '14px', accentColor: 'var(--accent)', cursor: 'pointer' }} />
                  <label htmlFor="create-temp-pwd" className="u-fs-fs-base" style={{ color: 'var(--text)', cursor: 'pointer', flex: 1 }}>Require password change on first login</label>
                </div>
                <div className="u-gap-12px u-p-12px-12px" style={{ display: 'flex', alignItems: 'center', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)' }}>
                  <input type="checkbox" id="create-household-2" checked={createForm.create_household} onChange={(e) => setCreateForm((f) => ({ ...f, create_household: e.target.checked }))} style={{ width: '14px', height: '14px', accentColor: 'var(--accent)', cursor: 'pointer' }} />
                  <label htmlFor="create-household-2" className="u-fs-fs-base" style={{ color: 'var(--text)', cursor: 'pointer', flex: 1 }}>Create linked household</label>
                  <span className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>Optional</span>
                </div>
              </div>
              <div className="form-actions">
                <button type="button" className="btn" onClick={() => setShowCreate(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={creating}>
                  {creating ? 'Creating…' : 'Create Account'}
                </button>
              </div>
            </form>
</Modal>

      {/* Invite Modal */}
      <Modal open={Boolean(showInvite)} onClose={() => setShowInvite(false)} title="Invite user" description="Provisions a Keycloak account and sends a welcome email." width={480}>
{inviteSuccess && (
              <div className="u-p-12px-16px u-mb-16px u-fs-fs-base" style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px' }}>
                {inviteSuccess}
              </div>
            )}
{inviteError && (
              <div className="u-p-12px-16px u-mb-16px u-fs-fs-base" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px' }}>
                {inviteError}
              </div>
            )}
<form onSubmit={handleInvite} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div className="cols-2 u-gap-12px" >
                <div>
                  <label className="form-label">First name</label>
                  <input aria-label="Jane"
                    style={INPUT_STYLE}
                    value={inviteForm.first_name}
                    onChange={(e) => setInviteForm((f) => ({ ...f, first_name: e.target.value }))}
                    placeholder="Jane"
                  />
                </div>
                <div>
                  <label className="form-label">Last name</label>
                  <input aria-label="Smith"
                    style={INPUT_STYLE}
                    value={inviteForm.last_name}
                    onChange={(e) => setInviteForm((f) => ({ ...f, last_name: e.target.value }))}
                    placeholder="Smith"
                  />
                </div>
              </div>
              <div>
                <label className="form-label form-label-required">Email</label>
                <input aria-label="jane@example.com"
                  required
                  type="email"
                  style={INPUT_STYLE}
                  value={inviteForm.email}
                  onChange={(e) => setInviteForm((f) => ({ ...f, email: e.target.value }))}
                  placeholder="jane@example.com"
                />
              </div>
              <div>
                <label className="form-label form-label-required">Role</label>
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

              <div className="u-gap-12px u-p-12px-12px" style={{ display: 'flex', alignItems: 'center', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)' }}>
                <input
                  type="checkbox"
                  id="create-household"
                  checked={inviteForm.create_household}
                  onChange={(e) => setInviteForm((f) => ({ ...f, create_household: e.target.checked }))}
                  style={{ width: '14px', height: '14px', accentColor: 'var(--accent)', cursor: 'pointer' }}
                />
                <label htmlFor="create-household" className="u-fs-fs-base" style={{ color: 'var(--text)', cursor: 'pointer', flex: 1 }}>
                  Create linked household
                </label>
                <span className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>Optional</span>
              </div>

              <div className="form-actions">
                <button type="button" className="btn" onClick={() => setShowInvite(false)}>Cancel</button>
                <button type="submit" className="btn btn-primary" disabled={inviting}>
                  {inviting ? 'Sending…' : 'Send invitation'}
                </button>
              </div>
            </form>
</Modal>
    </div>
  )
}
