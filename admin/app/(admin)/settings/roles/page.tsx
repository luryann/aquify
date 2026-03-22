'use client'

import { useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { assignRole, fetchMFAFactors, fetchRoleAssignments, type MFAFactorRecord, type RoleAssignmentRecord } from '@/lib/adminData'

const ROLE_OPTIONS = ['owner', 'admin', 'coach', 'parent', 'swimmer', 'billing', 'read-only'] as const
type RoleOption = (typeof ROLE_OPTIONS)[number]

const ROLE_COLORS: Record<string, { bg: string; border: string; color: string }> = {
  owner: { bg: 'rgba(235,93,30,0.1)', border: 'rgba(235,93,30,0.3)', color: 'var(--accent)' },
  admin: { bg: 'rgba(99,102,241,0.1)', border: 'rgba(99,102,241,0.25)', color: '#818cf8' },
  coach: { bg: 'rgba(74,222,128,0.08)', border: 'var(--success-border)', color: 'var(--success)' },
  billing: { bg: 'rgba(251,191,36,0.08)', border: 'var(--warning-border)', color: 'var(--warning)' },
  'read-only': { bg: 'var(--bg-elevated)', border: 'var(--border)', color: 'var(--text-muted)' },
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

export default function RoleManagementPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [assignments, setAssignments] = useState<RoleAssignmentRecord[]>([])
  const [mfaFactors, setMFAFactors] = useState<MFAFactorRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
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
      fetchMFAFactors(api),
    ])
      .then(([nextAssignments, nextFactors]) => {
        if (cancelled) return
        setAssignments(nextAssignments)
        setMFAFactors(nextFactors)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        if (err instanceof ApiError) {
          setError(err.message)
          return
        }
        setError('Unable to load role management data.')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [api, session])

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
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Unable to assign role.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="shield-check" title="Sign in required" description="Authenticate to manage live role assignments." /></div>
  }

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1100px' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Settings
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em', marginBottom: '6px' }}>
          Role Management
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
          Live role assignments and your current MFA enrollment status.
        </p>
      </div>

      {error ? (
        <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Assignments', value: assignments.length.toString() },
          { label: 'Role Types', value: roleCounts.size.toString() },
          { label: 'Your Role', value: session.user.role },
          { label: 'Your MFA', value: mfaSummary(mfaFactors) },
        ].map((stat) => (
          <div key={stat.label} className="card-elevated" style={{ padding: '14px 16px' }}>
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{stat.label}</p>
            <p style={{ fontSize: '18px', fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{stat.value}</p>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr', gap: '20px', alignItems: 'start' }}>
        <div className="card" style={{ padding: '20px' }}>
          <h2 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)', marginBottom: '16px' }}>Assign Role</h2>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
            <div>
              <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', textTransform: 'uppercase' }}>
                Target User ID
              </label>
              <input
                value={form.target_user_id}
                onChange={(event) => setForm((current) => ({ ...current, target_user_id: event.target.value }))}
                placeholder="user-123"
                style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }}
              />
            </div>
            <div>
              <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', textTransform: 'uppercase' }}>
                Role
              </label>
              <select
                value={form.role}
                onChange={(event) => setForm((current) => ({ ...current, role: event.target.value as RoleOption }))}
                style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }}
              >
                {ROLE_OPTIONS.map((role) => <option key={role} value={role}>{role}</option>)}
              </select>
            </div>
            <div>
              <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', marginBottom: '6px', textTransform: 'uppercase' }}>
                Reason
              </label>
              <textarea
                value={form.reason}
                onChange={(event) => setForm((current) => ({ ...current, reason: event.target.value }))}
                rows={4}
                placeholder="Document why this access is required."
                style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none', resize: 'vertical' }}
              />
            </div>
            <button
              onClick={handleAssignRole}
              disabled={submitting || !form.target_user_id.trim() || !form.reason.trim()}
              style={{
                padding: '9px 16px',
                background: submitting || !form.target_user_id.trim() || !form.reason.trim() ? 'var(--bg-elevated)' : 'var(--accent)',
                border: submitting || !form.target_user_id.trim() || !form.reason.trim() ? '1px solid var(--border)' : 'none',
                borderRadius: '7px',
                color: submitting || !form.target_user_id.trim() || !form.reason.trim() ? 'var(--text-muted)' : '#fff',
                fontSize: '13px',
                fontWeight: 600,
                cursor: submitting || !form.target_user_id.trim() || !form.reason.trim() ? 'not-allowed' : 'pointer',
                fontFamily: 'var(--font-body)',
              }}
            >
              {submitting ? 'Assigning…' : 'Assign Role'}
            </button>
          </div>
        </div>

        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
            <h2 style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>Assignment History</h2>
          </div>
          {loading ? (
            <div style={{ padding: '24px' }}>
              <EmptyState icon="shield-check" title="Loading assignments" description="Fetching live role assignments from the API." />
            </div>
          ) : assignments.length === 0 ? (
            <div style={{ padding: '24px' }}>
              <EmptyState icon="shield-check" title="No role assignments yet" description="New assignments will appear here as they are created." />
            </div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)' }}>
                  {['Target User', 'Role', 'Reason', 'Actor', 'Assigned'].map((header) => (
                    <th key={header} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
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
                      <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text)' }}>{assignment.target_user_id}</td>
                      <td style={{ padding: '11px 14px' }}>
                        <span style={{ fontSize: '11px', padding: '2px 8px', borderRadius: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, background: colors.bg, color: colors.color, border: `1px solid ${colors.border}` }}>
                          {assignment.role.toUpperCase()}
                        </span>
                      </td>
                      <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>{assignment.reason}</td>
                      <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{assignment.actor_user_id}</td>
                      <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text-muted)' }}>{formatDateTime(assignment.assigned_at)}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}

          <div style={{ padding: '14px 18px', borderTop: '1px solid var(--border)', background: 'var(--bg-surface)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--text-subtle)', fontSize: '12px' }}>
              <Icon name="shield-check" size={12} />
              Current session MFA status: {mfaSummary(mfaFactors)}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
