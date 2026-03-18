'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'

type SuppressionEntry = {
  id: string
  email: string
  reason: 'unsubscribed' | 'bounced' | 'complaint' | 'manual'
  suppressedAt: string
  suppressedBy: string
  campaignId: string | null
  campaignName: string | null
}

const MOCK_ENTRIES: SuppressionEntry[] = [
  { id: 'sup1', email: 'parent1@example.com', reason: 'unsubscribed', suppressedAt: '2026-03-10T14:22:00Z', suppressedBy: 'system', campaignId: 'c1', campaignName: 'Spring Registration Reminder' },
  { id: 'sup2', email: 'guardian@test.org',   reason: 'bounced',      suppressedAt: '2026-03-08T09:15:00Z', suppressedBy: 'system', campaignId: 'c2', campaignName: 'Fee Invoice Notice' },
  { id: 'sup3', email: 'user@oldmail.net',    reason: 'complaint',    suppressedAt: '2026-03-05T16:40:00Z', suppressedBy: 'system', campaignId: 'c1', campaignName: 'Spring Registration Reminder' },
  { id: 'sup4', email: 'removed@family.com',  reason: 'manual',       suppressedAt: '2026-02-28T11:00:00Z', suppressedBy: 'admin',  campaignId: null, campaignName: null },
  { id: 'sup5', email: 'optout@swim.club',    reason: 'unsubscribed', suppressedAt: '2026-02-20T08:30:00Z', suppressedBy: 'system', campaignId: 'c3', campaignName: 'Season Schedule Update' },
  { id: 'sup6', email: 'nomail@example.net',  reason: 'bounced',      suppressedAt: '2026-02-15T13:50:00Z', suppressedBy: 'system', campaignId: 'c2', campaignName: 'Fee Invoice Notice' },
]

const REASON_CONFIG: Record<string, { label: string; bg: string; border: string; color: string }> = {
  unsubscribed: { label: 'Unsubscribed', bg: 'var(--info-bg)',    border: 'var(--info-border)',    color: 'var(--info)' },
  bounced:      { label: 'Bounced',      bg: 'var(--error-bg)',   border: 'var(--error-border)',   color: 'var(--error)' },
  complaint:    { label: 'Complaint',    bg: 'var(--warning-bg)', border: 'var(--warning-border)', color: 'var(--warning)' },
  manual:       { label: 'Manual',       bg: 'var(--bg-elevated)', border: 'var(--border)',        color: 'var(--text-muted)' },
}

function fmtDate(d: string) { return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) }

