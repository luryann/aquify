'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  createTimeStandard,
  deleteTimeStandard,
  fetchTimeStandards,
  updateTimeStandard,
  type TimeStandardRecord,
} from '@/lib/adminData'

function formatSwimTime(milliseconds: number) {
  const totalSeconds = milliseconds / 1000
  if (totalSeconds < 60) return totalSeconds.toFixed(2)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = (totalSeconds % 60).toFixed(2).padStart(5, '0')
  return `${minutes}:${seconds}`
}

function msFromParts(minutes: string, seconds: string, hundredths: string): number {
  const m = Number(minutes) || 0
  const s = Number(seconds) || 0
  const h = Number(hundredths) || 0
  return Math.round((m * 60 + s) * 1000 + h * 10)
}

const FIELD_LABEL_STYLE: React.CSSProperties = {
  display: 'block', fontSize: '10.5px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)',
  textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: '5px',
}
const FIELD_INPUT_STYLE: React.CSSProperties = {
  width: '100%', padding: '9px 12px', borderRadius: '8px', border: '1px solid var(--border)',
  background: 'var(--bg-elevated)', color: 'var(--text)', boxSizing: 'border-box',
}

export default function TimeStandardsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [standards, setStandards] = useState<TimeStandardRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [workingId, setWorkingId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [form, setForm] = useState({
    name: '2026 Senior Cuts',
    season_year: '2026',
    standard_code: 'A',
    event_distance: '100',
    stroke: 'free',
    course: 'SCY',
    gender: 'F',
  })
  const [cutTime, setCutTime] = useState({ minutes: '0', seconds: '58', hundredths: '00' })

  async function loadStandards() {
    if (!session?.user.orgId) return
    const next = await fetchTimeStandards(api, session.user.orgId)
    setStandards(next)
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
        const next = await fetchTimeStandards(api, session.user.orgId)
        if (!cancelled) setStandards(next)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load time standards.')
          setStandards([])
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

  const activeCount = useMemo(() => standards.filter((standard) => standard.is_active).length, [standards])

  async function handleCreate() {
    if (!session?.user.orgId) return
    setSubmitting(true)
    setError(null)
    try {
      await createTimeStandard(api, session.user.orgId, {
        name: form.name.trim(),
        season_year: Number(form.season_year),
        standard_code: form.standard_code.trim(),
        event_distance: Number(form.event_distance),
        stroke: form.stroke,
        course: form.course,
        gender: form.gender,
        time_ms: msFromParts(cutTime.minutes, cutTime.seconds, cutTime.hundredths),
        is_active: true,
      })
      await loadStandards()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to create time standard.')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleToggle(standard: TimeStandardRecord) {
    if (!session?.user.orgId) return
    setWorkingId(standard.id)
    setError(null)
    try {
      await updateTimeStandard(api, session.user.orgId, standard.id, {
        is_active: !standard.is_active,
      })
      await loadStandards()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to update time standard.')
    } finally {
      setWorkingId(null)
    }
  }

  async function handleDelete(standardId: string) {
    if (!session?.user.orgId) return
    setWorkingId(standardId)
    setError(null)
    try {
      await deleteTimeStandard(api, session.user.orgId, standardId)
      await loadStandards()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Unable to delete time standard.')
    } finally {
      setWorkingId(null)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="check-circle" title="Sign in required" description="Authenticate to manage live time standards." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1180, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Performance
        </p>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '30px', fontWeight: 400, color: 'var(--text)', marginBottom: '6px' }}>
          Time Standards
        </h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>
          Live cut standards with admin create, toggle, and delete actions backed by the API.
        </p>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'grid', gridTemplateColumns: '360px 1fr', gap: '20px', alignItems: 'start' }}>
        <div className="card" style={{ padding: '18px' }}>
          <h2 style={{ fontSize: '14px', color: 'var(--text)', marginBottom: '12px' }}>Create Standard</h2>
          <div style={{ display: 'grid', gap: '14px' }}>
            <div>
              <label style={FIELD_LABEL_STYLE}>Standard Name</label>
              <input value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} placeholder="e.g. 2026 Senior Cuts" style={FIELD_INPUT_STYLE} />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
              <div>
                <label style={FIELD_LABEL_STYLE}>Season</label>
                <input value={form.season_year} onChange={(event) => setForm((current) => ({ ...current, season_year: event.target.value }))} placeholder="2026" style={FIELD_INPUT_STYLE} />
              </div>
              <div>
                <label style={FIELD_LABEL_STYLE}>Standard Code</label>
                <input value={form.standard_code} onChange={(event) => setForm((current) => ({ ...current, standard_code: event.target.value }))} placeholder="A / AA / AAA" style={FIELD_INPUT_STYLE} />
              </div>
            </div>
            <div>
              <label style={FIELD_LABEL_STYLE}>Distance ({form.course === 'LCM' || form.course === 'SCM' ? 'm' : 'yd'})</label>
              <input value={form.event_distance} onChange={(event) => setForm((current) => ({ ...current, event_distance: event.target.value }))} placeholder="100" style={FIELD_INPUT_STYLE} />
            </div>
            <div>
              <label style={FIELD_LABEL_STYLE}>Cut Time (min : sec . hundredths)</label>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr auto 1fr auto 1fr', gap: '6px', alignItems: 'center' }}>
                <input
                  value={cutTime.minutes}
                  onChange={(event) => setCutTime((current) => ({ ...current, minutes: event.target.value.replace(/\D/g, '') }))}
                  placeholder="0"
                  inputMode="numeric"
                  style={{ ...FIELD_INPUT_STYLE, textAlign: 'center', fontFamily: 'var(--font-mono)' }}
                />
                <span style={{ color: 'var(--text-muted)' }}>:</span>
                <input
                  value={cutTime.seconds}
                  onChange={(event) => setCutTime((current) => ({ ...current, seconds: event.target.value.replace(/\D/g, '').slice(0, 2) }))}
                  placeholder="58"
                  inputMode="numeric"
                  style={{ ...FIELD_INPUT_STYLE, textAlign: 'center', fontFamily: 'var(--font-mono)' }}
                />
                <span style={{ color: 'var(--text-muted)' }}>.</span>
                <input
                  value={cutTime.hundredths}
                  onChange={(event) => setCutTime((current) => ({ ...current, hundredths: event.target.value.replace(/\D/g, '').slice(0, 2) }))}
                  placeholder="00"
                  inputMode="numeric"
                  style={{ ...FIELD_INPUT_STYLE, textAlign: 'center', fontFamily: 'var(--font-mono)' }}
                />
              </div>
              <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '5px', fontFamily: 'var(--font-mono)' }}>
                = {formatSwimTime(msFromParts(cutTime.minutes, cutTime.seconds, cutTime.hundredths))}
              </p>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: '10px' }}>
              <div>
                <label style={FIELD_LABEL_STYLE}>Stroke</label>
                <select value={form.stroke} onChange={(event) => setForm((current) => ({ ...current, stroke: event.target.value }))} style={FIELD_INPUT_STYLE}>
                  <option value="free">free</option>
                  <option value="back">back</option>
                  <option value="breast">breast</option>
                  <option value="fly">fly</option>
                  <option value="im">im</option>
                </select>
              </div>
              <div>
                <label style={FIELD_LABEL_STYLE}>Course</label>
                <select value={form.course} onChange={(event) => setForm((current) => ({ ...current, course: event.target.value }))} style={FIELD_INPUT_STYLE}>
                  <option value="SCY">SCY</option>
                  <option value="SCM">SCM</option>
                  <option value="LCM">LCM</option>
                </select>
              </div>
              <div>
                <label style={FIELD_LABEL_STYLE}>Gender</label>
                <select value={form.gender} onChange={(event) => setForm((current) => ({ ...current, gender: event.target.value }))} style={FIELD_INPUT_STYLE}>
                  <option value="F">F</option>
                  <option value="M">M</option>
                  <option value="X">X</option>
                </select>
              </div>
            </div>
            <button onClick={() => void handleCreate()} disabled={submitting} style={{ padding: '9px 14px', borderRadius: '8px', border: 'none', background: 'var(--accent)', color: '#fff', fontSize: '12px', fontWeight: 600, cursor: submitting ? 'not-allowed' : 'pointer' }}>
              {submitting ? 'Creating…' : 'Create Standard'}
            </button>
          </div>
        </div>

        <div className="card" style={{ overflow: 'hidden' }}>
          <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', gap: '12px', flexWrap: 'wrap' }}>
            <h2 style={{ fontSize: '14px', color: 'var(--text)' }}>Live Standards</h2>
            <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{standards.length} total · {activeCount} active</span>
          </div>
          {loading ? (
            <div style={{ padding: '20px' }}><EmptyState icon="check-circle" title="Loading standards" description="Fetching the live time-standard registry." /></div>
          ) : standards.length === 0 ? (
            <div style={{ padding: '20px' }}><EmptyState icon="check-circle" title="No standards yet" description="Create a standard to populate this table." /></div>
          ) : (
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border)' }}>
                  {['Name', 'Code', 'Event', 'Cut', 'Season', 'Status', ''].map((header) => (
                    <th key={header} style={{ padding: '10px 14px', textAlign: 'left', fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>{header}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {standards.map((standard) => (
                  <tr key={standard.id} style={{ borderBottom: '1px solid var(--border)' }}>
                    <td style={{ padding: '11px 14px', fontSize: '13px', color: 'var(--text)' }}>{standard.name}</td>
                    <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--accent)' }}>{standard.standard_code}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>{standard.event_distance} {standard.stroke} · {standard.course} · {standard.gender}</td>
                    <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--text)' }}>{formatSwimTime(standard.time_ms)}</td>
                    <td style={{ padding: '11px 14px', fontSize: '12px', color: 'var(--text-subtle)' }}>{standard.season_year}</td>
                    <td style={{ padding: '11px 14px', fontFamily: 'var(--font-mono)', fontSize: '11px', color: standard.is_active ? 'var(--success)' : 'var(--text-muted)' }}>{standard.is_active ? 'active' : 'inactive'}</td>
                    <td style={{ padding: '11px 14px' }}>
                      <div style={{ display: 'flex', gap: '8px' }}>
                        <button onClick={() => void handleToggle(standard)} disabled={workingId === standard.id} style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: '12px', cursor: workingId === standard.id ? 'not-allowed' : 'pointer' }}>
                          {standard.is_active ? 'Pause' : 'Activate'}
                        </button>
                        <button onClick={() => void handleDelete(standard.id)} disabled={workingId === standard.id} style={{ padding: '6px 10px', borderRadius: '6px', border: '1px solid var(--error-border)', background: 'var(--error-bg)', color: 'var(--error)', fontSize: '12px', cursor: workingId === standard.id ? 'not-allowed' : 'pointer' }}>
                          Delete
                        </button>
                      </div>
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
