'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Modal } from '@/components/ui/Modal'
import { Table, col } from '@/components/ui/Table'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import { createDataExportRequest, fetchDataExportRequests, fetchHouseholds, type DataExportRequestRecord, type HouseholdRecord } from '@/lib/adminData'

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
  width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)',
  background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', boxSizing: 'border-box',
}

type ExportRow = DataExportRequestRecord & { householdName: string }

export default function DataExportsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [requests, setRequests] = useState<DataExportRequestRecord[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [selectedHouseholdId, setSelectedHouseholdId] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [modalOpen, setModalOpen] = useState(false)

  async function loadData() {
    if (!session?.user.orgId) return
    const [nextRequests, nextHouseholds] = await Promise.all([
      fetchDataExportRequests(api, session.user.orgId),
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
          fetchDataExportRequests(api, session.user.orgId),
          fetchHouseholds(api, session.user.orgId),
        ])
        if (cancelled) return
        setRequests(nextRequests)
        setHouseholds(nextHouseholds)
        if (!selectedHouseholdId && nextHouseholds[0]) setSelectedHouseholdId(nextHouseholds[0].id)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load data export requests.')
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
  }, [api, selectedHouseholdId, session?.user.orgId])

  const householdNameById = useMemo(() => new Map(households.map((household) => [household.id, household.household_name])), [households])

  const rows: ExportRow[] = useMemo(
    () => requests.map((r) => ({ ...r, householdName: householdNameById.get(r.household_id) ?? r.household_id })),
    [requests, householdNameById],
  )

  async function handleCreate() {
    if (!session?.user.orgId || !selectedHouseholdId) return
    setSubmitting(true)
    setError(null)
    try {
      await createDataExportRequest(api, session.user.orgId, selectedHouseholdId)
      await loadData()
      setModalOpen(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to create data export request.')
    } finally {
      setSubmitting(false)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="download" title="Sign in required" description="Authenticate to manage export requests." /></div>
  }

  const columns = [
    col<ExportRow>({ key: 'householdName', label: 'Household', sortable: true }),
    col<ExportRow>({
      key: 'status',
      label: 'Status',
      sortable: true,
      render: (row) => (
        <span className={`badge ${row.status === 'ready' ? 'badge-success' : 'badge-default'}`}>{row.status}</span>
      ),
    }),
    col<ExportRow>({ key: 'requested_at', label: 'Requested', sortable: true, render: (row) => formatDateTime(row.requested_at) }),
    col<ExportRow>({ key: 'expires_at', label: 'Expires', sortable: true, render: (row) => formatDateTime(row.expires_at) }),
    col<ExportRow>({
      key: 'download_url',
      label: 'File',
      sortable: false,
      render: (row) =>
        row.download_url ? (
          <a href={row.download_url} target="_blank" rel="noreferrer" style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none' }}>
            Open artifact
          </a>
        ) : (
          <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>Pending</span>
        ),
    }),
  ]

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1240, margin: '0 auto' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px', marginBottom: '24px', flexWrap: 'wrap' }}>
        <div>
          <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
            Compliance
          </p>
          <h1 className="page-title">Data Export Requests</h1>
          <p className="page-subtitle">A parent asked you to download their family&rsquo;s data — start here.</p>
        </div>
        <button onClick={() => setModalOpen(true)} className="btn btn-primary">
          <Icon name="plus" size={14} />
          New Request
        </button>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
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
        emptyTitle="No export requests yet"
        emptyDescription="Create a request when a family asks for a copy of their data."
      />

      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title="New Export Request"
        description="Generates a downloadable file of everything on file for this household."
        footer={
          <>
            <button onClick={() => setModalOpen(false)} className="btn btn-secondary">Cancel</button>
            <button onClick={() => void handleCreate()} disabled={submitting || !selectedHouseholdId} className="btn btn-primary">
              {submitting ? 'Creating…' : 'Create Request'}
            </button>
          </>
        }
      >
        <div>
          <label style={{ display: 'block', fontSize: '10.5px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '5px' }}>Household</label>
          <select value={selectedHouseholdId} onChange={(event) => setSelectedHouseholdId(event.target.value)} style={FIELD_INPUT_STYLE}>
            {households.map((household) => <option key={household.id} value={household.id}>{household.household_name}</option>)}
          </select>
        </div>
      </Modal>
    </div>
  )
}
