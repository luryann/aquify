'use client'

import { useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { assignRole, fetchMFAFactors, fetchRoleAssignments, fetchUsers, type MFAFactorRecord, type RoleAssignmentRecord, type UserRecord } from '@/lib/adminData'

const ROLE_OPTIONS = ['admin', 'coach', 'parent', 'swimmer'] as const
type RoleOption = (typeof ROLE_OPTIONS)[number]

const ROLE_COLORS: Record<string, { bg: string; border: string; color: string }> = {
  admin: { bg: 'rgba(99,102,241,0.1)', border: 'rgba(99,102,241,0.25)', color: '#818cf8' },
  coach: { bg: 'rgba(74,222,128,0.08)', border: 'var(--success-border)', color: 'var(--success)' },
  parent: { bg: 'rgba(96,165,250,0.08)', border: 'var(--info-border)', color: 'var(--info)' },
  swimmer: { bg: 'rgba(52,211,153,0.08)', border: 'rgba(52,211,153,0.2)', color: '#34d399' },
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function mfaSummary(factors: MFAFactorRecord[]) {
  if (factors.length === 0) return 'No MFA factors enrolled'
  const verified = factors.filter((factor) => factor.is_verified).length
  return `${verified}/${factors.length} factor${factors.length === 1 ? '' : 's'} verified`
}

// Never surface raw backend error text (e.g. header names, internal codes) directly
// in the UI. Map known-safe messages through, and fall back to a generic message
// for anything that looks technical or is otherwise unrecognized.
function toFriendlyErrorMessage(err: unknown, fallback: string): string {
  if (!(err instanceof ApiError)) return fallback
  const looksTechnical = /header|x-org-id|x-csrf|null|undefined|panic|stack trace/i.test(err.message)
  if (looksTechnical || !err.message.trim()) return fallback
  return err.message
}

function userDisplayName(user: UserRecord): string {
  const name = `${user.firstName ?? ''} ${user.lastName ?? ''}`.trim()
  return name || user.username || user.email
}

export function RolesPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [assignments, setAssignments] = useState<RoleAssignmentRecord[]>([])
  const [mfaFactors, setMFAFactors] = useState<MFAFactorRecord[]>([])
  const [orgUsers, setOrgUsers] = useState<UserRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [userQuery, setUserQuery] = useState('')
  const [form, setForm] = useState({
    target_user_id: '',
    role: 'coach' as RoleOption,
    reason: '',
  })

  useEffect(() => {
    if (!session) return
    let cancelled = false
    setLoading(true)
    setError(null)

    Promise.all([
      fetchRoleAssignments(api, session.user.orgId),
      fetchMFAFactors(api, session.user.orgId),
      fetchUsers(api, session.user.orgId, { limit: 200 }),
    ])
      .then(([nextAssignments, nextFactors, nextUsers]) => {
        if (cancelled) return
        setAssignments(nextAssignments)
        setMFAFactors(nextFactors)
        setOrgUsers(nextUsers.users)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setError(toFriendlyErrorMessage(err, 'Couldn’t load role management data. Try again.'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [api, session])

  const userNameById = useMemo(
    () => new Map(orgUsers.map((u) => [u.id, userDisplayName(u)])),
    [orgUsers],
  )

  const filteredUsers = useMemo(() => {
    const q = userQuery.trim().toLowerCase()
    const sorted = [...orgUsers].sort((a, b) => userDisplayName(a).localeCompare(userDisplayName(b)))
    if (!q) return sorted
    return sorted.filter((user) => {
      const name = userDisplayName(user).toLowerCase()
      const email = (user.email ?? '').toLowerCase()
      return name.includes(q) || email.includes(q)
    })
  }, [orgUsers, userQuery])

  const roleCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const assignment of assignments) {
      counts.set(assignment.role, (counts.get(assignment.role) ?? 0) + 1)
    }
    return counts
  }, [assignments])

  async function handleAssignRole() {
    if (!session || !form.target_user_id.trim() || !form.reason.trim()) return
    setSubmitting(true)
    setError(null)
    try {
      const created = await assignRole(api, session.user.orgId, {
        target_user_id: form.target_user_id.trim(),
        role: form.role,
        reason: form.reason.trim(),
      })
      setAssignments((current) => [created, ...current])
      setForm({ target_user_id: '', role: 'coach', reason: '' })
      setUserQuery('')
    } catch (err) {
      setError(toFriendlyErrorMessage(err, 'Couldn’t assign role. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  if (!session) {
    return <div className="u-p-32px-0" ><EmptyState icon="shield-check" title="Sign in required" description="Authenticate to manage live role assignments." /></div>
  }

  return (
    <div>
      {error ? (
        <div className="u-p-12px-16px u-mb-20px u-fs-fs-sm" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)' }}>
          {error}
        </div>
      ) : null}

      <div className="cols-4 u-gap-12px u-mb-24px" >
        {[
          { label: 'Assignments', value: assignments.length.toString() },
          { label: 'Role types', value: roleCounts.size.toString() },
          { label: 'Your role', value: session.user.role },
          { label: 'Your MFA', value: mfaSummary(mfaFactors) },
        ].map((stat) => (
          <div key={stat.label} className="card-elevated u-p-16px-16px" >
            <p className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</p>
            <p className="u-fs-fs-xl u-ff-font-body" style={{ color: 'var(--text)' }}>{stat.value}</p>
          </div>
        ))}
      </div>

      <div className="split-list u-gap-20px" style={{ alignItems: 'start' }}>
        <div className="card u-p-20px" >
          <h2 className="card-title u-mb-16px" >Assign role</h2>
          <div className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
            <div>
              <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                Target user
              </label>
              <input aria-label="Search users by name or email"
                value={userQuery}
                onChange={(event) => setUserQuery(event.target.value)}
                placeholder="Search users by name or email…"
                className="input-search u-p-8px-12px u-mb-8px u-fs-fs-base u-ff-font-body" style={{ width: '100%' }}
              />
              <select
                value={form.target_user_id}
                onChange={(event) => setForm((current) => ({ ...current, target_user_id: event.target.value }))}
                className="u-p-8px-12px u-fs-fs-base u-ff-font-body" style={{ width: '100%' }}
              >
                <option value="">{filteredUsers.length === 0 ? 'No matching users' : 'Select user…'}</option>
                {filteredUsers.map((u) => (
                  <option key={u.id} value={u.id}>
                    {userDisplayName(u)} — {u.email}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                Role
              </label>
              <select
                value={form.role}
                onChange={(event) => setForm((current) => ({ ...current, role: event.target.value as RoleOption }))}
                className="u-p-8px-12px u-fs-fs-base u-ff-font-body" style={{ width: '100%' }}
              >
                {ROLE_OPTIONS.map((role) => <option key={role} value={role}>{role}</option>)}
              </select>
            </div>
            <div>
              <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                Reason
              </label>
              <textarea aria-label="Document why this access is required."
                value={form.reason}
                onChange={(event) => setForm((current) => ({ ...current, reason: event.target.value }))}
                rows={4}
                placeholder="Document why this access is required."
                className="u-p-8px-12px u-fs-fs-base u-ff-font-body" style={{ width: '100%', resize: 'vertical' }}
              />
            </div>
            <button type="button"
              onClick={handleAssignRole}
              disabled={submitting || !form.target_user_id.trim() || !form.reason.trim()}
              className="u-p-8px-16px u-fs-fs-base u-fw-600 u-ff-font-body" style={{ background: submitting || !form.target_user_id.trim() || !form.reason.trim() ? 'var(--bg-elevated)' : 'var(--accent)', border: submitting || !form.target_user_id.trim() || !form.reason.trim() ? '1px solid var(--border)' : 'none', borderRadius: '8px', color: submitting || !form.target_user_id.trim() || !form.reason.trim() ? 'var(--text-muted)' : '#fff', cursor: submitting || !form.target_user_id.trim() || !form.reason.trim() ? 'not-allowed' : 'pointer' }}
            >
              {submitting ? 'Assigning…' : 'Assign Role'}
            </button>
          </div>
        </div>

        <div className="card" style={{ overflow: 'hidden' }}>
          <div className="u-p-16px-20px" style={{ borderBottom: '1px solid var(--border)' }}>
            <h2 className="card-title">Assignment history</h2>
          </div>
          {loading ? (
            <div className="u-p-24px" >
              <EmptyState icon="shield-check" title="Loading assignments" description="Fetching live role assignments from the API." />
            </div>
          ) : assignments.length === 0 ? (
            <div className="u-p-24px" >
              <EmptyState icon="shield-check" title="No role assignments yet" description="New assignments will appear here as they are created." />
            </div>
          ) : (
            <div className="table-scroll" style={{ overflowX: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--border)' }}>
                    {['Target User', 'Role', 'Reason', 'Actor', 'Assigned'].map((header) => (
                      <th key={header} className="col-head u-p-12px-16px" style={{ textAlign: 'left' }}>
                        {header}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {assignments.map((assignment) => {
                    const colors = ROLE_COLORS[assignment.role] ?? ROLE_COLORS.parent
                    return (
                      <tr key={`${assignment.target_user_id}-${assignment.assigned_at}-${assignment.role}`} style={{ borderBottom: '1px solid var(--border)' }}>
                        <td className="u-p-12px-16px u-fs-fs-sm" style={{ color: 'var(--text)' }}>{userNameById.get(assignment.target_user_id) ?? assignment.target_user_id}</td>
                        <td className="u-p-12px-16px" >
                          <span className="u-fs-fs-xs u-p-2px-8px u-ff-font-body u-fw-700" style={{ borderRadius: '16px', background: colors.bg, color: colors.color, border: `1px solid ${colors.border}` }}>
                            {assignment.role.toUpperCase()}
                          </span>
                        </td>
                        <td className="u-p-12px-16px u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{assignment.reason}</td>
                        <td className="u-p-12px-16px u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{userNameById.get(assignment.actor_user_id) ?? assignment.actor_user_id}</td>
                        <td className="u-p-12px-16px u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{formatDateTime(assignment.assigned_at)}</td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}

          <div className="u-p-16px-20px" style={{ borderTop: '1px solid var(--border)', background: 'var(--bg-surface)' }}>
            <div className="u-gap-8px u-fs-fs-sm" style={{ display: 'flex', alignItems: 'center', color: 'var(--text-subtle)' }}>
              <Icon name="shield-check" size={12} />
              Current session MFA status: {mfaSummary(mfaFactors)}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
