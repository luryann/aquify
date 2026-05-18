'use client'

import { useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'

// ─── Types ────────────────────────────────────────────────────────────────────

type Practice = {
  id: string
  title: string
  date: string         // YYYY-MM-DD
  startTime: string    // HH:MM
  endTime: string
  pool: string
  groups: string[]
  notes?: string
  recurring?: 'none' | 'daily' | 'weekly'
}

// ─── Mock Data ────────────────────────────────────────────────────────────────

const POOLS = ['Main Pool', 'Warm-Up Pool', 'Diving Well', 'Outdoor Pool']
const GROUPS = ['Senior A', 'Senior B', 'Junior A', 'Junior B', 'Age Group 1', 'Age Group 2', '10 & Under', 'Developmental']

const MOCK_PRACTICES: Practice[] = [
  { id: 'pr-001', title: 'Morning Practice', date: '2026-03-17', startTime: '05:30', endTime: '07:30', pool: 'Main Pool', groups: ['Senior A', 'Senior B'], recurring: 'weekly' },
  { id: 'pr-002', title: 'Afternoon Practice', date: '2026-03-17', startTime: '16:00', endTime: '18:00', pool: 'Main Pool', groups: ['Junior A', 'Junior B'], recurring: 'weekly' },
  { id: 'pr-003', title: 'Age Group Morning', date: '2026-03-17', startTime: '08:00', endTime: '09:30', pool: 'Warm-Up Pool', groups: ['Age Group 1', 'Age Group 2'], recurring: 'weekly' },
  { id: 'pr-004', title: 'Morning Practice', date: '2026-03-18', startTime: '05:30', endTime: '07:30', pool: 'Main Pool', groups: ['Senior A', 'Senior B'], recurring: 'weekly' },
  { id: 'pr-005', title: 'Afternoon Practice', date: '2026-03-18', startTime: '16:00', endTime: '18:00', pool: 'Main Pool', groups: ['Junior A', 'Junior B'], recurring: 'weekly' },
  { id: 'pr-006', title: 'Saturday Dryland + Swim', date: '2026-03-21', startTime: '09:00', endTime: '11:30', pool: 'Main Pool', groups: ['Senior A', 'Junior A'], recurring: 'none' },
  { id: 'pr-007', title: 'Morning Practice', date: '2026-03-19', startTime: '05:30', endTime: '07:30', pool: 'Main Pool', groups: ['Senior A'], recurring: 'weekly' },
  { id: 'pr-008', title: '10&U Skills Day', date: '2026-03-20', startTime: '10:00', endTime: '11:30', pool: 'Warm-Up Pool', groups: ['10 & Under', 'Developmental'], recurring: 'none', notes: 'Focus on starts and turns. Parents welcome to observe.' },
  { id: 'pr-009', title: 'Morning Practice', date: '2026-03-24', startTime: '05:30', endTime: '07:30', pool: 'Main Pool', groups: ['Senior A', 'Senior B'], recurring: 'weekly' },
  { id: 'pr-010', title: 'Afternoon Practice', date: '2026-03-24', startTime: '16:00', endTime: '18:00', pool: 'Main Pool', groups: ['Junior A', 'Junior B'], recurring: 'weekly' },
  { id: 'pr-011', title: 'Taper Practice', date: '2026-03-24', startTime: '07:45', endTime: '09:00', pool: 'Main Pool', groups: ['Senior A'], notes: 'Pre-meet taper. Race pace sets only.' },
  { id: 'pr-012', title: 'Age Group Clinic', date: '2026-03-26', startTime: '09:00', endTime: '11:00', pool: 'Warm-Up Pool', groups: ['Age Group 1', 'Age Group 2', '10 & Under'], notes: 'Technique clinic with visiting coach.' },
]

// ─── Calendar grid helpers ────────────────────────────────────────────────────

const MONTH_NAMES = ['January','February','March','April','May','June','July','August','September','October','November','December']
const DAY_LABELS = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat']

function getDaysInMonth(year: number, month: number) {
  return new Date(year, month + 1, 0).getDate()
}
function getFirstDayOfWeek(year: number, month: number) {
  return new Date(year, month, 1).getDay()
}
function toDateStr(year: number, month: number, day: number) {
  return `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`
}

function toDateInputValue(date: Date) {
  const localDate = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return localDate.toISOString().slice(0, 10)
}

function getDateRangeSummary(start: Date, end: Date) {
  return `${start.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })} – ${end.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}`
}

const GROUP_COLORS: Record<string, string> = {
  'Senior A':    '#eb5d1e',
  'Senior B':    '#f97316',
  'Junior A':    '#3b82f6',
  'Junior B':    '#60a5fa',
  'Age Group 1': '#8b5cf6',
  'Age Group 2': '#a78bfa',
  '10 & Under':  '#10b981',
  'Developmental': '#6ee7b7',
}

// ─── Create Practice Modal ────────────────────────────────────────────────────

function CreateModal({
  onClose,
  onSave,
  defaultDate,
  initialPractice,
}: {
  onClose: () => void
  onSave: (practice: Practice) => void
  defaultDate?: string
  initialPractice?: Practice | null
}) {
  const [form, setForm] = useState({
    title: initialPractice?.title ?? '',
    date: initialPractice?.date ?? defaultDate ?? '',
    startTime: initialPractice?.startTime ?? '',
    endTime: initialPractice?.endTime ?? '',
    pool: initialPractice?.pool ?? '',
    groups: initialPractice?.groups ?? [] as string[],
    notes: initialPractice?.notes ?? '',
    recurring: initialPractice?.recurring ?? 'none',
  })
  const isEditing = Boolean(initialPractice)
  const toggle = (g: string) =>
    setForm((f) => ({
      ...f,
      groups: f.groups.includes(g) ? f.groups.filter((x) => x !== g) : [...f.groups, g],
    }))

  const canSave =
    form.title.trim().length > 0 &&
    form.date.length > 0 &&
    form.startTime.length > 0 &&
    form.endTime.length > 0 &&
    form.pool.length > 0 &&
    form.groups.length > 0

  function handleSave() {
    if (!canSave) return

    onSave({
      id: initialPractice?.id ?? `pr-${Date.now()}`,
      title: form.title.trim(),
      date: form.date,
      startTime: form.startTime,
      endTime: form.endTime,
      pool: form.pool,
      groups: [...form.groups].sort(),
      notes: form.notes.trim() || undefined,
      recurring: form.recurring,
    })
    onClose()
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.55)', zIndex: 60, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '16px' }}>
      <div className="card" style={{ padding: '28px', width: '560px', maxWidth: '100%', maxHeight: '90vh', overflowY: 'auto' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '24px' }}>
          <div>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 600, color: 'var(--text)', marginBottom: '2px' }}>
              {isEditing ? 'Edit Practice' : 'Schedule Practice'}
            </h3>
            <p style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
              {isEditing ? 'Update timing, groups, and notes for this practice.' : 'Create a new practice session for one or more groups.'}
            </p>
          </div>
          <button className="btn btn-ghost" onClick={onClose}><Icon name="x" size={16} /></button>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {/* Title */}
          <div>
            <label style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', display: 'block', marginBottom: '6px' }}>Practice Title</label>
            <input className="input" value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} placeholder="e.g. Morning Practice, Dryland + Swim" style={{ width: '100%' }} />
          </div>

          {/* Date + Times */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: '12px' }}>
            <div>
              <label style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', display: 'block', marginBottom: '6px' }}>Date</label>
              <input className="input" type="date" value={form.date} onChange={(e) => setForm({ ...form, date: e.target.value })} style={{ width: '100%' }} />
            </div>
            <div>
              <label style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', display: 'block', marginBottom: '6px' }}>Start Time</label>
              <input className="input" type="time" value={form.startTime} onChange={(e) => setForm({ ...form, startTime: e.target.value })} style={{ width: '100%' }} />
            </div>
            <div>
              <label style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', display: 'block', marginBottom: '6px' }}>End Time</label>
              <input className="input" type="time" value={form.endTime} onChange={(e) => setForm({ ...form, endTime: e.target.value })} style={{ width: '100%' }} />
            </div>
          </div>

          {/* Pool */}
          <div>
            <label style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', display: 'block', marginBottom: '6px' }}>Pool / Location</label>
            <select className="input" value={form.pool} onChange={(e) => setForm({ ...form, pool: e.target.value })} style={{ width: '100%' }}>
              <option value="">Select pool...</option>
              {POOLS.map((p) => <option key={p} value={p}>{p}</option>)}
              <option value="custom">Other (custom)</option>
            </select>
          </div>

          {/* Groups */}
          <div>
            <label style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', display: 'block', marginBottom: '8px' }}>Groups</label>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px' }}>
              {GROUPS.map((g) => {
                const active = form.groups.includes(g)
                const col = GROUP_COLORS[g] ?? 'var(--accent)'
                return (
                  <button
                    key={g}
                    onClick={() => toggle(g)}
                    style={{
                      fontSize: '12px', padding: '5px 12px', borderRadius: '20px', cursor: 'pointer',
                      background: active ? col + '22' : 'var(--bg-elevated)',
                      color: active ? col : 'var(--text-subtle)',
                      border: `1px solid ${active ? col : 'var(--border)'}`,
                      fontWeight: active ? 700 : 400,
                      transition: 'all var(--dur-fast)',
                    }}
                  >{g}</button>
                )
              })}
            </div>
          </div>

          {/* Recurring */}
          <div>
            <label style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', display: 'block', marginBottom: '6px' }}>Recurring</label>
            <div style={{ display: 'flex', gap: '8px' }}>
              {([
                { v: 'none', l: 'One-time' },
                { v: 'weekly', l: 'Weekly' },
                { v: 'daily', l: 'Daily' },
              ] as const).map((opt) => (
                <button
                  key={opt.v}
                  onClick={() => setForm({ ...form, recurring: opt.v })}
                  style={{
                    fontSize: '12px', padding: '5px 14px', borderRadius: '20px', cursor: 'pointer',
                    background: form.recurring === opt.v ? 'var(--accent)' : 'var(--bg-elevated)',
                    color: form.recurring === opt.v ? '#fff' : 'var(--text-subtle)',
                    border: `1px solid ${form.recurring === opt.v ? 'var(--accent)' : 'var(--border)'}`,
                    transition: 'all var(--dur-fast)',
                  }}
                >{opt.l}</button>
              ))}
            </div>
          </div>

          {/* Notes */}
          <div>
            <label style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', display: 'block', marginBottom: '6px' }}>Notes (optional)</label>
            <textarea className="input" value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} rows={3} placeholder="Coach instructions, special focus, parent notes..." style={{ width: '100%', resize: 'vertical' }} />
          </div>
        </div>

        <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end', marginTop: '24px' }}>
          <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={handleSave} disabled={!canSave}>
            {isEditing ? 'Save Changes' : 'Schedule Practice'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── Practice Card ────────────────────────────────────────────────────────────

function PracticeCard({
  practice,
  compact = false,
  onEdit,
  onDelete,
}: {
  practice: Practice
  compact?: boolean
  onEdit?: (practice: Practice) => void
  onDelete?: (practice: Practice) => void
}) {
  return (
    <div
      style={{
        background: 'var(--bg-surface)',
        border: '1px solid var(--border)',
        borderRadius: 'var(--r-sm)',
        padding: compact ? '4px 7px' : '12px 14px',
        cursor: 'pointer',
        transition: 'border-color var(--dur-fast)',
        marginBottom: compact ? '3px' : '8px',
        borderLeft: `3px solid ${GROUP_COLORS[practice.groups[0]] ?? 'var(--accent)'}`,
      }}
      onMouseEnter={(e) => (e.currentTarget.style.borderColor = GROUP_COLORS[practice.groups[0]] ?? 'var(--accent)')}
      onMouseLeave={(e) => (e.currentTarget.style.borderColor = 'var(--border)')}
    >
      {compact ? (
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>{practice.startTime}</span>
          <span style={{ fontSize: '11px', fontWeight: 600, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>{practice.title}</span>
        </div>
      ) : (
        <>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '8px' }}>
            <div>
              <div style={{ fontWeight: 700, fontSize: '14px', color: 'var(--text)', marginBottom: '2px' }}>{practice.title}</div>
              <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                {practice.startTime} – {practice.endTime}
              </div>
            </div>
            <div style={{ display: 'flex', gap: '4px', flexShrink: 0 }}>
              {practice.recurring && practice.recurring !== 'none' && (
                <span style={{ fontSize: '10px', padding: '1px 6px', borderRadius: '6px', background: 'var(--info-bg)', color: 'var(--info)', fontFamily: 'var(--font-mono)', border: '1px solid var(--info)' }}>
                  ↻ {practice.recurring}
                </span>
              )}
              {onEdit && (
                <button className="btn btn-ghost" style={{ padding: '2px 6px' }} onClick={() => onEdit(practice)}>
                  <Icon name="edit-2" size={12} />
                </button>
              )}
              {onDelete && (
                <button className="btn btn-ghost" style={{ padding: '2px 6px' }} onClick={() => onDelete(practice)}>
                  <Icon name="trash-2" size={12} />
                </button>
              )}
            </div>
          </div>
          <div style={{ display: 'flex', gap: '6px', alignItems: 'center', flexWrap: 'wrap' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '4px', fontSize: '11px', color: 'var(--text-muted)' }}>
              <Icon name="map-pin" size={11} />
              <span>{practice.pool}</span>
            </div>
            <span style={{ color: 'var(--border-strong)' }}>·</span>
            <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
              {practice.groups.map((g) => (
                <span key={g} style={{ fontSize: '10px', padding: '1px 7px', borderRadius: '8px', background: (GROUP_COLORS[g] ?? '#999') + '22', color: GROUP_COLORS[g] ?? 'var(--text-subtle)', border: `1px solid ${(GROUP_COLORS[g] ?? '#999') + '44'}`, fontWeight: 600, fontFamily: 'var(--font-mono)' }}>
                  {g}
                </span>
              ))}
            </div>
          </div>
          {practice.notes && (
            <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '8px', lineHeight: 1.5, borderTop: '1px solid var(--border)', paddingTop: '8px' }}>{practice.notes}</p>
          )}
        </>
      )}
    </div>
  )
}

