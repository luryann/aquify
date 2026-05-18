'use client'

import { useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import {
  fetchUser,
  fetchAuditEvents,
  updateUser,
  disableUser,
  enableUser,
  deleteUser,
  assignUserRole,
  removeUserRole,
  sendPasswordReset,
  type UserRecord,
  type UserRoleRecord,
  type AuditEventRecord,
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

function RoleChip({ roleName, onRemove, canRemove }: { roleName: string; onRemove?: () => void; canRemove: boolean }) {
  const c = ROLE_COLORS[roleName] ?? { bg: 'var(--bg-elevated)', color: 'var(--text-subtle)' }
  const label = ROLE_OPTIONS.find((r) => r.value === roleName)?.label ?? roleName
  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: '5px',
      fontSize: '11px',
      fontWeight: 700,
      letterSpacing: '0.04em',
      textTransform: 'uppercase',
      padding: '4px 8px',
      borderRadius: '6px',
      background: c.bg,
      color: c.color,
      fontFamily: 'var(--font-mono)',
    }}>
      {label}
      {canRemove && onRemove && (
        <button
          onClick={onRemove}
          style={{
            background: 'none', border: 'none', cursor: 'pointer',
            color: c.color, opacity: 0.7, padding: '0', lineHeight: 1,
            display: 'flex', alignItems: 'center',
          }}
          title={`Remove ${label} role`}
        >
          <Icon name="x" size={11} />
        </button>
      )}
    </span>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="card" style={{ padding: '20px 24px', marginBottom: '16px' }}>
      <h3 style={{
        fontSize: '11px',
        fontWeight: 700,
        color: 'var(--text-muted)',
        textTransform: 'uppercase',
        letterSpacing: '0.06em',
        fontFamily: 'var(--font-mono)',
        marginBottom: '16px',
        paddingBottom: '10px',
        borderBottom: '1px solid var(--border)',
      }}>{title}</h3>
      {children}
    </div>
  )
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div style={{ marginBottom: '14px' }}>
      <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '4px' }}>{label}</div>
      <div style={{ fontSize: '14px', color: 'var(--text)' }}>{value}</div>
    </div>
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

