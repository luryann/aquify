'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Modal } from '@/components/ui/Modal'
import { Table, col } from '@/components/ui/Table'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  createTimeStandard,
  deleteTimeStandard,
  fetchTimeStandards,
  updateTimeStandard,
  type TimeStandardRecord,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

function formatSwimTime(milliseconds: number) {
  const totalSeconds = milliseconds / 1000
  if (totalSeconds < 60) return totalSeconds.toFixed(2)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = (totalSeconds % 60).toFixed(2).padStart(5, '0')
  return `${minutes}:${seconds}`
}

function genderLabel(gender: string) {
  if (gender === 'F') return 'Girls'
  if (gender === 'M') return 'Boys'
  if (gender === 'X') return 'Mixed'
  return gender
}

function msFromParts(minutes: string, seconds: string, hundredths: string): number {
  const m = Number(minutes) || 0
  const s = Number(seconds) || 0
  const h = Number(hundredths) || 0
  return Math.round((m * 60 + s) * 1000 + h * 10)
}

const FIELD_LABEL_STYLE: React.CSSProperties = {
  display: 'block', fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '4px',
}
const FIELD_INPUT_STYLE: React.CSSProperties = {
  width: '100%', padding: '8px 12px', borderRadius: '8px', border: '1px solid var(--border)',
  background: 'var(--bg-elevated)', color: 'var(--text)', boxSizing: 'border-box',
}

const STROKE_OPTIONS = ['free', 'back', 'breast', 'fly', 'im']
const COURSE_OPTIONS = ['SCY', 'SCM', 'LCM']

