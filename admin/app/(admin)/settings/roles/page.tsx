'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

type Role = 'owner' | 'admin' | 'coach' | 'parent' | 'swimmer' | 'billing' | 'read-only'
type MfaStatus = 'enrolled' | 'not_enrolled' | 'required'

type UserRoleAssignment = {
  id: string
  userId: string
  email: string
  displayName: string
  role: Role
  mfaStatus: MfaStatus
  mfaRequired: boolean
  assignedAt: string
  assignedBy: string
  lastActiveAt: string | null
}

const MOCK_ASSIGNMENTS: UserRoleAssignment[] = [
  { id: 'r1', userId: 'u1', email: 'owner@dareaquatics.com',     displayName: 'Club Owner',         role: 'owner',     mfaStatus: 'enrolled',     mfaRequired: true,  assignedAt: '2026-01-01', assignedBy: 'system',       lastActiveAt: '2026-03-15T14:20:00Z' },
  { id: 'r2', userId: 'u2', email: 'jessica@dareaquatics.com',   displayName: 'Jessica Park',       role: 'admin',     mfaStatus: 'enrolled',     mfaRequired: true,  assignedAt: '2026-01-15', assignedBy: 'stub-owner',  lastActiveAt: '2026-03-15T11:00:00Z' },
  { id: 'r3', userId: 'u3', email: 'coach.marcus@dareaq.com',    displayName: 'Marcus Williams',    role: 'coach',     mfaStatus: 'enrolled',     mfaRequired: true,  assignedAt: '2026-01-20', assignedBy: 'stub-owner',  lastActiveAt: '2026-03-14T17:45:00Z' },
  { id: 'r4', userId: 'u4', email: 'coach.sara@dareaq.com',      displayName: 'Sara Chen',          role: 'coach',     mfaStatus: 'not_enrolled', mfaRequired: true,  assignedAt: '2026-02-01', assignedBy: 'stub-admin',  lastActiveAt: '2026-03-13T09:00:00Z' },
  { id: 'r5', userId: 'u5', email: 'billing@dareaquatics.com',   displayName: 'Finance Team',       role: 'billing',   mfaStatus: 'enrolled',     mfaRequired: true,  assignedAt: '2026-01-10', assignedBy: 'stub-owner',  lastActiveAt: '2026-03-15T10:30:00Z' },
  { id: 'r6', userId: 'u6', email: 'readonly@dareaquatics.com',  displayName: 'Board Observer',     role: 'read-only', mfaStatus: 'not_enrolled', mfaRequired: false, assignedAt: '2026-02-15', assignedBy: 'stub-admin',  lastActiveAt: '2026-03-10T14:00:00Z' },
  { id: 'r7', userId: 'u7', email: 'linda.allen@example.com',    displayName: 'Linda Allen',        role: 'parent',    mfaStatus: 'not_enrolled', mfaRequired: false, assignedAt: '2026-02-01', assignedBy: 'system',      lastActiveAt: '2026-03-15T08:15:00Z' },
  { id: 'r8', userId: 'u8', email: 'carlos.martinez@example.com',displayName: 'Carlos Martinez',    role: 'parent',    mfaStatus: 'not_enrolled', mfaRequired: false, assignedAt: '2026-02-05', assignedBy: 'system',      lastActiveAt: '2026-03-14T19:00:00Z' },
]

const ROLE_CONFIG: Record<Role, { label: string; bg: string; border: string; color: string }> = {
  'owner':     { label: 'Owner',     bg: 'rgba(235,93,30,0.1)',       border: 'rgba(235,93,30,0.3)',   color: 'var(--accent)' },
  'admin':     { label: 'Admin',     bg: 'rgba(99,102,241,0.1)',       border: 'rgba(99,102,241,0.25)', color: '#818cf8' },
  'coach':     { label: 'Coach',     bg: 'rgba(74,222,128,0.08)',      border: 'var(--success-border)', color: 'var(--success)' },
  'billing':   { label: 'Billing',   bg: 'rgba(251,191,36,0.08)',      border: 'var(--warning-border)', color: 'var(--warning)' },
  'read-only': { label: 'Read-Only', bg: 'var(--bg-elevated)',         border: 'var(--border)',         color: 'var(--text-muted)' },
  'parent':    { label: 'Parent',    bg: 'rgba(96,165,250,0.08)',      border: 'var(--info-border)',    color: 'var(--info)' },
  'swimmer':   { label: 'Swimmer',   bg: 'rgba(52,211,153,0.08)',      border: 'rgba(52,211,153,0.2)',  color: '#34d399' },
}

