'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Modal } from '@/components/ui/Modal'
import { Table, col } from '@/components/ui/Table'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { createDataExportRequest, fetchDataExportRequests, fetchHouseholds, type DataExportRequestRecord, type HouseholdRecord } from '@/lib/adminData'
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

const FIELD_INPUT_STYLE: React.CSSProperties = { width: '100%' }

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
          setError(uiErrorMessage(err, 'Couldn’t load data export requests. Try again.'))
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
      setError(uiErrorMessage(err, 'Couldn’t create data export request. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  if (!session) {
    return <div className="u-p-32px" ><EmptyState icon="download" title="Sign in required" description="Authenticate to manage export requests." /></div>
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
          <a href={row.download_url} target="_blank" rel="noreferrer" className="u-fs-fs-sm" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>
            Open artifact
          </a>
        ) : (
          <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Pending</span>
        ),
    }),
  ]

  return (
    <div className="page">
      <div className="u-gap-16px u-mb-24px" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', flexWrap: 'wrap' }}>
        <div>
          <h1 className="page-title">Data export requests</h1>
        </div>
        <button type="button" onClick={() => setModalOpen(true)} className="btn btn-primary">
          <Icon name="plus" size={14} />
          New request
        </button>
      </div>

      {error ? (
        <div className="u-mb-20px u-p-12px-16px u-fs-fs-sm" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' }}>
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
        title="New export request"
        description="Generates a downloadable file of everything on file for this household."
        footer={
          <>
            <button type="button" onClick={() => setModalOpen(false)} className="btn btn-secondary">Cancel</button>
            <button type="button" onClick={() => void handleCreate()} disabled={submitting || !selectedHouseholdId} className="btn btn-primary">
              {submitting ? 'Creating…' : 'Create Request'}
            </button>
          </>
        }
      >
        <div>
          <label className="u-fs-fs-sm u-ff-font-body u-mb-4px" style={{ display: 'block', color: 'var(--text-muted)' }}>Household</label>
          <select value={selectedHouseholdId} onChange={(event) => setSelectedHouseholdId(event.target.value)} style={FIELD_INPUT_STYLE}>
            {households.map((household) => <option key={household.id} value={household.id}>{household.household_name}</option>)}
          </select>
        </div>
      </Modal>
    </div>
  )
}