export default function UserDetailPage() {
  const params = useParams()
  const router = useRouter()
  const userId = params.id as string
  const { session } = useSession()
  const api = useApiClient(session?.token)

  const [user, setUser] = useState<UserRecord | null>(null)
  const [roles, setRoles] = useState<UserRoleRecord[]>([])
  const [auditEvents, setAuditEvents] = useState<AuditEventRecord[]>([])
  const [householdId, setHouseholdId] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Edit profile
  const [editMode, setEditMode] = useState(false)
  const [editForm, setEditForm] = useState({ first_name: '', last_name: '', email: '' })
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saveSuccess, setSaveSuccess] = useState(false)

  // Role management
  const [addingRole, setAddingRole] = useState(false)
  const [newRole, setNewRole] = useState('')
  const [roleError, setRoleError] = useState<string | null>(null)
  const [roleLoading, setRoleLoading] = useState(false)

  // Action states
  const [actionLoading, setActionLoading] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)
  const [actionSuccess, setActionSuccess] = useState<string | null>(null)

  // Confirmation dialogs
  const [showDisableConfirm, setShowDisableConfirm] = useState(false)
  const [showEnableConfirm, setShowEnableConfirm] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [deleteEmailConfirm, setDeleteEmailConfirm] = useState('')

  const actorRole = session?.user.role as Role | undefined
  const actorTier = actorRole ? ROLE_HIERARCHY[actorRole] : 0
  const isOwner = actorRole === 'owner'

  function canManageRole(roleName: string): boolean {
    const tier = ROLE_HIERARCHY[roleName as Role] ?? 1
    return tier <= actorTier
  }

  async function load() {
    if (!session?.user.orgId) return
    setLoading(true)
    setError(null)
    try {
      const [{ user: u, roles: r }, auditRes] = await Promise.all([
        fetchUser(api, session.user.orgId, userId),
        fetchAuditEvents(api, session.user.orgId, {
          resource_id: userId,
          limit: '50',
        }).catch(() => ({ events: [] as AuditEventRecord[] })),
      ])
      setUser(u)
      setRoles(r)
      setAuditEvents(auditRes.events ?? [])
      setHouseholdId(u.attributes?.household_id?.[0] ?? u.householdId ?? null)
      setEditForm({ first_name: u.firstName, last_name: u.lastName, email: u.email })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to load user.')
    } finally {
      setLoading(false)
    }
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void load() }, [session?.user.orgId, userId])

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId || !user) return
    setSaving(true)
    setSaveError(null)
    setSaveSuccess(false)
    try {
      await updateUser(api, session.user.orgId, userId, editForm)
      setSaveSuccess(true)
      setEditMode(false)
      await load()
    } catch (err) {
      setSaveError(err instanceof ApiError ? err.message : 'Failed to update user.')
    } finally {
      setSaving(false)
    }
  }

  async function handleAddRole() {
    if (!session?.user.orgId || !newRole) return
    setRoleLoading(true)
    setRoleError(null)
    try {
      await assignUserRole(api, session.user.orgId, userId, newRole)
      setAddingRole(false)
      setNewRole('')
      await load()
    } catch (err) {
      setRoleError(err instanceof ApiError ? err.message : 'Failed to assign role.')
    } finally {
      setRoleLoading(false)
    }
  }

  async function handleRemoveRole(roleName: string) {
    if (!session?.user.orgId) return
    setRoleLoading(true)
    setRoleError(null)
    try {
      await removeUserRole(api, session.user.orgId, userId, roleName)
      await load()
    } catch (err) {
      setRoleError(err instanceof ApiError ? err.message : 'Failed to remove role.')
    } finally {
      setRoleLoading(false)
    }
  }

  async function handlePasswordReset() {
    if (!session?.user.orgId) return
    setActionLoading(true)
    setActionError(null)
    setActionSuccess(null)
    try {
      await sendPasswordReset(api, session.user.orgId, userId)
      setActionSuccess('Password reset email sent.')
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : 'Failed to send password reset.')
    } finally {
      setActionLoading(false)
    }
  }

  async function handleDisable() {
    if (!session?.user.orgId) return
    setActionLoading(true)
    setActionError(null)
    setActionSuccess(null)
    setShowDisableConfirm(false)
    try {
      await disableUser(api, session.user.orgId, userId)
      setActionSuccess('User account disabled.')
      await load()
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : 'Failed to disable user.')
    } finally {
      setActionLoading(false)
    }
  }

  async function handleEnable() {
    if (!session?.user.orgId) return
    setActionLoading(true)
    setActionError(null)
    setActionSuccess(null)
    setShowEnableConfirm(false)
    try {
      await enableUser(api, session.user.orgId, userId)
      setActionSuccess('User account enabled.')
      await load()
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : 'Failed to enable user.')
    } finally {
      setActionLoading(false)
    }
  }

  async function handleDelete() {
    if (!session?.user.orgId || !user) return
    if (deleteEmailConfirm !== user.email) return
    setActionLoading(true)
    setActionError(null)
    setShowDeleteConfirm(false)
    try {
      await deleteUser(api, session.user.orgId, userId)
      router.push('/users')
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : 'Failed to delete user.')
      setActionLoading(false)
    }
  }

  if (loading) {
    return (
      <div style={{ padding: '24px', display: 'flex', alignItems: 'center', gap: '10px', color: 'var(--text-muted)', fontSize: '13px' }}>
        <Icon name="loader" size={16} />
        Loading user…
      </div>
    )
  }

  if (error || !user) {
    return (
      <div style={{ padding: '24px' }}>
        <div className="card" style={{ padding: '32px', textAlign: 'center', color: 'var(--error)', fontSize: '13px' }}>
          <Icon name="alert-circle" size={20} style={{ marginBottom: '8px' }} />
          <div>{error ?? 'User not found.'}</div>
          <Link href="/users" style={{ color: 'var(--accent)', textDecoration: 'none', fontSize: '12px', marginTop: '12px', display: 'inline-block' }}>
            ← Back to Users
          </Link>
        </div>
      </div>
    )
  }

  const existingRoleNames = new Set(roles.map((r) => r.name))
  const availableRolesToAdd = ROLE_OPTIONS.filter(
    (r) => !existingRoleNames.has(r.value) && canManageRole(r.value)
  )

  return (
    <div style={{ padding: '24px', maxWidth: '860px' }}>
      {/* Breadcrumb */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '20px', fontSize: '12px', color: 'var(--text-muted)' }}>
        <Link href="/users" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>Users</Link>
        <Icon name="chevron-right" size={12} />
        <span style={{ color: 'var(--text)' }}>{user.firstName || user.lastName ? `${user.firstName} ${user.lastName}`.trim() : user.email}</span>
      </div>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: '16px', marginBottom: '24px' }}>
        <div style={{
          width: '48px', height: '48px', borderRadius: '50%',
          background: 'var(--accent-subtle)',
          border: '2px solid var(--accent-border)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
          fontSize: '18px', fontWeight: 700, color: 'var(--accent)',
          fontFamily: 'var(--font-mono)', flexShrink: 0,
        }}>
          {((user.firstName?.[0] ?? '') + (user.lastName?.[0] ?? '')).toUpperCase() || user.email[0].toUpperCase()}
        </div>
        <div style={{ flex: 1 }}>
          <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>
            {user.firstName || user.lastName ? `${user.firstName} ${user.lastName}`.trim() : <span style={{ color: 'var(--text-muted)' }}>Unnamed User</span>}
          </h1>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
            <span style={{ fontSize: '13px', color: 'var(--text-subtle)', fontFamily: 'var(--font-mono)' }}>{user.email}</span>
            <span style={{
              display: 'inline-flex', alignItems: 'center', gap: '4px',
              fontSize: '11px', fontWeight: 600, padding: '2px 8px', borderRadius: '20px',
              background: user.enabled ? 'var(--success-bg)' : 'var(--bg-elevated)',
              color: user.enabled ? 'var(--success)' : 'var(--text-muted)',
              fontFamily: 'var(--font-mono)',
            }}>
              <span style={{ width: '5px', height: '5px', borderRadius: '50%', background: user.enabled ? 'var(--success)' : 'var(--text-disabled)' }} />
              {user.enabled ? 'Active' : 'Disabled'}
            </span>
            {!user.emailVerified && (
              <span style={{ fontSize: '11px', color: 'var(--warning)', display: 'flex', alignItems: 'center', gap: '3px' }}>
                <Icon name="alert-triangle" size={11} /> Email not verified
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Action feedback */}
      {actionSuccess && (
        <div style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px', display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Icon name="check-circle" size={14} /> {actionSuccess}
        </div>
      )}
      {actionError && (
        <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>
          {actionError}
        </div>
      )}
      {saveSuccess && (
        <div style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px', display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Icon name="check-circle" size={14} /> Profile updated.
        </div>
      )}

      {/* Profile */}
      <Section title="Profile">
        {editMode ? (
          <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
            {saveError && (
              <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '10px 14px', fontSize: '13px' }}>
                {saveError}
              </div>
            )}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px' }}>
              <div>
                <label style={{ fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '5px', fontFamily: 'var(--font-mono)' }}>
                  First Name
                </label>
                <input style={INPUT_STYLE} value={editForm.first_name} onChange={(e) => setEditForm((f) => ({ ...f, first_name: e.target.value }))} />
              </div>
              <div>
                <label style={{ fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '5px', fontFamily: 'var(--font-mono)' }}>
                  Last Name
                </label>
                <input style={INPUT_STYLE} value={editForm.last_name} onChange={(e) => setEditForm((f) => ({ ...f, last_name: e.target.value }))} />
              </div>
            </div>
            <div>
              <label style={{ fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '5px', fontFamily: 'var(--font-mono)' }}>
                Email
              </label>
              <input type="email" style={INPUT_STYLE} value={editForm.email} onChange={(e) => setEditForm((f) => ({ ...f, email: e.target.value }))} />
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              <button type="submit" className="btn btn-primary" style={{ fontSize: '12px', padding: '7px 14px' }} disabled={saving}>
                {saving ? 'Saving…' : 'Save Changes'}
              </button>
              <button type="button" className="btn" style={{ fontSize: '12px', padding: '7px 14px' }} onClick={() => { setEditMode(false); setSaveError(null) }}>
                Cancel
              </button>
            </div>
          </form>
        ) : (
          <div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '0' }}>
              <Field label="First Name" value={user.firstName || <span style={{ color: 'var(--text-muted)' }}>—</span>} />
              <Field label="Last Name" value={user.lastName || <span style={{ color: 'var(--text-muted)' }}>—</span>} />
              <Field label="Email" value={<span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>{user.email}</span>} />
              <Field label="Username" value={<span style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text-subtle)' }}>{user.username}</span>} />
              <Field label="Email Verified" value={
                user.emailVerified
                  ? <span style={{ color: 'var(--success)', fontSize: '13px' }}>Verified</span>
                  : <span style={{ color: 'var(--warning)', fontSize: '13px' }}>Not verified</span>
              } />
              <Field label="Keycloak ID" value={<span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)' }}>{user.id}</span>} />
            </div>
            <button className="btn" style={{ fontSize: '12px', padding: '6px 12px', marginTop: '4px' }} onClick={() => { setEditMode(true); setSaveSuccess(false) }}>
              <Icon name="edit-2" size={12} style={{ marginRight: '5px' }} />
              Edit Profile
            </button>
          </div>
        )}
      </Section>

      {/* Roles */}
      <Section title="Roles">
        {roleError && (
          <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '8px 12px', marginBottom: '12px', fontSize: '12px' }}>
            {roleError}
          </div>
        )}
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginBottom: '12px', alignItems: 'center' }}>
          {roles.length === 0 ? (
            <span style={{ fontSize: '13px', color: 'var(--text-muted)' }}>No roles assigned.</span>
          ) : (
            roles.map((r) => (
              <RoleChip
                key={r.id || r.name}
                roleName={r.name}
                canRemove={canManageRole(r.name)}
                onRemove={() => { void handleRemoveRole(r.name) }}
              />
            ))
          )}
          {!addingRole && availableRolesToAdd.length > 0 && (
            <button
              className="btn"
              style={{ fontSize: '11px', padding: '4px 10px', display: 'flex', alignItems: 'center', gap: '4px' }}
              onClick={() => { setAddingRole(true); setRoleError(null) }}
              disabled={roleLoading}
            >
              <Icon name="plus" size={11} /> Add Role
            </button>
          )}
        </div>
        {addingRole && (
          <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
            <select
              value={newRole}
              onChange={(e) => setNewRole(e.target.value)}
              style={{ padding: '7px 10px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }}
            >
              <option value="">Select role…</option>
              {availableRolesToAdd.map((r) => (
                <option key={r.value} value={r.value}>{r.label}</option>
              ))}
            </select>
            <button
              className="btn btn-primary"
              style={{ fontSize: '12px', padding: '7px 12px' }}
              onClick={() => { void handleAddRole() }}
              disabled={!newRole || roleLoading}
            >
              {roleLoading ? 'Adding…' : 'Assign'}
            </button>
            <button
              className="btn"
              style={{ fontSize: '12px', padding: '7px 12px' }}
              onClick={() => { setAddingRole(false); setNewRole('') }}
            >
              Cancel
            </button>
          </div>
        )}
      </Section>

      {/* Linked Household */}
      <Section title="Linked Household">
        {householdId ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px', padding: '12px 14px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '10px' }}>
              <Icon name="home" size={16} style={{ color: 'var(--accent)', flexShrink: 0, marginTop: '1px' }} />
              <div>
                <div style={{ fontSize: '13px', fontWeight: 500, color: 'var(--text)' }}>Linked household</div>
                <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{householdId}</div>
                <div style={{ fontSize: '11px', color: 'var(--text-disabled)', marginTop: '4px', lineHeight: 1.5, maxWidth: '440px' }}>
                  Household swimmers linked here appear in the parent portal through Members. They do not create separate login accounts in <span style={{ fontFamily: 'var(--font-mono)' }}>/users</span> unless you create a user for them.
                </div>
              </div>
            </div>
            <Link href={`/members/households/${householdId}`} style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '3px' }}>
              View <Icon name="chevron-right" size={12} />
            </Link>
          </div>
        ) : (
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px', padding: '12px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <Icon name="home" size={16} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
            <div>
              <div style={{ fontSize: '13px', color: 'var(--text-muted)' }}>No household linked</div>
              <div style={{ fontSize: '11px', color: 'var(--text-disabled)', marginTop: '2px' }}>
                Link a household from Members to give a parent access to that family&apos;s swimmers in the portal.
              </div>
            </div>
          </div>
        )}
      </Section>

      {/* Actions */}
      <Section title="Actions">
        <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
          <button
            className="btn"
            style={{ fontSize: '12px', padding: '7px 14px', display: 'flex', alignItems: 'center', gap: '6px' }}
            onClick={() => { void handlePasswordReset() }}
            disabled={actionLoading}
          >
            <Icon name="key" size={13} />
            Reset Password
          </button>

          {user.enabled ? (
            <button
              className="btn"
              style={{ fontSize: '12px', padding: '7px 14px', display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--warning)', borderColor: 'var(--warning-border)' }}
              onClick={() => setShowDisableConfirm(true)}
              disabled={actionLoading}
            >
              <Icon name="user-x" size={13} />
              Disable Account
            </button>
          ) : (
            <button
              className="btn"
              style={{ fontSize: '12px', padding: '7px 14px', display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--success)', borderColor: 'var(--success-border)' }}
              onClick={() => setShowEnableConfirm(true)}
              disabled={actionLoading}
            >
              <Icon name="user-check" size={13} />
              Enable Account
            </button>
          )}

          {isOwner && (
            <button
              className="btn"
              style={{ fontSize: '12px', padding: '7px 14px', display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--error)', borderColor: 'var(--error-border)', marginLeft: 'auto' }}
              onClick={() => { setShowDeleteConfirm(true); setDeleteEmailConfirm('') }}
              disabled={actionLoading}
            >
              <Icon name="trash-2" size={13} />
              Delete User
            </button>
          )}
        </div>
      </Section>

      {/* Audit Trail */}
      <Section title="Audit Trail">
        {auditEvents.length === 0 ? (
          <div style={{ fontSize: '13px', color: 'var(--text-muted)', padding: '8px 0' }}>No audit events recorded for this user.</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0' }}>
            {auditEvents.slice(0, 20).map((event, i) => (
              <div key={event.id} style={{
                display: 'flex',
                alignItems: 'flex-start',
                gap: '12px',
                padding: '10px 0',
                borderBottom: i < Math.min(auditEvents.length, 20) - 1 ? '1px solid var(--border)' : 'none',
              }}>
                <div style={{
                  width: '28px', height: '28px', borderRadius: '50%',
                  background: 'var(--bg-elevated)',
                  border: '1px solid var(--border)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  flexShrink: 0,
                }}>
                  <Icon name="shield" size={12} style={{ color: 'var(--text-muted)' }} />
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontSize: '12px', color: 'var(--text)', fontFamily: 'var(--font-mono)' }}>{event.action}</div>
                  <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '2px' }}>
                    by <span style={{ color: 'var(--text-subtle)' }}>{event.actor_user_id.slice(0, 8)}…</span>
                    {' · '}
                    {new Date(event.occurred_at).toLocaleString('en-US', {
                      month: 'short', day: 'numeric', year: 'numeric',
                      hour: '2-digit', minute: '2-digit', hour12: false,
                    })}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </Section>

      {/* Disable Confirmation */}
      {showDisableConfirm && (
        <ConfirmModal
          title="Disable Account"
          message={`Are you sure you want to disable ${user.email}? They will not be able to log in until re-enabled.`}
          confirmLabel="Disable"
          danger={false}
          onConfirm={() => { void handleDisable() }}
          onCancel={() => setShowDisableConfirm(false)}
        />
      )}

      {/* Enable Confirmation */}
      {showEnableConfirm && (
        <ConfirmModal
          title="Enable Account"
          message={`Re-enable ${user.email}? They will regain login access immediately.`}
          confirmLabel="Enable"
          danger={false}
          onConfirm={() => { void handleEnable() }}
          onCancel={() => setShowEnableConfirm(false)}
        />
      )}

      {/* Delete Confirmation */}
      {showDeleteConfirm && (
        <div
          style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.7)' }}
          onClick={(e) => { if (e.target === e.currentTarget) setShowDeleteConfirm(false) }}
        >
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--error-border)', borderRadius: '14px', padding: '28px', width: '100%', maxWidth: '460px', boxShadow: 'var(--shadow-lg)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '12px' }}>
              <div style={{ width: '36px', height: '36px', borderRadius: '50%', background: 'var(--error-bg)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                <Icon name="trash-2" size={16} style={{ color: 'var(--error)' }} />
              </div>
              <h2 style={{ fontSize: '16px', fontFamily: 'var(--font-display)', color: 'var(--text)', margin: 0 }}>Permanently Delete User</h2>
            </div>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '8px', lineHeight: 1.5 }}>
              This will permanently delete <strong style={{ color: 'var(--text)' }}>{user.email}</strong> from Keycloak and anonymize all associated DARE records. This action <strong style={{ color: 'var(--error)' }}>cannot be undone</strong>.
            </p>
            <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '16px' }}>
              Type the user&apos;s email address to confirm:
            </p>
            <input
              type="text"
              style={{ ...INPUT_STYLE, borderColor: deleteEmailConfirm === user.email ? 'var(--error)' : 'var(--border)', marginBottom: '16px' }}
              placeholder={user.email}
              value={deleteEmailConfirm}
              onChange={(e) => setDeleteEmailConfirm(e.target.value)}
            />
            <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
              <button className="btn" style={{ fontSize: '13px' }} onClick={() => setShowDeleteConfirm(false)}>Cancel</button>
              <button
                className="btn"
                style={{ fontSize: '13px', background: 'var(--error)', color: '#fff', border: 'none' }}
                disabled={deleteEmailConfirm !== user.email || actionLoading}
                onClick={() => { void handleDelete() }}
              >
                {actionLoading ? 'Deleting…' : 'Delete Permanently'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function ConfirmModal({
  title,
  message,
  confirmLabel,
  danger,
  onConfirm,
  onCancel,
}: {
  title: string
  message: string
  confirmLabel: string
  danger: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <div
      style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'rgba(0,0,0,0.65)' }}
      onClick={(e) => { if (e.target === e.currentTarget) onCancel() }}
    >
      <div style={{ background: 'var(--bg-card)', border: '1px solid var(--border-strong)', borderRadius: '12px', padding: '24px', width: '100%', maxWidth: '400px', boxShadow: 'var(--shadow-lg)' }}>
        <h2 style={{ fontSize: '16px', fontFamily: 'var(--font-display)', color: 'var(--text)', marginBottom: '10px' }}>{title}</h2>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.5, marginBottom: '20px' }}>{message}</p>
        <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
          <button className="btn" style={{ fontSize: '13px' }} onClick={onCancel}>Cancel</button>
          <button
            className="btn btn-primary"
            style={danger ? { background: 'var(--error)', border: 'none', fontSize: '13px' } : { fontSize: '13px' }}
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
