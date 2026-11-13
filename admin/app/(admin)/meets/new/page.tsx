'use client'

import { useState, useRef, useEffect, useMemo, useId, type ReactNode } from 'react'
import { useRouter } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  createMeet,
  fetchMeets,
  fetchOrgSettings,
  parseMeetEntryFile,
  type MeetRecord,
  type ParsedMeetEventDraftRecord,
} from '@/lib/adminData'
import {
  buildCreateMeetPayload,
  getNextSessionNumber,
  isMeetEventFileName,
  mapParsedSessionsToDraftSessions,
  MEET_EVENT_FILE_EXTENSIONS,
  maxEntriesPayloadValue,
  type Course,
  type MeetCreateDraftState,
  type MeetSessionDraft,
} from '@/lib/meet-draft'
import { formatDateOnly } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { FIELD_INPUT_STYLE } from '@/lib/form-styles'
import { COURSE_LABELS, formatStroke } from '@/lib/meet-events'
import './meet-create.css'

type MeetEventDraft = ParsedMeetEventDraftRecord
type MeetDraft = MeetCreateDraftState

const COURSES: Course[] = ['SCY', 'SCM', 'LCM']
const MAX_ENTRY_OPTIONS = ['2', '3', '4', '5', '6', 'Unlimited']
const EVENT_PREVIEW_LIMIT = 8

function maxEntriesButtonValue(value: number): string {
  return value >= 6 ? 'Unlimited' : String(value)
}

function ageLabel(event: MeetEventDraft): string | null {
  const { min_age: min, max_age: max } = event
  if (min == null && max == null) return null
  if (min != null && max != null) return `${min}-${max}`
  if (min != null) return `${min}&O`
  return `${max}&U`
}

function formatEventAgeGroup(event: MeetEventDraft): string {
  const gender = event.gender === 'F' ? 'Girls' : event.gender === 'M' ? 'Boys' : event.gender === 'X' ? 'Mixed' : ''
  return [gender, ageLabel(event) ?? 'Open'].filter(Boolean).join(' ')
}

