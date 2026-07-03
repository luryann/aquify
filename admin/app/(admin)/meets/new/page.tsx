'use client'

import { useState, useRef, useEffect, useMemo } from 'react'
import { useRouter } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { createMeet, fetchMeets, parseMeetEntryFile, type MeetRecord, type ParsedMeetEventDraftRecord } from '@/lib/adminData'
import {
  buildCreateMeetPayload,
  getNextSessionNumber,
  mapParsedSessionsToDraftSessions,
  maxEntriesPayloadValue,
  type Course,
  type MeetCreateDraftState,
  type MeetSessionDraft,
} from '@/lib/meet-draft'

type MeetEventDraft = ParsedMeetEventDraftRecord

type MeetDraft = MeetCreateDraftState & {
  name: string
  description: string
  ageGroups: string[]
}

type PackageFile = { name: string; size: number; type: string; file?: File }

const STEP_LABELS = ['Basic Info', 'Schedule', 'Entry Rules', 'Review']
const COURSES: Course[] = ['SCY', 'SCM', 'LCM']
const DEFAULT_AGE_GROUPS = ['8 & Under', '9-10', '11-12', '13-14', '15-16', '17-18', 'Open']
const MEET_ENTRY_EXTS = ['.cl2', '.sd3', '.zip', '.hy3', '.hyv', '.ev3']

// ── Input style helpers ─────────────────────────────────────────────────────

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '10px 12px',
  background: 'var(--bg-elevated)', border: '1px solid var(--border)',
  borderRadius: '8px', color: 'var(--text)', fontSize: '13px',
  fontFamily: 'var(--font-body)', outline: 'none', transition: 'border-color 0.15s',
}

function FieldLabel({ children, required }: { children: React.ReactNode; required?: boolean }) {
  return (
    <label style={{
      display: 'block', fontSize: '11px', fontFamily: 'var(--font-mono)',
      color: 'var(--text-muted)', letterSpacing: '0.06em',
      textTransform: 'uppercase', marginBottom: '7px',
    }}>
      {children}{required && <span style={{ color: 'var(--error)', marginLeft: '3px' }}>*</span>}
    </label>
  )
}

function InputField({
  value, onChange, placeholder, type = 'text', hint,
}: {
  value: string; onChange: (v: string) => void; placeholder?: string; type?: string; hint?: string
}) {
  return (
    <div>
      <input
        type={type} value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        style={inputStyle}
        onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
        onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
      />
      {hint && <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '4px' }}>{hint}</p>}
    </div>
  )
}

function deriveAgeGroupLabel(event: MeetEventDraft): string | null {
  const min = event.min_age
  const max = event.max_age
  if (min == null && max == null) return null
  if (min != null && max != null) return `${min}-${max}`
  if (min != null) return `${min}&O`
  return `Under ${max}`
}

function deriveAgeGroupsFromEvents(events: MeetEventDraft[]): string[] {
  const unique = new Set<string>()
  for (const event of events) {
    const label = deriveAgeGroupLabel(event)
    if (label) unique.add(label)
  }
  return Array.from(unique)
}

function maxEntriesButtonValue(value: number): string {
  if (value >= 6) return 'Unlimited'
  return String(value)
}

function formatEventStroke(stroke: string | undefined): string {
  if (!stroke) return 'Event'
  return stroke.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}

function formatEventAgeGroup(event: MeetEventDraft): string {
  const gender = event.gender === 'F' ? 'Girls' : event.gender === 'M' ? 'Boys' : event.gender === 'X' ? 'Mixed' : ''
  const age = deriveAgeGroupLabel(event) ?? 'Open'
  return [gender, age].filter(Boolean).join(' ')
}

// ── File upload zone ────────────────────────────────────────────────────────