export default function TimeStandardsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [standards, setStandards] = useState<TimeStandardRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [workingId, setWorkingId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [modalOpen, setModalOpen] = useState(false)

  const [search, setSearch] = useState('')
  const [strokeFilter, setStrokeFilter] = useState('')
  const [courseFilter, setCourseFilter] = useState('')

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
          setError(uiErrorMessage(err, 'Couldn’t load time standards. Try again.'))
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

  const filteredStandards = useMemo(() => {
    const q = search.trim().toLowerCase()
    return standards.filter((standard) => {
      if (strokeFilter && standard.stroke !== strokeFilter) return false
      if (courseFilter && standard.course !== courseFilter) return false
      if (q && !`${standard.name} ${standard.standard_code}`.toLowerCase().includes(q)) return false
      return true
    })
  }, [standards, search, strokeFilter, courseFilter])

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
      setModalOpen(false)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t create time standard. Try again.'))
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
      setError(uiErrorMessage(err, 'Couldn’t update time standard. Try again.'))
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
      setError(uiErrorMessage(err, 'Couldn’t delete time standard. Try again.'))
    } finally {
      setWorkingId(null)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="check-circle" title="Sign in required" description="Authenticate to manage live time standards." /></div>
  }

  const columns = [
    col<TimeStandardRecord>({ key: 'name', label: 'Name', sortable: true }),
    col<TimeStandardRecord>({
      key: 'standard_code',
      label: 'Code',
      sortable: true,
      render: (row) => <span className="badge badge-accent" title="Qualifying time standard level">{row.standard_code}</span>,
    }),
    col<TimeStandardRecord>({
      key: 'event_distance',
      label: 'Event',
      sortable: true,
      render: (row) => (
        <span style={{ color: 'var(--text-subtle)' }}>{row.event_distance} {row.stroke} · {genderLabel(row.gender)}</span>
      ),
    }),
    col<TimeStandardRecord>({ key: 'course', label: 'Course', sortable: true }),
    col<TimeStandardRecord>({
      key: 'time_ms',
      label: 'Cut time',
      sortable: true,
      render: (row) => <span style={{ fontFamily: 'var(--font-body)' }} title="Time a swimmer must beat to qualify">{formatSwimTime(row.time_ms)}</span>,
    }),
    col<TimeStandardRecord>({ key: 'season_year', label: 'Season', sortable: true }),
    col<TimeStandardRecord>({
      key: 'is_active',
      label: 'Status',
      sortable: true,
      render: (row) => (
        <span className={`badge ${row.is_active ? 'badge-success' : 'badge-default'}`}>
          {row.is_active ? 'Active' : 'Paused'}
        </span>
      ),
    }),
    col<TimeStandardRecord>({
      key: 'actions',
      label: '',
      sortable: false,
      align: 'right',
      render: (row) => (
        <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
          <button
            onClick={(e) => { e.stopPropagation(); void handleToggle(row) }}
            disabled={workingId === row.id}
            className="btn btn-sm btn-secondary"
          >
            {row.is_active ? 'Pause' : 'Activate'}
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); void handleDelete(row.id) }}
            disabled={workingId === row.id}
            className="btn btn-sm btn-danger"
          >
            Delete
          </button>
        </div>
      ),
    }),
  ]

  return (
    <div className="page">
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px', marginBottom: '24px', flexWrap: 'wrap' }}>
        <div>
          <h1 className="page-title">Time standards</h1>
        </div>
        <button onClick={() => setModalOpen(true)} className="btn btn-primary">
          <Icon name="plus" size={14} />
          New Standard
        </button>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: 'var(--fs-sm)' }}>
          {error}
        </div>
      ) : null}

      <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '16px', flexWrap: 'wrap' }}>
        <div style={{ position: 'relative', flex: '1 1 220px', maxWidth: '320px' }}>
          <span style={{ position: 'absolute', left: '11px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }}>
            <Icon name="search" size={14} />
          </span>
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name or code…"
            style={{ ...FIELD_INPUT_STYLE, paddingLeft: '32px' }}
          />
        </div>
        <select value={strokeFilter} onChange={(e) => setStrokeFilter(e.target.value)} style={{ ...FIELD_INPUT_STYLE, width: 'auto' }}>
          <option value="">All strokes</option>
          {STROKE_OPTIONS.map((s) => <option key={s} value={s}>{s}</option>)}
        </select>
        <select value={courseFilter} onChange={(e) => setCourseFilter(e.target.value)} style={{ ...FIELD_INPUT_STYLE, width: 'auto' }}>
          <option value="">All courses</option>
          {COURSE_OPTIONS.map((c) => <option key={c} value={c}>{c}</option>)}
        </select>
        <span style={{ marginLeft: 'auto', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', whiteSpace: 'nowrap' }}>
          {standards.length} total · {activeCount} active
        </span>
      </div>

      <Table
        columns={columns}
        data={filteredStandards}
        keyField="id"
        loading={loading}
        paginated
        pageSize={20}
        emptyTitle={standards.length === 0 ? 'No standards yet' : 'No matches'}
        emptyDescription={standards.length === 0 ? 'Create a standard to populate this table.' : 'Try a different search or filter.'}
      />

      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title="New time standard"
        description="Add a qualifying cut time used to evaluate swimmer results."
        footer={
          <>
            <button onClick={() => setModalOpen(false)} className="btn btn-secondary">Cancel</button>
            <button onClick={() => void handleCreate()} disabled={submitting} className="btn btn-primary">
              {submitting ? 'Creating…' : 'Create Standard'}
            </button>
          </>
        }
      >
        <div style={{ display: 'grid', gap: '16px' }}>
          <div>
            <label style={FIELD_LABEL_STYLE}>Standard name</label>
            <input value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} placeholder="e.g. 2026 Senior Cuts" style={FIELD_INPUT_STYLE} />
          </div>
          <div className="cols-2" style={{ gap: '12px' }}>
            <div>
              <label style={FIELD_LABEL_STYLE}>Season</label>
              <input value={form.season_year} onChange={(event) => setForm((current) => ({ ...current, season_year: event.target.value }))} placeholder="2026" style={FIELD_INPUT_STYLE} />
            </div>
            <div>
              <label style={FIELD_LABEL_STYLE}>Standard code</label>
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
                style={{ ...FIELD_INPUT_STYLE, textAlign: 'center', fontFamily: 'var(--font-body)' }}
              />
              <span style={{ color: 'var(--text-muted)' }}>:</span>
              <input
                value={cutTime.seconds}
                onChange={(event) => setCutTime((current) => ({ ...current, seconds: event.target.value.replace(/\D/g, '').slice(0, 2) }))}
                placeholder="58"
                inputMode="numeric"
                style={{ ...FIELD_INPUT_STYLE, textAlign: 'center', fontFamily: 'var(--font-body)' }}
              />
              <span style={{ color: 'var(--text-muted)' }}>.</span>
              <input
                value={cutTime.hundredths}
                onChange={(event) => setCutTime((current) => ({ ...current, hundredths: event.target.value.replace(/\D/g, '').slice(0, 2) }))}
                placeholder="00"
                inputMode="numeric"
                style={{ ...FIELD_INPUT_STYLE, textAlign: 'center', fontFamily: 'var(--font-body)' }}
              />
            </div>
            <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginTop: '4px', fontFamily: 'var(--font-body)' }}>
              = {formatSwimTime(msFromParts(cutTime.minutes, cutTime.seconds, cutTime.hundredths))}
            </p>
          </div>
          <div className="cols-3" style={{ gap: '12px' }}>
            <div>
              <label style={FIELD_LABEL_STYLE}>Stroke</label>
              <select value={form.stroke} onChange={(event) => setForm((current) => ({ ...current, stroke: event.target.value }))} style={FIELD_INPUT_STYLE}>
                {STROKE_OPTIONS.map((s) => <option key={s} value={s}>{s}</option>)}
              </select>
            </div>
            <div>
              <label style={FIELD_LABEL_STYLE}>Course</label>
              <select value={form.course} onChange={(event) => setForm((current) => ({ ...current, course: event.target.value }))} style={FIELD_INPUT_STYLE}>
                {COURSE_OPTIONS.map((c) => <option key={c} value={c}>{c}</option>)}
              </select>
            </div>
            <div>
              <label style={FIELD_LABEL_STYLE}>Gender</label>
              <select value={form.gender} onChange={(event) => setForm((current) => ({ ...current, gender: event.target.value }))} style={FIELD_INPUT_STYLE}>
                <option value="F">Girls</option>
                <option value="M">Boys</option>
                <option value="X">Mixed</option>
              </select>
            </div>
          </div>
        </div>
      </Modal>
    </div>
  )
}
