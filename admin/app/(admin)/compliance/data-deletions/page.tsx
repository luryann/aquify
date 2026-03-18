'use client'

import { useState } from 'react'

type DeletionStatus = 'pending_review' | 'approved' | 'processing' | 'completed' | 'rejected'

type DataDeletionRequest = {
  id: string
  householdId: string
  householdName: string
  requestedBy: string
  requestedAt: string
  reviewDeadline: string
  completedAt: string | null
  status: DeletionStatus
  reviewNote: string | null
  scope: string[]
  hasActiveInvoices: boolean
  hasOpenRegistrations: boolean
}

const MOCK_REQUESTS: DataDeletionRequest[] = [
  {
    id: 'ddel1', householdId: 'hh5', householdName: 'Thompson Family',
    requestedBy: 'parent (dan.thompson@example.com)', requestedAt: '2026-03-10T14:00:00Z',
    reviewDeadline: '2026-04-09T14:00:00Z', completedAt: null, status: 'pending_review',
    reviewNote: null, scope: ['profile', 'swimmers', 'registrations', 'invoices', 'commitments'],
    hasActiveInvoices: true, hasOpenRegistrations: false,
  },
  {
    id: 'ddel2', householdId: 'hh9', householdName: 'Garcia Household',
    requestedBy: 'parent (maria.garcia@example.com)', requestedAt: '2026-03-01T09:00:00Z',
    reviewDeadline: '2026-03-31T09:00:00Z', completedAt: null, status: 'approved',
    reviewNote: 'Household confirmed they are leaving the club. No outstanding balances.', scope: ['profile', 'swimmers', 'all_data'],
    hasActiveInvoices: false, hasOpenRegistrations: false,
  },
  {
    id: 'ddel3', householdId: 'hh3', householdName: 'Torres Family',
    requestedBy: 'admin', requestedAt: '2026-02-15T11:00:00Z',
    reviewDeadline: '2026-03-17T11:00:00Z', completedAt: '2026-03-10T16:30:00Z', status: 'completed',
    reviewNote: 'Account fully anonymized. Financial records retained for tax compliance (7-year rule).', scope: ['profile', 'swimmers'],
    hasActiveInvoices: false, hasOpenRegistrations: false,
  },
  {
    id: 'ddel4', householdId: 'hh11', householdName: 'Kim Family',
    requestedBy: 'parent (park.kim@example.com)', requestedAt: '2026-02-20T14:00:00Z',
    reviewDeadline: '2026-03-21T14:00:00Z', completedAt: '2026-03-05T12:00:00Z', status: 'rejected',
    reviewNote: 'Rejected: active invoice balance of $840. Deletion cannot proceed until balance is resolved.',
    scope: ['all_data'], hasActiveInvoices: true, hasOpenRegistrations: true,
  },
]

const STATUS_CONFIG: Record<DeletionStatus, { label: string; bg: string; border: string; color: string }> = {
  pending_review: { label: 'Pending Review', bg: 'var(--warning-bg)', border: 'var(--warning-border)', color: 'var(--warning)' },
  approved:       { label: 'Approved',       bg: 'var(--info-bg)',    border: 'var(--info-border)',    color: 'var(--info)' },
  processing:     { label: 'Processing',     bg: 'var(--info-bg)',    border: 'var(--info-border)',    color: 'var(--info)' },
  completed:      { label: 'Completed',      bg: 'var(--success-bg)', border: 'var(--success-border)', color: 'var(--success)' },
  rejected:       { label: 'Rejected',       bg: 'var(--error-bg)',   border: 'var(--error-border)',   color: 'var(--error)' },
}

function fmtDt(d: string) { return new Date(d).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) }
function fmtDate(d: string) { return new Date(d).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) }

function daysUntil(d: string): number {
  return Math.ceil((new Date(d).getTime() - Date.now()) / 86400000)
}

