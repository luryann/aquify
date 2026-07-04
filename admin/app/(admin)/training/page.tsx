'use client'

import { useState, useMemo, useCallback } from 'react'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  fetchGroups, fetchPractices, createPractice, deletePractice,
  fetchPracticeAttendance, setPracticeAttendance,
  type GroupRecord, type PracticeRecord, type AttendanceRosterEntry, type AttendanceStatus,
} from '@/lib/adminData'
import { useEffect } from 'react'
import { Icon } from '@/components/ui/icons'

type WorkoutType = 'water' | 'dryland' | 'video' | 'rest'

const WORKOUT_TYPE_LABELS: Record<WorkoutType, string> = {
  water:   'Water',
  dryland: 'Dryland',
  video:   'Video/Meeting',
  rest:    'Rest Day',
}

const WORKOUT_TYPE_COLORS: Record<WorkoutType, string> = {
  water:   'var(--info)',
  dryland: 'var(--accent)',
  video:   'var(--warning)',
  rest:    'var(--text-muted)',
}

// Local view type that mirrors PracticeRecord shape
type Workout = {
  id: string
  date: string
  groupId: string
  type: WorkoutType
  description: string
  yardageTarget: number
  attendance?: number
  strokes: { free: number; back: number; breast: number; fly: number; im: number }
}

function practiceToWorkout(p: PracticeRecord): Workout {
  return {
    id: p.id,
    date: p.date,
    groupId: p.group_id ?? '',
    type: p.type as WorkoutType,
    description: p.description,
    yardageTarget: p.yardage_target,
    attendance: p.attendance,
    strokes: p.strokes ?? { free: 0, back: 0, breast: 0, fly: 0, im: 0 },
  }
}