function UploadZone({
  label,
  hint,
  accept,
  files,
  onFiles,
  accent,
  multiple = true,
}: {
  label: string
  hint: string
  accept: string
  files: PackageFile[]
  onFiles: (files: PackageFile[]) => void
  accent?: boolean
  multiple?: boolean
}) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)

  function handleFileList(fileList: FileList | null) {
    if (!fileList) return
    const newFiles: PackageFile[] = Array.from(fileList).map((f) => ({
      name: f.name,
      size: f.size,
      type: f.type || 'application/octet-stream',
      file: f,
    }))
    onFiles(newFiles)
  }

  function formatBytes(b: number) {
    if (b < 1024) return `${b} B`
    if (b < 1024 * 1024) return `${(b / 1024).toFixed(0)} KB`
    return `${(b / 1024 / 1024).toFixed(1)} MB`
  }

  const borderColor = dragging ? 'var(--accent)' : accent ? 'var(--accent-border)' : 'var(--border)'
  const bgColor = dragging ? 'rgba(235,93,30,0.05)' : accent ? 'rgba(235,93,30,0.02)' : 'var(--bg-elevated)'

  return (
    <div>
      <FieldLabel>{label}</FieldLabel>
      <div
        onClick={() => inputRef.current?.click()}
        onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault()
          setDragging(false)
          handleFileList(e.dataTransfer.files)
        }}
        style={{
          border: `2px dashed ${borderColor}`,
          borderRadius: '10px',
          background: bgColor,
          padding: '20px',
          cursor: 'pointer',
          transition: 'all 0.15s',
          textAlign: 'center',
        }}
      >
        <Icon name="upload-cloud" size={22} style={{ color: accent ? 'var(--accent)' : 'var(--text-muted)', margin: '0 auto 8px', display: 'block' }} />
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)', marginBottom: '4px' }}>
          Drop files or <span style={{ color: accent ? 'var(--accent)' : 'var(--info)', fontWeight: 600 }}>browse</span>
        </p>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>{hint}</p>
        <input
          ref={inputRef}
          type="file"
          accept={accept}
          multiple={multiple}
          style={{ display: 'none' }}
          onChange={(e) => handleFileList(e.target.files)}
        />
      </div>

      {files.length > 0 && (
        <div style={{ marginTop: '10px', display: 'flex', flexDirection: 'column', gap: '6px' }}>
          {files.map((f, i) => (
            <div key={i} style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '8px 12px', borderRadius: '7px',
              background: 'var(--bg-surface)', border: '1px solid var(--border)',
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', minWidth: 0 }}>
                <Icon name="file-text" size={13} style={{ color: accent ? 'var(--accent)' : 'var(--info)', flexShrink: 0 }} />
                <span style={{ fontSize: '12px', color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{f.name}</span>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexShrink: 0 }}>
                <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>{formatBytes(f.size)}</span>
                <button
                  onClick={(e) => { e.stopPropagation(); onFiles(files.filter((_, j) => j !== i)) }}
                  className="btn-icon"
                  style={{ color: 'var(--text-muted)' }}
                  onMouseEnter={(ev) => { ev.currentTarget.style.color = 'var(--error)' }}
                  onMouseLeave={(ev) => { ev.currentTarget.style.color = 'var(--text-muted)' }}
                >
                  <Icon name="x" size={12} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Session card ─────────────────────────────────────────────────────────────

function SessionCard({
  session,
  canRemove,
  onChange,
  onRemove,
}: {
  session: MeetSessionDraft
  canRemove: boolean
  onChange: (key: keyof MeetSessionDraft, value: string) => void
  onRemove: () => void
}) {
  return (
    <div style={{
      border: '1px solid var(--border)', borderRadius: '10px',
      padding: '16px', background: 'var(--bg-surface)',
    }}>
      {/* Card header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '14px' }}>
        <span style={{
          fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--accent)',
          textTransform: 'uppercase', letterSpacing: '0.08em',
        }}>
          Session {session.sessionNumber}
        </span>
        {canRemove && (
          <button
            onClick={onRemove}
            className="btn btn-ghost"
            style={{ fontSize: '11px', gap: '5px', color: 'var(--text-muted)', padding: '4px 8px' }}
            onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--error)' }}
            onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-muted)' }}
          >
            <Icon name="trash-2" size={12} /> Remove
          </button>
        )}
      </div>

      {/* Top row: Day Label + Session Name */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', marginBottom: '12px' }}>
        <div>
          <FieldLabel>Day Label</FieldLabel>
          <input
            className="input"
            value={session.section}
            onChange={(e) => onChange('section', e.target.value)}
            placeholder="e.g. Day 1"
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
            onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
          />
        </div>
        <div>
          <FieldLabel>Session Name</FieldLabel>
          <input
            className="input"
            value={session.name}
            onChange={(e) => onChange('name', e.target.value)}
            placeholder="e.g. Morning Session"
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
            onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
          />
        </div>
      </div>

      {/* Bottom row: Date + Warm-up + Start Time */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: '12px' }}>
        <div>
          <FieldLabel>Date</FieldLabel>
          <input
            type="date"
            value={session.date}
            onChange={(e) => onChange('date', e.target.value)}
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
            onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
          />
        </div>
        <div>
          <FieldLabel>Warm-up Time</FieldLabel>
          <input
            type="time"
            value={session.warmupTime}
            onChange={(e) => onChange('warmupTime', e.target.value)}
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
            onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
          />
        </div>
        <div>
          <FieldLabel>Start Time</FieldLabel>
          <input
            type="time"
            value={session.startTime}
            onChange={(e) => onChange('startTime', e.target.value)}
            style={inputStyle}
            onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
            onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
          />
        </div>
      </div>
    </div>
  )
}

// ── Main page ────────────────────────────────────────────────────────────────

export default function NewMeetPage() {
  const router = useRouter()
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [step, setStep] = useState(0)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [existingMeets, setExistingMeets] = useState<MeetRecord[]>([])

  useEffect(() => {
    if (!session?.user.orgId) return
    let cancelled = false
    fetchMeets(api, session.user.orgId)
      .then((meets) => { if (!cancelled) setExistingMeets(meets) })
      .catch(() => { /* duplicate-name check is a soft nicety, not worth failing the page over */ })
    return () => { cancelled = true }
  }, [api, session?.user.orgId])

  // File upload state
  const [packageFiles, setPackageFiles] = useState<PackageFile[]>([])
  const [entryFiles, setEntryFiles] = useState<PackageFile[]>([])
  const [entryFileParsing, setEntryFileParsing] = useState(false)
  const [entryFileParsed, setEntryFileParsed] = useState(false)
  const [entryFileParseError, setEntryFileParseError] = useState<string | null>(null)
  const [entryFileParseSummary, setEntryFileParseSummary] = useState<string | null>(null)
  const [parsedEvents, setParsedEvents] = useState<MeetEventDraft[]>([])

  const [draft, setDraft] = useState<MeetDraft>({
    name: '',
    course: 'SCY',
    hostClub: '',
    location: '',
    startDate: '',
    endDate: '',
    commitmentDeadline: '',
    entryDeadline: '',
    description: '',
    ageGroups: ['9-10', '11-12', '13-14', '15-16', '17-18'],
    maxEntriesPerSwimmer: '3',
    swimmerSelfSignup: true,
    sessions: [
      { id: 'session-1', sessionNumber: 1, section: 'Day 1', name: 'Morning Session', date: '', warmupTime: '', startTime: '' },
    ],
  })

  const duplicateMeet = useMemo(() => {
    const name = draft.name.trim().toLowerCase()
    if (!name || !draft.startDate) return null
    return existingMeets.find((m) => m.name.trim().toLowerCase() === name && m.start_date === draft.startDate) ?? null
  }, [draft.name, draft.startDate, existingMeets])

  function update<K extends keyof MeetDraft>(key: K, value: MeetDraft[K]) {
    setDraft((d) => ({ ...d, [key]: value }))
  }

  function toggleAgeGroup(ag: string) {
    setDraft((d) => ({
      ...d,
      ageGroups: d.ageGroups.includes(ag) ? d.ageGroups.filter((a) => a !== ag) : [...d.ageGroups, ag],
    }))
  }

  function canProceed(): boolean {
    if (step === 0) return !!draft.name && !!draft.course && !!draft.hostClub
    if (step === 1) return !!draft.startDate && !!draft.endDate && draft.sessions.every((s) => s.section && s.name && s.date)
    if (step === 2) return draft.ageGroups.length > 0
    return true
  }

  async function handleEntryFileUpload(files: PackageFile[]) {
    setEntryFiles(files)
    setEntryFileParseError(null)
    setEntryFileParseSummary(null)
    if (files.length === 0) {
      setEntryFileParsed(false)
      setParsedEvents([])
      return
    }
    const f = files[0]
    const ext = f.name.toLowerCase().split('.').pop() ?? ''
    if (!MEET_ENTRY_EXTS.includes('.' + ext) || !f.file || !session?.user.orgId) {
      setEntryFileParsed(false)
      return
    }

    setEntryFileParsing(true)
    setEntryFileParsed(false)
    try {
      const parsed = await parseMeetEntryFile(session.token, session.user.orgId, f.file)
      setEntryFileParsing(false)
      setEntryFileParsed(true)
      setParsedEvents(parsed.events ?? [])
      const parsedAgeGroups = deriveAgeGroupsFromEvents(parsed.events ?? [])
      setDraft((d) => ({
        ...d,
        name: d.name || parsed.meet_name || d.name,
        course: parsed.course === 'SCY' || parsed.course === 'SCM' || parsed.course === 'LCM' ? parsed.course : d.course,
        startDate: d.startDate || parsed.start_date || d.startDate,
        endDate: d.endDate || parsed.end_date || d.endDate,
        location: d.location || parsed.location || d.location,
        ageGroups: parsedAgeGroups.length > 0 ? parsedAgeGroups : d.ageGroups,
        maxEntriesPerSwimmer: maxEntriesButtonValue(parsed.signup_settings?.max_entries_per_swimmer ?? maxEntriesPayloadValue(d.maxEntriesPerSwimmer)),
        swimmerSelfSignup: parsed.signup_settings?.self_signup_enabled ?? d.swimmerSelfSignup,
        sessions: parsed.sessions.length > 0
          ? mapParsedSessionsToDraftSessions(parsed.sessions)
          : d.sessions,
      }))
      setEntryFileParseSummary(
        parsed.parse_warnings && parsed.parse_warnings.length > 0
          ? `${parsed.sessions.length} session${parsed.sessions.length === 1 ? '' : 's'} and ${parsed.events.length} event${parsed.events.length === 1 ? '' : 's'} derived. Review parse warnings before saving.`
          : `${parsed.sessions.length} session${parsed.sessions.length === 1 ? '' : 's'} and ${parsed.events.length} event${parsed.events.length === 1 ? '' : 's'} derived from ${parsed.source_file}.`,
      )
      if (parsed.parse_warnings && parsed.parse_warnings.length > 0) {
        setEntryFileParseError(parsed.parse_warnings[0] ?? null)
      }
    } catch (err) {
      setEntryFileParsing(false)
      setEntryFileParsed(false)
      setParsedEvents([])
      setEntryFileParseError(err instanceof Error ? err.message : 'Unable to parse meet file.')
    }
  }

  function updateSession(sessionId: string, key: keyof MeetSessionDraft, value: string) {
    setDraft((current) => ({
      ...current,
      sessions: current.sessions.map((s) => s.id === sessionId ? { ...s, [key]: value } : s),
    }))
  }

  function addSession() {
    setDraft((current) => ({
      ...current,
      sessions: [
        ...current.sessions,
        {
          id: `session-${Date.now()}`,
          sessionNumber: getNextSessionNumber(current.sessions),
          section: `Day ${current.sessions.length + 1}`,
          name: 'Session',
          date: current.startDate,
          warmupTime: '',
          startTime: '',
        },
      ],
    }))
  }

  function removeSession(sessionId: string) {
    setDraft((current) => ({
      ...current,
      sessions: current.sessions.length === 1
        ? current.sessions
        : current.sessions.filter((s) => s.id !== sessionId),
    }))
  }

  async function handleSave() {
    if (!session?.user.orgId) return
    setSaving(true)
    setSaveError(null)
    try {
      await createMeet(api, session.user.orgId, buildCreateMeetPayload(draft, parsedEvents))

      router.push('/meets?created=1')
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'An error occurred.'
      setSaveError(`Failed to create meet: ${msg}`)
      setSaving(false)
    }
  }

  return (
    <div style={{ padding: '32px', maxWidth: '780px', margin: '0 auto' }}>
      {/* Header */}
      <div style={{ marginBottom: '32px' }}>
        <button
          onClick={() => router.push('/meets')}
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', fontSize: '12px', fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '16px', padding: 0 }}
          onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--text)' }}
          onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-muted)' }}
        >
          <Icon name="arrow-left" size={13} />
          Back to Meets
        </button>
        <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em' }}>
          Create Meet
        </h1>
      </div>

      {/* Step indicator */}
      <div style={{ display: 'flex', gap: '0', marginBottom: '32px' }}>
        {STEP_LABELS.map((label, idx) => (
          <div key={label} style={{ display: 'flex', alignItems: 'center', flex: idx < STEP_LABELS.length - 1 ? '1' : undefined }}>
            <div
              onClick={() => idx < step && setStep(idx)}
              style={{ display: 'flex', alignItems: 'center', gap: '8px', cursor: idx < step ? 'pointer' : 'default' }}
            >
              <div style={{
                width: '28px', height: '28px', borderRadius: '50%', flexShrink: 0,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                background: idx < step ? 'var(--accent)' : idx === step ? 'var(--bg-elevated)' : 'var(--bg-surface)',
                border: `2px solid ${idx < step ? 'var(--accent)' : idx === step ? 'var(--accent-border)' : 'var(--border)'}`,
                fontSize: '11px', fontFamily: 'var(--font-mono)', fontWeight: 700,
                color: idx < step ? '#fff' : idx === step ? 'var(--accent)' : 'var(--text-muted)',
                transition: 'all 0.2s ease',
              }}>
                {idx < step ? (
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                ) : idx + 1}
              </div>
              <span style={{ fontSize: '12px', fontWeight: idx === step ? 600 : 400, color: idx === step ? 'var(--text)' : 'var(--text-muted)', whiteSpace: 'nowrap' }}>
                {label}
              </span>
            </div>
            {idx < STEP_LABELS.length - 1 && (
              <div style={{ flex: 1, height: '2px', margin: '0 12px', background: idx < step ? 'var(--accent)' : 'var(--border)', transition: 'background 0.3s ease', minWidth: '24px' }} />
            )}
          </div>
        ))}
      </div>

      {/* Step content */}
      <div className="card" style={{ padding: '32px' }}>

        {/* ── Step 0: Basic Info ───────────────────────────────── */}
        {step === 0 && (
          <div>
            <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '24px' }}>
              Basic Information
            </h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
              <div>
                <FieldLabel required>Meet Name</FieldLabel>
                <InputField
                  value={draft.name}
                  onChange={(v) => update('name', v)}
                  placeholder="e.g. Spring Invitational 2026"
                />
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
                <div>
                  <FieldLabel required>Course</FieldLabel>
                  <div style={{ display: 'flex', gap: '4px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', padding: '3px' }}>
                    {COURSES.map((c) => (
                      <button key={c} onClick={() => update('course', c)}
                        style={{
                          flex: 1, padding: '7px', borderRadius: '6px', border: 'none', cursor: 'pointer',
                          background: draft.course === c ? 'var(--accent)' : 'transparent',
                          color: draft.course === c ? '#fff' : 'var(--text-muted)',
                          fontSize: '12px', fontWeight: 700, fontFamily: 'var(--font-mono)', transition: 'all 0.15s',
                        }}>{c}</button>
                    ))}
                  </div>
                </div>
                <div>
                  <FieldLabel required>Host Club</FieldLabel>
                  <InputField
                    value={draft.hostClub}
                    onChange={(v) => update('hostClub', v)}
                    placeholder="e.g. Aquify Aquatics"
                  />
                </div>
              </div>

              <div>
                <FieldLabel>Venue / Location</FieldLabel>
                <InputField
                  value={draft.location}
                  onChange={(v) => update('location', v)}
                  placeholder="e.g. Aquatic Center, 123 Pool Rd, City, ST"
                />
              </div>

              <div>
                <FieldLabel>Description</FieldLabel>
                <textarea
                  value={draft.description}
                  onChange={(e) => update('description', e.target.value)}
                  placeholder="Optional notes, special rules, travel info…"
                  rows={3}
                  style={{ ...inputStyle, resize: 'vertical' }}
                  onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                  onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                />
              </div>

              {/* Divider */}
              <div style={{ borderTop: '1px solid var(--border)', paddingTop: '20px' }}>
                <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '16px' }}>
                  Files
                </p>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
                  <UploadZone
                    label="Meet Package Files"
                    hint="Accepts PDF, PNG, JPG"
                    accept=".pdf,.png,.jpg,.jpeg,.doc,.docx"
                    files={packageFiles}
                    onFiles={setPackageFiles}
                  />

                  <div>
                    <UploadZone
                      label="Meet Entry File"
                      hint="Accepts cl2, sd3, zip, hy3, hyv, ev3"
                      accept=".cl2,.sd3,.zip,.hy3,.hyv,.ev3"
                      files={entryFiles}
                      onFiles={handleEntryFileUpload}
                      accent
                      multiple={false}
                    />
                    {entryFileParsing && (
                      <div style={{ marginTop: '10px', display: 'flex', alignItems: 'center', gap: '8px', padding: '10px 14px', borderRadius: '8px', background: 'var(--info-bg)', border: '1px solid var(--info-border)' }}>
                        <div style={{ width: '12px', height: '12px', borderRadius: '50%', border: '2px solid rgba(96,165,250,0.3)', borderTopColor: 'var(--info)', animation: 'spin 0.8s linear infinite', flexShrink: 0 }} />
                        <span style={{ fontSize: '12px', color: 'var(--info)', fontFamily: 'var(--font-mono)' }}>Parsing meet file…</span>
                      </div>
                    )}
                    {entryFileParseError && !entryFileParsing && (
                      <div style={{ marginTop: '10px', display: 'flex', alignItems: 'center', gap: '8px', padding: '10px 14px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error)' }}>
                        <Icon name="alert-circle" size={14} style={{ color: 'var(--error)', flexShrink: 0 }} />
                        <span style={{ fontSize: '12px', color: 'var(--error)', fontFamily: 'var(--font-mono)' }}>{entryFileParseError}</span>
                      </div>
                    )}
                    {entryFileParsed && (
                      <div style={{ marginTop: '10px', display: 'flex', alignItems: 'center', gap: '8px', padding: '10px 14px', borderRadius: '8px', background: 'var(--success-bg)', border: '1px solid var(--success-border)' }}>
                        <Icon name="check-circle" size={14} style={{ color: 'var(--success)', flexShrink: 0 }} />
                        <span style={{ fontSize: '12px', color: 'var(--success)', fontFamily: 'var(--font-mono)' }}>
                          {entryFileParseSummary ?? 'Meet setup auto-populated from meet file — review schedule and signup details before saving.'}
                        </span>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* ── Step 1: Schedule & Deadlines ─────────────────────── */}
        {step === 1 && (
          <div>
            <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '24px' }}>
              Schedule & Deadlines
            </h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
                <div>
                  <FieldLabel required>Start Date</FieldLabel>
                  <InputField type="date" value={draft.startDate} onChange={(v) => update('startDate', v)} />
                </div>
                <div>
                  <FieldLabel required>End Date</FieldLabel>
                  <InputField type="date" value={draft.endDate} onChange={(v) => update('endDate', v)} />
                </div>
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '16px' }}>
                <div>
                  <FieldLabel>Commitment Deadline</FieldLabel>
                  <InputField type="datetime-local" value={draft.commitmentDeadline} onChange={(v) => update('commitmentDeadline', v)} hint="When parents must respond yes/no by" />
                </div>
                <div>
                  <FieldLabel>Entry Deadline</FieldLabel>
                  <InputField type="datetime-local" value={draft.entryDeadline} onChange={(v) => update('entryDeadline', v)} hint="Submission deadline to meet host" />
                </div>
              </div>

              {/* Session builder */}
              <div style={{ paddingTop: '8px', borderTop: '1px solid var(--border)' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '16px' }}>
                  <div>
                    <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', letterSpacing: '0.06em', textTransform: 'uppercase', marginBottom: '4px' }}>
                      Sessions
                    </p>
                    <p style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                      Define the day/session structure. Each session gets its own heat sheet.
                    </p>
                  </div>
                  <button className="btn btn-secondary" type="button" onClick={addSession} style={{ gap: '6px', fontSize: '12px', flexShrink: 0 }}>
                    <Icon name="plus" size={13} /> Add Session
                  </button>
                </div>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                  {draft.sessions.map((s) => (
                    <SessionCard
                      key={s.id}
                      session={s}
                      canRemove={draft.sessions.length > 1}
                      onChange={(key, value) => updateSession(s.id, key, value)}
                      onRemove={() => removeSession(s.id)}
                    />
                  ))}
                </div>
              </div>
            </div>
          </div>
        )}

        {/* ── Step 2: Entry Rules ──────────────────────────────── */}
        {step === 2 && (
          <div>
            <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '24px' }}>
              Entry Rules
            </h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
              <div>
                <FieldLabel required>Age Groups</FieldLabel>
                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                  {DEFAULT_AGE_GROUPS.map((ag) => (
                    <button
                      key={ag}
                      onClick={() => toggleAgeGroup(ag)}
                      style={{
                        padding: '6px 14px', borderRadius: '20px',
                        background: draft.ageGroups.includes(ag) ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                        border: `1px solid ${draft.ageGroups.includes(ag) ? 'var(--accent-border)' : 'var(--border)'}`,
                        color: draft.ageGroups.includes(ag) ? 'var(--accent)' : 'var(--text-muted)',
                        fontSize: '12px', fontWeight: draft.ageGroups.includes(ag) ? 600 : 400,
                        cursor: 'pointer', fontFamily: 'var(--font-body)', transition: 'all 0.15s',
                      }}
                    >{ag}</button>
                  ))}
                </div>
              </div>
              <div>
                <FieldLabel>Max Individual Events per Swimmer</FieldLabel>
                <div style={{ display: 'flex', gap: '4px', width: 'fit-content' }}>
                  {['2', '3', '4', '5', '6', 'Unlimited'].map((n) => (
                    <button key={n} onClick={() => update('maxEntriesPerSwimmer', n)}
                      style={{
                        padding: '7px 14px', borderRadius: '6px', border: `1px solid ${draft.maxEntriesPerSwimmer === n ? 'var(--accent-border)' : 'var(--border)'}`,
                        background: draft.maxEntriesPerSwimmer === n ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                        color: draft.maxEntriesPerSwimmer === n ? 'var(--accent)' : 'var(--text-muted)',
                        fontSize: '12px', fontFamily: 'var(--font-body)', cursor: 'pointer', transition: 'all 0.15s',
                        fontWeight: draft.maxEntriesPerSwimmer === n ? 600 : 400,
                      }}>{n}</button>
                  ))}
                </div>
              </div>

              <div>
                <FieldLabel>Swimmer Self-Signup</FieldLabel>
                <button
                  type="button"
                  onClick={() => update('swimmerSelfSignup', !draft.swimmerSelfSignup)}
                  style={{
                    display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                    width: '100%', padding: '12px 14px', borderRadius: '10px',
                    border: `1px solid ${draft.swimmerSelfSignup ? 'var(--accent-border)' : 'var(--border)'}`,
                    background: draft.swimmerSelfSignup ? 'var(--accent-subtle)' : 'var(--bg-elevated)',
                    color: 'var(--text)', cursor: 'pointer',
                  }}
                >
                  <span style={{ fontSize: '13px' }}>
                    {draft.swimmerSelfSignup ? 'Enabled for swimmer portal event selection' : 'Disabled — parent-only commitment flow'}
                  </span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: draft.swimmerSelfSignup ? 'var(--accent)' : 'var(--text-muted)' }}>
                    {draft.swimmerSelfSignup ? 'ON' : 'OFF'}
                  </span>
                </button>
              </div>

              <div>
                <FieldLabel>Imported Event Catalog</FieldLabel>
                <div style={{ padding: '12px 14px', borderRadius: '10px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                  <div style={{ fontSize: '13px', color: 'var(--text)', fontWeight: 600, marginBottom: parsedEvents.length > 0 ? '10px' : 0 }}>
                    {parsedEvents.length > 0 ? `${parsedEvents.length} signup event${parsedEvents.length === 1 ? '' : 's'} ready to create` : 'No parsed event catalog yet'}
                  </div>
                  {parsedEvents.length > 0 ? (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                      {parsedEvents.slice(0, 8).map((event) => (
                        <div key={`${event.event_number}-${event.session_number ?? 0}`} style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', fontSize: '12px' }}>
                          <span style={{ color: 'var(--text)' }}>#{event.event_number} · {event.distance} {formatEventStroke(event.stroke)}</span>
                          <span style={{ color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{formatEventAgeGroup(event)}</span>
                        </div>
                      ))}
                      {parsedEvents.length > 8 && (
                        <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                          +{parsedEvents.length - 8} more events
                        </div>
                      )}
                    </div>
                  ) : (
                    <div style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                      Upload a meet entry file to auto-build the event list used by swimmer signup.
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}

        {/* ── Step 3: Review ───────────────────────────────────── */}
        {step === 3 && (
          <div>
            <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 400, color: 'var(--text)', marginBottom: '20px' }}>
              Review & Create
            </h2>
            {duplicateMeet && (
              <div style={{ marginBottom: '16px', display: 'flex', alignItems: 'flex-start', gap: '10px', padding: '12px 14px', borderRadius: '8px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)' }}>
                <Icon name="alert-triangle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
                <p style={{ margin: 0, fontSize: '12.5px', color: 'var(--text)', lineHeight: 1.5 }}>
                  A meet named &quot;{duplicateMeet.name}&quot; on {duplicateMeet.start_date} already exists — check you&apos;re not creating a duplicate before continuing.
                </p>
              </div>
            )}
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              {[
                { label: 'Name',         value: draft.name || '—' },
                { label: 'Course',       value: draft.course },
                { label: 'Host Club',    value: draft.hostClub || '—' },
                { label: 'Location',     value: draft.location || '—' },
                { label: 'Dates',        value: draft.startDate && draft.endDate ? `${draft.startDate} → ${draft.endDate}` : '—' },
                { label: 'Commit By',    value: draft.commitmentDeadline || '—' },
                { label: 'Entry By',     value: draft.entryDeadline || '—' },
                { label: 'Age Groups',   value: draft.ageGroups.join(', ') || '—' },
                { label: 'Max Events',   value: draft.maxEntriesPerSwimmer },
                { label: 'Self-Signup',  value: draft.swimmerSelfSignup ? 'Enabled' : 'Disabled' },
                { label: 'Sessions',     value: `${draft.sessions.length} configured across ${new Set(draft.sessions.map((s) => s.section)).size} section(s)` },
                { label: 'Events',       value: parsedEvents.length > 0 ? `${parsedEvents.length} parsed for signup` : '—' },
                { label: 'Package Files', value: packageFiles.length > 0 ? packageFiles.map((f) => f.name).join(', ') : '—' },
                { label: 'Entry File',   value: entryFiles.length > 0 ? entryFiles.map((f) => f.name).join(', ') : '—' },
              ].map((r) => (
                <div key={r.label} style={{ display: 'flex', gap: '16px', padding: '10px 0', borderBottom: '1px solid var(--border)' }}>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', minWidth: '110px', flexShrink: 0, paddingTop: '1px' }}>{r.label}</span>
                  <span style={{ fontSize: '13px', color: 'var(--text)', fontWeight: 500 }}>{r.value}</span>
                </div>
              ))}
            </div>
            <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '8px' }}>
              {draft.sessions.map((s) => (
                <div key={s.id} style={{ padding: '12px 14px', borderRadius: '10px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                  <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)' }}>{s.section} · {s.name}</div>
                  <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '3px' }}>
                    {s.date || 'Date TBD'} · Warm-up {s.warmupTime || 'TBD'} · Start {s.startTime || 'TBD'}
                  </div>
                </div>
              ))}
            </div>
            {parsedEvents.length > 0 && (
              <div style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {parsedEvents.slice(0, 10).map((event) => (
                  <div key={`${event.event_number}-${event.session_number ?? 0}`} style={{ padding: '12px 14px', borderRadius: '10px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                    <div style={{ fontSize: '12px', fontWeight: 600, color: 'var(--text)' }}>
                      #{event.event_number} · {event.distance} {formatEventStroke(event.stroke)}
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginTop: '3px' }}>
                      {formatEventAgeGroup(event)} · Session {event.session_number || 'Unassigned'}
                    </div>
                  </div>
                ))}
                {parsedEvents.length > 10 && (
                  <p style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', margin: 0 }}>
                    +{parsedEvents.length - 10} additional events will also be created.
                  </p>
                )}
              </div>
            )}
            <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '16px' }}>
              The meet will be created in <strong style={{ color: 'var(--text)' }}>draft</strong> state. Publish it when you&apos;re ready to open commitments.
            </p>
          </div>
        )}
      </div>

      {saveError && (
        <div style={{ marginTop: '16px', background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error-border)', borderRadius: '8px', padding: '10px 14px', fontSize: '13px' }}>
          {saveError}
        </div>
      )}

      {/* Nav buttons */}
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '20px' }}>
        <button
          onClick={() => step > 0 ? setStep(step - 1) : router.push('/meets')}
          style={{ padding: '10px 22px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text-subtle)', fontSize: '13px', cursor: 'pointer', fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '7px', transition: 'all 0.15s' }}
          onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' }}
          onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
        >
          <Icon name="arrow-left" size={13} />
          {step === 0 ? 'Cancel' : 'Back'}
        </button>
        <button
          onClick={() => step < 3 ? setStep(step + 1) : handleSave()}
          disabled={!canProceed() || saving}
          style={{
            padding: '10px 28px', borderRadius: '8px', border: 'none',
            background: canProceed() && !saving ? 'var(--accent)' : 'var(--bg-elevated)',
            color: canProceed() && !saving ? '#fff' : 'var(--text-muted)',
            fontSize: '13px', fontWeight: 600, cursor: canProceed() && !saving ? 'pointer' : 'not-allowed',
            fontFamily: 'var(--font-body)', display: 'flex', alignItems: 'center', gap: '8px', transition: 'background 0.15s',
          }}
          onMouseEnter={(e) => { if (canProceed() && !saving) e.currentTarget.style.background = '#d44f14' }}
          onMouseLeave={(e) => { if (canProceed() && !saving) e.currentTarget.style.background = 'var(--accent)' }}
        >
          {saving ? (
            <><div style={{ width: '13px', height: '13px', borderRadius: '50%', border: '2px solid rgba(255,255,255,0.3)', borderTopColor: '#fff', animation: 'spin 0.8s linear infinite' }} />Creating…</>
          ) : step < 3 ? (
            <>Next <Icon name="arrow-right" size={13} /></>
          ) : (
            <>Create Meet <Icon name="check" size={13} /></>
          )}
        </button>
      </div>
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}
