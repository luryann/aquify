'use client'

import { useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { Modal } from '@/components/ui/Modal'
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
import { ROLE_COLORS, ROLE_OPTIONS } from '@/lib/user-roles'

function RoleChip({ roleName, onRemove, canRemove }: { roleName: string; onRemove?: () => void; canRemove: boolean }) {
  const c = ROLE_COLORS[roleName] ?? { bg: 'var(--bg-elevated)', color: 'var(--text-subtle)' }
  const label = ROLE_OPTIONS.find((r) => r.value === roleName)?.label ?? roleName
  return (
    <span className="u-gap-4px u-fs-fs-xs u-fw-700 u-p-4px-8px u-ff-font-body" style={{ display: 'inline-flex', alignItems: 'center', borderRadius: '6px', background: c.bg, color: c.color }}>
      {label}
      {canRemove && onRemove && (
        <button type="button"
          onClick={onRemove}
          className="u-p-0 u-lh-1" style={{ background: 'none', border: 'none', cursor: 'pointer', color: c.color, opacity: 0.7, display: 'flex', alignItems: 'center' }}
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
    <div className="card u-p-20px-24px u-mb-16px" >
      <h3 className="card-title u-mb-16px u-pb-12px" style={{ borderBottom: '1px solid var(--border)' }}>{title}</h3>
      {children}
    </div>
  )
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="u-mb-16px" >
      <div className="u-fs-fs-sm u-ff-font-body u-mb-4px" style={{ color: 'var(--text-muted)' }}>{label}</div>
      <div className="u-fs-fs-md" style={{ color: 'var(--text)' }}>{value}</div>
    </div>
  )
}

const INPUT_STYLE: React.CSSProperties = { width: '100%' }

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
      <div className="u-p-24px u-gap-12px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', color: 'var(--text-muted)' }}>
        <Icon name="loader" size={16} />
        Loading user…
      </div>
    )
  }

  if (error || !user) {
    return (
      <div className="u-p-24px" >
        <div className="card u-p-32px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--error)' }}>
          <Icon name="alert-circle" size={20} style={{ marginBottom: '8px' }} />
          <div>{error ?? 'User not found.'}</div>
          <Link href="/users" className="u-fs-fs-sm u-mt-12px" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none', display: 'inline-block' }}>
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
      <div className="u-gap-6px u-mb-20px u-fs-fs-sm" style={{ display: 'flex', alignItems: 'center', color: 'var(--text-muted)' }}>
        <Link href="/users" style={{ color: 'var(--text-muted)', textDecoration: 'none' }}>Users</Link>
        <Icon name="chevron-right" size={12} />
        <span style={{ color: 'var(--text)' }}>{user.firstName || user.lastName ? `${user.firstName} ${user.lastName}`.trim() : user.email}</span>
      </div>

      {/* Header */}
      <div className="u-gap-16px u-mb-24px" style={{ display: 'flex', alignItems: 'flex-start' }}>
        <div className="u-fs-fs-xl u-fw-700 u-ff-font-body" style={{ width: '48px', height: '48px', borderRadius: '50%', background: 'var(--accent-subtle)', border: '2px solid var(--accent-border)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--accent-text, var(--accent))', flexShrink: 0 }}>
          {((user.firstName?.[0] ?? '') + (user.lastName?.[0] ?? '')).toUpperCase() || user.email[0].toUpperCase()}
        </div>
        <div style={{ flex: 1 }}>
          <h1 className="page-title">
            {user.firstName || user.lastName ? `${user.firstName} ${user.lastName}`.trim() : <span style={{ color: 'var(--text-muted)' }}>Unnamed user</span>}
          </h1>
          <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
            <span className="u-fs-fs-base u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>{user.email}</span>
            <span className="u-gap-4px u-fs-fs-xs u-fw-600 u-p-2px-8px u-ff-font-body" style={{ display: 'inline-flex', alignItems: 'center', borderRadius: '16px', background: user.enabled ? 'var(--success-bg)' : 'var(--bg-elevated)', color: user.enabled ? 'var(--success)' : 'var(--text-muted)' }}>
              <span style={{ width: '5px', height: '5px', borderRadius: '50%', background: user.enabled ? 'var(--success)' : 'var(--text-disabled)' }} />
              {user.enabled ? 'Active' : 'Disabled'}
            </span>
            {!user.emailVerified && (
              <span className="u-fs-fs-xs u-gap-4px" style={{ color: 'var(--warning)', display: 'flex', alignItems: 'center' }}>
                <Icon name="alert-triangle" size={11} /> Email not verified
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Action feedback */}
      {actionSuccess && (
        <div className="u-p-12px-16px u-mb-16px u-fs-fs-base u-gap-8px" style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px', display: 'flex', alignItems: 'center' }}>
          <Icon name="check-circle" size={14} /> {actionSuccess}
        </div>
      )}
      {actionError && (
        <div className="u-p-12px-16px u-mb-16px u-fs-fs-base" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px' }}>
          {actionError}
        </div>
      )}
      {saveSuccess && (
        <div className="u-p-12px-16px u-mb-16px u-fs-fs-base u-gap-8px" style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', color: 'var(--success)', borderRadius: '8px', display: 'flex', alignItems: 'center' }}>
          <Icon name="check-circle" size={14} /> Profile updated.
        </div>
      )}

      {/* Profile */}
      <Section title="Profile">
        {editMode ? (
          <form onSubmit={handleSave} className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
            {saveError && (
              <div className="u-p-12px-16px u-fs-fs-base" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px' }}>
                {saveError}
              </div>
            )}
            <div className="cols-2 u-gap-12px" >
              <div>
                <label className="u-fs-fs-sm u-fw-600 u-mb-4px u-ff-font-body" style={{ color: 'var(--text-muted)', display: 'block' }}>
                  First name
                </label>
                <input style={INPUT_STYLE} value={editForm.first_name} onChange={(e) => setEditForm((f) => ({ ...f, first_name: e.target.value }))} />
              </div>
              <div>
                <label className="u-fs-fs-sm u-fw-600 u-mb-4px u-ff-font-body" style={{ color: 'var(--text-muted)', display: 'block' }}>
                  Last name
                </label>
                <input style={INPUT_STYLE} value={editForm.last_name} onChange={(e) => setEditForm((f) => ({ ...f, last_name: e.target.value }))} />
              </div>
            </div>
            <div>
              <label className="u-fs-fs-sm u-fw-600 u-mb-4px u-ff-font-body" style={{ color: 'var(--text-muted)', display: 'block' }}>
                Email
              </label>
              <input type="email" style={INPUT_STYLE} value={editForm.email} onChange={(e) => setEditForm((f) => ({ ...f, email: e.target.value }))} />
            </div>
            <div className="u-gap-8px" style={{ display: 'flex' }}>
              <button type="submit" className="btn btn-primary u-fs-fs-sm u-p-8px-16px" disabled={saving}>
                {saving ? 'Saving…' : 'Save Changes'}
              </button>
              <button type="button" className="btn u-fs-fs-sm u-p-8px-16px" onClick={() => { setEditMode(false); setSaveError(null) }}>
                Cancel
              </button>
            </div>
          </form>
        ) : (
          <div>
            <div className="cols-2 u-gap-0" >
              <Field label="First name" value={user.firstName || <span style={{ color: 'var(--text-muted)' }}>—</span>} />
              <Field label="Last name" value={user.lastName || <span style={{ color: 'var(--text-muted)' }}>—</span>} />
              <Field label="Email" value={<span className="u-ff-font-body u-fs-fs-base" >{user.email}</span>} />
              <Field label="Username" value={<span className="u-ff-font-body u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>{user.username}</span>} />
              <Field label="Email verified" value={
                user.emailVerified
                  ? <span className="u-fs-fs-base" style={{ color: 'var(--success)' }}>Verified</span>
                  : <span className="u-fs-fs-base" style={{ color: 'var(--warning)' }}>Not verified</span>
              } />
              <Field label="User ID" value={<span className="u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>{user.id}</span>} />
            </div>
            <button type="button" className="btn u-fs-fs-sm u-p-6px-12px u-mt-4px" onClick={() => { setEditMode(true); setSaveSuccess(false) }}>
              <Icon name="edit-2" size={12} style={{ marginRight: '4px' }} />
              Edit Profile
            </button>
          </div>
        )}
      </Section>

      {/* Roles */}
      <Section title="Roles">
        {roleError && (
          <div className="u-p-8px-12px u-mb-12px u-fs-fs-sm" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px' }}>
            {roleError}
          </div>
        )}
        <div className="u-gap-8px u-mb-12px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>
          {roles.length === 0 ? (
            <span className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>No roles assigned.</span>
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
            <button type="button"
              className="btn u-fs-fs-xs u-p-4px-12px u-gap-4px"
              style={{ display: 'flex', alignItems: 'center' }}
              onClick={() => { setAddingRole(true); setRoleError(null) }}
              disabled={roleLoading}
            >
              <Icon name="plus" size={11} /> Add role
            </button>
          )}
        </div>
        {addingRole && (
          <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center' }}>
            <select
              value={newRole}
              onChange={(e) => setNewRole(e.target.value)}
              className="u-p-8px-12px u-fs-fs-base" 
            >
              <option value="">Select role…</option>
              {availableRolesToAdd.map((r) => (
                <option key={r.value} value={r.value}>{r.label}</option>
              ))}
            </select>
            <button type="button"
              className="btn btn-primary u-fs-fs-sm u-p-8px-12px"
              
              onClick={() => { void handleAddRole() }}
              disabled={!newRole || roleLoading}
            >
              {roleLoading ? 'Adding…' : 'Assign'}
            </button>
            <button type="button"
              className="btn u-fs-fs-sm u-p-8px-12px"
              
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
          <div className="u-p-8px-12px u-mb-12px u-fs-fs-sm" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px' }}>
            {householdLinkError}
          </div>
        )}
        {linkedHousehold ? (
          <div className="u-gap-12px u-p-12px-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'flex-start' }}>
              <Icon name="home" size={16} style={{ color: 'var(--accent-text, var(--accent))', flexShrink: 0, marginTop: '1px' }} />
              <div>
                <div className="u-fs-fs-base u-fw-500" style={{ color: 'var(--text)' }}>{linkedHousehold.household_name}</div>
                <div className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>{linkedHousehold.primary_email || linkedHousehold.id}</div>
                <div className="u-fs-fs-xs u-mt-4px u-lh-1_5" style={{ color: 'var(--text-muted)', maxWidth: '440px' }}>
                  When this user signs in as a parent, the portal shows this household&apos;s swimmers, invoices, documents, and meet commitments.
                </div>
              </div>
            </div>
            <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center' }}>
              <button type="button"
                className="btn u-fs-fs-sm u-p-6px-12px"
                style={{ color: 'var(--error)', borderColor: 'var(--error-border)' }}
                onClick={() => { void handleUnlinkHousehold() }}
                disabled={householdLinkLoading}
              >
                {householdLinkLoading ? 'Unlinking…' : 'Unlink'}
              </button>
              <Link href={`/members/households/${linkedHousehold.id}`} className="u-fs-fs-sm u-gap-4px" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none', display: 'flex', alignItems: 'center' }}>
                View <Icon name="chevron-right" size={12} />
              </Link>
            </div>
          </div>
        ) : (
          <div className="u-p-12px" style={{ background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <div className="u-gap-12px u-mb-12px" style={{ display: 'flex', alignItems: 'center' }}>
              <Icon name="home" size={16} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
              <div>
                <div className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>No household linked</div>
                <div className="u-fs-fs-xs u-mt-2px" style={{ color: 'var(--text-muted)' }}>
                  Link a household to give this parent portal access to that family&apos;s swimmers.
                </div>
              </div>
            </div>
            <div className="u-gap-8px" style={{ display: 'flex' }}>
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
              <button type="button"
                className="btn btn-primary u-fs-fs-sm u-p-8px-16px"
                style={{ whiteSpace: 'nowrap' }}
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
          <div className="u-p-8px-12px u-mb-12px u-fs-fs-sm" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', borderRadius: '8px' }}>
            {swimmerLinkError}
          </div>
        )}
        {linkedSwimmer ? (
          <div className="u-gap-12px u-p-12px-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'flex-start' }}>
              <Icon name="user" size={16} style={{ color: 'var(--accent-text, var(--accent))', flexShrink: 0, marginTop: '1px' }} />
              <div>
                <div className="u-fs-fs-base u-fw-500" style={{ color: 'var(--text)' }}>
                  {linkedSwimmer.first_name} {linkedSwimmer.last_name}
                </div>
                <div className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>{linkedSwimmer.id}</div>
                <div className="u-fs-fs-xs u-mt-4px u-lh-1_5" style={{ color: 'var(--text-muted)', maxWidth: '440px' }}>
                  When this user logs in with the swimmer role, their portal session resolves to this exact swimmer record — never a guess from the household roster.
                </div>
              </div>
            </div>
            <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center' }}>
              <button type="button"
                className="btn u-fs-fs-sm u-p-6px-12px"
                style={{ color: 'var(--error)', borderColor: 'var(--error-border)' }}
                onClick={() => { void handleUnlinkSwimmer() }}
                disabled={swimmerLinkLoading}
              >
                {swimmerLinkLoading ? 'Unlinking…' : 'Unlink'}
              </button>
              <Link href={`/members/swimmers/${linkedSwimmer.id}`} className="u-fs-fs-sm u-gap-4px" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none', display: 'flex', alignItems: 'center' }}>
                View <Icon name="chevron-right" size={12} />
              </Link>
            </div>
          </div>
        ) : (
          <div className="u-p-12px" style={{ background: 'var(--bg-elevated)', borderRadius: 'var(--r-md)', border: '1px solid var(--border)' }}>
            <div className="u-gap-12px u-mb-12px" style={{ display: 'flex', alignItems: 'center' }}>
              <Icon name="user" size={16} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
              <div>
                <div className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>No swimmer linked</div>
                <div className="u-fs-fs-xs u-mt-2px" style={{ color: 'var(--text-muted)' }}>
                  Required for this user&apos;s swim portal session to know which swimmer is &quot;them&quot; when the household has more than one.
                </div>
              </div>
            </div>
            <div className="u-gap-8px" style={{ display: 'flex' }}>
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
              <button type="button"
                className="btn btn-primary u-fs-fs-sm u-p-8px-16px"
                style={{ whiteSpace: 'nowrap' }}
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
        <div className="u-gap-12px" style={{ display: 'flex', flexWrap: 'wrap' }}>
          <button type="button"
            className="btn u-fs-fs-sm u-p-8px-16px u-gap-6px"
            style={{ display: 'flex', alignItems: 'center' }}
            onClick={() => { void handlePasswordReset() }}
            disabled={actionLoading}
          >
            <Icon name="key" size={13} />
            Reset Password
          </button>

          {user.enabled ? (
            <button type="button"
              className="btn u-fs-fs-sm u-p-8px-16px u-gap-6px"
              style={{ display: 'flex', alignItems: 'center', color: 'var(--warning)', borderColor: 'var(--warning-border)' }}
              onClick={() => setShowDisableConfirm(true)}
              disabled={actionLoading}
            >
              <Icon name="user-x" size={13} />
              Disable Account
            </button>
          ) : (
            <button type="button"
              className="btn u-fs-fs-sm u-p-8px-16px u-gap-6px"
              style={{ display: 'flex', alignItems: 'center', color: 'var(--success)', borderColor: 'var(--success-border)' }}
              onClick={() => setShowEnableConfirm(true)}
              disabled={actionLoading}
            >
              <Icon name="user-check" size={13} />
              Enable Account
            </button>
          )}

          {isAdmin && (
            <button type="button"
              className="btn u-fs-fs-sm u-p-8px-16px u-gap-6px u-ml-auto"
              style={{ display: 'flex', alignItems: 'center', color: 'var(--error)', borderColor: 'var(--error-border)' }}
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
          <div className="u-fs-fs-base u-p-8px-0" style={{ color: 'var(--text-muted)' }}>No audit events recorded for this user.</div>
        ) : (
          <div className="u-gap-0" style={{ display: 'flex', flexDirection: 'column' }}>
            {auditEvents.slice(0, 20).map((event, i) => (
              <div key={event.id} className="u-gap-12px u-p-12px-0" style={{ display: 'flex', alignItems: 'flex-start', borderBottom: i < Math.min(auditEvents.length, 20) - 1 ? '1px solid var(--border)' : 'none' }}>
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
                  <div className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text)' }}>{event.action}</div>
                  <div className="u-fs-fs-xs u-mt-2px" style={{ color: 'var(--text-muted)' }}>
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
      <Modal open={Boolean(showDeleteConfirm)} onClose={() => setShowDeleteConfirm(false)} title="Permanently delete user" width={460}>
<p className="u-fs-fs-base u-mb-8px u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>
              This will permanently delete <strong style={{ color: 'var(--text)' }}>{user.email}</strong> from the identity provider and anonymize all associated DARE records. This action <strong style={{ color: 'var(--error)' }}>cannot be undone</strong>.
            </p>
<p className="u-fs-fs-sm u-mb-16px" style={{ color: 'var(--text-muted)' }}>
              Type the user&apos;s email address to confirm:
            </p>
<input
              type="text"
              style={{ ...INPUT_STYLE, marginBottom: '16px' }}
              placeholder={user.email}
              value={deleteEmailConfirm}
              onChange={(e) => setDeleteEmailConfirm(e.target.value)}
            />
<div className="u-gap-8px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button type="button" className="btn u-fs-fs-base" onClick={() => setShowDeleteConfirm(false)}>Cancel</button>
              <button type="button"
                className="btn u-fs-fs-base"
                style={{ background: 'var(--error)', color: '#fff', border: 'none' }}
                disabled={deleteEmailConfirm !== user.email || actionLoading}
                onClick={() => { void handleDelete() }}
              >
                {actionLoading ? 'Deleting…' : 'Delete permanently'}
              </button>
            </div>
</Modal>
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
    <Modal open={true} onClose={onCancel} title={title} width={400}>
<p className="u-fs-fs-base u-lh-1_5 u-mb-20px" style={{ color: 'var(--text-subtle)' }}>{message}</p>
<div className="u-gap-8px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <button type="button" className="btn u-fs-fs-base" onClick={onCancel}>Cancel</button>
          <button type="button"
            className="btn btn-primary"
            style={danger ? { background: 'var(--error)', border: 'none', fontSize: 'var(--fs-base)' } : { fontSize: 'var(--fs-base)' }}
            onClick={onConfirm}
          >
            {confirmLabel}
          </button>
        </div>
</Modal>
  )
}
