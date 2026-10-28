'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Modal } from '@/components/ui/Modal'
import { Table, col } from '@/components/ui/Table'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { approveDataDeletionRequest, createDataDeletionRequest, fetchDataDeletionRequests, rejectDataDeletionRequest, fetchHouseholds, type DataDeletionRequestRecord, type HouseholdRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

function formatDateTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const FIELD_INPUT_STYLE: React.CSSProperties = {
  width: '100%', padding: '8px 12px', borderRadius: '8px', border: '1px solid var(--border)',
  background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: 'var(--fs-base)', boxSizing: 'border-box',
}

type DeletionRow = DataDeletionRequestRecord & { householdName: string }

export default function DataDeletionsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [requests, setRequests] = useState<DataDeletionRequestRecord[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [selectedHouseholdId, setSelectedHouseholdId] = useState('')
  const [reason, setReason] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [review, setReview] = useState<{ row: DeletionRow; action: 'approve' | 'reject' } | null>(null)
  const [reviewNotes, setReviewNotes] = useState('')
  const [reviewing, setReviewing] = useState(false)
  const [reviewError, setReviewError] = useState<string | null>(null)

  async function loadData() {
    if (!session?.user.orgId) return
    const [nextRequests, nextHouseholds] = await Promise.all([
      fetchDataDeletionRequests(api, session.user.orgId),
      fetchHouseholds(api, session.user.orgId),
    ])
    setRequests(nextRequests)
    setHouseholds(nextHouseholds)
    if (!selectedHouseholdId && nextHouseholds[0]) {
      setSelectedHouseholdId(nextHouseholds[0].id)
    }
  }

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId) {
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const [nextRequests, nextHouseholds] = await Promise.all([
          fetchDataDeletionRequests(api, session.user.orgId),
          fetchHouseholds(api, session.user.orgId),
        ])
        if (cancelled) return
        setRequests(nextRequests)
        setHouseholds(nextHouseholds)
        if (!selectedHouseholdId && nextHouseholds[0]) setSelectedHouseholdId(nextHouseholds[0].id)
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load data deletion requests. Try again.'))
          setRequests([])
          setHouseholds([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
    // selectedHouseholdId is intentionally excluded: it's only read here to
    // seed a default when empty, and including it would refetch immediately
    // after that seed runs.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, session?.user.orgId])

  const householdNameById = useMemo(() => new Map(households.map((household) => [household.id, household.household_name])), [households])

  const rows: DeletionRow[] = useMemo(
    () => requests.map((r) => ({ ...r, householdName: householdNameById.get(r.household_id) ?? r.household_id })),
    [requests, householdNameById],
  )

  async function handleCreate() {
    if (!session?.user.orgId || !selectedHouseholdId) return
    setSubmitting(true)
    setError(null)
    try {
      await createDataDeletionRequest(api, session.user.orgId, {
        household_id: selectedHouseholdId,
        reason: reason.trim(),
      })
      setReason('')
      await loadData()
      setModalOpen(false)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t create data deletion request. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  async function handleReview() {
    if (!session?.user.orgId || !review) return
    if (review.action === 'reject' && !reviewNotes.trim()) {
      setReviewError('Add a note explaining why the request is rejected.')
      return
    }
    setReviewing(true)
    setReviewError(null)
    try {
      if (review.action === 'approve') {
        await approveDataDeletionRequest(api, session.user.orgId, review.row.id, reviewNotes.trim())
      } else {
        await rejectDataDeletionRequest(api, session.user.orgId, review.row.id, reviewNotes.trim())
      }
      setReview(null)
      setReviewNotes('')
      await loadData()
    } catch (err) {
      setReviewError(uiErrorMessage(err, 'Couldn’t review this request. Try again.'))
    } finally {
      setReviewing(false)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="trash-2" title="Sign in required" description="Authenticate to manage deletion requests." /></div>
  }

  const columns = [
    col<DeletionRow>({ key: 'householdName', label: 'Household', sortable: true }),
    col<DeletionRow>({
      key: 'status',
      label: 'Status',
      sortable: true,
      render: (row) => (
        <span className={`badge ${row.status === 'completed' ? 'badge-success' : row.status === 'rejected' ? 'badge-error' : 'badge-default'}`}>
          {row.status}
        </span>
      ),
    }),
    col<DeletionRow>({ key: 'requested_at', label: 'Requested', sortable: true, render: (row) => formatDateTime(row.requested_at) }),
    col<DeletionRow>({ key: 'expires_at', label: 'Review deadline', sortable: true, render: (row) => formatDateTime(row.expires_at) }),
    col<DeletionRow>({ key: 'reason', label: 'Reason', sortable: false, cellStyle: { color: 'var(--text-subtle)' }, render: (row) => row.reason || '—' }),
    col<DeletionRow>({
      key: 'id',
      label: '',
      sortable: false,
      render: (row) => (row.status === 'pending' || row.status === 'reviewing' || row.status === 'failed') ? (
        <div style={{ display: 'flex', gap: '6px', justifyContent: 'flex-end' }}>
          <button className="btn btn-sm btn-secondary" onClick={() => { setReview({ row, action: 'approve' }); setReviewNotes(''); setReviewError(null) }}>
            {row.status === 'failed' ? 'Retry erasure' : 'Approve'}
          </button>
          {row.status !== 'failed' ? (
            <button className="btn btn-sm btn-secondary" style={{ color: 'var(--error)' }} onClick={() => { setReview({ row, action: 'reject' }); setReviewNotes(''); setReviewError(null) }}>
              Reject
            </button>
          ) : null}
        </div>
      ) : null,
    }),
  ]

  return (
    <div className="page">
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px', marginBottom: '24px', flexWrap: 'wrap' }}>
        <div>
          <h1 className="page-title">Data deletion requests</h1>
        </div>
        <button onClick={() => setModalOpen(true)} className="btn btn-primary">
          <Icon name="plus" size={14} />
          New Request
        </button>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: 'var(--fs-sm)' }}>
          {error}
        </div>
      ) : null}

      <Table
        columns={columns}
        data={rows}
        keyField="id"
        loading={loading}
        paginated
        pageSize={20}
        emptyTitle="No deletion requests yet"
        emptyDescription="Create a request when a family asks you to erase their data."
      />

      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title="New deletion request"
        description="Starts a review — this does not delete anything immediately."
        footer={
          <>
            <button onClick={() => setModalOpen(false)} className="btn btn-secondary">Cancel</button>
            <button onClick={() => void handleCreate()} disabled={submitting || !selectedHouseholdId} className="btn btn-primary">
              {submitting ? 'Creating…' : 'Create Request'}
            </button>
          </>
        }
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          <div>
            <label style={{ display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '4px' }}>Household</label>
            <select value={selectedHouseholdId} onChange={(event) => setSelectedHouseholdId(event.target.value)} style={FIELD_INPUT_STYLE}>
              {households.map((household) => <option key={household.id} value={household.id}>{household.household_name}</option>)}
            </select>
          </div>
          <div>
            <label style={{ display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '4px' }}>Reason</label>
            <textarea value={reason} onChange={(event) => setReason(event.target.value)} rows={4} placeholder="Why this family asked for deletion" style={{ ...FIELD_INPUT_STYLE, resize: 'vertical' }} />
          </div>
        </div>
      </Modal>
      <Modal
        open={review !== null}
        onClose={() => setReview(null)}
        title={review?.action === 'approve' ? 'Approve data deletion' : 'Reject deletion request'}
        description={review?.action === 'approve'
          ? `Erases ${review?.row.householdName ?? 'this household'}’s swimmers, contacts, documents, messages, and login accounts. Financial records are kept but anonymized. This cannot be undone.`
          : 'The family’s data is kept. Explain why so the decision is on record.'}
        footer={
          <>
            <button onClick={() => setReview(null)} className="btn btn-secondary">Cancel</button>
            <button onClick={() => void handleReview()} disabled={reviewing} className="btn btn-primary" style={review?.action === 'approve' ? { background: 'var(--error)' } : undefined}>
              {reviewing ? 'Saving…' : review?.action === 'approve' ? 'Approve and erase' : 'Reject request'}
            </button>
          </>
        }
      >
        <label htmlFor="deletion-review-notes" style={{ display: 'block', fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', marginBottom: '6px' }}>
          Review notes{review?.action === 'reject' ? ' (required)' : ' (optional)'}
        </label>
        <textarea id="deletion-review-notes" rows={3} value={reviewNotes} onChange={(e) => setReviewNotes(e.target.value)} style={{ ...FIELD_INPUT_STYLE, resize: 'vertical' }} />
        {reviewError ? <p style={{ marginTop: '8px', fontSize: 'var(--fs-sm)', color: 'var(--error)' }}>{reviewError}</p> : null}
      </Modal>
    </div>
  )
}
