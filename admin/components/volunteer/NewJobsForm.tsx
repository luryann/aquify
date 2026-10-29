'use client'

import { useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { bulkCreateVolunteerJobs, createVolunteerJob } from '@/lib/volunteerData'
import { VOLUNTEER_ROLE_LABELS, localInputToISO, type VolunteerRole } from '@/lib/volunteer-state'
import { InlineNotice } from './VolunteerPrimitives'

type Shift = { date: string; start: string; end: string; label: string }

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="form-field">
      <span className="form-label">{label}</span>
      {children}
    </label>
  )
}

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

  // Single-job mode uses only the first shift; ignore any extras left from template mode.
  const visibleShifts = mode === 'single' ? shifts.slice(0, 1) : shifts

  function patchShift(i: number, patch: Partial<Shift>) {
    setShifts((list) => list.map((s, idx) => (idx === i ? { ...s, ...patch } : s)))
  }

  function shiftISO(s: Shift) {
    return { starts_at: localInputToISO(`${s.date}T${s.start}`), ends_at: localInputToISO(`${s.date}T${s.end}`) }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const isoShifts = visibleShifts.map((s) => ({ ...shiftISO(s), label: s.label.trim() || undefined }))
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

  return (
    <form onSubmit={submit} className="card u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
      <div role="radiogroup" aria-label="What to create" className="u-gap-6px" style={{ display: 'flex', flexWrap: 'wrap' }}>
        {(['template', 'single'] as const).map((m) => (
          <button key={m} type="button" role="radio" aria-checked={mode === m} onClick={() => setMode(m)}
            className={mode === m ? 'btn btn-secondary btn-sm' : 'btn btn-ghost btn-sm'}
            style={mode === m ? { borderColor: 'var(--accent-border)', color: 'var(--accent-text, var(--accent))' } : undefined}>
            {m === 'template' ? (meetId ? 'Standard meet crew' : 'Event crew template') : 'Single job'}
          </button>
        ))}
      </div>
      {mode === 'template' ? (
        <p className="u-m-0 u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>
          {meetId
            ? 'Creates timers (16), stroke & turn officials (4), marshals, clerks, concessions, hospitality, and setup crew for each session below.'
            : 'Creates setup, check-in, helper, and cleanup crews for each shift below.'}
        </p>
      ) : null}
      <div className="u-gap-12px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))' }}>
        {!meetId ? (
          <Field label="Event name">
            <input aria-label="Fall fundraiser" className="form-input" required value={eventName} onChange={(e) => setEventName(e.target.value)} placeholder="Fall fundraiser" />
          </Field>
        ) : null}
        {mode === 'single' ? (
          <>
            <Field label="Role">
              <select className="form-input" value={role} onChange={(e) => setRole(e.target.value as VolunteerRole)}>
                {ROLES.map((r) => <option key={r} value={r}>{VOLUNTEER_ROLE_LABELS[r]}</option>)}
              </select>
            </Field>
            <Field label="Title">
              <input className="form-input" value={title} onChange={(e) => setTitle(e.target.value)} placeholder={VOLUNTEER_ROLE_LABELS[role]} />
            </Field>
            <Field label="Spots">
              <input className="form-input" type="number" min={1} max={500} value={capacity} onChange={(e) => setCapacity(e.target.value)} />
            </Field>
          </>
        ) : null}
        <Field label="Location">
          <input aria-label="Optional" className="form-input" value={location} onChange={(e) => setLocation(e.target.value)} placeholder="Optional" />
        </Field>
      </div>
      {mode === 'single' ? (
        <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center' }}>
          <input type="checkbox" checked={certified} onChange={(e) => setCertified(e.target.checked)} />
          Certified volunteers only (e.g. officials)
        </label>
      ) : null}
      {visibleShifts.map((s, i) => (
        <div key={i} className="u-gap-12px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', alignItems: 'end' }}>
          <Field label="Date">
            <input className="form-input" type="date" required value={s.date} onChange={(e) => patchShift(i, { date: e.target.value })} />
          </Field>
          <Field label="Starts">
            <input className="form-input" type="time" required value={s.start} onChange={(e) => patchShift(i, { start: e.target.value })} />
          </Field>
          <Field label="Ends">
            <input className="form-input" type="time" required value={s.end} onChange={(e) => patchShift(i, { end: e.target.value })} />
          </Field>
          {mode === 'template' ? (
            <Field label="Shift name">
              <input aria-label="Session 1" className="form-input" value={s.label} onChange={(e) => patchShift(i, { label: e.target.value })} placeholder="Session 1" />
            </Field>
          ) : null}
          {mode === 'template' && shifts.length > 1 ? (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => setShifts((l) => l.filter((_, idx) => idx !== i))}>Remove shift</button>
          ) : null}
        </div>
      ))}
      {error ? <InlineNotice tone="error">{error}</InlineNotice> : null}
      <div className="u-gap-8px" style={{ display: 'flex', flexWrap: 'wrap' }}>
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
