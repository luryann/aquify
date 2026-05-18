'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
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

export default function DataExportsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [requests, setRequests] = useState<DataExportRequestRecord[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [selectedHouseholdId, setSelectedHouseholdId] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

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

  async function handleCreate() {
    if (!session?.user.orgId || !selectedHouseholdId) return
    setSubmitting(true)
    setError(null)
    try {
      await createDataExportRequest(api, session.user.orgId, selectedHouseholdId)
      await loadData()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to create data export request.')
    } finally {
      setSubmitting(false)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="download" title="Sign in required" description="Authenticate to manage live export requests." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1120, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Compliance
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', fontWeight: 400, color: 'var(--text)', marginBottom: '6px' }}>
          Data Export Requests
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
          Live GDPR/household export requests with backend-issued expiration and artifact status.
        </p>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr', gap: '20px', alignItems: 'start' }}>
        <div className="card" style={{ padding: '18px' }}>
          <h2 style={{ fontSize: '14px', color: 'var(--text)', marginBottom: '12px' }}>Open Request</h2>
          <select value={selectedHouseholdId} onChange={(event) => setSelectedHouseholdId(event.target.value)} style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', marginBottom: '12px' }}>
            {households.map((household) => <option key={household.id} value={household.id}>{household.household_name}</option>)}
          </select>
          <button onClick={() => void handleCreate()} disabled={submitting || !selectedHouseholdId} style={{ padding: '9px 14px', borderRadius: '8px', border: 'none', background: 'var(--accent)', color: '#fff', fontSize: '12px', fontWeight: 600, cursor: submitting ? 'not-allowed' : 'pointer' }}>
            {submitting ? 'Creating…' : 'Create Export Request'}
          </button>
        </div>

        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
            <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Live Requests</h2>
          </div>
          {loading ? (
            <div style={{ padding: '20px' }}><EmptyState icon="download" title="Loading requests" description="Fetching live compliance export requests." /></div>
          ) : requests.length === 0 ? (
            <div style={{ padding: '20px' }}><EmptyState icon="download" title="No export requests yet" description="Create a request to populate the compliance queue." /></div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)' }}>
                  {['Household', 'Status', 'Requested', 'Expires', 'Download'].map((header) => (
                    <th key={header} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>{header}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {requests.map((request) => (
                  <tr key={request.id} style={{ borderBottom: '1px solid var(--border)' }}>
                    <td style={{ padding: '11px 14px', fontSize: '13px', color: 'var(--text)' }}>{householdNameById.get(request.household_id) ?? request.household_id}</td>
                    <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '11px', color: request.status === 'ready' ? 'var(--success)' : 'var(--text-subtle)' }}>{request.status}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatDateTime(request.requested_at)}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatDateTime(request.expires_at)}</td>
                    <td style={{ padding: '11px 14px' }}>
                      {request.download_url ? <a href={request.download_url} target="_blank" rel="noreferrer" style={{ fontSize: '12px', color: 'var(--accent)', textDecoration: 'none' }}>Open artifact</a> : <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>Pending</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}