// ─── Calendar View ────────────────────────────────────────────────────────────

function CalendarView({
  practices,
  onDayClick,
  today,
}: {
  practices: Practice[]
  onDayClick: (date: string) => void
  today: string
}) {
  const initialDate = new Date(today + 'T12:00:00')
  const [year, setYear] = useState(initialDate.getFullYear())
  const [month, setMonth] = useState(initialDate.getMonth())

  const days = getDaysInMonth(year, month)
  const firstDow = getFirstDayOfWeek(year, month)

  const practicesByDate: Record<string, Practice[]> = {}
  practices.forEach((p) => {
    if (!practicesByDate[p.date]) practicesByDate[p.date] = []
    practicesByDate[p.date].push(p)
  })

  return (
    <div>
      {/* Month nav */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
        <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 600, color: 'var(--text)' }}>
          {MONTH_NAMES[month]} {year}
        </h2>
        <div style={{ display: 'flex', gap: '4px' }}>
          <button className="btn btn-ghost" style={{ padding: '6px 10px' }} onClick={() => { if (month === 0) { setMonth(11); setYear(y => y - 1) } else setMonth(m => m - 1) }}>
            <Icon name="chevron-left" size={15} />
          </button>
          <button className="btn btn-ghost" style={{ padding: '6px 10px' }} onClick={() => { if (month === 11) { setMonth(0); setYear(y => y + 1) } else setMonth(m => m + 1) }}>
            <Icon name="chevron-right" size={15} />
          </button>
        </div>
      </div>

      {/* Day labels */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: '2px', marginBottom: '4px' }}>
        {DAY_LABELS.map((d) => (
          <div key={d} style={{ textAlign: 'center', fontSize: '11px', fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', padding: '4px 0' }}>{d}</div>
        ))}
      </div>

      {/* Calendar grid */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: '2px' }}>
        {Array.from({ length: firstDow }).map((_, i) => <div key={`blank-${i}`} />)}
        {Array.from({ length: days }).map((_, i) => {
          const day = i + 1
          const dateStr = toDateStr(year, month, day)
          const dayPractices = practicesByDate[dateStr] ?? []
          const isToday = dateStr === today
          const isWeekend = ((firstDow + i) % 7 === 0 || (firstDow + i) % 7 === 6)

          return (
            <div
              key={dateStr}
              onClick={() => onDayClick(dateStr)}
              style={{
                minHeight: '110px', padding: '8px 7px',
                background: isToday ? 'var(--accent-subtle)' : isWeekend ? 'var(--bg-surface)' : 'var(--bg)',
                border: `1px solid ${isToday ? 'var(--accent)' : 'var(--border)'}`,
                borderRadius: 'var(--r-sm)', cursor: 'pointer',
                transition: 'background var(--dur-fast)',
              }}
              onMouseEnter={(e) => { if (!isToday) e.currentTarget.style.background = 'var(--bg-surface)' }}
              onMouseLeave={(e) => { if (!isToday) e.currentTarget.style.background = isWeekend ? 'var(--bg-surface)' : 'var(--bg)' }}
            >
              <div style={{
                fontSize: '12px', fontWeight: isToday ? 700 : 500,
                color: isToday ? 'var(--accent)' : 'var(--text-subtle)',
                fontFamily: 'var(--font-mono)', marginBottom: '4px',
              }}>{day}</div>
              <div>
                {dayPractices.slice(0, 3).map((p) => (
                  <div key={p.id} style={{
                    fontSize: '10px', padding: '2px 5px', marginBottom: '2px',
                    borderRadius: '4px',
                    background: (GROUP_COLORS[p.groups[0]] ?? '#888') + '22',
                    color: GROUP_COLORS[p.groups[0]] ?? 'var(--text-subtle)',
                    fontWeight: 600,
                    overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                  }}>
                    {p.startTime} {p.title}
                  </div>
                ))}
                {dayPractices.length > 3 && (
                  <div style={{ fontSize: '10px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                    +{dayPractices.length - 3} more
                  </div>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function PracticesPage() {
  const [practices, setPractices] = useState<Practice[]>(MOCK_PRACTICES)
  const [view, setView] = useState<'calendar' | 'list'>('calendar')
  const [showCreate, setShowCreate] = useState(false)
  const [createDate, setCreateDate] = useState<string | undefined>()
  const [editingPractice, setEditingPractice] = useState<Practice | null>(null)
  const [filterGroup, setFilterGroup] = useState<string>('all')
  const [filterPool, setFilterPool] = useState<string>('all')

  const today = toDateInputValue(new Date())

  const weekSummary = useMemo(() => {
    const start = new Date(today + 'T12:00:00')
    start.setDate(start.getDate() - start.getDay())
    const end = new Date(start)
    end.setDate(start.getDate() + 6)
    return {
      start: toDateInputValue(start),
      end: toDateInputValue(end),
      label: getDateRangeSummary(start, end),
    }
  }, [today])

  const monthSummary = useMemo(() => {
    const current = new Date(today + 'T12:00:00')
    return {
      prefix: today.slice(0, 7),
      label: current.toLocaleDateString('en-US', { month: 'long', year: 'numeric' }),
    }
  }, [today])

  function handleDayClick(date: string) {
    setEditingPractice(null)
    setCreateDate(date)
    setShowCreate(true)
  }

  function handleSavePractice(practice: Practice) {
    setPractices((prev) => {
      const exists = prev.some((item) => item.id === practice.id)
      const next = exists
        ? prev.map((item) => (item.id === practice.id ? practice : item))
        : [...prev, practice]
      return next.sort((a, b) => a.date.localeCompare(b.date) || a.startTime.localeCompare(b.startTime))
    })
  }

  function handleEditPractice(practice: Practice) {
    setEditingPractice(practice)
    setCreateDate(practice.date)
    setShowCreate(true)
  }

  function handleDeletePractice(practice: Practice) {
    if (!window.confirm(`Delete "${practice.title}" on ${practice.date}?`)) return
    setPractices((prev) => prev.filter((item) => item.id !== practice.id))
  }

  function handleCloseModal() {
    setShowCreate(false)
    setEditingPractice(null)
  }

  const filtered = practices
    .filter((p) => filterGroup === 'all' || p.groups.includes(filterGroup))
    .filter((p) => filterPool === 'all' || p.pool === filterPool)
    .sort((a, b) => a.date.localeCompare(b.date) || a.startTime.localeCompare(b.startTime))

  // Group by date for list view
  const byDate: Record<string, Practice[]> = {}
  filtered.forEach((p) => {
    if (!byDate[p.date]) byDate[p.date] = []
    byDate[p.date].push(p)
  })

  // Stats
  const totalThisWeek = practices.filter((p) => p.date >= weekSummary.start && p.date <= weekSummary.end).length
  const totalThisMonth = practices.filter((p) => p.date.startsWith(monthSummary.prefix)).length
  const uniqueGroups = new Set(practices.flatMap((p) => p.groups)).size

  return (
    <div style={{ padding: '24px', maxWidth: '1300px' }}>
      {/* Demo data notice */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '10px', padding: '10px 14px', marginBottom: '20px', background: 'var(--warning-bg)', border: '1px solid var(--warning-border)', borderRadius: '7px', fontSize: '12px', color: 'var(--warning)', fontFamily: 'var(--font-mono)' }}>
        <Icon name="alert-circle" size={14} color="var(--warning)" />
        <span>
          <strong>Demo data</strong> — Practice scheduler API is not yet available. Displayed schedules are illustrative only and changes are not persisted.
        </span>
      </div>

      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '28px' }}>
        <div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 600, color: 'var(--text)', marginBottom: '4px' }}>Practice Scheduler</h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>Schedule and manage practices by group, pool, and date.</p>
        </div>
        <button className="btn btn-primary" style={{ display: 'flex', alignItems: 'center', gap: '6px' }} onClick={() => { setEditingPractice(null); setCreateDate(undefined); setShowCreate(true) }}>
          <Icon name="plus" size={14} /> Schedule Practice
        </button>
      </div>

      {/* Stats strip */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px', marginBottom: '24px' }}>
        {[
          { label: 'This Week', value: totalThisWeek, sub: weekSummary.label },
          { label: 'This Month', value: totalThisMonth, sub: monthSummary.label },
          { label: 'Groups Active', value: uniqueGroups, sub: 'across all practices' },
        ].map((s) => (
          <div key={s.label} className="card" style={{ padding: '16px 18px' }}>
            <div style={{ fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '6px' }}>{s.label}</div>
            <div style={{ fontSize: '26px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--accent)', marginBottom: '2px' }}>{s.value}</div>
            <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>{s.sub}</div>
          </div>
        ))}
      </div>

      {/* Toolbar */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px', gap: '12px', flexWrap: 'wrap' }}>
        {/* Filters */}
        <div style={{ display: 'flex', gap: '10px', alignItems: 'center', flexWrap: 'wrap' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>Group:</span>
            <select
              value={filterGroup}
              onChange={(e) => setFilterGroup(e.target.value)}
              className="input"
              style={{ fontSize: '12px', padding: '4px 8px', minWidth: '140px' }}
            >
              <option value="all">All Groups</option>
              {GROUPS.map((g) => <option key={g} value={g}>{g}</option>)}
            </select>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>Pool:</span>
            <select
              value={filterPool}
              onChange={(e) => setFilterPool(e.target.value)}
              className="input"
              style={{ fontSize: '12px', padding: '4px 8px', minWidth: '140px' }}
            >
              <option value="all">All Pools</option>
              {POOLS.map((p) => <option key={p} value={p}>{p}</option>)}
            </select>
          </div>
        </div>

        {/* View toggle */}
        <div style={{ display: 'flex', gap: '2px', background: 'var(--bg-surface)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', padding: '3px' }}>
          {[{ id: 'calendar', icon: 'calendar', label: 'Calendar' }, { id: 'list', icon: 'clipboard-list', label: 'List' }].map((v) => (
            <button
              key={v.id}
              onClick={() => setView(v.id as 'calendar' | 'list')}
              style={{
                display: 'flex', alignItems: 'center', gap: '5px',
                padding: '5px 12px', borderRadius: 'calc(var(--r-md) - 2px)',
                fontSize: '12px', cursor: 'pointer',
                background: view === v.id ? 'var(--accent)' : 'transparent',
                color: view === v.id ? '#fff' : 'var(--text-subtle)',
                border: 'none', transition: 'all var(--dur-fast)',
              }}
            >
              <Icon name={v.icon as 'calendar' | 'clipboard-list'} size={13} />
              {v.label}
            </button>
          ))}
        </div>
      </div>

      {/* Content */}
      {view === 'calendar' ? (
        <div className="card" style={{ padding: '20px' }}>
          <CalendarView practices={filtered} onDayClick={handleDayClick} today={today} />
        </div>
      ) : (
        <div>
          {Object.keys(byDate).length === 0 && (
            <div className="card" style={{ padding: '48px', textAlign: 'center' }}>
              <Icon name="calendar" size={32} style={{ color: 'var(--text-muted)', marginBottom: '12px' }} />
              <p style={{ color: 'var(--text-muted)', fontSize: '14px' }}>No practices match the selected filters.</p>
            </div>
          )}
          {Object.entries(byDate).map(([date, dayPractices]) => {
            const d = new Date(date + 'T12:00:00')
            const dayLabel = d.toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' })
            return (
              <div key={date} style={{ marginBottom: '24px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '10px' }}>
                  <span style={{ fontFamily: 'var(--font-display)', fontWeight: 700, fontSize: '15px', color: 'var(--text)' }}>{dayLabel}</span>
                  <div style={{ flex: 1, height: '1px', background: 'var(--border)' }} />
                  <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>{dayPractices.length} practice{dayPractices.length !== 1 ? 's' : ''}</span>
                </div>
                {dayPractices.map((p) => (
                  <PracticeCard
                    key={p.id}
                    practice={p}
                    onEdit={handleEditPractice}
                    onDelete={handleDeletePractice}
                  />
                ))}
              </div>
            )
          })}
        </div>
      )}

      {/* Group legend */}
      <div className="card" style={{ padding: '16px 20px', marginTop: '24px' }}>
        <div style={{ fontSize: '11px', textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginBottom: '10px' }}>Group Legend</div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
          {GROUPS.map((g) => (
            <div key={g} style={{ display: 'flex', alignItems: 'center', gap: '5px', fontSize: '12px', color: 'var(--text-subtle)' }}>
              <div style={{ width: '10px', height: '10px', borderRadius: '50%', background: GROUP_COLORS[g] ?? '#888', flexShrink: 0 }} />
              {g}
            </div>
          ))}
        </div>
      </div>

      {/* Create modal */}
      {showCreate && (
        <CreateModal
          onClose={handleCloseModal}
          onSave={handleSavePractice}
          defaultDate={createDate}
          initialPractice={editingPractice}
        />
      )}
    </div>
  )
}