function StrokeBar({ value, total, color, label }: { value: number; total: number; color: string; label: string }) {
  const pct = total > 0 ? Math.round(value / total * 100) : 0
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
      <span style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', width: '36px', textTransform: 'uppercase', letterSpacing: '0.04em', flexShrink: 0 }}>
        {label}
      </span>
      <div style={{ flex: 1, height: '5px', borderRadius: '3px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
        <div style={{ height: '100%', width: `${pct}%`, background: color, borderRadius: '3px', transition: 'width 0.4s ease' }} />
      </div>
      <span style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', width: '28px', textAlign: 'right', flexShrink: 0 }}>
        {pct}%
      </span>
    </div>
  )
}

function WorkoutRow({ workout, groupName, onDelete, onTakeAttendance }: { workout: Workout; groupName: string; onDelete: () => void; onTakeAttendance: () => void }) {
  const typeColor = WORKOUT_TYPE_COLORS[workout.type]
  const totalYards = Object.values(workout.strokes).reduce((a, b) => a + b, 0)

  return (
    <div
      className="training-row"
      style={{
        display: 'grid',
        gridTemplateColumns: '100px 120px 1fr 90px 80px 68px',
        alignItems: 'center',
        gap: '12px',
        padding: '0 16px',
        height: '48px',
        borderBottom: '1px solid var(--border)',
        transition: 'background 0.1s',
      }}
      onMouseEnter={(e) => { e.currentTarget.style.background = 'rgba(255,255,255,0.02)' }}
      onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
    >
      <span style={{ fontSize: '12px', fontFamily: 'var(--font-body)', color: 'var(--text-subtle)' }}>
        {new Date(workout.date + 'T00:00:00').toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
      </span>
      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
        <span style={{ width: '5px', height: '5px', borderRadius: '50%', background: typeColor, display: 'inline-block', flexShrink: 0 }} />
        <span style={{ fontSize: '11px', color: typeColor, fontFamily: 'var(--font-body)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          {WORKOUT_TYPE_LABELS[workout.type]}
        </span>
      </div>
      <div style={{ minWidth: 0 }}>
        <span style={{ fontSize: '12px', color: 'var(--text)', display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {workout.description}
        </span>
        <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)' }}>
          {groupName}
        </span>
      </div>
      <span style={{ fontSize: '12px', fontFamily: 'var(--font-body)', color: workout.yardageTarget > 0 ? 'var(--text)' : 'var(--text-muted)', textAlign: 'right' }}>
        {totalYards > 0 ? `${totalYards.toLocaleString()} yds` : '—'}
      </span>
      <button
        onClick={onTakeAttendance}
        title="Take attendance"
        style={{
          fontSize: '11px', fontFamily: 'var(--font-body)', textAlign: 'center',
          color: workout.attendance != null ? 'var(--text)' : 'var(--text-muted)',
          background: 'transparent', border: '1px solid var(--border)', borderRadius: '6px',
          padding: '5px 6px', cursor: 'pointer', transition: 'all 0.15s',
        }}
        onMouseEnter={(e) => { e.currentTarget.style.borderColor = 'var(--accent-border)'; e.currentTarget.style.color = 'var(--accent)' }}
        onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.color = workout.attendance != null ? 'var(--text)' : 'var(--text-muted)' }}
      >
        {workout.attendance != null ? `${workout.attendance} att` : 'Take'}
      </button>
      <button
        onClick={onDelete}
        className="btn-icon"
        title="Delete workout"
        style={{ color: 'var(--text-muted)' }}
        onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--error)' }}
        onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--text-muted)' }}
      >
        <Icon name="trash-2" size={13} />
      </button>
    </div>
  )
}

const ATTENDANCE_OPTIONS: { status: AttendanceStatus; label: string; color: string }[] = [
  { status: 'present', label: 'Here',    color: 'var(--success)' },
  { status: 'excused', label: 'Excused', color: 'var(--warning)' },
  { status: 'absent',  label: 'Absent',  color: 'var(--error)' },
]

function AttendanceSheet({
  workout, groupName, onClose,
  roster, loading, error, savingSwimmerId, onMark,
}: {
  workout: Workout
  groupName: string
  onClose: () => void
  roster: AttendanceRosterEntry[]
  loading: boolean
  error: string | null
  savingSwimmerId: string | null
  onMark: (swimmerId: string, status: AttendanceStatus) => void
}) {
  const marked = roster.filter((r) => r.status).length

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Take attendance"
      style={{
        position: 'fixed', inset: 0, zIndex: 60,
        background: 'rgba(4,10,16,0.6)',
        display: 'flex', alignItems: 'flex-end', justifyContent: 'center',
      }}
      onClick={onClose}
    >
      <div
        className="attendance-sheet"
        onClick={(e) => e.stopPropagation()}
        style={{
          background: 'var(--bg-surface)',
          border: '1px solid var(--border)',
          borderBottom: 'none',
          width: '100%', maxWidth: '480px',
          maxHeight: '85vh',
          borderRadius: '18px 18px 0 0',
          display: 'flex', flexDirection: 'column',
          animation: 'attendanceSheetUp 0.22s ease-out',
        }}
      >
        <div style={{ padding: '18px 20px 14px', borderBottom: '1px solid var(--border)', flexShrink: 0 }}>
          <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '12px' }}>
            <div>
              <h2 style={{ fontFamily: 'var(--font-display)', fontSize: '19px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em' }}>
                Take Attendance
              </h2>
              <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '3px' }}>
                {groupName} · {new Date(workout.date + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}
              </p>
            </div>
            <button onClick={onClose} className="btn-icon" title="Close" style={{ color: 'var(--text-muted)' }}>
              <Icon name="x" size={16} />
            </button>
          </div>
          {roster.length > 0 && (
            <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-subtle)', marginTop: '10px', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
              {marked} of {roster.length} marked
            </p>
          )}
        </div>

        <div style={{ overflowY: 'auto', flex: 1, padding: '8px 10px' }}>
          {loading ? (
            <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>Loading roster…</div>
          ) : error ? (
            <div style={{ padding: '40px', textAlign: 'center', color: 'var(--error)', fontSize: '13px' }}>{error}</div>
          ) : roster.length === 0 ? (
            <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
              No swimmers are assigned to this group yet.
            </div>
          ) : (
            roster.map((swimmer) => (
              <div
                key={swimmer.swimmer_id}
                style={{
                  display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '10px',
                  padding: '12px 10px', borderRadius: '10px',
                }}
              >
                <span style={{ fontSize: '14px', color: 'var(--text)', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {swimmer.swimmer_name}
                </span>
                <div style={{ display: 'flex', gap: '6px', flexShrink: 0 }}>
                  {ATTENDANCE_OPTIONS.map((opt) => {
                    const active = swimmer.status === opt.status
                    const isSaving = savingSwimmerId === swimmer.swimmer_id
                    return (
                      <button
                        key={opt.status}
                        onClick={() => onMark(swimmer.swimmer_id, opt.status)}
                        disabled={isSaving}
                        style={{
                          minHeight: '42px', minWidth: '58px', padding: '0 12px',
                          borderRadius: '9px', fontSize: '12px', fontWeight: 600,
                          fontFamily: 'var(--font-body)', cursor: isSaving ? 'wait' : 'pointer',
                          border: `1px solid ${active ? opt.color : 'var(--border)'}`,
                          background: active ? opt.color : 'transparent',
                          color: active ? 'var(--text-on-accent)' : 'var(--text-muted)',
                          opacity: isSaving ? 0.6 : 1,
                          transition: 'all 0.12s',
                        }}
                      >
                        {opt.label}
                      </button>
                    )
                  })}
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '9px 11px',
  background: 'var(--bg-elevated)', border: '1px solid var(--border)',
  borderRadius: '7px', color: 'var(--text)', fontSize: '13px',
  fontFamily: 'var(--font-body)', outline: 'none', transition: 'border-color 0.15s',
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <label style={{
      display: 'block', fontSize: '10px', fontFamily: 'var(--font-body)',
      color: 'var(--text-muted)', letterSpacing: '0.07em',
      textTransform: 'uppercase', marginBottom: '6px',
    }}>
      {children}
    </label>
  )
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof ApiError) return error.message
  if (error instanceof Error && error.message) return error.message
  return fallback
}

export default function TrainingPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [activeTab, setActiveTab] = useState<'schedule' | 'stats'>('schedule')
  const [workouts, setWorkouts] = useState<Workout[]>([])
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [filterGroup, setFilterGroup] = useState<string>('all')
  const [showAddForm, setShowAddForm] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [mutationError, setMutationError] = useState<string | null>(null)

  const [attendanceWorkout, setAttendanceWorkout] = useState<Workout | null>(null)
  const [attendanceRoster, setAttendanceRoster] = useState<AttendanceRosterEntry[]>([])
  const [attendanceLoading, setAttendanceLoading] = useState(false)
  const [attendanceError, setAttendanceError] = useState<string | null>(null)
  const [savingSwimmerId, setSavingSwimmerId] = useState<string | null>(null)

  const [newWorkout, setNewWorkout] = useState({
    date: new Date().toISOString().slice(0, 10),
    groupId: '',
    type: 'water' as WorkoutType,
    description: '',
    yardageTarget: '',
    attendance: '',
    freeYds: '', backYds: '', breastYds: '', flyYds: '', imYds: '',
  })

  const loadPractices = useCallback(async () => {
    if (!orgId) return
    setLoading(true)
    setLoadError(null)
    try {
      const [practices, fetchedGroups] = await Promise.all([
        fetchPractices(api, orgId),
        fetchGroups(api, orgId),
      ])
      setWorkouts(practices.map(practiceToWorkout))
      setGroups(fetchedGroups)
      setFilterGroup((current) => current === 'all' || fetchedGroups.some((group) => group.id === current) ? current : 'all')
      setNewWorkout((current) => ({
        ...current,
        groupId: current.groupId || fetchedGroups[0]?.id || '',
      }))
    } catch (error) {
      setWorkouts([])
      setGroups([])
      setLoadError(errorMessage(error, 'Unable to load training data.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId])

  useEffect(() => {
    loadPractices()
  }, [loadPractices])

  function getGroupName(groupId: string) {
    const g = groups.find((g) => g.id === groupId)
    return g ? g.name : 'Other group'
  }

  const filteredWorkouts = useMemo(() => {
    const ws = filterGroup === 'all' ? workouts : workouts.filter((w) => w.groupId === filterGroup)
    return [...ws].sort((a, b) => b.date.localeCompare(a.date))
  }, [workouts, filterGroup])

  async function handleAddWorkout() {
    if (!newWorkout.date || !newWorkout.groupId || !newWorkout.description) return
    setSaving(true)
    setMutationError(null)
    try {
      const p = await createPractice(api, orgId, {
        date: newWorkout.date,
        group_id: newWorkout.groupId,
        type: newWorkout.type,
        description: newWorkout.description,
        yardage_target: parseInt(newWorkout.yardageTarget) || 0,
        attendance: newWorkout.attendance ? parseInt(newWorkout.attendance) : undefined,
        strokes: {
          free:   parseInt(newWorkout.freeYds)   || 0,
          back:   parseInt(newWorkout.backYds)   || 0,
          breast: parseInt(newWorkout.breastYds) || 0,
          fly:    parseInt(newWorkout.flyYds)    || 0,
          im:     parseInt(newWorkout.imYds)     || 0,
        },
      })
      setWorkouts((ws) => [practiceToWorkout(p), ...ws])
      setShowAddForm(false)
      setNewWorkout({
        date: new Date().toISOString().slice(0, 10),
        groupId: groups[0]?.id ?? '',
        type: 'water',
        description: '',
        yardageTarget: '',
        attendance: '',
        freeYds: '', backYds: '', breastYds: '', flyYds: '', imYds: '',
      })
    } catch (error) {
      setMutationError(errorMessage(error, 'Unable to save workout.'))
    } finally {
      setSaving(false)
    }
  }

  async function handleDeleteWorkout(id: string) {
    setMutationError(null)
    try {
      await deletePractice(api, orgId, id)
      setWorkouts((ws) => ws.filter((w) => w.id !== id))
    } catch (error) {
      setMutationError(errorMessage(error, 'Unable to delete workout.'))
    }
  }

  async function openAttendance(workout: Workout) {
    setAttendanceWorkout(workout)
    setAttendanceRoster([])
    setAttendanceError(null)
    setAttendanceLoading(true)
    try {
      const roster = await fetchPracticeAttendance(api, orgId, workout.id)
      setAttendanceRoster(roster)
    } catch (error) {
      setAttendanceError(errorMessage(error, 'Unable to load attendance roster.'))
    } finally {
      setAttendanceLoading(false)
    }
  }

  async function handleMarkAttendance(swimmerId: string, status: AttendanceStatus) {
    if (!attendanceWorkout) return
    setSavingSwimmerId(swimmerId)
    try {
      await setPracticeAttendance(api, orgId, attendanceWorkout.id, swimmerId, status)
      setAttendanceRoster((roster) => {
        const next = roster.map((r) => r.swimmer_id === swimmerId ? { ...r, status } : r)
        const presentCount = next.filter((r) => r.status === 'present').length
        setWorkouts((ws) => ws.map((w) => w.id === attendanceWorkout.id ? { ...w, attendance: presentCount } : w))
        return next
      })
    } catch (error) {
      setAttendanceError(errorMessage(error, 'Unable to save attendance.'))
    } finally {
      setSavingSwimmerId(null)
    }
  }

  // Weekly stats per group
  const weeklyStats = useMemo(() => {
    const now = new Date()
    const weekStart = new Date(now)
    weekStart.setDate(now.getDate() - now.getDay())
    weekStart.setHours(0, 0, 0, 0)

    const thisWeek = workouts.filter((w) => new Date(w.date + 'T00:00:00') >= weekStart)

    return groups.map((g) => {
      const gWorkouts = thisWeek.filter((w) => w.groupId === g.id)
      const totalYards = gWorkouts.reduce((sum, w) => {
        return sum + Object.values(w.strokes).reduce((a, b) => a + b, 0)
      }, 0)
      const strokes = {
        free:   gWorkouts.reduce((s, w) => s + w.strokes.free, 0),
        back:   gWorkouts.reduce((s, w) => s + w.strokes.back, 0),
        breast: gWorkouts.reduce((s, w) => s + w.strokes.breast, 0),
        fly:    gWorkouts.reduce((s, w) => s + w.strokes.fly, 0),
        im:     gWorkouts.reduce((s, w) => s + w.strokes.im, 0),
      }
      const sessions = gWorkouts.filter((w) => w.type === 'water').length
      const totalAtt = gWorkouts.reduce((s, w) => s + (w.attendance ?? 0), 0)
      return { group: g, totalYards, strokes, sessions, totalAtt, workouts: gWorkouts.length }
    })
  }, [workouts, groups])

  const tab = (t: 'schedule' | 'stats', label: string) => (
    <button
      onClick={() => setActiveTab(t)}
      style={{
        padding: '8px 18px', fontSize: '13px', fontFamily: 'var(--font-body)',
        fontWeight: activeTab === t ? 600 : 400,
        color: activeTab === t ? 'var(--text)' : 'var(--text-muted)',
        background: activeTab === t ? 'var(--bg-elevated)' : 'transparent',
        border: activeTab === t ? '1px solid var(--border)' : '1px solid transparent',
        borderRadius: '7px', cursor: 'pointer', transition: 'all 0.15s',
      }}
    >{label}</button>
  )

  return (
    <div style={{ padding: '32px', maxWidth: '1200px', margin: '0 auto' }}>
      {/* Page header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            Training
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
            Schedule workouts, track yardage, and monitor group training loads.
          </p>
        </div>
        {activeTab === 'schedule' && (
          <button
            onClick={() => setShowAddForm((v) => !v)}
            disabled={loading || groups.length === 0}
            className="btn btn-primary"
            style={{ gap: '7px', fontSize: '13px' }}
          >
            <Icon name={showAddForm ? 'x' : 'plus'} size={13} />
            {showAddForm ? 'Cancel' : 'Add Workout'}
          </button>
        )}
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: '4px', marginBottom: '24px', background: 'var(--bg-surface)', padding: '4px', borderRadius: '9px', width: 'fit-content' }}>
        {tab('schedule', 'Schedule')}
        {tab('stats', 'Weekly Stats')}
      </div>

      {/* ── Schedule Tab ── */}
      {(loadError || mutationError) && (
        <div style={{ marginBottom: '16px', padding: '12px 14px', borderRadius: '8px', border: '1px solid rgba(220,38,38,0.22)', background: 'rgba(220,38,38,0.08)', color: 'var(--error)', fontSize: '13px', display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center' }}>
          <span>{mutationError ?? loadError}</span>
          {loadError && (
            <button type="button" className="btn btn-secondary" onClick={loadPractices} style={{ fontSize: '12px', padding: '6px 10px' }}>
              Retry
            </button>
          )}
        </div>
      )}

      {activeTab === 'schedule' && (
        <div>
          {/* Add workout form */}
          {showAddForm && (
            <div className="card" style={{ padding: '24px', marginBottom: '20px', border: '1px solid var(--accent-border)', background: 'rgba(235,93,30,0.02)' }}>
              <p style={{ fontSize: '12px', fontFamily: 'var(--font-body)', color: 'var(--accent)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '18px' }}>
                New Workout
              </p>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr 1fr', gap: '14px', marginBottom: '14px' }}>
                <div>
                  <FieldLabel>Date</FieldLabel>
                  <input type="date" value={newWorkout.date}
                    onChange={(e) => setNewWorkout((d) => ({ ...d, date: e.target.value }))}
                    style={inputStyle}
                    onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                    onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                  />
                </div>
                <div>
                  <FieldLabel>Group</FieldLabel>
                  <select value={newWorkout.groupId}
                    onChange={(e) => setNewWorkout((d) => ({ ...d, groupId: e.target.value }))}
                    style={{ ...inputStyle, appearance: 'none' }}
                    onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                    onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                  >
                    <option value="">Select group…</option>
                    {groups.map((g) => (
                      <option key={g.id} value={g.id}>{g.name}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <FieldLabel>Type</FieldLabel>
                  <select value={newWorkout.type}
                    onChange={(e) => setNewWorkout((d) => ({ ...d, type: e.target.value as WorkoutType }))}
                    style={{ ...inputStyle, appearance: 'none' }}
                    onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                    onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                  >
                    {Object.entries(WORKOUT_TYPE_LABELS).map(([k, v]) => (
                      <option key={k} value={k}>{v}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <FieldLabel>Attendance</FieldLabel>
                  <input type="number" min="0" value={newWorkout.attendance}
                    onChange={(e) => setNewWorkout((d) => ({ ...d, attendance: e.target.value }))}
                    placeholder="# swimmers"
                    style={inputStyle}
                    onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                    onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                  />
                </div>
              </div>

              <div style={{ marginBottom: '14px' }}>
                <FieldLabel>Description / Sets</FieldLabel>
                <textarea
                  value={newWorkout.description}
                  onChange={(e) => setNewWorkout((d) => ({ ...d, description: e.target.value }))}
                  placeholder="e.g. 400 warm-up, 10×200 on 3:00, 8×50 sprint, 200 cool-down"
                  rows={2}
                  style={{ ...inputStyle, resize: 'vertical', lineHeight: '1.5' }}
                  onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                  onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                />
              </div>

              {newWorkout.type === 'water' && (
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: '10px', marginBottom: '16px' }}>
                  {[
                    { key: 'freeYds',   label: 'Free' },
                    { key: 'backYds',   label: 'Back' },
                    { key: 'breastYds', label: 'Breast' },
                    { key: 'flyYds',    label: 'Fly' },
                    { key: 'imYds',     label: 'IM' },
                  ].map(({ key, label }) => (
                    <div key={key}>
                      <FieldLabel>{label} (yds)</FieldLabel>
                      <input
                        type="number" min="0"
                        value={newWorkout[key as keyof typeof newWorkout]}
                        onChange={(e) => setNewWorkout((d) => ({ ...d, [key]: e.target.value }))}
                        placeholder="0"
                        style={inputStyle}
                        onFocus={(e) => { e.currentTarget.style.borderColor = 'var(--border-focus)' }}
                        onBlur={(e) => { e.currentTarget.style.borderColor = 'var(--border)' }}
                      />
                    </div>
                  ))}
                </div>
              )}

              <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                <button
                  onClick={handleAddWorkout}
                  disabled={!newWorkout.date || !newWorkout.groupId || !newWorkout.description || saving || groups.length === 0}
                  className="btn btn-primary"
                  style={{ fontSize: '13px', gap: '7px' }}
                >
                  <Icon name="plus" size={13} /> {saving ? 'Saving…' : 'Add Workout'}
                </button>
              </div>
            </div>
          )}

          {/* Filter bar */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '16px' }}>
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
              Filter:
            </span>
            <button
              onClick={() => setFilterGroup('all')}
              style={{
                padding: '4px 12px', borderRadius: '20px', fontSize: '12px', cursor: 'pointer',
                border: `1px solid ${filterGroup === 'all' ? 'var(--accent-border)' : 'var(--border)'}`,
                background: filterGroup === 'all' ? 'var(--accent-subtle)' : 'transparent',
                color: filterGroup === 'all' ? 'var(--accent)' : 'var(--text-muted)',
                fontFamily: 'var(--font-body)', transition: 'all 0.15s',
              }}
            >All Groups</button>
            {groups.map((g) => (
              <button
                key={g.id}
                onClick={() => setFilterGroup(g.id)}
                style={{
                  padding: '4px 12px', borderRadius: '20px', fontSize: '12px', cursor: 'pointer',
                  border: `1px solid ${filterGroup === g.id ? 'var(--accent-border)' : 'var(--border)'}`,
                  background: filterGroup === g.id ? 'var(--accent-subtle)' : 'transparent',
                  color: filterGroup === g.id ? 'var(--accent)' : 'var(--text-muted)',
                  fontFamily: 'var(--font-body)', transition: 'all 0.15s',
                }}
              >{g.name}</button>
            ))}
          </div>

          {/* Workout table */}
          <div className="card" style={{ overflow: 'hidden' }}>
            {/* Table header */}
            <div style={{
              display: 'grid',
              gridTemplateColumns: '100px 120px 1fr 90px 80px 36px',
              gap: '12px', padding: '0 16px', height: '36px',
              alignItems: 'center', borderBottom: '1px solid var(--border)',
              background: 'var(--bg-surface)',
            }}>
              {['Date', 'Type', 'Workout', 'Yardage', 'Att.', ''].map((h, i) => (
                <span key={i} style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', textAlign: i === 3 ? 'right' : i === 4 ? 'center' : 'left' }}>
                  {h}
                </span>
              ))}
            </div>
            {loading ? (
              <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
                Loading…
              </div>
            ) : filteredWorkouts.length === 0 ? (
              <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>
                No workouts logged yet.
              </div>
            ) : (
              filteredWorkouts.map((w) => (
                <WorkoutRow
                  key={w.id}
                  workout={w}
                  groupName={getGroupName(w.groupId)}
                  onDelete={() => handleDeleteWorkout(w.id)}
                  onTakeAttendance={() => openAttendance(w)}
                />
              ))
            )}
          </div>
        </div>
      )}

      {/* ── Stats Tab ── */}
      {activeTab === 'stats' && (
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '20px' }}>
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
              This Week
            </span>
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-subtle)' }}>
              {(() => {
                const now = new Date()
                const ws = new Date(now); ws.setDate(now.getDate() - now.getDay())
                const we = new Date(ws); we.setDate(ws.getDate() + 6)
                return `${ws.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })} – ${we.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}`
              })()}
            </span>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: '16px' }}>
            {weeklyStats.map(({ group, totalYards, strokes, sessions, totalAtt, workouts: wCount }) => (
              <div key={group.id} className="card" style={{ padding: '20px' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                    <div style={{
                      width: '32px', height: '32px', borderRadius: '8px',
                      background: 'var(--accent-subtle)', border: '1px solid var(--accent-border)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      fontSize: '11px', fontFamily: 'var(--font-body)', fontWeight: 700,
                      color: 'var(--accent)', textTransform: 'uppercase',
                    }}>
                      {(group as unknown as { code?: string }).code ?? group.name?.slice(0, 2)}
                    </div>
                    <div>
                      <p style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{group.name}</p>
                      <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)' }}>
                        {sessions} water · {wCount} total workouts
                      </p>
                    </div>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <p style={{ fontSize: '18px', fontFamily: 'var(--font-body)', fontWeight: 700, color: 'var(--text)', letterSpacing: '-0.02em' }}>
                      {totalYards > 0 ? `${(totalYards / 1000).toFixed(1)}k` : '—'}
                    </p>
                    <p style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase' }}>yds</p>
                  </div>
                </div>

                {totalYards > 0 ? (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                    <StrokeBar value={strokes.free}   total={totalYards} color='var(--info)'    label="Free" />
                    <StrokeBar value={strokes.back}   total={totalYards} color='var(--success)' label="Back" />
                    <StrokeBar value={strokes.breast} total={totalYards} color='var(--warning)'  label="Brs" />
                    <StrokeBar value={strokes.fly}    total={totalYards} color='var(--accent)'   label="Fly" />
                    <StrokeBar value={strokes.im}     total={totalYards} color='var(--text-muted)' label="IM" />
                  </div>
                ) : (
                  <p style={{ fontSize: '12px', color: 'var(--text-muted)', fontStyle: 'italic' }}>No water workouts this week.</p>
                )}

                {totalAtt > 0 && (
                  <div style={{ marginTop: '14px', paddingTop: '12px', borderTop: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between' }}>
                    <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>Attendance</span>
                    <span style={{ fontSize: '12px', fontFamily: 'var(--font-body)', color: 'var(--text)' }}>
                      avg {wCount > 0 ? Math.round(totalAtt / wCount) : 0} / session
                    </span>
                  </div>
                )}
              </div>
            ))}
          </div>

          {/* Total summary row */}
          <div className="card" style={{ padding: '16px 20px', marginTop: '16px', display: 'flex', alignItems: 'center', gap: '32px' }}>
            <div>
              <p style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '3px' }}>Total Yardage</p>
              <p style={{ fontSize: '20px', fontFamily: 'var(--font-body)', fontWeight: 700, color: 'var(--text)' }}>
                {weeklyStats.reduce((s, g) => s + g.totalYards, 0).toLocaleString()} <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontWeight: 400 }}>yds</span>
              </p>
            </div>
            <div style={{ width: '1px', height: '32px', background: 'var(--border)' }} />
            <div>
              <p style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '3px' }}>Workouts Logged</p>
              <p style={{ fontSize: '20px', fontFamily: 'var(--font-body)', fontWeight: 700, color: 'var(--text)' }}>
                {weeklyStats.reduce((s, g) => s + g.workouts, 0)}
              </p>
            </div>
            <div style={{ width: '1px', height: '32px', background: 'var(--border)' }} />
            <div>
              <p style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '3px' }}>Active Groups</p>
              <p style={{ fontSize: '20px', fontFamily: 'var(--font-body)', fontWeight: 700, color: 'var(--text)' }}>
                {weeklyStats.filter((g) => g.workouts > 0).length}
              </p>
            </div>
          </div>
        </div>
      )}

      {attendanceWorkout && (
        <AttendanceSheet
          workout={attendanceWorkout}
          groupName={getGroupName(attendanceWorkout.groupId)}
          onClose={() => setAttendanceWorkout(null)}
          roster={attendanceRoster}
          loading={attendanceLoading}
          error={attendanceError}
          savingSwimmerId={savingSwimmerId}
          onMark={handleMarkAttendance}
        />
      )}

      <style>{`
        @keyframes attendanceSheetUp {
          from { transform: translateY(24px); opacity: 0; }
          to { transform: translateY(0); opacity: 1; }
        }
        @media (min-width: 640px) {
          .attendance-sheet {
            margin-bottom: 8vh;
            border-radius: 18px;
            border-bottom: 1px solid var(--border);
          }
        }
      `}</style>
    </div>
  )
}
