'use client'

import { useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { bulkCreateVolunteerJobs, createVolunteerJob } from '@/lib/volunteerData'
import { VOLUNTEER_ROLE_LABELS, localInputToISO, type VolunteerRole } from '@/lib/volunteer-state'
import { InlineNotice } from './VolunteerPrimitives'

type Shift = { date: string; start: string; end: string; label: string }

const ROLES = Object.keys(VOLUNTEER_ROLE_LABELS) as VolunteerRole[]

/**
 * Create volunteer jobs: a single slot, or (for a meet) the standard meet
 * template stamped onto each session shift in one atomic request.
 */
export function NewJobsForm({ api, orgId, meetId, defaultDate, onCreated }: {
  api: ApiClient
  orgId: string
  meetId?: string
  defaultDate?: string
  onCreated: (count: number) => void
}) {
  const [mode, setMode] = useState<'template' | 'single'>(meetId ? 'template' : 'single')
  const [eventName, setEventName] = useState('')
  const [role, setRole] = useState<VolunteerRole>('timer')
  const [title, setTitle] = useState('')
  const [capacity, setCapacity] = useState('4')
  const [location, setLocation] = useState('')
  const [certified, setCertified] = useState(false)
  const [shifts, setShifts] = useState<Shift[]>([{ date: defaultDate ?? '', start: '08:00', end: '12:00', label: meetId ? 'Session 1' : '' }])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function patchShift(i: number, patch: Partial<Shift>) {
    setShifts((list) => list.map((s, idx) => (idx === i ? { ...s, ...patch } : s)))
  }

  function shiftISO(s: Shift) {
    return { starts_at: localInputToISO(`${s.date}T${s.start}`), ends_at: localInputToISO(`${s.date}T${s.end}`) }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const isoShifts = shifts.map((s) => ({ ...shiftISO(s), label: s.label.trim() || undefined }))
    if (isoShifts.some((s) => !s.starts_at || !s.ends_at)) {
      setError('Every shift needs a date, start time, and end time.')
      return
    }
    setSaving(true)
    try {
      if (mode === 'template') {
        const count = await bulkCreateVolunteerJobs(api, orgId, {
          meet_id: meetId, event_name: meetId ? undefined : eventName.trim(), location: location.trim() || undefined,
          template_id: meetId ? 'standard_meet' : 'club_event', shifts: isoShifts,
        })
        onCreated(count)
      } else {
        await createVolunteerJob(api, orgId, {
          meet_id: meetId, event_name: meetId ? undefined : eventName.trim(), role, title: title.trim() || VOLUNTEER_ROLE_LABELS[role],
          location: location.trim() || undefined, starts_at: isoShifts[0].starts_at, ends_at: isoShifts[0].ends_at,
          capacity: Number(capacity) || 1, requires_certification: certified,
        })
        onCreated(1)
      }
    } catch (err) {
      setError(reportUIError('volunteer.create_jobs', err, 'Couldn’t create the jobs. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  const visibleShifts = mode === 'single' ? shifts.slice(0, 1) : shifts

  return (
    <form onSubmit={submit} className="card" style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
      <div role="radiogroup" aria-label="What to create" style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
        {(['template', 'single'] as const).map((m) => (
          <button key={m} type="button" role="radio" aria-checked={mode === m} onClick={() => setMode(m)}
            className={mode === m ? 'btn btn-secondary btn-sm' : 'btn btn-ghost btn-sm'}
            style={mode === m ? { borderColor: 'var(--accent-border)', color: 'var(--accent)' } : undefined}>
            {m === 'template' ? (meetId ? 'Standard meet crew' : 'Event crew template') : 'Single job'}
          </button>
        ))}
      </div>
      {mode === 'template' ? (
        <p style={{ margin: 0, fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>
          {meetId
            ? 'Creates timers (16), stroke & turn officials (4), marshals, clerks, concessions, hospitality, and setup crew for each session below.'
            : 'Creates setup, check-in, helper, and cleanup crews for each shift below.'}
        </p>
      ) : null}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '12px' }}>
        {!meetId ? (
          <label className="form-field">
            <span className="form-label">Event name</span>
            <input className="form-input" required value={eventName} onChange={(e) => setEventName(e.target.value)} placeholder="Fall fundraiser" />
          </label>
        ) : null}
        {mode === 'single' ? (
          <>
            <label className="form-field">
              <span className="form-label">Role</span>
              <select className="form-input" value={role} onChange={(e) => setRole(e.target.value as VolunteerRole)}>
                {ROLES.map((r) => <option key={r} value={r}>{VOLUNTEER_ROLE_LABELS[r]}</option>)}
              </select>
            </label>
            <label className="form-field">
              <span className="form-label">Title</span>
              <input className="form-input" value={title} onChange={(e) => setTitle(e.target.value)} placeholder={VOLUNTEER_ROLE_LABELS[role]} />
            </label>
            <label className="form-field">
              <span className="form-label">Spots</span>
              <input className="form-input" type="number" min={1} max={500} value={capacity} onChange={(e) => setCapacity(e.target.value)} />
            </label>
          </>
        ) : null}
        <label className="form-field">
          <span className="form-label">Location</span>
          <input className="form-input" value={location} onChange={(e) => setLocation(e.target.value)} placeholder="Optional" />
        </label>
      </div>
      {mode === 'single' ? (
        <label style={{ display: 'flex', gap: '8px', alignItems: 'center', fontSize: 'var(--fs-base)' }}>
          <input type="checkbox" checked={certified} onChange={(e) => setCertified(e.target.checked)} />
          Certified volunteers only (e.g. officials)
        </label>
      ) : null}
      {visibleShifts.map((s, i) => (
        <div key={i} style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: '12px', alignItems: 'end' }}>
          <label className="form-field">
            <span className="form-label">Date</span>
            <input className="form-input" type="date" required value={s.date} onChange={(e) => patchShift(i, { date: e.target.value })} />
          </label>
          <label className="form-field">
            <span className="form-label">Starts</span>
            <input className="form-input" type="time" required value={s.start} onChange={(e) => patchShift(i, { start: e.target.value })} />
          </label>
          <label className="form-field">
            <span className="form-label">Ends</span>
            <input className="form-input" type="time" required value={s.end} onChange={(e) => patchShift(i, { end: e.target.value })} />
          </label>
          {mode === 'template' ? (
            <label className="form-field">
              <span className="form-label">Shift name</span>
              <input className="form-input" value={s.label} onChange={(e) => patchShift(i, { label: e.target.value })} placeholder="Session 1" />
            </label>
          ) : null}
          {mode === 'template' && shifts.length > 1 ? (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => setShifts((l) => l.filter((_, idx) => idx !== i))}>Remove shift</button>
          ) : null}
        </div>
      ))}
      {error ? <InlineNotice tone="error">{error}</InlineNotice> : null}
      <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
        {mode === 'template' ? (
          <button type="button" className="btn btn-secondary btn-sm"
            onClick={() => setShifts((l) => [...l, { ...l[l.length - 1], label: meetId ? `Session ${l.length + 1}` : '' }])}>
            Add shift
          </button>
        ) : null}
        <button type="submit" className="btn btn-primary btn-sm" disabled={saving}>
          {saving ? 'Creating…' : mode === 'template' ? 'Create crew' : 'Create job'}
        </button>
      </div>
    </form>
  )
}
