'use client'

import { useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
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
  fetchUserSwimmer,
  linkUserSwimmer,
  unlinkUserSwimmer,
  fetchUserHousehold,
  linkUserHousehold,
  unlinkUserHousehold,
  fetchHouseholds,
  fetchSwimmers,
  type HouseholdRecord,
  type UserRecord,
  type UserRoleRecord,
  type AuditEventRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import { ROLE_HIERARCHY, type Role } from '@/lib/roles'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

const ROLE_OPTIONS: { value: string; label: string }[] = [
  { value: 'admin', label: 'Admin' },
  { value: 'coach', label: 'Coach' },
  { value: 'parent', label: 'Parent' },
  { value: 'swimmer', label: 'Swimmer' },
]

const ROLE_COLORS: Record<string, { bg: string; color: string }> = {
  admin:           { bg: 'rgba(251,191,36,0.12)',   color: '#fbbf24' },
  coach:           { bg: 'rgba(74,222,128,0.10)',   color: '#4ade80' },
  parent:          { bg: 'rgba(196,148,255,0.10)',  color: '#c484ff' },
  swimmer:         { bg: 'rgba(248,113,113,0.10)',  color: '#f87171' },
}

function RoleChip({ roleName, onRemove, canRemove }: { roleName: string; onRemove?: () => void; canRemove: boolean }) {
  const c = ROLE_COLORS[roleName] ?? { bg: 'var(--bg-elevated)', color: 'var(--text-subtle)' }
  const label = ROLE_OPTIONS.find((r) => r.value === roleName)?.label ?? roleName
  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: '4px',
      fontSize: 'var(--fs-xs)',
      fontWeight: 700,
      padding: '4px 8px',
      borderRadius: '6px',
      background: c.bg,
      color: c.color,
      fontFamily: 'var(--font-body)',
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
      <h3 className="card-title" style={{ marginBottom: '16px', paddingBottom: '12px', borderBottom: '1px solid var(--border)' }}>{title}</h3>
      {children}
    </div>
  )
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div style={{ marginBottom: '16px' }}>
      <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginBottom: '4px' }}>{label}</div>
      <div style={{ fontSize: 'var(--fs-md)', color: 'var(--text)' }}>{value}</div>
    </div>
  )
}