export default function SuppressionPage() {
  const [search, setSearch] = useState('')
  const [reasonFilter, setReasonFilter] = useState<string>('ALL')
  const [showAddModal, setShowAddModal] = useState(false)
  const [addEmail, setAddEmail] = useState('')
  const [addReason, setAddReason] = useState<'manual'>('manual')
  const [confirmRemoveId, setConfirmRemoveId] = useState<string | null>(null)

  const filtered = MOCK_ENTRIES.filter((e) => {
    if (reasonFilter !== 'ALL' && e.reason !== reasonFilter) return false
    if (search && !e.email.toLowerCase().includes(search.toLowerCase())) return false
    return true
  })

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '960px' }}>
      {/* Header */}
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Communications
        </p>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap' }}>
          <div>
            <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em', marginBottom: '4px' }}>
              Suppression List
            </h1>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
              Email addresses excluded from all campaign sends. Suppression is enforced automatically at send time.
            </p>
          </div>
          <button
            onClick={() => setShowAddModal(true)}
            style={{
              padding: '8px 18px', background: 'var(--accent)', border: 'none', borderRadius: '7px',
              color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer',
              fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '7px',
            }}
            onMouseEnter={(e) => { e.currentTarget.style.background = '#d44f14' }}
            onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--accent)' }}
          >
            <Icon name="plus" size={14} />
            Add Address
          </button>
        </div>
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total Suppressed',  value: MOCK_ENTRIES.length.toString() },
          { label: 'Unsubscribed',      value: MOCK_ENTRIES.filter(e => e.reason === 'unsubscribed').length.toString() },
          { label: 'Hard Bounces',      value: MOCK_ENTRIES.filter(e => e.reason === 'bounced').length.toString() },
          { label: 'Complaints',        value: MOCK_ENTRIES.filter(e => e.reason === 'complaint').length.toString() },
        ].map((s) => (
          <div key={s.label} className="card-elevated" style={{ padding: '14px 16px' }}>
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{s.label}</p>
            <p style={{ fontFamily: 'var(--font-display)', fontSize: '22px', color: 'var(--text)' }}>{s.value}</p>
          </div>
        ))}
      </div>

      {/* Postmark notice */}
      <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--info-bg)', border: '1px solid var(--info-border)', borderRadius: '8px', display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
        <Icon name="info" size={14} style={{ color: 'var(--info)', marginTop: '1px', flexShrink: 0 }} />
        <p style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.6, margin: 0 }}>
          Addresses added here are enforced at campaign send time before the Postmark API is called.
          Suppressed recipients receive a delivery record with <code style={{ fontFamily: 'var(--font-mono)', fontSize: '11px' }}>status=suppressed</code> without a network request.
        </p>
      </div>

      {/* Filters */}
      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: '200px', maxWidth: '320px' }}>
          <Icon name="search" size={12} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input
            placeholder="Search email address…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{
              width: '100%', paddingLeft: '30px', paddingRight: '10px', height: '34px',
              background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '6px',
              color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none',
            }}
          />
        </div>
        <div style={{ display: 'flex', gap: '4px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '6px', padding: '3px' }}>
          {['ALL', 'unsubscribed', 'bounced', 'complaint', 'manual'].map((r) => (
            <button
              key={r}
              onClick={() => setReasonFilter(r)}
              style={{
                padding: '4px 12px', borderRadius: '4px', border: 'none', cursor: 'pointer',
                background: reasonFilter === r ? 'var(--bg-elevated)' : 'transparent',
                color: reasonFilter === r ? 'var(--text)' : 'var(--text-muted)',
                fontSize: '11px', fontWeight: reasonFilter === r ? 600 : 400,
                fontFamily: 'var(--font-body)', boxShadow: reasonFilter === r ? 'var(--shadow-xs)' : 'none',
                transition: 'all 0.15s ease', textTransform: 'capitalize',
              }}
            >
              {r === 'ALL' ? 'All' : r}
            </button>
          ))}
        </div>
      </div>

      {/* Table */}
      <div className="card" style={{ overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--border)' }}>
              {['Email Address', 'Reason', 'Suppressed', 'Source', 'Campaign', ''].map((h) => (
                <th key={h} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 ? (
              <tr><td colSpan={6} style={{ padding: '48px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>No suppressed addresses match this filter.</td></tr>
            ) : (
              filtered.map((entry) => {
                const rc = REASON_CONFIG[entry.reason]
                return (
                  <tr
                    key={entry.id}
                    style={{ borderBottom: '1px solid var(--border)', transition: 'background 0.1s ease' }}
                    onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bg-surface)' }}
                    onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                  >
                    <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--text)' }}>
                      {entry.email}
                    </td>
                    <td style={{ padding: '11px 14px' }}>
                      <span style={{ fontSize: '10px', padding: '2px 8px', borderRadius: '20px', fontFamily: 'var(--font-mono)', fontWeight: 600, letterSpacing: '0.04em', background: rc.bg, color: rc.color, border: `1px solid ${rc.border}` }}>
                        {rc.label.toUpperCase()}
                      </span>
                    </td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
                      {fmtDate(entry.suppressedAt)}
                    </td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>
                      {entry.suppressedBy === 'system' ? 'Automatic' : 'Manual (admin)'}
                    </td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-muted)' }}>
                      {entry.campaignName ?? '—'}
                    </td>
                    <td style={{ padding: '11px 14px', textAlign: 'right' }}>
                      <button
                        onClick={() => setConfirmRemoveId(entry.id)}
                        style={{
                          padding: '4px 10px', background: 'var(--error-bg)', border: '1px solid var(--error-border)',
                          borderRadius: '5px', color: 'var(--error)', fontSize: '11px', cursor: 'pointer',
                          fontFamily: 'var(--font-body)', display: 'inline-flex', alignItems: 'center', gap: '4px',
                          transition: 'all 0.15s ease',
                        }}
                        onMouseEnter={(e) => { e.currentTarget.style.background = 'rgba(248,113,113,0.16)' }}
                        onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--error-bg)' }}
                      >
                        <Icon name="x" size={10} />
                        Remove
                      </button>
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
      </div>

      <p style={{ marginTop: '12px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
        {filtered.length} of {MOCK_ENTRIES.length} addresses shown
      </p>

      {/* Add modal */}
      {showAddModal && (
        <div style={{ position: 'fixed', inset: 0, background: 'var(--bg-overlay)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          onClick={(e) => { if (e.target === e.currentTarget) setShowAddModal(false) }}>
          <div style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '14px', padding: '32px', width: '420px', boxShadow: 'var(--shadow-lg)' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '20px' }}>Add to Suppression List</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px', marginBottom: '20px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '6px' }}>Email Address</label>
                <input value={addEmail} onChange={(e) => setAddEmail(e.target.value)} placeholder="user@example.com" type="email"
                  style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }} />
              </div>
              <div>
                <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '6px' }}>Reason</label>
                <select value={addReason} onChange={(e) => setAddReason(e.target.value as 'manual')} style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none' }}>
                  <option value="manual">Manual</option>
                </select>
              </div>
            </div>
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button onClick={() => setShowAddModal(false)} style={{ padding: '8px 18px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Cancel</button>
              <button onClick={() => setShowAddModal(false)} style={{ padding: '8px 18px', background: 'var(--accent)', border: 'none', borderRadius: '7px', color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Add</button>
            </div>
          </div>
        </div>
      )}

      {/* Confirm remove */}
      {confirmRemoveId && (
        <div style={{ position: 'fixed', inset: 0, background: 'var(--bg-overlay)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          onClick={(e) => { if (e.target === e.currentTarget) setConfirmRemoveId(null) }}>
          <div style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '14px', padding: '28px', width: '380px', boxShadow: 'var(--shadow-lg)' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '18px', fontWeight: 400, color: 'var(--text)', marginBottom: '10px' }}>Remove from suppression?</h3>
            <p style={{ fontSize: '13px', color: 'var(--text-subtle)', lineHeight: 1.6, marginBottom: '20px' }}>
              This address will be re-enabled for campaign sends. Make sure the email is valid before proceeding.
            </p>
            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button onClick={() => setConfirmRemoveId(null)} style={{ padding: '8px 18px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Cancel</button>
              <button onClick={() => setConfirmRemoveId(null)} style={{ padding: '8px 18px', background: 'var(--error)', border: 'none', borderRadius: '7px', color: '#fff', fontSize: '13px', fontWeight: 600, cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Remove</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
