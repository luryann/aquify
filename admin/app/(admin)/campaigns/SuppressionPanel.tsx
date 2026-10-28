'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Modal } from '@/components/ui/Modal'
import { Table, col } from '@/components/ui/Table'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { createSuppression, fetchSuppression, removeSuppression, type SuppressionRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

function formatDateTime(value: string) {
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

export function SuppressionPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [entries, setEntries] = useState<SuppressionRecord[]>([])
  const [email, setEmail] = useState('')
  const [reason, setReason] = useState('manual')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [workingId, setWorkingId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [modalOpen, setModalOpen] = useState(false)

  async function loadEntries() {
    if (!session?.user.orgId) return
    const next = await fetchSuppression(api, session.user.orgId)
    setEntries(next)
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
        const next = await fetchSuppression(api, session.user.orgId)
        if (!cancelled) setEntries(next)
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load suppression entries. Try again.'))
          setEntries([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId])

  const stats = useMemo(() => ({
    total: entries.length,
    bounce: entries.filter((entry) => entry.reason === 'bounce').length,
    complaint: entries.filter((entry) => entry.reason === 'complaint').length,
    manual: entries.filter((entry) => entry.reason === 'manual').length,
  }), [entries])

  const visibleEntries = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return entries
    return entries.filter((e) => e.email.toLowerCase().includes(q))
  }, [entries, search])

  async function handleAdd() {
    if (!session?.user.orgId || !email.trim()) return
    setSubmitting(true)
    setError(null)
    try {
      await createSuppression(api, session.user.orgId, {
        email: email.trim(),
        reason,
      })
      setEmail('')
      setReason('manual')
      await loadEntries()
      setModalOpen(false)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t add suppression entry. Try again.'))
    } finally {
      setSubmitting(false)
    }
  }

  async function handleRemove(id: string) {
    if (!session?.user.orgId) return
    setWorkingId(id)
    setError(null)
    try {
      await removeSuppression(api, session.user.orgId, id)
      await loadEntries()
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t remove suppression entry. Try again.'))
    } finally {
      setWorkingId(null)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px 0' }}><EmptyState icon="mail" title="Sign in required" description="Authenticate to manage suppressed addresses." /></div>
  }

  const columns = [
    col<SuppressionRecord>({ key: 'email', label: 'Email', sortable: true }),
    col<SuppressionRecord>({
      key: 'reason',
      label: 'Reason',
      sortable: true,
      render: (row) => (
        <span className={`badge ${row.reason === 'bounce' ? 'badge-warning' : row.reason === 'complaint' ? 'badge-error' : 'badge-default'}`}>
          {row.reason}
        </span>
      ),
    }),
    col<SuppressionRecord>({ key: 'created_at', label: 'Added', sortable: true, render: (row) => formatDateTime(row.created_at) }),
    col<SuppressionRecord>({ key: 'created_by', label: 'Added by', sortable: true, cellStyle: { color: 'var(--text-muted)' } }),
    col<SuppressionRecord>({
      key: 'actions',
      label: '',
      sortable: false,
      align: 'right',
      render: (row) => (
        <button
          onClick={(e) => { e.stopPropagation(); void handleRemove(row.id) }}
          disabled={workingId === row.id}
          className="btn btn-sm btn-secondary"
        >
          Remove
        </button>
      ),
    }),
  ]

  return (
    <div>
      <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', marginBottom: '20px', lineHeight: 1.5, maxWidth: '640px' }}>
        Addresses here are never emailed again, whether they bounced, complained about a past send, or were added by hand.
      </p>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: 'var(--fs-sm)' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '16px', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 220px', maxWidth: '320px' }}>
          <span style={{ position: 'absolute', left: '11px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }}>
            <Icon name="search" size={14} />
          </span>
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search addresses…"
            style={{ ...FIELD_INPUT_STYLE, paddingLeft: '32px' }}
          />
        </div>
        <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          <span className="badge badge-default">{stats.total} total</span>
          <span className="badge badge-warning">{stats.bounce} bounce</span>
          <span className="badge badge-error">{stats.complaint} complaint</span>
          <span className="badge badge-default">{stats.manual} manual</span>
        </div>
        <button onClick={() => setModalOpen(true)} className="btn btn-primary" style={{ marginLeft: 'auto' }}>
          <Icon name="plus" size={14} />
          Add Suppression
        </button>
      </div>

      <Table
        columns={columns}
        data={visibleEntries}
        keyField="id"
        loading={loading}
        paginated
        pageSize={25}
        emptyTitle={entries.length === 0 ? 'No suppressed addresses' : 'No matches'}
        emptyDescription={entries.length === 0 ? 'The suppression list is currently empty.' : 'Try a different search.'}
      />

      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title="Add suppression"
        description="This address will never be emailed by any future campaign."
        footer={
          <>
            <button onClick={() => setModalOpen(false)} className="btn btn-secondary">Cancel</button>
            <button onClick={() => void handleAdd()} disabled={submitting || !email.trim()} className="btn btn-primary">
              {submitting ? 'Adding…' : 'Add Suppression'}
            </button>
          </>
        }
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          <input value={email} onChange={(event) => setEmail(event.target.value)} placeholder="parent@example.com" style={FIELD_INPUT_STYLE} />
          <select value={reason} onChange={(event) => setReason(event.target.value)} style={FIELD_INPUT_STYLE}>
            <option value="manual">Manual</option>
            <option value="bounce">Bounce</option>
            <option value="complaint">Complaint</option>
          </select>
        </div>
      </Modal>
    </div>
  )
}
