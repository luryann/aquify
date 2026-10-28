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
import { parseBackendDate } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type MeetEventDraft = ParsedMeetEventDraftRecord

type MeetDraft = MeetCreateDraftState & {
  name: string
  description: string
  ageGroups: string[]
}

type PackageFile = { name: string; size: number; type: string; file?: File }

const STEP_LABELS = ['Basic Info', 'Schedule', 'Entry Rules', 'Review']
function formatReviewDate(value?: string | null) {
  if (!value) return '—'
  const parsed = parseBackendDate(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function formatReviewDateTime(value?: string | null) {
  if (!value) return '—'
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' })
}

const COURSES: Course[] = ['SCY', 'SCM', 'LCM']
const COURSE_FULL_NAMES: Record<Course, string> = {
  SCY: 'Short Course Yards',
  SCM: 'Short Course Meters',
  LCM: 'Long Course Meters',
}
const DEFAULT_AGE_GROUPS = ['8 & Under', '9-10', '11-12', '13-14', '15-16', '17-18', 'Open']
const MEET_ENTRY_EXTS = ['.cl2', '.sd3', '.zip', '.hy3', '.hyv', '.ev3']

// ── Input style helpers ─────────────────────────────────────────────────────

const inputStyle: React.CSSProperties = { width: '100%' }

function FieldLabel({ children, required }: { children: React.ReactNode; required?: boolean }) {
  return (
    <label className="u-fs-fs-sm u-ff-font-body u-mb-8px" style={{ display: 'block', color: 'var(--text-muted)' }}>
      {children}{required && <span className="u-ml-4px" style={{ color: 'var(--error)' }}>*</span>}
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
        aria-label={placeholder ?? type} type={type} value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        style={inputStyle}
      />
      {hint && <p className="u-fs-fs-xs u-mt-4px" style={{ color: 'var(--text-muted)' }}>{hint}</p>}
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
        className="u-p-20px" style={{ border: `2px dashed ${borderColor}`, borderRadius: '12px', background: bgColor, cursor: 'pointer', transition: 'all 0.15s', textAlign: 'center' }}
      >
        <Icon name="upload-cloud" size={22} style={{ color: accent ? 'var(--accent)' : 'var(--text-muted)', margin: '0 auto 8px', display: 'block' }} />
        <p className="u-fs-fs-base u-mb-4px" style={{ color: 'var(--text-subtle)' }}>
          Drop files or <span className="u-fw-600" style={{ color: accent ? 'var(--accent)' : 'var(--info)' }}>browse</span>
        </p>
        <p className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>{hint}</p>
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
        <div className="u-mt-12px u-gap-6px" style={{ display: 'flex', flexDirection: 'column' }}>
          {files.map((f, i) => (
            <div key={i} className="u-p-8px-12px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderRadius: '8px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
              <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', minWidth: 0 }}>
                <Icon name="file-text" size={13} style={{ color: accent ? 'var(--accent)' : 'var(--info)', flexShrink: 0 }} />
                <span className="u-fs-fs-sm" style={{ color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{f.name}</span>
              </div>
              <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', flexShrink: 0 }}>
                <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>{formatBytes(f.size)}</span>
                <button type="button"
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
    <div className="u-p-16px" style={{ border: '1px solid var(--border)', borderRadius: '12px', background: 'var(--bg-surface)' }}>
      {/* Card header */}
      <div className="u-mb-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <span className="u-fs-fs-base u-fw-600 u-ff-font-body" style={{ color: 'var(--accent-text, var(--accent))' }}>
          Session {session.sessionNumber}
        </span>
        {canRemove && (
          <button type="button"
            onClick={onRemove}
            className="btn btn-ghost u-fs-fs-xs u-gap-4px u-p-4px-8px"
            style={{ color: 'var(--text-muted)' }}
            onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--error)' }}
            onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-muted)' }}
          >
            <Icon name="trash-2" size={12} /> Remove
          </button>
        )}
      </div>

      {/* Top row: Day Label + Session Name */}
      <div className="cols-2 u-gap-12px u-mb-12px" >
        <div>
          <FieldLabel>Day label</FieldLabel>
          <input aria-label="e.g. Day 1"
            className="input"
            value={session.section}
            onChange={(e) => onChange('section', e.target.value)}
            placeholder="e.g. Day 1"
            style={inputStyle}
          />
        </div>
        <div>
          <FieldLabel>Session name</FieldLabel>
          <input aria-label="e.g. Morning Session"
            className="input"
            value={session.name}
            onChange={(e) => onChange('name', e.target.value)}
            placeholder="e.g. Morning Session"
            style={inputStyle}
          />
        </div>
      </div>

      {/* Bottom row: Date + Warm-up + Start Time */}
      <div className="cols-3 u-gap-12px" >
        <div>
          <FieldLabel>Date</FieldLabel>
          <input
            type="date"
            value={session.date}
            onChange={(e) => onChange('date', e.target.value)}
            style={inputStyle}
          />
        </div>
        <div>
          <FieldLabel>Warm-up time</FieldLabel>
          <input
            type="time"
            value={session.warmupTime}
            onChange={(e) => onChange('warmupTime', e.target.value)}
            style={inputStyle}
          />
        </div>
        <div>
          <FieldLabel>Start time</FieldLabel>
          <input
            type="time"
            value={session.startTime}
            onChange={(e) => onChange('startTime', e.target.value)}
            style={inputStyle}
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
      { id: 'session-1', sessionNumber: 1, section: 'Day 1', name: 'Morning session', date: '', warmupTime: '', startTime: '' },
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
      setEntryFileParseError(uiErrorMessage(err, 'Couldn’t parse meet file. Try again.'))
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
      const msg = uiErrorMessage(err, 'An error occurred.')
      setSaveError(`Failed to create meet: ${msg}`)
      setSaving(false)
    }
  }

  return (
    <div className="page">
      {/* Header */}
      <div className="u-mb-32px" >
        <button type="button"
          onClick={() => router.push('/meets')}
          className="u-fs-fs-sm u-ff-font-body u-gap-6px u-mb-16px u-p-0" style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', display: 'flex', alignItems: 'center' }}
          onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--text)' }}
          onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-muted)' }}
        >
          <Icon name="arrow-left" size={13} />
          Back to Meets
        </button>
        <h1 className="page-title">
          Create meet
        </h1>
      </div>

      {/* Step indicator */}
      <div className="u-gap-0 u-mb-32px" style={{ display: 'flex' }}>
        {STEP_LABELS.map((label, idx) => (
          <div key={label} style={{ display: 'flex', alignItems: 'center', flex: idx < STEP_LABELS.length - 1 ? '1' : undefined }}>
            <div
              onClick={() => idx < step && setStep(idx)}
              className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', cursor: idx < step ? 'pointer' : 'default' }}
            >
              <div className="u-fs-fs-xs u-ff-font-body u-fw-700" style={{ width: '28px', height: '28px', borderRadius: '50%', flexShrink: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', background: idx < step ? 'var(--accent)' : idx === step ? 'var(--bg-elevated)' : 'var(--bg-surface)', border: `2px solid ${idx < step ? 'var(--accent)' : idx === step ? 'var(--accent-border)' : 'var(--border)'}`, color: idx < step ? '#fff' : idx === step ? 'var(--accent)' : 'var(--text-muted)', transition: 'all 0.2s ease' }}>
                {idx < step ? (
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                ) : idx + 1}
              </div>
              <span className={idx === step ? 'step-label is-current' : 'step-label'} style={{ fontSize: 'var(--fs-sm)', fontWeight: idx === step ? 600 : 400, color: idx === step ? 'var(--text)' : 'var(--text-muted)', whiteSpace: 'nowrap' }}>
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
      <div className="card u-p-32px" >

        {/* ── Step 0: Basic Info ───────────────────────────────── */}
        {step === 0 && (
          <div>
            <h2 className="section-title u-mb-24px" >
              Basic Information
            </h2>
            <div className="u-gap-20px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div>
                <FieldLabel required>Meet name</FieldLabel>
                <InputField
                  value={draft.name}
                  onChange={(v) => update('name', v)}
                  placeholder="e.g. Spring Invitational 2026"
                />
              </div>

              <div className="cols-2 u-gap-16px" >
                <div>
                  <FieldLabel required>Course</FieldLabel>
                  <div className="u-gap-4px u-p-4px" style={{ display: 'flex', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px' }}>
                    {COURSES.map((c) => (
                      <button type="button" key={c} onClick={() => update('course', c)} title={COURSE_FULL_NAMES[c]}
                        className="u-p-8px u-fs-fs-sm u-fw-700 u-ff-font-body" style={{ flex: 1, borderRadius: '6px', border: 'none', cursor: 'pointer', background: draft.course === c ? 'var(--accent)' : 'transparent', color: draft.course === c ? '#fff' : 'var(--text-muted)', transition: 'all 0.15s' }}>{c}</button>
                    ))}
                  </div>
                  <div className="u-fs-fs-xs u-mt-4px" style={{ color: 'var(--text-muted)' }}>{COURSE_FULL_NAMES[draft.course as Course]}</div>
                </div>
                <div>
                  <FieldLabel required>Host club</FieldLabel>
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
                <textarea aria-label="Optional notes, special rules, travel info"
                  value={draft.description}
                  onChange={(e) => update('description', e.target.value)}
                  placeholder="Optional notes, special rules, travel info…"
                  rows={3}
                  style={{ ...inputStyle, resize: 'vertical' }}
                />
              </div>

              {/* Divider */}
              <div style={{ borderTop: '1px solid var(--border)', paddingTop: '20px' }}>
                <h2 className="card-title u-mb-16px" >
                  Files
                </h2>
                <div className="u-gap-20px" style={{ display: 'flex', flexDirection: 'column' }}>
                  <UploadZone
                    label="Meet package files"
                    hint="Accepts PDF, PNG, JPG"
                    accept=".pdf,.png,.jpg,.jpeg,.doc,.docx"
                    files={packageFiles}
                    onFiles={setPackageFiles}
                  />

                  <div>
                    <UploadZone
                      label="Meet entry file (optional)"
                      hint="Export this from your meet timing software (e.g. Meet Manager, TouchPad). Skip this if you don't have one yet."
                      accept=".cl2,.sd3,.zip,.hy3,.hyv,.ev3"
                      files={entryFiles}
                      onFiles={handleEntryFileUpload}
                      accent
                      multiple={false}
                    />
                    {entryFileParsing && (
                      <div className="u-mt-12px u-gap-8px u-p-12px-16px" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', background: 'var(--info-bg)', border: '1px solid var(--info-border)' }}>
                        <div style={{ width: '12px', height: '12px', borderRadius: '50%', border: '2px solid rgba(96,165,250,0.3)', borderTopColor: 'var(--info)', animation: 'spin 0.8s linear infinite', flexShrink: 0 }} />
                        <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--info)' }}>Parsing meet file…</span>
                      </div>
                    )}
                    {entryFileParseError && !entryFileParsing && (
                      <div className="u-mt-12px u-gap-8px u-p-12px-16px" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error)' }}>
                        <Icon name="alert-circle" size={14} style={{ color: 'var(--error)', flexShrink: 0 }} />
                        <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--error)' }}>{entryFileParseError}</span>
                      </div>
                    )}
                    {entryFileParsed && (
                      <div className="u-mt-12px u-gap-8px u-p-12px-16px" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', background: 'var(--success-bg)', border: '1px solid var(--success-border)' }}>
                        <Icon name="check-circle" size={14} style={{ color: 'var(--success)', flexShrink: 0 }} />
                        <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--success)' }}>
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
            <h2 className="section-title u-mb-24px" >
              Schedule & Deadlines
            </h2>
            <div className="u-gap-20px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div className="cols-2 u-gap-16px" >
                <div>
                  <FieldLabel required>Start date</FieldLabel>
                  <InputField type="date" value={draft.startDate} onChange={(v) => update('startDate', v)} />
                </div>
                <div>
                  <FieldLabel required>End date</FieldLabel>
                  <InputField type="date" value={draft.endDate} onChange={(v) => update('endDate', v)} />
                </div>
              </div>
              <div className="cols-2 u-gap-16px" >
                <div>
                  <FieldLabel>Commitment deadline</FieldLabel>
                  <InputField type="datetime-local" value={draft.commitmentDeadline} onChange={(v) => update('commitmentDeadline', v)} hint="When parents must respond yes/no by" />
                </div>
                <div>
                  <FieldLabel>Entry deadline</FieldLabel>
                  <InputField type="datetime-local" value={draft.entryDeadline} onChange={(v) => update('entryDeadline', v)} hint="Submission deadline to meet host" />
                </div>
              </div>

              {/* Session builder */}
              <div style={{ paddingTop: '8px', borderTop: '1px solid var(--border)' }}>
                <div className="u-mb-16px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <div>
                    <h2 className="card-title u-mb-4px" >
                      Sessions
                    </h2>
                  </div>
                  <button className="btn btn-secondary u-gap-6px u-fs-fs-sm" type="button" onClick={addSession} style={{ flexShrink: 0 }}>
                    <Icon name="plus" size={13} /> Add session
                  </button>
                </div>

                <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
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
            <h2 className="section-title u-mb-24px" >
              Entry Rules
            </h2>
            <div className="u-gap-20px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div>
                <FieldLabel required>Age groups</FieldLabel>
                <div className="u-gap-8px" style={{ display: 'flex', flexWrap: 'wrap' }}>
                  {DEFAULT_AGE_GROUPS.map((ag) => (
                    <button type="button"
                      key={ag}
                      onClick={() => toggleAgeGroup(ag)}
                      className="u-p-6px-16px u-fs-fs-sm u-ff-font-body" style={{ borderRadius: '16px', background: draft.ageGroups.includes(ag) ? 'var(--accent-subtle)' : 'var(--bg-elevated)', border: `1px solid ${draft.ageGroups.includes(ag) ? 'var(--accent-border)' : 'var(--border)'}`, color: draft.ageGroups.includes(ag) ? 'var(--accent)' : 'var(--text-muted)', fontWeight: draft.ageGroups.includes(ag) ? 600 : 400, cursor: 'pointer', transition: 'all 0.15s' }}
                    >{ag}</button>
                  ))}
                </div>
              </div>
              <div>
                <FieldLabel>Max individual events per swimmer</FieldLabel>
                <div className="u-gap-4px" style={{ display: 'flex', width: 'fit-content' }}>
                  {['2', '3', '4', '5', '6', 'Unlimited'].map((n) => (
                    <button type="button" key={n} onClick={() => update('maxEntriesPerSwimmer', n)}
                      className="u-p-8px-16px u-fs-fs-sm u-ff-font-body" style={{ borderRadius: '6px', border: `1px solid ${draft.maxEntriesPerSwimmer === n ? 'var(--accent-border)' : 'var(--border)'}`, background: draft.maxEntriesPerSwimmer === n ? 'var(--accent-subtle)' : 'var(--bg-elevated)', color: draft.maxEntriesPerSwimmer === n ? 'var(--accent)' : 'var(--text-muted)', cursor: 'pointer', transition: 'all 0.15s', fontWeight: draft.maxEntriesPerSwimmer === n ? 600 : 400 }}>{n}</button>
                  ))}
                </div>
              </div>

              <div>
                <FieldLabel>Swimmer self-signup</FieldLabel>
                <button
                  type="button"
                  onClick={() => update('swimmerSelfSignup', !draft.swimmerSelfSignup)}
                  className="u-p-12px-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%', borderRadius: '12px', border: `1px solid ${draft.swimmerSelfSignup ? 'var(--accent-border)' : 'var(--border)'}`, background: draft.swimmerSelfSignup ? 'var(--accent-subtle)' : 'var(--bg-elevated)', color: 'var(--text)', cursor: 'pointer' }}
                >
                  <span className="u-fs-fs-base" >
                    {draft.swimmerSelfSignup ? 'Enabled for swimmer portal event selection' : 'Disabled — parent-only commitment flow'}
                  </span>
                  <span className="u-ff-font-body u-fs-fs-xs" style={{ color: draft.swimmerSelfSignup ? 'var(--accent)' : 'var(--text-muted)' }}>
                    {draft.swimmerSelfSignup ? 'ON' : 'OFF'}
                  </span>
                </button>
              </div>

              <div>
                <FieldLabel>Imported event catalog</FieldLabel>
                <div className="u-p-12px-16px" style={{ borderRadius: '12px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                  <div className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)', marginBottom: parsedEvents.length > 0 ? '12px' : 0 }}>
                    {parsedEvents.length > 0 ? `${parsedEvents.length} signup event${parsedEvents.length === 1 ? '' : 's'} ready to create` : 'No parsed event catalog yet'}
                  </div>
                  {parsedEvents.length > 0 ? (
                    <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
                      {parsedEvents.slice(0, 8).map((event) => (
                        <div key={`${event.event_number}-${event.session_number ?? 0}`} className="u-gap-12px u-fs-fs-sm" style={{ display: 'flex', justifyContent: 'space-between' }}>
                          <span style={{ color: 'var(--text)' }}>#{event.event_number} · {event.distance} {formatEventStroke(event.stroke)}</span>
                          <span className="u-ff-font-body" style={{ color: 'var(--text-muted)' }}>{formatEventAgeGroup(event)}</span>
                        </div>
                      ))}
                      {parsedEvents.length > 8 && (
                        <div className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
                          +{parsedEvents.length - 8} more events
                        </div>
                      )}
                    </div>
                  ) : (
                    <div className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>
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
            <h2 className="section-title u-mb-20px" >
              Review & Create
            </h2>
            {duplicateMeet && (
              <div className="u-mb-16px u-gap-12px u-p-12px-16px" style={{ display: 'flex', alignItems: 'flex-start', borderRadius: '8px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)' }}>
                <Icon name="alert-triangle" size={15} style={{ color: 'var(--warning)', marginTop: '1px', flexShrink: 0 }} />
                <p className="u-m-0 u-fs-fs-sm u-lh-1_5" style={{ color: 'var(--text)' }}>
                  A meet named &quot;{duplicateMeet.name}&quot; on {duplicateMeet.start_date} already exists. Check you&apos;re not creating a duplicate before continuing.
                </p>
              </div>
            )}
            <div className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
              {[
                { label: 'Name',         value: draft.name || '—' },
                { label: 'Course',       value: draft.course },
                { label: 'Host club',    value: draft.hostClub || '—' },
                { label: 'Location',     value: draft.location || '—' },
                { label: 'Dates',        value: draft.startDate && draft.endDate ? `${formatReviewDate(draft.startDate)} to ${formatReviewDate(draft.endDate)}` : '—' },
                { label: 'Commit by',    value: formatReviewDateTime(draft.commitmentDeadline) },
                { label: 'Entry by',     value: formatReviewDateTime(draft.entryDeadline) },
                { label: 'Age groups',   value: draft.ageGroups.join(', ') || '—' },
                { label: 'Max events',   value: draft.maxEntriesPerSwimmer },
                { label: 'Self-Signup',  value: draft.swimmerSelfSignup ? 'Enabled' : 'Disabled' },
                { label: 'Sessions',     value: `${draft.sessions.length} configured across ${new Set(draft.sessions.map((s) => s.section)).size} section(s)` },
                { label: 'Events',       value: parsedEvents.length > 0 ? `${parsedEvents.length} parsed for signup` : '—' },
                { label: 'Package files', value: packageFiles.length > 0 ? packageFiles.map((f) => f.name).join(', ') : '—' },
                { label: 'Entry file',   value: entryFiles.length > 0 ? entryFiles.map((f) => f.name).join(', ') : '—' },
              ].map((r) => (
                <div key={r.label} className="u-gap-16px u-p-12px-0" style={{ display: 'flex', borderBottom: '1px solid var(--border)' }}>
                  <span className="u-ff-font-body u-fs-fs-sm" style={{ color: 'var(--text-muted)', minWidth: '110px', flexShrink: 0, paddingTop: '1px' }}>{r.label}</span>
                  <span className="u-fs-fs-base u-fw-500" style={{ color: 'var(--text)' }}>{r.value}</span>
                </div>
              ))}
            </div>
            <div className="u-mt-16px u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
              {draft.sessions.map((s) => (
                <div key={s.id} className="u-p-12px-16px" style={{ borderRadius: '12px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                  <div className="u-fs-fs-sm u-fw-600" style={{ color: 'var(--text)' }}>{s.section} · {s.name}</div>
                  <div className="u-fs-fs-xs u-ff-font-body u-mt-4px" style={{ color: 'var(--text-muted)' }}>
                    {s.date || 'Date TBD'} · Warm-up {s.warmupTime || 'TBD'} · Start {s.startTime || 'TBD'}
                  </div>
                </div>
              ))}
            </div>
            {parsedEvents.length > 0 && (
              <div className="u-mt-16px u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
                {parsedEvents.slice(0, 10).map((event) => (
                  <div key={`${event.event_number}-${event.session_number ?? 0}`} className="u-p-12px-16px" style={{ borderRadius: '12px', background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                    <div className="u-fs-fs-sm u-fw-600" style={{ color: 'var(--text)' }}>
                      #{event.event_number} · {event.distance} {formatEventStroke(event.stroke)}
                    </div>
                    <div className="u-fs-fs-xs u-ff-font-body u-mt-4px" style={{ color: 'var(--text-muted)' }}>
                      {formatEventAgeGroup(event)} · Session {event.session_number || 'Unassigned'}
                    </div>
                  </div>
                ))}
                {parsedEvents.length > 10 && (
                  <p className="u-fs-fs-xs u-ff-font-body u-m-0" style={{ color: 'var(--text-muted)' }}>
                    +{parsedEvents.length - 10} additional events will also be created.
                  </p>
                )}
              </div>
            )}
            <p className="u-fs-fs-sm u-mt-16px" style={{ color: 'var(--text-muted)' }}>
              The meet will be created in <strong style={{ color: 'var(--text)' }}>draft</strong> state. Publish it when you&apos;re ready to open commitments.
            </p>
          </div>
        )}
      </div>

      {saveError && (
        <div className="u-mt-16px u-p-12px-16px u-fs-fs-base" style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error-border)', borderRadius: '8px' }}>
          {saveError}
        </div>
      )}

      {/* Nav buttons */}
      <div className="u-mt-20px" style={{ display: 'flex', justifyContent: 'space-between' }}>
        <button type="button"
          onClick={() => step > 0 ? setStep(step - 1) : router.push('/meets')}
          className="u-p-12px-24px u-fs-fs-base u-ff-font-body u-gap-8px" style={{ background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text-subtle)', cursor: 'pointer', display: 'flex', alignItems: 'center', transition: 'all 0.15s' }}
          onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--border-strong)'; e.currentTarget.style.color = 'var(--text)' }}
          onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = 'var(--text-subtle)' }}
        >
          <Icon name="arrow-left" size={13} />
          {step === 0 ? 'Cancel' : 'Back'}
        </button>
        <button type="button"
          onClick={() => step < 3 ? setStep(step + 1) : handleSave()}
          disabled={!canProceed() || saving}
          className="u-p-12px-24px u-fs-fs-base u-fw-600 u-ff-font-body u-gap-8px" style={{ borderRadius: '8px', border: 'none', background: canProceed() && !saving ? 'var(--accent)' : 'var(--bg-elevated)', color: canProceed() && !saving ? '#fff' : 'var(--text-muted)', cursor: canProceed() && !saving ? 'pointer' : 'not-allowed', display: 'flex', alignItems: 'center', transition: 'background 0.15s' }}
          onMouseEnter={(e) => { if (canProceed() && !saving) e.currentTarget.style.background = 'var(--accent-hover)' }}
          onMouseLeave={(e) => { if (canProceed() && !saving) e.currentTarget.style.background = 'var(--accent)' }}
        >
          {saving ? (
            <><span className="spinner spinner-sm" aria-hidden />Creating…</>
          ) : step < 3 ? (
            <>Next <Icon name="arrow-right" size={13} /></>
          ) : (
            <>Create meet <Icon name="check" size={13} /></>
          )}
        </button>
      </div>
      <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  )
}