export default function DataDeletionsPage() {
  const [statusFilter, setStatusFilter] = useState<DeletionStatus | 'ALL'>('ALL')
  const [reviewModal, setReviewModal] = useState<DataDeletionRequest | null>(null)
  const [reviewAction, setReviewAction] = useState<'approve' | 'reject' | null>(null)
  const [reviewNote, setReviewNote] = useState('')

  const filtered = MOCK_REQUESTS.filter((r) => statusFilter === 'ALL' || r.status === statusFilter)

  function handleReview(req: DataDeletionRequest) {
    setReviewModal(req)
    setReviewAction(null)
    setReviewNote('')
  }

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1100px' }}>
      {/* Header */}
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Compliance
        </p>
        <div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em', marginBottom: '6px' }}>
            Data Deletion Requests
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
            Right to erasure requests (GDPR Art. 17 / CCPA). Each request requires manual review within the configured window.
          </p>
        </div>
      </div>

      {/* Warning notice */}
      <div style={{ padding: '12px 16px', marginBottom: '20px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)', borderRadius: '8px', display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="var(--warning)" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0, marginTop: '1px' }}>
          <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
          <line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" />
        </svg>
        <p style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.6, margin: 0 }}>
          Deletion is <strong style={{ color: 'var(--text)' }}>irreversible</strong>. Financial records (invoices, payments, ledger entries) are retained
          for tax compliance per the 7-year rule even after approval. Review deadline is <strong style={{ color: 'var(--text)' }}>30 days</strong> from request
          (COMPLIANCE_DELETION_REVIEW_DAYS).
        </p>
      </div>

      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total Requests',  value: MOCK_REQUESTS.length.toString() },
          { label: 'Pending Review',  value: MOCK_REQUESTS.filter(r => r.status === 'pending_review').length.toString(), warn: true },
          { label: 'Completed',       value: MOCK_REQUESTS.filter(r => r.status === 'completed').length.toString() },
          { label: 'Rejected',        value: MOCK_REQUESTS.filter(r => r.status === 'rejected').length.toString() },
        ].map((s) => (
          <div key={s.label} className="card-elevated" style={{ padding: '14px 16px', borderLeft: s.warn ? '3px solid var(--warning)' : '3px solid transparent' }}>
            <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{s.label}</p>
            <p style={{ fontFamily: 'var(--font-display)', fontSize: '22px', color: s.warn ? 'var(--warning)' : 'var(--text)' }}>{s.value}</p>
          </div>
        ))}
      </div>

      {/* Status filter */}
      <div style={{ display: 'flex', gap: '4px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', padding: '3px', width: 'fit-content', marginBottom: '16px' }}>
        {(['ALL', 'pending_review', 'approved', 'processing', 'completed', 'rejected'] as const).map((s) => (
          <button key={s} onClick={() => setStatusFilter(s)}
            style={{
              padding: '4px 12px', borderRadius: '5px', border: 'none', cursor: 'pointer',
              background: statusFilter === s ? 'var(--bg-elevated)' : 'transparent',
              color: statusFilter === s ? 'var(--text)' : 'var(--text-muted)',
              fontSize: '11px', fontWeight: statusFilter === s ? 600 : 400, fontFamily: 'var(--font-body)',
              boxShadow: statusFilter === s ? 'var(--shadow-xs)' : 'none', transition: 'all 0.15s ease',
            }}
          >{s === 'ALL' ? 'All' : s.replace('_', ' ')}</button>
        ))}
      </div>

      {/* Table */}
      <div className="card" style={{ overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ borderBottom: '1px solid var(--border)' }}>
              {['Household', 'Requested By', 'Requested', 'Review By', 'Status', 'Flags', ''].map((h) => (
                <th key={h} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {filtered.map((req) => {
              const sc = STATUS_CONFIG[req.status]
              const days = daysUntil(req.reviewDeadline)
              const reviewUrgent = req.status === 'pending_review' && days <= 7
              return (
                <tr key={req.id}
                  style={{ borderBottom: '1px solid var(--border)', transition: 'background 0.1s ease' }}
                  onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bg-surface)' }}
                  onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
                >
                  <td style={{ padding: '11px 14px', fontSize: '13px', fontWeight: 500, color: 'var(--text)' }}>
                    {req.householdName}
                  </td>
                  <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>
                    {req.requestedBy}
                  </td>
                  <td style={{ padding: '11px 14px', fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
                    {fmtDt(req.requestedAt)}
                  </td>
                  <td style={{ padding: '11px 14px' }}>
                    <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: reviewUrgent ? 'var(--error)' : 'var(--text-subtle)' }}>
                      {req.status === 'completed' || req.status === 'rejected' ? '—' : (
                        <>
                          {fmtDate(req.reviewDeadline)}
                          {req.status === 'pending_review' && (
                            <span style={{ marginLeft: '6px', fontSize: '10px', color: reviewUrgent ? 'var(--error)' : 'var(--warning)' }}>
                              ({days > 0 ? `${days}d left` : 'overdue'})
                            </span>
                          )}
                        </>
                      )}
                    </span>
                  </td>
                  <td style={{ padding: '11px 14px' }}>
                    <span style={{ fontSize: '10px', padding: '2px 7px', borderRadius: '20px', fontFamily: 'var(--font-mono)', fontWeight: 600, letterSpacing: '0.04em', background: sc.bg, color: sc.color, border: `1px solid ${sc.border}` }}>
                      {sc.label.toUpperCase()}
                    </span>
                  </td>
                  <td style={{ padding: '11px 14px' }}>
                    <div style={{ display: 'flex', gap: '4px' }}>
                      {req.hasActiveInvoices && (
                        <span style={{ fontSize: '10px', padding: '1px 6px', background: 'var(--warning-bg)', color: 'var(--warning)', border: '1px solid var(--warning-border)', borderRadius: '4px', fontFamily: 'var(--font-mono)', fontWeight: 600 }}>
                          INVOICE
                        </span>
                      )}
                      {req.hasOpenRegistrations && (
                        <span style={{ fontSize: '10px', padding: '1px 6px', background: 'var(--info-bg)', color: 'var(--info)', border: '1px solid var(--info-border)', borderRadius: '4px', fontFamily: 'var(--font-mono)', fontWeight: 600 }}>
                          ACTIVE REG
                        </span>
                      )}
                      {!req.hasActiveInvoices && !req.hasOpenRegistrations && (
                        <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>—</span>
                      )}
                    </div>
                  </td>
                  <td style={{ padding: '11px 14px', textAlign: 'right' }}>
                    {req.status === 'pending_review' && (
                      <button
                        onClick={() => handleReview(req)}
                        style={{
                          padding: '5px 12px', background: 'var(--accent)', border: 'none', borderRadius: '5px',
                          color: '#fff', fontSize: '11px', fontWeight: 600, cursor: 'pointer',
                          fontFamily: 'var(--font-body)',
                        }}
                        onMouseEnter={(e) => { e.currentTarget.style.background = '#d44f14' }}
                        onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--accent)' }}
                      >
                        Review
                      </button>
                    )}
                    {req.status === 'approved' && (
                      <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--info)' }}>Awaiting job</span>
                    )}
                    {(req.status === 'completed' || req.status === 'rejected') && req.reviewNote && (
                      <button style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', textDecoration: 'underline' }}>
                        View note
                      </button>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {/* Review modal */}
      {reviewModal && (
        <div style={{ position: 'fixed', inset: 0, background: 'var(--bg-overlay)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          onClick={(e) => { if (e.target === e.currentTarget) setReviewModal(null) }}>
          <div style={{ background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '14px', padding: '32px', width: '500px', boxShadow: 'var(--shadow-lg)' }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '6px' }}>
              Review Deletion Request
            </h3>
            <p style={{ fontSize: '12px', color: 'var(--text-subtle)', marginBottom: '20px' }}>
              Household: <strong style={{ color: 'var(--text)' }}>{reviewModal.householdName}</strong> · Requested by {reviewModal.requestedBy}
            </p>

            {/* Flags */}
            {(reviewModal.hasActiveInvoices || reviewModal.hasOpenRegistrations) && (
              <div style={{ padding: '10px 12px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)', borderRadius: '7px', fontSize: '12px', color: 'var(--warning)', marginBottom: '16px' }}>
                ⚠ {reviewModal.hasActiveInvoices && 'Active invoice balance · '}
                {reviewModal.hasOpenRegistrations && 'Open registrations · '}
                Resolution required before deletion can proceed.
              </div>
            )}

            {/* Action selection */}
            <div style={{ display: 'flex', gap: '8px', marginBottom: '16px' }}>
              {(['approve', 'reject'] as const).map((a) => (
                <button key={a} onClick={() => setReviewAction(a)}
                  style={{
                    flex: 1, padding: '10px', borderRadius: '8px', border: `2px solid ${reviewAction === a ? (a === 'approve' ? 'var(--success)' : 'var(--error)') : 'var(--border)'}`,
                    background: reviewAction === a ? (a === 'approve' ? 'var(--success-bg)' : 'var(--error-bg)') : 'var(--bg-surface)',
                    color: reviewAction === a ? (a === 'approve' ? 'var(--success)' : 'var(--error)') : 'var(--text-subtle)',
                    fontSize: '13px', fontWeight: 600, cursor: 'pointer', fontFamily: 'var(--font-body)',
                    textTransform: 'capitalize', transition: 'all 0.15s ease',
                  }}>
                  {a}
                </button>
              ))}
            </div>

            <div style={{ marginBottom: '20px' }}>
              <label style={{ display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '6px' }}>
                Review Note (required)
              </label>
              <textarea value={reviewNote} onChange={(e) => setReviewNote(e.target.value)} rows={3}
                placeholder="Document your decision reason…"
                style={{ width: '100%', padding: '9px 12px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text)', fontSize: '13px', fontFamily: 'var(--font-body)', outline: 'none', resize: 'vertical' }} />
            </div>

            <div style={{ display: 'flex', gap: '10px', justifyContent: 'flex-end' }}>
              <button onClick={() => setReviewModal(null)} style={{ padding: '9px 20px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '7px', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer', fontFamily: 'var(--font-body)' }}>Cancel</button>
              <button
                onClick={() => setReviewModal(null)}
                disabled={!reviewAction || !reviewNote}
                style={{
                  padding: '9px 20px', borderRadius: '7px', border: 'none', fontSize: '13px', fontWeight: 600, cursor: reviewAction && reviewNote ? 'pointer' : 'not-allowed', fontFamily: 'var(--font-body)',
                  background: !reviewAction || !reviewNote ? 'var(--bg-elevated)' : (reviewAction === 'approve' ? 'var(--success)' : 'var(--error)'),
                  color: !reviewAction || !reviewNote ? 'var(--text-muted)' : '#fff',
                }}
              >
                {reviewAction === 'approve' ? 'Approve Deletion' : reviewAction === 'reject' ? 'Reject Request' : 'Submit'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