const INPUT_STYLE: React.CSSProperties = {
  width: '100%',
  padding: '8px 12px',
  background: 'var(--bg-elevated)',
  border: '1px solid var(--border)',
  borderRadius: 'var(--r-md)',
  fontSize: 'var(--fs-base)',
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
  const [linkedHousehold, setLinkedHousehold] = useState<HouseholdRecord | null>(null)
  const [householdOptions, setHouseholdOptions] = useState<HouseholdRecord[]>([])
  const [swimmerOptions, setSwimmerOptions] = useState<SwimmerRecord[]>([])
  const [householdIdInput, setHouseholdIdInput] = useState('')
  const [householdLinkLoading, setHouseholdLinkLoading] = useState(false)
  const [householdLinkError, setHouseholdLinkError] = useState<string | null>(null)
  const [linkedSwimmer, setLinkedSwimmer] = useState<SwimmerRecord | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Swimmer linkage
  const [swimmerIdInput, setSwimmerIdInput] = useState('')
  const [swimmerLinkLoading, setSwimmerLinkLoading] = useState(false)
  const [swimmerLinkError, setSwimmerLinkError] = useState<string | null>(null)

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
  const isAdmin = actorRole === 'admin'

  function canManageRole(roleName: string): boolean {
    const tier = ROLE_HIERARCHY[roleName as Role] ?? 1
    return tier <= actorTier
  }

  async function load() {
    if (!session?.user.orgId) return
    setLoading(true)
    setError(null)
    try {
      const [{ user: u, roles: r }, auditRes, swimmer, household, households, swimmers] = await Promise.all([
        fetchUser(api, session.user.orgId, userId),
        fetchAuditEvents(api, session.user.orgId, {
          resource_id: userId,
          limit: '50',
        }).catch(() => ({ events: [] as AuditEventRecord[] })),
        fetchUserSwimmer(api, session.user.orgId, userId).catch(() => null),
        fetchUserHousehold(api, session.user.orgId, userId).catch(() => null),
        fetchHouseholds(api, session.user.orgId).catch(() => [] as HouseholdRecord[]),
        fetchSwimmers(api, session.user.orgId).catch(() => [] as SwimmerRecord[]),
      ])
      setUser(u)
      setRoles(r)
      setAuditEvents(auditRes.events ?? [])
      setLinkedHousehold(household)
      setHouseholdOptions(households)
      setSwimmerOptions(swimmers)
      setLinkedSwimmer(swimmer)
      setEditForm({ first_name: u.firstName, last_name: u.lastName, email: u.email })
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t load user. Try again.'))
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
      setSaveError(uiErrorMessage(err, 'Couldn’t update user. Try again.'))
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
      setRoleError(uiErrorMessage(err, 'Couldn’t assign role. Try again.'))
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
      setRoleError(uiErrorMessage(err, 'Couldn’t remove role. Try again.'))
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
      setActionError(uiErrorMessage(err, 'Couldn’t send password reset. Try again.'))
    } finally {
      setActionLoading(false)
    }
  }

  async function handleLinkHousehold() {
    if (!session?.user.orgId || !householdIdInput) return
    setHouseholdLinkLoading(true)
    setHouseholdLinkError(null)
    try {
      await linkUserHousehold(api, session.user.orgId, userId, householdIdInput)
      setHouseholdIdInput('')
      await load()
    } catch (err) {
      setHouseholdLinkError(uiErrorMessage(err, 'Couldn’t link household. Try again.'))
    } finally {
      setHouseholdLinkLoading(false)
    }
  }

  async function handleUnlinkHousehold() {
    if (!session?.user.orgId || !linkedHousehold) return
    setHouseholdLinkLoading(true)
    setHouseholdLinkError(null)
    try {
      await unlinkUserHousehold(api, session.user.orgId, userId, linkedHousehold.id)
      await load()
    } catch (err) {
      setHouseholdLinkError(uiErrorMessage(err, 'Couldn’t unlink household. Try again.'))
    } finally {
      setHouseholdLinkLoading(false)
    }
  }

  async function handleLinkSwimmer() {
    if (!session?.user.orgId || !swimmerIdInput.trim()) return
    setSwimmerLinkLoading(true)
    setSwimmerLinkError(null)
    try {
      await linkUserSwimmer(api, session.user.orgId, userId, swimmerIdInput.trim())
      setSwimmerIdInput('')
      await load()
    } catch (err) {
      setSwimmerLinkError(uiErrorMessage(err, 'Couldn’t link swimmer. Try again.'))
    } finally {
      setSwimmerLinkLoading(false)
    }
  }

  async function handleUnlinkSwimmer() {
    if (!session?.user.orgId || !linkedSwimmer) return
    setSwimmerLinkLoading(true)
    setSwimmerLinkError(null)
    try {
      await unlinkUserSwimmer(api, session.user.orgId, userId, linkedSwimmer.id)
      await load()
    } catch (err) {
      setSwimmerLinkError(uiErrorMessage(err, 'Couldn’t unlink swimmer. Try again.'))
    } finally {
      setSwimmerLinkLoading(false)
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
      setActionError(uiErrorMessage(err, 'Couldn’t disable user. Try again.'))
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
      setActionError(uiErrorMessage(err, 'Couldn’t enable user. Try again.'))
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
      setActionError(uiErrorMessage(err, 'Couldn’t delete user. Try again.'))
      setActionLoading(false)
    }
  }

  if (loading) {
    return (
      <div style={{ padding: '24px', display: 'flex', alignItems: 'center', gap: '12px', color: 'var(--text-muted)', fontSize: 'var(--fs-base)' }}>
        <Icon name="loader" size={16} />
        Loading user…
      </div>
    )
  }

  if (error || !user) {
    return (
      <div style={{ padding: '24px' }}>
        <div className="card" style={{ padding: '32px', textAlign: 'center', color: 'var(--error)', fontSize: 'var(--fs-base)' }}>
          <Icon name="alert-circle" size={20} style={{ marginBottom: '8px' }} />
          <div>{error ?? 'User not found.'}</div>
          <Link href="/users" style={{ color: 'var(--accent)', textDecoration: 'none', fontSize: 'var(--fs-sm)', marginTop: '12px', display: 'inline-block' }}>
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
    <div className="page">
      {/* Breadcrumb */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '20px', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
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
          fontSize: 'var(--fs-xl)', fontWeight: 700, color: 'var(--accent)',
          fontFamily: 'var(--font-body)', flexShrink: 0,
        }}>
          {((user.firstName?.[0] ?? '') + (user.lastName?.[0] ?? '')).toUpperCase() || user.email[0].toUpperCase()}
        </div>
        <div style={{ flex: 1 }}>
          <h1 className="page-title">
            {user.firstName || user.lastName ? `${user.firstName} ${user.lastName}`.trim() : <span style={{ color: 'var(--text-muted)' }}>Unnamed user</span>}
          </h1>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
            <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', fontFamily: 'var(--font-body)' }}>{user.email}</span>
            <span style={{
              display: 'inline-flex', alignItems: 'center', gap: '4px',
              fontSize: 'var(--fs-xs)', fontWeight: 600, padding: '2px 8px', borderRadius: '16px',
              background: user.enabled ? 'var(--success-bg)' : 'var(--bg-elevated)',
              color: user.enabled ? 'var(--success)' : 'var(--text-muted)',
              fontFamily: 'var(--font-body)',
            }}>
              <span style={{ width: '5px', height: '5px', borderRadius: '50%', background: user.enabled ? 'var(--success)' : 'var(--text-disabled)' }} />
              {user.enabled ? 'Active' : 'Disabled'}
            </span>
            {!user.emailVerified && (
              <span style={{ fontSize: 'var(--fs-xs)', color: 'var(--warning)', display: 'flex', alignItems: 'center', gap: '4px' }}>
                <Icon name="alert-triangle" size={11} /> Email not verified
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Action feedback */}
      {actionSuccess && (
        <div style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px', padding: '12px 16px', marginBottom: '16px', fontSize: 'var(--fs-base)', display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Icon name="check-circle" size={14} /> {actionSuccess}
        </div>
      )}
      {actionError && (
        <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '12px 16px', marginBottom: '16px', fontSize: 'var(--fs-base)' }}>
          {actionError}
        </div>
      )}
      {saveSuccess && (
        <div style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px', padding: '12px 16px', marginBottom: '16px', fontSize: 'var(--fs-base)', display: 'flex', alignItems: 'center', gap: '8px' }}>
          <Icon name="check-circle" size={14} /> Profile updated.
        </div>
      )}

      {/* Profile */}
      <Section title="Profile">
        {editMode ? (
          <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
            {saveError && (
              <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '12px 16px', fontSize: 'var(--fs-base)' }}>
                {saveError}
              </div>
            )}
            <div className="cols-2" style={{ gap: '12px' }}>
              <div>
                <label style={{ fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px', fontFamily: 'var(--font-body)' }}>
                  First name
                </label>
                <input style={INPUT_STYLE} value={editForm.first_name} onChange={(e) => setEditForm((f) => ({ ...f, first_name: e.target.value }))} />
              </div>
              <div>
                <label style={{ fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px', fontFamily: 'var(--font-body)' }}>
                  Last name
                </label>
                <input style={INPUT_STYLE} value={editForm.last_name} onChange={(e) => setEditForm((f) => ({ ...f, last_name: e.target.value }))} />
              </div>
            </div>
            <div>
              <label style={{ fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px', fontFamily: 'var(--font-body)' }}>
                Email
              </label>
              <input type="email" style={INPUT_STYLE} value={editForm.email} onChange={(e) => setEditForm((f) => ({ ...f, email: e.target.value }))} />
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              <button type="submit" className="btn btn-primary" style={{ fontSize: 'var(--fs-sm)', padding: '8px 16px' }} disabled={saving}>
                {saving ? 'Saving…' : 'Save Changes'}
              </button>
              <button type="button" className="btn" style={{ fontSize: 'var(--fs-sm)', padding: '8px 16px' }} onClick={() => { setEditMode(false); setSaveError(null) }}>
                Cancel
              </button>
            </div>
          </form>
        ) : (
          <div>
            <div className="cols-2" style={{ gap: '0' }}>
              <Field label="First name" value={user.firstName || <span style={{ color: 'var(--text-muted)' }}>—</span>} />
              <Field label="Last name" value={user.lastName || <span style={{ color: 'var(--text-muted)' }}>—</span>} />
              <Field label="Email" value={<span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-base)' }}>{user.email}</span>} />
              <Field label="Username" value={<span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-base)', color: 'var(--text-subtle)' }}>{user.username}</span>} />
              <Field label="Email verified" value={
                user.emailVerified
                  ? <span style={{ color: 'var(--success)', fontSize: 'var(--fs-base)' }}>Verified</span>
                  : <span style={{ color: 'var(--warning)', fontSize: 'var(--fs-base)' }}>Not verified</span>
              } />
              <Field label="User ID" value={<span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-xs)', color: 'var(--text-muted)' }}>{user.id}</span>} />
            </div>
            <button className="btn" style={{ fontSize: 'var(--fs-sm)', padding: '6px 12px', marginTop: '4px' }} onClick={() => { setEditMode(true); setSaveSuccess(false) }}>
              <Icon name="edit-2" size={12} style={{ marginRight: '4px' }} />
              Edit Profile
            </button>
          </div>
        )}
      </Section>

      {/* Roles */}
      <Section title="Roles">
        {roleError && (
          <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '8px 12px', marginBottom: '12px', fontSize: 'var(--fs-sm)' }}>
            {roleError}
          </div>
        )}
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', marginBottom: '12px', alignItems: 'center' }}>
          {roles.length === 0 ? (
            <span style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>No roles assigned.</span>
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
              style={{ fontSize: 'var(--fs-xs)', padding: '4px 12px', display: 'flex', alignItems: 'center', gap: '4px' }}
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
              style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none' }}
            >
              <option value="">Select role…</option>
              {availableRolesToAdd.map((r) => (
                <option key={r.value} value={r.value}>{r.label}</option>
              ))}
            </select>
            <button
              className="btn btn-primary"
              style={{ fontSize: 'var(--fs-sm)', padding: '8px 12px' }}
              onClick={() => { void handleAddRole() }}
              disabled={!newRole || roleLoading}
            >
              {roleLoading ? 'Adding…' : 'Assign'}
            </button>
            <button
              className="btn"
              style={{ fontSize: 'var(--fs-sm)', padding: '8px 12px' }}
              onClick={() => { setAddingRole(false); setNewRole('') }}
            >
              Cancel
            </button>
          </div>
        )}
      </Section>

      {/* Linked Household */}
      <Section title="Linked household">
        {householdLinkError && (
          <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '8px 12px', marginBottom: '12px', fontSize: 'var(--fs-sm)' }}>
            {householdLinkError}
          </div>
        )}
        {linkedHousehold ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px', padding: '12px 16px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
              <Icon name="home" size={16} style={{ color: 'var(--accent)', flexShrink: 0, marginTop: '1px' }} />
              <div>
                <div style={{ fontSize: 'var(--fs-base)', fontWeight: 500, color: 'var(--text)' }}>{linkedHousehold.household_name}</div>
                <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>{linkedHousehold.primary_email || linkedHousehold.id}</div>
                <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-disabled)', marginTop: '4px', lineHeight: 1.5, maxWidth: '440px' }}>
                  When this user signs in as a parent, the portal shows this household&apos;s swimmers, invoices, documents, and meet commitments.
                </div>
              </div>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <button
                className="btn"
                style={{ fontSize: 'var(--fs-sm)', padding: '6px 12px', color: 'var(--error)', borderColor: 'var(--error-border)' }}
                onClick={() => { void handleUnlinkHousehold() }}
                disabled={householdLinkLoading}
              >
                {householdLinkLoading ? 'Unlinking…' : 'Unlink'}
              </button>
              <Link href={`/members/households/${linkedHousehold.id}`} style={{ fontSize: 'var(--fs-sm)', color: 'var(--accent)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px' }}>
                View <Icon name="chevron-right" size={12} />
              </Link>
            </div>
          </div>
        ) : (
          <div style={{ padding: '12px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '12px' }}>
              <Icon name="home" size={16} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
              <div>
                <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>No household linked</div>
                <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-disabled)', marginTop: '2px' }}>
                  Link a household to give this parent portal access to that family&apos;s swimmers.
                </div>
              </div>
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              <select
                aria-label="Household to link"
                value={householdIdInput}
                onChange={(e) => setHouseholdIdInput(e.target.value)}
                style={INPUT_STYLE}
              >
                <option value="">Select a household…</option>
                {[...householdOptions]
                  .sort((a, b) => a.household_name.localeCompare(b.household_name))
                  .map((h) => (
                    <option key={h.id} value={h.id}>
                      {h.household_name}{h.primary_email ? ` — ${h.primary_email}` : ''}
                    </option>
                  ))}
              </select>
              <button
                className="btn btn-primary"
                style={{ fontSize: 'var(--fs-sm)', padding: '8px 16px', whiteSpace: 'nowrap' }}
                onClick={() => { void handleLinkHousehold() }}
                disabled={!householdIdInput || householdLinkLoading}
              >
                {householdLinkLoading ? 'Linking…' : 'Link'}
              </button>
            </div>
          </div>
        )}
      </Section>

      {/* Linked Swimmer */}
      <Section title="Linked swimmer">
        {swimmerLinkError && (
          <div style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px', padding: '8px 12px', marginBottom: '12px', fontSize: 'var(--fs-sm)' }}>
            {swimmerLinkError}
          </div>
        )}
        {linkedSwimmer ? (
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px', padding: '12px 16px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
              <Icon name="user" size={16} style={{ color: 'var(--accent)', flexShrink: 0, marginTop: '1px' }} />
              <div>
                <div style={{ fontSize: 'var(--fs-base)', fontWeight: 500, color: 'var(--text)' }}>
                  {linkedSwimmer.first_name} {linkedSwimmer.last_name}
                </div>
                <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>{linkedSwimmer.id}</div>
                <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-disabled)', marginTop: '4px', lineHeight: 1.5, maxWidth: '440px' }}>
                  When this user logs in with the swimmer role, their portal session resolves to this exact swimmer record — never a guess from the household roster.
                </div>
              </div>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
              <button
                className="btn"
                style={{ fontSize: 'var(--fs-sm)', padding: '6px 12px', color: 'var(--error)', borderColor: 'var(--error-border)' }}
                onClick={() => { void handleUnlinkSwimmer() }}
                disabled={swimmerLinkLoading}
              >
                {swimmerLinkLoading ? 'Unlinking…' : 'Unlink'}
              </button>
              <Link href={`/members/swimmers/${linkedSwimmer.id}`} style={{ fontSize: 'var(--fs-sm)', color: 'var(--accent)', textDecoration: 'none', display: 'flex', alignItems: 'center', gap: '4px' }}>
                View <Icon name="chevron-right" size={12} />
              </Link>
            </div>
          </div>
        ) : (
          <div style={{ padding: '12px', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '12px' }}>
              <Icon name="user" size={16} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
              <div>
                <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>No swimmer linked</div>
                <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-disabled)', marginTop: '2px' }}>
                  Required for this user&apos;s swim portal session to know which swimmer is &quot;them&quot; when the household has more than one.
                </div>
              </div>
            </div>
            <div style={{ display: 'flex', gap: '8px' }}>
              <select
                aria-label="Swimmer to link"
                value={swimmerIdInput}
                onChange={(e) => setSwimmerIdInput(e.target.value)}
                style={INPUT_STYLE}
              >
                <option value="">Select a swimmer…</option>
                {[...swimmerOptions]
                  .sort((a, b) => `${a.last_name} ${a.first_name}`.localeCompare(`${b.last_name} ${b.first_name}`))
                  .map((sw) => (
                    <option key={sw.id} value={sw.id}>
                      {sw.last_name}, {sw.first_name}{sw.date_of_birth ? ` (${sw.date_of_birth.slice(0, 10)})` : ''}
                    </option>
                  ))}
              </select>
              <button
                className="btn btn-primary"
                style={{ fontSize: 'var(--fs-sm)', padding: '8px 16px', whiteSpace: 'nowrap' }}
                onClick={() => { void handleLinkSwimmer() }}
                disabled={!swimmerIdInput.trim() || swimmerLinkLoading}
              >
                {swimmerLinkLoading ? 'Linking…' : 'Link'}
              </button>
            </div>
          </div>
        )}
      </Section>

      {/* Actions */}
      <Section title="Account access">
        <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap' }}>
          <button
            className="btn"
            style={{ fontSize: 'var(--fs-sm)', padding: '8px 16px', display: 'flex', alignItems: 'center', gap: '6px' }}
            onClick={() => { void handlePasswordReset() }}
            disabled={actionLoading}
          >
            <Icon name="key" size={13} />
            Reset Password
          </button>

          {user.enabled ? (
            <button
              className="btn"
              style={{ fontSize: 'var(--fs-sm)', padding: '8px 16px', display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--warning)', borderColor: 'var(--warning-border)' }}
              onClick={() => setShowDisableConfirm(true)}
              disabled={actionLoading}
            >
              <Icon name="user-x" size={13} />
              Disable Account
            </button>
          ) : (
            <button
              className="btn"
              style={{ fontSize: 'var(--fs-sm)', padding: '8px 16px', display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--success)', borderColor: 'var(--success-border)' }}
              onClick={() => setShowEnableConfirm(true)}
              disabled={actionLoading}
            >
              <Icon name="user-check" size={13} />
              Enable Account
            </button>
          )}

          {isAdmin && (
            <button
              className="btn"
              style={{ fontSize: 'var(--fs-sm)', padding: '8px 16px', display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--error)', borderColor: 'var(--error-border)', marginLeft: 'auto' }}
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
      <Section title="Audit trail">
        {auditEvents.length === 0 ? (
          <div style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', padding: '8px 0' }}>No audit events recorded for this user.</div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '0' }}>
            {auditEvents.slice(0, 20).map((event, i) => (
              <div key={event.id} style={{
                display: 'flex',
                alignItems: 'flex-start',
                gap: '12px',
                padding: '12px 0',
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
                  <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text)', fontFamily: 'var(--font-body)' }}>{event.action}</div>
                  <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginTop: '2px' }}>
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
          title="Disable account"
          message={`${user.email} won’t be able to sign in until you enable the account again.`}
          confirmLabel="Disable account"
          danger={false}
          onConfirm={() => { void handleDisable() }}
          onCancel={() => setShowDisableConfirm(false)}
        />
      )}

      {/* Enable Confirmation */}
      {showEnableConfirm && (
        <ConfirmModal
          title="Enable account"
          message={`Re-enable ${user.email}? They will regain login access immediately.`}
          confirmLabel="Enable account"
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
          <div style={{ background: 'var(--bg-card)', border: '1px solid var(--error-border)', borderRadius: '12px', padding: '24px', width: '100%', maxWidth: '460px', boxShadow: 'var(--shadow-lg)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '12px' }}>
              <div style={{ width: '36px', height: '36px', borderRadius: '50%', background: 'var(--error-bg)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                <Icon name="trash-2" size={16} style={{ color: 'var(--error)' }} />
              </div>
              <h2 className="section-title" style={{ margin: 0 }}>Permanently delete user</h2>
            </div>
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginBottom: '8px', lineHeight: 1.5 }}>
              This will permanently delete <strong style={{ color: 'var(--text)' }}>{user.email}</strong> from the identity provider and anonymize all associated DARE records. This action <strong style={{ color: 'var(--error)' }}>cannot be undone</strong>.
            </p>
            <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: '16px' }}>
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
              <button className="btn" style={{ fontSize: 'var(--fs-base)' }} onClick={() => setShowDeleteConfirm(false)}>Cancel</button>
              <button
                className="btn"
                style={{ fontSize: 'var(--fs-base)', background: 'var(--error)', color: '#fff', border: 'none' }}
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
        <h2 className="section-title" style={{ marginBottom: '12px' }}>{title}</h2>
        <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', lineHeight: 1.5, marginBottom: '20px' }}>{message}</p>
        <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
          <button className="btn" style={{ fontSize: 'var(--fs-base)' }} onClick={onCancel}>Cancel</button>
          <button
            className="btn btn-primary"
            style={danger ? { background: 'var(--error)', border: 'none', fontSize: 'var(--fs-base)' } : { fontSize: 'var(--fs-base)' }}
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