const MFA_CONFIG: Record<MfaStatus, { label: string; color: string; icon: string }> = {
  enrolled:     { label: 'Enrolled',     color: 'var(--success)', icon: '✓' },
  not_enrolled: { label: 'Not Enrolled', color: 'var(--error)',   icon: '✗' },
  required:     { label: 'Required',     color: 'var(--warning)', icon: '⚠' },
}

function fmtDate(d: string) { return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) }
function fmtRelative(d: string) {
  const diff = Date.now() - new Date(d).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

export default function RoleManagementPage() {
  const [roleFilter, setRoleFilter] = useState<Role | 'ALL'>('ALL')
  const [showAssignModal, setShowAssignModal] = useState(false)
  const [, setSelectedUser] = useState<UserRoleAssignment | null>(null)

  const filtered = MOCK_ASSIGNMENTS.filter((a) => roleFilter === 'ALL' || a.role === roleFilter)

  const privilegedRoles: Role[] = ['owner', 'admin', 'coach', 'billing']
  const mfaNonCompliant = MOCK_ASSIGNMENTS.filter(
    (a) => privilegedRoles.includes(a.role) && a.mfaStatus === 'not_enrolled'
  )

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1100px' }}>
      {/* Header */}
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Settings
        </p>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap' }}>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em' }}>
            Role Management
          </h1>
          <button
            onClick={() => setShowAssignModal(true)}
            style={{
              padding: '8px 18px', background: 'var(--accent)', border: 'none', borderRadius: '7px',
              color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer',
              fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '7px',
            }}
            onMouseEnter={(e) => { e.currentTarget.style.background = '#d44f14' }}
            onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--accent)' }}
          >
            <Icon name="plus" size={14} />
            Assign Role
          </button>
        </div>
      </div>

      {/* MFA non-compliance alert */}
      {mfaNonCompliant.length > 0 && (
        <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)', borderRadius: '8px', display: 'flex', gap: '10px', alignItems: 'center' }}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--warning)" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
          </svg>
          <span style={{ fontSize: '12px', color: 'var(--warning)', fontWeight: 500 }}>
            {mfaNonCompliant.length} privileged user{mfaNonCompliant.length > 1 ? 's' : ''} without MFA enrolled:&nbsp;
            {mfaNonCompliant.map((u) => u.displayName).join(', ')}
          </span>
        </div>
      )}

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total Users',      value: MOCK_ASSIGNMENTS.length.toString() },
          { label: 'Privileged',       value: MOCK_ASSIGNMENTS.filter(a => privilegedRoles.includes(a.role)).length.toString() },
          { label: 'MFA Enrolled',     value: MOCK_ASSIGNMENTS.filter(a => a.mfaStatus === 'enrolled').length.toString() },
          { label: 'MFA Missing',      value: mfaNonCompliant.length.toString(), warn: mfaNonCompliant.length > 0 },
          { label: 'Role Types',       value: Array.from(new Set(MOCK_ASSIGNMENTS.map(a => a.role))).length.toString() },
        ].map((s) => (
          <div key={s.label} className="card-elevated" style={{ padding: '14px 16px', borderLeft: (s as {warn?: boolean}).warn ? '3px solid var(--warning)' : '3px solid transparent' }}>
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{s.label}</p>
            <p style={{ fontFamily: 'var(--font-display)', fontSize: '22px', color: (s as {warn?: boolean}).warn ? 'var(--warning)' : 'var(--text)' }}>{s.value}</p>
          </div>
        ))}
      </div>

      {/* Role filter */}
      <div style={{ display: 'flex', gap: '4px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', padding: '3px', width: 'fit-content', marginBottom: '16px', flexWrap: 'wrap' }}>
        {(['ALL', 'owner', 'admin', 'coach', 'billing', 'read-only', 'parent', 'swimmer'] as const).map((r) => (
          <button key={r} onClick={() => setRoleFilter(r)}
            style={{
              padding: '4px 12px', borderRadius: '5px', border: 'none', cursor: 'pointer',
              background: roleFilter === r ? 'var(--bg-elevated)' : 'transparent',
              color: roleFilter === r ? 'var(--text)' : 'var(--text-muted)',
              fontSize: '11px', fontWeight: roleFilter === r ? 600 : 400, fontFamily: 'var(--font-body)',
              boxShadow: roleFilter === r ? 'var(--shadow-xs)' : 'none', transition: 'all 0.15s ease',
            }}
          >{r === 'ALL' ? 'All' : r}</button>
        ))}
      </div>

      {/* Table */}
      <div className="card" style={{ overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--border)' }}>
              {['User', 'Role', 'MFA Status', 'Assigned', 'Assigned By', 'Last Active', ''].map((h) => (
                <th key={h} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {filtered.map((assignment) => {
              const rc = ROLE_CONFIG[assignment.role]
              const mc = MFA_CONFIG[assignment.mfaStatus]
              const mfaAlert = privilegedRoles.includes(assignment.role) && assignment.mfaStatus === 'not_enrolled'
              return (
                <tr key={assignment.id}
                  style={{ borderBottom: '1px solid var(--border)', transition: 'background 0.1s ease' }}
                  onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bg-surface)' }}
                  onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                >
                  <td style={{ padding: '11px 14px' }}>
                    <div>
                      <p style={{ fontSize: '13px', fontWeight: 500, color: 'var(--text)', marginBottom: '2px' }}>{assignment.displayName}</p>
                      <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>{assignment.email}</p>
                    </div>
                  </td>
                  <td style={{ padding: '11px 14px' }}>
                    <span style={{ fontSize: '11px', padding: '2px 8px', borderRadius: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, letterSpacing: '0.04em', background: rc.bg, color: rc.color, border: `1px solid ${rc.border}` }}>
                      {rc.label.toUpperCase()}
                    </span>
                  </td>
                  <td style={{ padding: '11px 14px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                      <span style={{ fontSize: '12px', color: mc.color }}>
                        {mc.icon} {mc.label}
                      </span>
                      {mfaAlert && (
                        <span style={{ fontSize: '10px', padding: '1px 5px', background: 'var(--warning-bg)', color: 'var(--warning)', border: '1px solid var(--warning-border)', borderRadius: '4px', fontFamily: 'var(--font-mono)', fontWeight: 600 }}>
                          REQUIRED
                        </span>
                      )}
                    </div>
                  </td>
                  <td style={{ padding: '11px 14px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
                    {fmtDate(assignment.assignedAt)}
                  </td>
                  <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>
                    {assignment.assignedBy}
                  </td>
                  <td style={{ padding: '11px 14px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
                    {assignment.lastActiveAt ? fmtRelative(assignment.lastActiveAt) : 'Never'}
                  </td>
                  <td style={{ padding: '11px 14px', textAlign: 'right' }}>
                    <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
                      <button
                        onClick={() => setSelectedUser(assignment)}
                        style={{ padding: '4px 10px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '5px', color: 'var(--text-muted)', fontSize: '11px', cursor: 'pointer', fontFamily: 'var(--font-body)', display: 'inline-flex', alignItems: 'center', gap: '4px', transition: 'all 0.15s' }}
                        onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' }}
                        onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-muted)' }}
                      >
                        <Icon name="edit" size={11} />
                        Edit
                      </button>
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {/* Assign role modal */}
      {showAssignModal && (
        <div style={{ position: 'fixed', inset: 0, background: 'var(--bg-overlay)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          onClick={(e) => { if (e.target === e.currentTarget) setShowAssignModal(false) }}>
          <div style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '14px', padding: '32px', width: '440px', boxShadow: 'var(--shadow-lg)' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '20px' }}>Assign Role</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px', marginBottom: '20px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '6px' }}>User Email</label>
                <input placeholder="user@example.com" type="email" style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }} />
              </div>
              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '6px' }}>Role</label>
                <select style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }}>
                  {(['admin', 'coach', 'billing', 'read-only', 'parent'] as Role[]).map((r) => (
                    <option key={r} value={r}>{ROLE_CONFIG[r].label}</option>
                  ))}
                </select>
              </div>
            </div>
            <div style={{ padding: '10px 12px', background: 'var(--info-bg)', border: '1px solid var(--info-border)', borderRadius: '7px', fontSize: '12px', color: 'var(--info)', marginBottom: '20px' }}>
              ℹ Role assignment is audit-logged. Owner assignment requires owner privileges.
            </div>
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button onClick={() => setShowAssignModal(false)} style={{ padding: '8px 18px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Cancel</button>
              <button onClick={() => setShowAssignModal(false)} style={{ padding: '8px 18px', background: 'var(--accent)', border: 'none', borderRadius: '7px', color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Assign</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
