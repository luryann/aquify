'use client'

import { useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { createMeetEvent } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

const FIELD_STYLE: React.CSSProperties = { width: '100%', padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none' }
const LABEL_STYLE: React.CSSProperties = { display: 'block', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: '4px', fontFamily: 'var(--font-body)' }

const STROKES = [
  { value: 'freestyle', label: 'Freestyle' },
  { value: 'backstroke', label: 'Backstroke' },
  { value: 'breaststroke', label: 'Breaststroke' },
  { value: 'butterfly', label: 'Butterfly' },
  { value: 'individual_medley', label: 'Individual medley' },
  { value: 'freestyle_relay', label: 'Freestyle relay' },
  { value: 'medley_relay', label: 'Medley relay' },
]
const DISTANCES = [25, 50, 100, 200, 400, 500, 800, 1000, 1500, 1650]
const GENDERS = [
  { value: 'F', label: 'Girls / Women' },
  { value: 'M', label: 'Boys / Men' },
  { value: 'X', label: 'Mixed' },
  { value: 'O', label: 'Open' },
]

type SessionOption = { id: string; name: string }

type Props = {
  meetId: string
  course: string
  sessions: SessionOption[]
  suggestedEventNumber: string
  onCreated: () => void
}

export function AddMeetEventForm({ meetId, course, sessions, suggestedEventNumber, onCreated }: Props) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [form, setForm] = useState({
    eventNumber: '',
    gender: 'F',
    minAge: '',
    maxAge: '',
    distance: '50',
    stroke: 'freestyle',
    sessionId: '',
    fee: '',
  })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!session?.user.orgId) return
    const eventNumber = (form.eventNumber || suggestedEventNumber).trim()
    const minAge = form.minAge.trim() === '' ? undefined : Number(form.minAge)
    const maxAge = form.maxAge.trim() === '' ? undefined : Number(form.maxAge)
    if (!eventNumber) {
      setError('Event number is required.')
      return
    }
    if (minAge != null && maxAge != null && minAge > maxAge) {
      setError('Min age cannot be greater than max age.')
      return
    }
    const feeCents = form.fee.trim() === '' ? 0 : Math.round(Number(form.fee) * 100)
    if (!Number.isFinite(feeCents) || feeCents < 0) {
      setError('Entry fee must be a positive dollar amount.')
      return
    }

    setSaving(true)
    setError(null)
    setSuccess(null)
    try {
      await createMeetEvent(api, session.user.orgId, meetId, {
        event_number: eventNumber,
        stroke: form.stroke,
        distance: Number(form.distance),
        course,
        gender: form.gender,
        min_age: minAge,
        max_age: maxAge,
        entry_fee_cents: feeCents,
        is_relay: form.stroke.endsWith('_relay'),
        session_id: form.sessionId || undefined,
      })
      setSuccess(`Event ${eventNumber} added.`)
      setForm((f) => ({ ...f, eventNumber: '' }))
      onCreated()
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t add event. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <form className="card" onSubmit={handleSubmit} style={{ padding: '16px', marginBottom: '16px' }}>
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: '12px', marginBottom: '12px', flexWrap: 'wrap' }}>
        <h3 className="card-title">Add event</h3>
        <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>For meets without an entry file — events are added in {course}.</p>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: '12px', alignItems: 'end' }}>
        <div>
          <label htmlFor="event-number" style={LABEL_STYLE}>Event #</label>
          <input id="event-number" value={form.eventNumber} placeholder={suggestedEventNumber} onChange={(e) => setForm((f) => ({ ...f, eventNumber: e.target.value }))} style={FIELD_STYLE} />
        </div>
        <div>
          <label htmlFor="event-gender" style={LABEL_STYLE}>Gender</label>
          <select id="event-gender" value={form.gender} onChange={(e) => setForm((f) => ({ ...f, gender: e.target.value }))} style={FIELD_STYLE}>
            {GENDERS.map((g) => <option key={g.value} value={g.value}>{g.label}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="event-min-age" style={LABEL_STYLE}>Min age</label>
          <input id="event-min-age" type="number" min="0" max="99" placeholder="Any" value={form.minAge} onChange={(e) => setForm((f) => ({ ...f, minAge: e.target.value }))} style={FIELD_STYLE} />
        </div>
        <div>
          <label htmlFor="event-max-age" style={LABEL_STYLE}>Max age</label>
          <input id="event-max-age" type="number" min="0" max="99" placeholder="Any" value={form.maxAge} onChange={(e) => setForm((f) => ({ ...f, maxAge: e.target.value }))} style={FIELD_STYLE} />
        </div>
        <div>
          <label htmlFor="event-distance" style={LABEL_STYLE}>Distance</label>
          <select id="event-distance" value={form.distance} onChange={(e) => setForm((f) => ({ ...f, distance: e.target.value }))} style={FIELD_STYLE}>
            {DISTANCES.map((d) => <option key={d} value={d}>{d}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="event-stroke" style={LABEL_STYLE}>Stroke</label>
          <select id="event-stroke" value={form.stroke} onChange={(e) => setForm((f) => ({ ...f, stroke: e.target.value }))} style={FIELD_STYLE}>
            {STROKES.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="event-session" style={LABEL_STYLE}>Session</label>
          <select id="event-session" value={form.sessionId} onChange={(e) => setForm((f) => ({ ...f, sessionId: e.target.value }))} style={FIELD_STYLE}>
            <option value="">Unscheduled</option>
            {sessions.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="event-fee" style={LABEL_STYLE}>Fee ($)</label>
          <input id="event-fee" type="number" min="0" step="0.01" placeholder="0.00" value={form.fee} onChange={(e) => setForm((f) => ({ ...f, fee: e.target.value }))} style={FIELD_STYLE} />
        </div>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: '12px', marginTop: '12px' }}>
        {error ? <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--danger)', marginRight: 'auto' }}>{error}</div> : null}
        {success ? <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--success)', marginRight: 'auto' }}>{success}</div> : null}
        <button type="submit" className="btn btn-primary" disabled={saving}>
          {saving ? 'Adding…' : 'Add Event'}
        </button>
      </div>
    </form>
  )
}
