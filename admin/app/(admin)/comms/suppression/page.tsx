'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import { createSuppression, fetchSuppression, removeSuppression, type SuppressionRecord } from '@/lib/adminData'

function formatDateTime(value: string) {
  return new Date(value).toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export default function SuppressionPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [entries, setEntries] = useState<SuppressionRecord[]>([])
  const [email, setEmail] = useState('')
  const [reason, setReason] = useState('manual')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [workingId, setWorkingId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

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
          setError(err instanceof ApiError ? err.message : 'Unable to load suppression entries.')
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
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to add suppression entry.')
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
      setError(err instanceof ApiError ? err.message : 'Unable to remove suppression entry.')
    } finally {
      setWorkingId(null)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="mail" title="Sign in required" description="Authenticate to manage the live suppression list." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1080, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Communications
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', fontWeight: 400, color: 'var(--text)', marginBottom: '6px' }}>
          Suppression List
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
          Bounce, complaint, and manual suppression state from the live communications service.
        </p>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'Total', value: String(stats.total) },
          { label: 'Bounce', value: String(stats.bounce) },
          { label: 'Complaint', value: String(stats.complaint) },
          { label: 'Manual', value: String(stats.manual) },
        ].map((stat) => (
          <div key={stat.label} className="card-elevated" style={{ padding: '14px 16px' }}>
            <div style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>{stat.label}</div>
            <div style={{ fontSize: '22px', fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>{stat.value}</div>
          </div>
        ))}
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr', gap: '20px', alignItems: 'start' }}>
        <div className="card" style={{ padding: '18px' }}>
          <h2 style={{ fontSize: '14px', color: 'var(--text)', marginBottom: '12px' }}>Add Entry</h2>
          <input value={email} onChange={(event) => setEmail(event.target.value)} placeholder="parent@example.com" style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', marginBottom: '12px' }} />
          <select value={reason} onChange={(event) => setReason(event.target.value)} style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', marginBottom: '12px' }}>
            <option value="manual">Manual</option>
            <option value="bounce">Bounce</option>
            <option value="complaint">Complaint</option>
          </select>
          <button onClick={() => void handleAdd()} disabled={submitting || !email.trim()} style={{ padding: '9px 14px', borderRadius: '8px', border: 'none', background: 'var(--accent)', color: '#fff', fontSize: '12px', fontWeight: 600, cursor: submitting ? 'not-allowed' : 'pointer' }}>
            {submitting ? 'Adding…' : 'Add Suppression'}
          </button>
        </div>

        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
            <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Live Entries</h2>
          </div>
          {loading ? (
            <div style={{ padding: '20px' }}><EmptyState icon="mail" title="Loading suppression list" description="Fetching current suppression entries from the backend." /></div>
          ) : entries.length === 0 ? (
            <div style={{ padding: '20px' }}><EmptyState icon="mail" title="No suppressed emails" description="The live suppression list is currently empty." /></div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)' }}>
                  {['Email', 'Reason', 'Created', 'Actor', ''].map((header) => (
                    <th key={header} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>{header}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.id} style={{ borderBottom: '1px solid var(--border)' }}>
                    <td style={{ padding: '11px 14px', fontSize: '13px', color: 'var(--text)' }}>{entry.email}</td>
                    <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '11px', color: entry.reason === 'bounce' ? 'var(--warning)' : entry.reason === 'complaint' ? 'var(--error)' : 'var(--text-subtle)' }}>{entry.reason}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>{formatDateTime(entry.created_at)}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-muted)' }}>{entry.created_by}</td>
                    <td style={{ padding: '11px 14px' }}>
                      <button onClick={() => void handleRemove(entry.id)} disabled={workingId === entry.id} style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '12px', cursor: workingId === entry.id ? 'not-allowed' : 'pointer' }}>
                        Remove
                      </button>
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