function formatBytes(b: number) {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(0)} KB`
  return `${(b / 1024 / 1024).toFixed(1)} MB`
}

function Field({ label, required, hint, children }: { label: string; required?: boolean; hint?: string; children: (id: string) => ReactNode }) {
  const id = useId()
  return (
    <div>
      <label className="mc-label" htmlFor={id}>
        {label}
        {required && <span className="mc-req" aria-hidden>*</span>}
      </label>
      {children(id)}
      {hint && <p className="field-hint">{hint}</p>}
    </div>
  )
}

function SectionHead({ id, title, hint, action }: { id: string; title: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="mc-section-head">
      <div>
        <h2 className="card-title" id={id}>{title}</h2>
        {hint && <p>{hint}</p>}
      </div>
      {action}
    </div>
  )
}

function emptySession(): MeetSessionDraft {
  return { id: 'session-1', sessionNumber: 1, section: 'Day 1', name: 'Session 1', date: '', warmupTime: '', startTime: '' }
}

export default function NewMeetPage() {
  const router = useRouter()
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId

  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [existingMeets, setExistingMeets] = useState<MeetRecord[]>([])

  const [eventFiles, setEventFiles] = useState<File[]>([])
  const [parsing, setParsing] = useState(false)
  const [parseError, setParseError] = useState<string | null>(null)
  const [parseWarning, setParseWarning] = useState<string | null>(null)
  const [parseSummary, setParseSummary] = useState<string | null>(null)
  const [parsedEvents, setParsedEvents] = useState<MeetEventDraft[]>([])
  const [showAllEvents, setShowAllEvents] = useState(false)
  const [dragging, setDragging] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const hostTouched = useRef(false)

  const [draft, setDraft] = useState<MeetDraft>({
    name: '',
    course: 'SCY',
    hostClub: '',
    location: '',
    startDate: '',
    endDate: '',
    ageUpDate: '',
    commitmentDeadline: '',
    entryDeadline: '',
    maxEntriesPerSwimmer: '3',
    swimmerFee: '',
    swimmerSelfSignup: true,
    sessions: [emptySession()],
  })

  useEffect(() => {
    if (!orgId) return
    let cancelled = false
    fetchMeets(api, orgId)
      .then((meets) => { if (!cancelled) setExistingMeets(meets) })
      .catch(() => { /* duplicate-name check is a soft nicety */ })
    fetchOrgSettings(api, orgId)
      .then((s) => {
        if (cancelled || hostTouched.current || !s.team_name) return
        setDraft((d) => (d.hostClub ? d : { ...d, hostClub: s.team_name }))
      })
      .catch(() => { /* host club stays editable */ })
    return () => { cancelled = true }
  }, [api, orgId])

  const duplicateMeet = useMemo(() => {
    const name = draft.name.trim().toLowerCase()
    if (!name || !draft.startDate) return null
    return existingMeets.find((m) => m.name.trim().toLowerCase() === name && m.start_date === draft.startDate) ?? null
  }, [draft.name, draft.startDate, existingMeets])

  function update<K extends keyof MeetDraft>(key: K, value: MeetDraft[K]) {
    setDraft((d) => ({ ...d, [key]: value }))
  }

  /** Picking a start date seeds the end date and any session that has no date yet. */
  function setStartDate(value: string) {
    setDraft((d) => ({
      ...d,
      startDate: value,
      endDate: !d.endDate || d.endDate < value ? value : d.endDate,
      sessions: d.sessions.map((s) => (s.date ? s : { ...s, date: value })),
    }))
  }

  function updateSession(id: string, key: keyof MeetSessionDraft, value: string) {
    setDraft((d) => ({ ...d, sessions: d.sessions.map((s) => (s.id === id ? { ...s, [key]: value } : s)) }))
  }

  function addSession() {
    setDraft((d) => {
      const last = d.sessions[d.sessions.length - 1]
      const number = getNextSessionNumber(d.sessions)
      return {
        ...d,
        sessions: [
          ...d.sessions,
          {
            id: `session-${Date.now()}`,
            sessionNumber: number,
            section: `Day ${d.sessions.length + 1}`,
            name: `Session ${number}`,
            date: last?.date || d.startDate,
            warmupTime: last?.warmupTime ?? '',
            startTime: '',
          },
        ],
      }
    })
  }

  function removeSession(id: string) {
    setDraft((d) => (d.sessions.length === 1 ? d : { ...d, sessions: d.sessions.filter((s) => s.id !== id) }))
  }

  function clearParsed() {
    setParsedEvents([])
    setParseSummary(null)
    setParseWarning(null)
  }

  async function handleEventFiles(list: FileList | File[] | null) {
    const files = list ? Array.from(list) : []
    if (files.length === 0) return
    setEventFiles(files)
    setParseError(null)
    clearParsed()

    const rejected = files.filter((f) => !isMeetEventFileName(f.name))
    if (rejected.length > 0) {
      setParseError(`${rejected.map((f) => f.name).join(', ')} is not a meet events file. Add the host’s .hyv and .ev3 files, or the .zip they came in.`)
      return
    }
    if (!session?.user.orgId) return

    setParsing(true)
    try {
      const parsed = await parseMeetEntryFile(session.token, session.user.orgId, files)
      const events = parsed.events ?? []
      setParsedEvents(events)
      setDraft((d) => ({
        ...d,
        name: d.name || parsed.meet_name || d.name,
        course: parsed.course === 'SCY' || parsed.course === 'SCM' || parsed.course === 'LCM' ? parsed.course : d.course,
        startDate: d.startDate || parsed.start_date || d.startDate,
        endDate: d.endDate || parsed.end_date || d.endDate,
        ageUpDate: d.ageUpDate || parsed.age_up_date || d.ageUpDate,
        location: d.location || parsed.location || d.location,
        maxEntriesPerSwimmer: maxEntriesButtonValue(parsed.signup_settings?.max_entries_per_swimmer ?? maxEntriesPayloadValue(d.maxEntriesPerSwimmer)),
        swimmerSelfSignup: parsed.signup_settings?.self_signup_enabled ?? d.swimmerSelfSignup,
        sessions: parsed.sessions.length > 0 ? mapParsedSessionsToDraftSessions(parsed.sessions) : d.sessions,
      }))
      const counts = `${parsed.sessions.length} session${parsed.sessions.length === 1 ? '' : 's'} and ${events.length} event${events.length === 1 ? '' : 's'}`
      const warnings = parsed.parse_warnings ?? []
      setParseSummary(
        warnings.length > 0
          ? `${counts} read from ${parsed.source_file}. Check the note below before creating.`
          : files.length > 1 || parsed.source_file.toLowerCase().endsWith('.zip')
            ? `${counts} read. The files agree with each other.`
            : `${counts} read from ${parsed.source_file}.`,
      )
      if (warnings.length > 0) {
        setParseWarning(warnings.length === 1 ? warnings[0] : `${warnings[0]} (and ${warnings.length - 1} more)`)
      }
    } catch (err) {
      setParseError(uiErrorMessage(err, 'Couldn’t read the meet events file. Try again.'))
    } finally {
      setParsing(false)
    }
  }

  function removeEventFiles() {
    setEventFiles([])
    setParseError(null)
    clearParsed()
  }

  // ── Readiness ──────────────────────────────────────────────────────────────
  const datesOk = !!draft.startDate && !!draft.endDate && draft.endDate >= draft.startDate
  const sessionsOk = draft.sessions.every((s) => s.name.trim() && s.date)
  const checks = [
    { key: 'details', label: 'Name and host club', done: !!draft.name.trim() && !!draft.hostClub.trim(), target: 'mc-details' },
    { key: 'dates', label: 'Start and end dates', done: datesOk, target: 'mc-dates' },
    { key: 'sessions', label: 'Every session has a name and date', done: sessionsOk, target: 'mc-sessions' },
  ]
  const ready = checks.every((c) => c.done) && !parsing
  const remaining = checks.filter((c) => !c.done).length

  function jumpTo(id: string) {
    const el = document.getElementById(id)
    el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    el?.querySelector<HTMLElement>('input, button')?.focus({ preventScroll: true })
  }

  async function handleSave() {
    if (!orgId || !ready || saving) return
    setSaving(true)
    setSaveError(null)
    try {
      await createMeet(api, orgId, buildCreateMeetPayload({ ...draft, name: draft.name.trim() }, parsedEvents))
      router.push('/meets?created=1')
    } catch (err) {
      setSaveError(`Failed to create meet: ${uiErrorMessage(err, 'An error occurred.')}`)
      setSaving(false)
    }
  }

  const visibleEvents = showAllEvents ? parsedEvents : parsedEvents.slice(0, EVENT_PREVIEW_LIMIT)

  return (
    <form className="page" onSubmit={(e) => { e.preventDefault(); void handleSave() }} noValidate>
      <div className="page-header">
        <div>
          <button
            type="button"
            onClick={() => router.push('/meets')}
            className="btn btn-ghost u-fs-fs-sm u-gap-6px u-mb-8px"
            style={{ marginLeft: 'calc(var(--space-2) * -1)' }}
          >
            <Icon name="arrow-left" size={13} /> Meets
          </button>
          <h1 className="page-title">Create meet</h1>
          <p className="page-subtitle">Start from the host’s events file to fill most of this in, or enter the details by hand.</p>
        </div>
      </div>

      <div className="split-aside u-gap-24px">
        {/* ── Main column ───────────────────────────────────────────── */}
        <div className="mc-main">
          {/* Events file */}
          <section className="card" aria-labelledby="mc-file">
            <SectionHead id="mc-file" title="Host’s events file" hint="Optional. Fills in the name, dates, sessions and event list for you." />
            {eventFiles.length === 0 ? (
              <button
                type="button"
                className="mc-drop"
                data-dragging={dragging}
                onClick={() => fileInputRef.current?.click()}
                onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
                onDragLeave={() => setDragging(false)}
                onDrop={(e) => { e.preventDefault(); setDragging(false); void handleEventFiles(e.dataTransfer.files) }}
              >
                <Icon name="upload-cloud" size={24} className="mc-drop-icon" />
                <div>
                  <strong>Drop the .hyv, .ev3 or .zip here, or browse</strong>
                  <span>Add both formats and we’ll check that they agree.</span>
                </div>
              </button>
            ) : (
              <div className="mc-filelist">
                {eventFiles.map((f, i) => (
                  <div key={`${f.name}-${i}`} className="mc-file">
                    <div className="mc-file-name">
                      <Icon name="file-text" size={13} style={{ color: 'var(--accent-text, var(--accent))', flexShrink: 0 }} />
                      <span>{f.name}</span>
                    </div>
                    <span style={{ color: 'var(--text-muted)', flexShrink: 0 }}>{formatBytes(f.size)}</span>
                  </div>
                ))}
                <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                  <button type="button" className="btn btn-secondary u-fs-fs-sm" onClick={() => fileInputRef.current?.click()}>Replace files</button>
                  <button type="button" className="btn btn-ghost u-fs-fs-sm" onClick={removeEventFiles}>Remove</button>
                </div>
              </div>
            )}
            <input
              ref={fileInputRef}
              type="file"
              accept={MEET_EVENT_FILE_EXTENSIONS.join(',')}
              multiple
              hidden
              onChange={(e) => { void handleEventFiles(e.target.files); e.target.value = '' }}
            />
            <div aria-live="polite">
              {parsing && (
                <div className="mc-banner mc-banner-info"><span className="spinner spinner-sm" aria-hidden /> Reading meet events…</div>
              )}
              {parseSummary && !parsing && (
                <div className="mc-banner mc-banner-ok"><Icon name="check-circle" size={14} /> {parseSummary}</div>
              )}
              {parseWarning && !parsing && (
                <div className="mc-banner mc-banner-warn"><Icon name="alert-triangle" size={14} /> {parseWarning}</div>
              )}
              {parseError && !parsing && (
                <div className="mc-banner mc-banner-err" role="alert"><Icon name="alert-circle" size={14} /> {parseError}</div>
              )}
            </div>
          </section>

          {/* Details */}
          <section className="card" id="mc-details" aria-labelledby="mc-details-h">
            <SectionHead id="mc-details-h" title="Details" />
            <div className="mc-fields">
              <Field label="Meet name" required>
                {(id) => <input id={id} className="input" value={draft.name} onChange={(e) => update('name', e.target.value)} placeholder="e.g. Spring Invitational 2026" style={FIELD_INPUT_STYLE} autoComplete="off" />}
              </Field>
              <div className="cols-2 u-gap-16px">
                <Field label="Host club" required>
                  {(id) => <input id={id} className="input" value={draft.hostClub} onChange={(e) => { hostTouched.current = true; update('hostClub', e.target.value) }} placeholder="e.g. Aquify Aquatics" style={FIELD_INPUT_STYLE} autoComplete="off" />}
                </Field>
                <Field label="Course" required hint={COURSE_LABELS[draft.course]}>
                  {() => (
                    <div className="seg" role="radiogroup" aria-label="Course" style={{ display: 'flex' }}>
                      {COURSES.map((c) => (
                        <button
                          key={c}
                          type="button"
                          role="radio"
                          aria-checked={draft.course === c}
                          title={COURSE_LABELS[c]}
                          className="seg-item"
                          style={{ flex: 1, justifyContent: 'center' }}
                          onClick={() => update('course', c)}
                        >
                          {c}
                        </button>
                      ))}
                    </div>
                  )}
                </Field>
              </div>
              <Field label="Venue">
                {(id) => <input id={id} className="input" value={draft.location} onChange={(e) => update('location', e.target.value)} placeholder="e.g. Aquatic Center, 123 Pool Rd, City, ST" style={FIELD_INPUT_STYLE} autoComplete="off" />}
              </Field>
            </div>
          </section>

          {/* Dates */}
          <section className="card" id="mc-dates" aria-labelledby="mc-dates-h">
            <SectionHead id="mc-dates-h" title="Dates and deadlines" />
            <div className="mc-fields">
              <div className="cols-2 u-gap-16px">
                <Field label="Start date" required>
                  {(id) => <input id={id} className="input" type="date" value={draft.startDate} onChange={(e) => setStartDate(e.target.value)} style={FIELD_INPUT_STYLE} />}
                </Field>
                <Field label="End date" required>
                  {(id) => <input id={id} className="input" type="date" value={draft.endDate} min={draft.startDate || undefined} onChange={(e) => update('endDate', e.target.value)} style={FIELD_INPUT_STYLE} aria-invalid={!!draft.startDate && !!draft.endDate && draft.endDate < draft.startDate} />}
                </Field>
              </div>
              {!!draft.startDate && !!draft.endDate && draft.endDate < draft.startDate && (
                <p className="field-error" role="alert">The end date is before the start date.</p>
              )}
              <Field label="Age-up date" hint="Swimmer ages in the entry files are worked out on this date. Leave blank to use the start date.">
                {(id) => <input id={id} className="input" type="date" value={draft.ageUpDate ?? ''} onChange={(e) => update('ageUpDate', e.target.value)} style={FIELD_INPUT_STYLE} />}
              </Field>
              <div className="cols-2 u-gap-16px">
                <Field label="Commitment deadline" hint="When parents must answer yes or no by.">
                  {(id) => <input id={id} className="input" type="datetime-local" value={draft.commitmentDeadline} onChange={(e) => update('commitmentDeadline', e.target.value)} style={FIELD_INPUT_STYLE} />}
                </Field>
                <Field label="Entry deadline" hint="When entries are due to the host.">
                  {(id) => <input id={id} className="input" type="datetime-local" value={draft.entryDeadline} onChange={(e) => update('entryDeadline', e.target.value)} style={FIELD_INPUT_STYLE} />}
                </Field>
              </div>
            </div>
          </section>

          {/* Sessions */}
          <section className="card" id="mc-sessions" aria-labelledby="mc-sessions-h">
            <SectionHead
              id="mc-sessions-h"
              title="Sessions"
              hint="Warm-up and start times can be filled in later."
              action={<button type="button" className="btn btn-secondary u-gap-6px u-fs-fs-sm" onClick={addSession}><Icon name="plus" size={13} /> Add session</button>}
            />
            <div className="mc-fields" style={{ gap: 'var(--space-3)' }}>
              {draft.sessions.map((s, idx) => (
                <div key={s.id} className="mc-session">
                  <span className="mc-session-num">{idx + 1}</span>
                  <Field label="Name" required>
                    {(id) => <input id={id} className="input" value={s.name} onChange={(e) => updateSession(s.id, 'name', e.target.value)} placeholder="e.g. Saturday morning" style={FIELD_INPUT_STYLE} />}
                  </Field>
                  <Field label="Date" required>
                    {(id) => <input id={id} className="input" type="date" value={s.date} onChange={(e) => updateSession(s.id, 'date', e.target.value)} style={FIELD_INPUT_STYLE} />}
                  </Field>
                  <Field label="Warm-up">
                    {(id) => <input id={id} className="input" type="time" value={s.warmupTime} onChange={(e) => updateSession(s.id, 'warmupTime', e.target.value)} style={FIELD_INPUT_STYLE} />}
                  </Field>
                  <Field label="Start">
                    {(id) => <input id={id} className="input" type="time" value={s.startTime} onChange={(e) => updateSession(s.id, 'startTime', e.target.value)} style={FIELD_INPUT_STYLE} />}
                  </Field>
                  <button
                    type="button"
                    className="btn-icon"
                    aria-label={`Remove session ${idx + 1}`}
                    disabled={draft.sessions.length === 1}
                    onClick={() => removeSession(s.id)}
                    style={{ color: 'var(--text-muted)' }}
                  >
                    <Icon name="trash-2" size={14} />
                  </button>
                </div>
              ))}
            </div>
          </section>

          {/* Signup rules */}
          <section className="card" aria-labelledby="mc-rules-h">
            <SectionHead id="mc-rules-h" title="Signup rules" />
            <div className="mc-fields">
              <Field label="Max individual events per swimmer">
                {() => (
                  <div className="seg" role="radiogroup" aria-label="Max individual events per swimmer" style={{ display: 'inline-flex' }}>
                    {MAX_ENTRY_OPTIONS.map((n) => (
                      <button
                        key={n}
                        type="button"
                        role="radio"
                        aria-checked={draft.maxEntriesPerSwimmer === n}
                        className="seg-item"
                        onClick={() => update('maxEntriesPerSwimmer', n)}
                      >
                        {n}
                      </button>
                    ))}
                  </div>
                )}
              </Field>
              <Field label="Per-swimmer fee from the host" hint="The surcharge the host bills for each swimmer you enter. It appears in the cost summary of your entry reports.">
                {(id) => (
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', maxWidth: 200 }}>
                    <span style={{ color: 'var(--text-muted)' }}>$</span>
                    <input id={id} className="input" inputMode="decimal" value={draft.swimmerFee ?? ''} onChange={(e) => update('swimmerFee', e.target.value.replace(/[^0-9.]/g, ''))} placeholder="0.00" style={FIELD_INPUT_STYLE} />
                  </div>
                )}
              </Field>
              <button
                type="button"
                role="switch"
                aria-checked={draft.swimmerSelfSignup}
                className="mc-switch"
                onClick={() => update('swimmerSelfSignup', !draft.swimmerSelfSignup)}
              >
                <span>
                  Swimmers can pick their own events
                  <small>{draft.swimmerSelfSignup ? 'Event selection is open in the swimmer portal.' : 'Off: parents handle the whole commitment flow.'}</small>
                </span>
                <span className="mc-switch-track" aria-hidden />
              </button>
            </div>
          </section>

          {/* Events */}
          {parsedEvents.length > 0 && (
            <section className="card" aria-labelledby="mc-events-h">
              <SectionHead id="mc-events-h" title={`${parsedEvents.length} event${parsedEvents.length === 1 ? '' : 's'} from the file`} hint="These become the signup list when you create the meet. Edit them from the meet’s Events tab." />
              <div className="mc-events">
                {visibleEvents.map((event) => (
                  <div key={`${event.event_number}-${event.session_number ?? 0}`} className="mc-event">
                    <span>#{event.event_number} · {event.distance} {formatStroke(event.stroke) || 'Event'}</span>
                    <span className="mc-event-meta">{formatEventAgeGroup(event)}{event.session_number ? ` · S${event.session_number}` : ''}</span>
                  </div>
                ))}
              </div>
              {parsedEvents.length > EVENT_PREVIEW_LIMIT && (
                <button type="button" className="btn btn-ghost u-fs-fs-sm u-mt-8px" onClick={() => setShowAllEvents((v) => !v)}>
                  {showAllEvents ? 'Show fewer' : `Show all ${parsedEvents.length}`}
                </button>
              )}
            </section>
          )}
        </div>

        {/* ── Summary rail ──────────────────────────────────────────── */}
        <aside className="mc-rail" aria-label="Meet summary">
          <div className="card">
            <h2 className="card-title u-mb-8px">{ready ? 'Ready to create' : `${remaining} thing${remaining === 1 ? '' : 's'} left`}</h2>
            <div className="mc-check">
              {checks.map((c) => (
                <button key={c.key} type="button" className="mc-check-item" data-done={c.done} onClick={() => jumpTo(c.target)}>
                  <span className="mc-check-dot" aria-hidden>{c.done && <Icon name="check" size={11} />}</span>
                  {c.label}
                </button>
              ))}
            </div>
          </div>

          <div className="card">
            <dl className="mc-facts">
              <dt>Meet</dt><dd>{draft.name.trim() || '—'}</dd>
              <dt>Course</dt><dd>{draft.course}</dd>
              <dt>Dates</dt><dd>{datesOk ? (draft.startDate === draft.endDate ? formatDateOnly(draft.startDate) : `${formatDateOnly(draft.startDate)} – ${formatDateOnly(draft.endDate)}`) : '—'}</dd>
              <dt>Sessions</dt><dd>{draft.sessions.length}</dd>
              <dt>Events</dt><dd>{parsedEvents.length > 0 ? parsedEvents.length : 'Add later'}</dd>
              <dt>Max events</dt><dd>{draft.maxEntriesPerSwimmer}</dd>
            </dl>
          </div>

          {duplicateMeet && (
            <div className="mc-banner mc-banner-warn" style={{ marginTop: 0 }}>
              <Icon name="alert-triangle" size={14} />
              <span>A meet named “{duplicateMeet.name}” already starts on {formatDateOnly(duplicateMeet.start_date)}. Check this isn’t a duplicate.</span>
            </div>
          )}
          {saveError && <div className="mc-banner mc-banner-err" role="alert" style={{ marginTop: 0 }}><Icon name="alert-circle" size={14} /> {saveError}</div>}

          <button type="submit" className="btn btn-primary" disabled={!ready || saving} style={{ width: '100%' }}>
            {saving ? <><span className="spinner spinner-sm" aria-hidden />Creating…</> : <>Create meet</>}
          </button>
          <p className="field-hint" style={{ margin: 0 }}>Saved as a draft. Publish it when you’re ready to open commitments.</p>
        </aside>
      </div>
    </form>
  )
}
