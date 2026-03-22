'use client'

import Link from 'next/link'
import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import { fetchSwimmerBestTimes, fetchSwimmers, type BestTimeRecord, type SwimmerRecord } from '@/lib/adminData'

function formatSwimTime(milliseconds?: number | null) {
  if (milliseconds == null) return 'NT'
  const totalSeconds = milliseconds / 1000
  if (totalSeconds < 60) return totalSeconds.toFixed(2)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = (totalSeconds % 60).toFixed(2).padStart(5, '0')
  return `${minutes}:${seconds}`
}

export default function BestTimesPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [selectedSwimmerId, setSelectedSwimmerId] = useState('')
  const [bestTimes, setBestTimes] = useState<BestTimeRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

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
        const nextSwimmers = await fetchSwimmers(api, session.user.orgId)
        if (cancelled) return
        setSwimmers(nextSwimmers)
        const initialId = selectedSwimmerId || nextSwimmers[0]?.id || ''
        setSelectedSwimmerId(initialId)
        if (initialId) {
          const nextBestTimes = await fetchSwimmerBestTimes(api, session.user.orgId, initialId)
          if (!cancelled) setBestTimes(nextBestTimes)
        } else {
          setBestTimes([])
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load live best times.')
          setSwimmers([])
          setBestTimes([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, selectedSwimmerId, session?.user.orgId])

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId || !selectedSwimmerId) return
      try {
        const nextBestTimes = await fetchSwimmerBestTimes(api, session.user.orgId, selectedSwimmerId)
        if (!cancelled) setBestTimes(nextBestTimes)
      } catch {
        if (!cancelled) setBestTimes([])
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, selectedSwimmerId, session?.user.orgId])

  const selectedSwimmer = swimmers.find((swimmer) => swimmer.id === selectedSwimmerId) ?? null
  const manualOverrides = useMemo(() => bestTimes.filter((record) => record.is_manual_override).length, [bestTimes])

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="bar-chart-2" title="Sign in required" description="Authenticate to inspect live best times." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1080, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Performance
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', fontWeight: 400, color: 'var(--text)', marginBottom: '6px' }}>
          Best Times Explorer
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
          Live best-time records from the performance service for each swimmer.
        </p>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: '280px 1fr', gap: '20px', alignItems: 'start' }}>
        <div className="card" style={{ padding: '18px' }}>
          <h2 style={{ fontSize: '14px', color: 'var(--text)', marginBottom: '12px' }}>Swimmer</h2>
          <select value={selectedSwimmerId} onChange={(event) => setSelectedSwimmerId(event.target.value)} style={{ width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '13px', marginBottom: '14px' }}>
            {swimmers.map((swimmer) => (
              <option key={swimmer.id} value={swimmer.id}>
                {swimmer.first_name} {swimmer.last_name}
              </option>
            ))}
          </select>
          {selectedSwimmer ? (
            <div style={{ fontSize: '12px', color: 'var(--text-subtle)', lineHeight: 1.6 }}>
              <div>Status: {selectedSwimmer.registration_status}</div>
              <div>Birth date: {selectedSwimmer.date_of_birth}</div>
              <div>Medical flags: {selectedSwimmer.medical_flags_redacted ? 'Redacted' : (selectedSwimmer.medical_flags?.join(', ') || 'None')}</div>
              <div style={{ marginTop: '10px' }}>
                <Link href={`/members/swimmers/${selectedSwimmer.id}`} style={{ color: 'var(--accent)', textDecoration: 'none' }}>
                  Open swimmer detail
                </Link>
              </div>
            </div>
          ) : null}
        </div>

        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap' }}>
            <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Live Best Times</h2>
            <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{bestTimes.length} records · {manualOverrides} manual overrides</span>
          </div>
          {loading ? (
            <div style={{ padding: '20px' }}><EmptyState icon="bar-chart-2" title="Loading best times" description="Fetching live best-time records." /></div>
          ) : bestTimes.length === 0 ? (
            <div style={{ padding: '20px' }}><EmptyState icon="bar-chart-2" title="No best times recorded" description="Upload meet results or add a manual override to populate this view." /></div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)' }}>
                  {['Event', 'Course', 'Time', 'Recorded', 'Source'].map((header) => (
                    <th key={header} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>{header}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {bestTimes.map((record) => (
                  <tr key={record.id} style={{ borderBottom: '1px solid var(--border)' }}>
                    <td style={{ padding: '11px 14px', fontSize: '13px', color: 'var(--text)' }}>{record.event_distance} {record.stroke}</td>
                    <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-subtle)' }}>{record.course}</td>
                    <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '13px', color: 'var(--accent)' }}>{formatSwimTime(record.best_time_ms)}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>{record.recorded_at ? new Date(record.recorded_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : '—'}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: record.is_manual_override ? 'var(--warning)' : 'var(--text-muted)' }}>
                      {record.is_manual_override ? `Manual override${record.override_reason ? ` · ${record.override_reason}` : ''}` : 'Imported result'}
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
