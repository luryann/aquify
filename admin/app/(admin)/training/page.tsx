'use client'

import { useState, useMemo, useCallback, useRef } from 'react'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  fetchGroups, fetchPractices, createPractice, deletePractice,
  fetchPracticeAttendance, setPracticeAttendance,
  type GroupRecord, type PracticeRecord, type AttendanceRosterEntry, type AttendanceStatus,
} from '@/lib/adminData'
import { useEffect } from 'react'
import { useDialogA11y } from '@/components/ui/useDialogA11y'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { clearPracticeWorkout, fetchWorkout, savePracticeWorkout, type TrainingPractice } from '@/lib/trainingData'
import { WorkoutComposer, type ComposerInitial } from './WorkoutComposer'
import { WorkoutLibraryPanel } from './WorkoutLibraryPanel'
import { TrainingAnalyticsPanel } from './TrainingAnalyticsPanel'
import { SeasonPlanPanel } from './SeasonPlanPanel'

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
  workoutId?: string
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
    workoutId: (p as TrainingPractice).workout_id || undefined,
  }
}

function WorkoutRow({ workout, groupName, onDelete, onTakeAttendance, onEditSets }: { workout: Workout; groupName: string; onDelete: () => void; onTakeAttendance: () => void; onEditSets: () => void }) {
  const typeColor = WORKOUT_TYPE_COLORS[workout.type]
  const totalYards = workout.workoutId ? workout.yardageTarget : Object.values(workout.strokes).reduce((a, b) => a + b, 0)

  return (
    <div
      className="training-row u-gap-12px u-p-0-16px"
      style={{ display: 'grid', gridTemplateColumns: '100px 120px 1fr 90px 92px 80px 68px', alignItems: 'center', height: '48px', minWidth: '700px', borderBottom: '1px solid var(--border)', transition: 'background 0.1s' }}
      onMouseEnter={(e) => { e.currentTarget.style.background = 'rgba(255,255,255,0.02)' }}
      onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent' }}
    >
      <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-subtle)' }}>
        {new Date(workout.date + 'T00:00:00').toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
      </span>
      <div className="u-gap-6px" style={{ display: 'flex', alignItems: 'center' }}>
        <span style={{ width: '5px', height: '5px', borderRadius: '50%', background: typeColor, display: 'inline-block', flexShrink: 0 }} />
        <span className="u-fs-fs-sm u-ff-font-body" style={{ color: typeColor }}>
          {WORKOUT_TYPE_LABELS[workout.type]}
        </span>
      </div>
      <div style={{ minWidth: 0 }}>
        <span className="u-fs-fs-sm" style={{ color: 'var(--text)', display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {workout.description}
        </span>
        <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
          {groupName}
        </span>
      </div>
      <span className="u-fs-fs-sm u-ff-font-body" style={{ color: workout.yardageTarget > 0 ? 'var(--text)' : 'var(--text-muted)', textAlign: 'right' }}>
        {totalYards > 0 ? `${totalYards.toLocaleString()} yds` : '—'}
      </span>
      <span className="u-gap-4px" style={{ display: 'flex', justifyContent: 'center' }}>
        <button type="button" onClick={onEditSets} title={workout.workoutId ? 'Edit sets' : 'Write sets'}
          className="u-fs-fs-xs u-p-4px-8px" style={{ background: 'transparent', border: '1px solid var(--border)', borderRadius: '6px', cursor: 'pointer', color: workout.workoutId ? 'var(--text)' : 'var(--text-muted)' }}>
          {workout.workoutId ? 'Sets' : '+ Sets'}
        </button>
        {workout.workoutId && (
          <Link href={`/deck/workouts/${workout.workoutId}`} target="_blank" className="btn-icon" title="Pool-deck view" aria-label="Open pool-deck view" style={{ color: 'var(--text-muted)' }}>
            <Icon name="external-link" size={13} />
          </Link>
        )}
      </span>
      <button
        onClick={onTakeAttendance}
        title="Take attendance"
        className="u-fs-fs-xs u-ff-font-body u-p-4px-6px" style={{ textAlign: 'center', color: workout.attendance != null ? 'var(--text)' : 'var(--text-muted)', background: 'transparent', border: '1px solid var(--border)', borderRadius: '6px', cursor: 'pointer', transition: 'all 0.15s' }}
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
  const sheetRef = useRef<HTMLDivElement>(null)
  useDialogA11y(sheetRef, onClose)
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
        ref={sheetRef}
        tabIndex={-1}
        className="attendance-sheet"
        onClick={(e) => e.stopPropagation()}
        style={{
          background: 'var(--bg-surface)',
          border: '1px solid var(--border)',
          borderBottom: 'none',
          width: '100%', maxWidth: '480px',
          maxHeight: '85vh',
          borderRadius: '16px 16px 0 0',
          display: 'flex', flexDirection: 'column',
          animation: 'attendanceSheetUp 0.22s ease-out',
        }}
      >
        <div style={{ padding: '20px 20px 16px', borderBottom: '1px solid var(--border)', flexShrink: 0 }}>
          <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
            <div>
              <h2 className="section-title">
                Take Attendance
              </h2>
              <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-muted)' }}>
                {groupName} · {new Date(workout.date + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}
              </p>
            </div>
            <button onClick={onClose} className="btn-icon" title="Close" style={{ color: 'var(--text-muted)' }}>
              <Icon name="x" size={16} />
            </button>
          </div>
          {roster.length > 0 && (
            <p className="u-fs-fs-xs u-ff-font-body u-mt-12px" style={{ color: 'var(--text-subtle)' }}>
              {marked} of {roster.length} marked
            </p>
          )}
        </div>

        <div className="u-p-8px-12px" style={{ overflowY: 'auto', flex: 1 }}>
          {loading ? (
            <div className="u-p-40px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-muted)' }}>Loading roster…</div>
          ) : error ? (
            <div className="u-p-40px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--error)' }}>{error}</div>
          ) : roster.length === 0 ? (
            <div className="u-p-40px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
              No swimmers are assigned to this group yet.
            </div>
          ) : (
            roster.map((swimmer) => (
              <div
                key={swimmer.swimmer_id}
                className="u-gap-12px u-p-12px-12px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderRadius: '12px' }}
              >
                <span className="u-fs-fs-md" style={{ color: 'var(--text)', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {swimmer.swimmer_name}
                </span>
                <div className="u-gap-6px" style={{ display: 'flex', flexShrink: 0 }}>
                  {ATTENDANCE_OPTIONS.map((opt) => {
                    const active = swimmer.status === opt.status
                    const isSaving = savingSwimmerId === swimmer.swimmer_id
                    return (
                      <button
                        key={opt.status}
                        onClick={() => onMark(swimmer.swimmer_id, opt.status)}
                        disabled={isSaving}
                        className="u-p-0-12px u-fs-fs-sm u-fw-600 u-ff-font-body" style={{ minHeight: '42px', minWidth: '58px', borderRadius: '8px', cursor: isSaving ? 'wait' : 'pointer', border: `1px solid ${active ? opt.color : 'var(--border)'}`, background: active ? opt.color : 'transparent', color: active ? 'var(--text-on-accent)' : 'var(--text-muted)', opacity: isSaving ? 0.6 : 1, transition: 'all 0.12s' }}
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

const inputStyle: React.CSSProperties = { width: '100%' }

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <label className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ display: 'block', color: 'var(--text-muted)' }}>
      {children}
    </label>
  )
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof ApiError) return error.message
  if (error instanceof Error && error.message) return error.message
  return fallback
}

type Tab = 'schedule' | 'library' | 'plan' | 'stats'

export default function TrainingPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [activeTab, setActiveTab] = useState<Tab>('schedule')
  const [setsTarget, setSetsTarget] = useState<{ workout: Workout; initial: ComposerInitial } | null>(null)
  useEffect(() => {
    if (!setsTarget) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setSetsTarget(null) }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [setsTarget])
  const [workouts, setWorkouts] = useState<Workout[]>([])
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [filterGroup, setFilterGroup] = useState<string>('all')
  const [showAddForm, setShowAddForm] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [mutationError, setMutationError] = useState<string | null>(null)

  const [attendanceWorkout, setAttendanceWorkout] = useState<Workout | null>(null)
  const attendanceRequestIdRef = useRef<string | null>(null)
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
      setLoadError(errorMessage(error, 'Couldn’t load training data. Try again.'))
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
      setMutationError(errorMessage(error, 'Couldn’t save workout. Try again.'))
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
      setMutationError(errorMessage(error, 'Couldn’t delete workout. Try again.'))
    }
  }

  async function openAttendance(workout: Workout) {
    attendanceRequestIdRef.current = workout.id
    setAttendanceWorkout(workout)
    setAttendanceRoster([])
    setAttendanceError(null)
    setAttendanceLoading(true)
    try {
      const roster = await fetchPracticeAttendance(api, orgId, workout.id)
      // A faster response for a since-superseded request must not overwrite
      // whichever workout's sheet is now open.
      if (attendanceRequestIdRef.current !== workout.id) return
      setAttendanceRoster(roster)
    } catch (error) {
      if (attendanceRequestIdRef.current !== workout.id) return
      setAttendanceError(errorMessage(error, 'Couldn’t load attendance roster. Try again.'))
    } finally {
      if (attendanceRequestIdRef.current === workout.id) {
        setAttendanceLoading(false)
      }
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
      setAttendanceError(errorMessage(error, 'Couldn’t save attendance. Try again.'))
    } finally {
      setSavingSwimmerId(null)
    }
  }

  async function openSets(workout: Workout) {
    setMutationError(null)
    if (!workout.workoutId) {
      setSetsTarget({ workout, initial: { title: workout.description, course: 'SCY', tags: [], text: '' } })
      return
    }
    try {
      const { workout: w, text } = await fetchWorkout(api, orgId, workout.workoutId)
      setSetsTarget({ workout, initial: { title: w.title, course: w.course, tags: w.tags, text } })
    } catch (error) {
      setMutationError(errorMessage(error, 'Couldn’t open the sets for this practice. Try again.'))
    }
  }

  async function handleClearSets() {
    if (!setsTarget) return
    try {
      await clearPracticeWorkout(api, orgId, setsTarget.workout.id)
      setWorkouts((ws) => ws.map((w) => w.id === setsTarget.workout.id ? { ...w, workoutId: undefined } : w))
      setSetsTarget(null)
    } catch (error) {
      setMutationError(errorMessage(error, 'Couldn’t remove the sets. Try again.'))
    }
  }

  const tab = (t: Tab, label: string) => (
    <button
      onClick={() => setActiveTab(t)}
      className="u-p-8px-20px u-fs-fs-base u-ff-font-body" style={{ fontWeight: activeTab === t ? 600 : 400, color: activeTab === t ? 'var(--text)' : 'var(--text-muted)', background: activeTab === t ? 'var(--bg-elevated)' : 'transparent', border: activeTab === t ? '1px solid var(--border)' : '1px solid transparent', borderRadius: '8px', cursor: 'pointer', transition: 'all 0.15s', whiteSpace: 'nowrap', flexShrink: 0 }}
    >{label}</button>
  )

  return (
    <div className="page">
      {/* Page header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            Training
          </h1>
        </div>
        {activeTab === 'schedule' && (
          <button
            onClick={() => setShowAddForm((v) => !v)}
            disabled={loading || groups.length === 0}
            className="btn btn-primary u-gap-8px u-fs-fs-base"
            
          >
            <Icon name={showAddForm ? 'x' : 'plus'} size={13} />
            {showAddForm ? 'Cancel' : 'Add Workout'}
          </button>
        )}
      </div>

      {/* Tabs */}
      <div className="tab-strip u-gap-4px u-mb-24px u-p-4px" style={{ display: 'flex', background: 'var(--bg-surface)', borderRadius: '8px', width: 'fit-content', maxWidth: '100%', overflowX: 'auto' }}>
        {tab('schedule', 'Schedule')}
        {tab('library', 'Library')}
        {tab('plan', 'Season Plan')}
        {tab('stats', 'Analytics')}
      </div>

      {/* ── Schedule Tab ── */}
      {(loadError || mutationError) && (
        <div className="u-mb-16px u-p-12px-16px u-fs-fs-base u-gap-12px" style={{ borderRadius: '8px', border: '1px solid rgba(220,38,38,0.22)', background: 'rgba(220,38,38,0.08)', color: 'var(--error)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{mutationError ?? loadError}</span>
          {loadError && (
            <button type="button" className="btn btn-secondary u-fs-fs-sm u-p-6px-12px" onClick={loadPractices} >
              Retry
            </button>
          )}
        </div>
      )}

      {activeTab === 'schedule' && (
        <div>
          {/* Add workout form */}
          {showAddForm && (
            <div className="card u-p-24px u-mb-20px" style={{ border: '1px solid var(--accent-border)', background: 'rgba(235,93,30,0.02)' }}>
              <div className="cols-4 u-gap-16px u-mb-16px" >
                <div>
                  <FieldLabel>Date</FieldLabel>
                  <input type="date" value={newWorkout.date}
                    onChange={(e) => setNewWorkout((d) => ({ ...d, date: e.target.value }))}
                    style={inputStyle}
                  />
                </div>
                <div>
                  <FieldLabel>Group</FieldLabel>
                  <select value={newWorkout.groupId}
                    onChange={(e) => setNewWorkout((d) => ({ ...d, groupId: e.target.value }))}
                    style={{ ...inputStyle }}
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
                    style={{ ...inputStyle }}
                  >
                    {Object.entries(WORKOUT_TYPE_LABELS).map(([k, v]) => (
                      <option key={k} value={k}>{v}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <FieldLabel>Attendance</FieldLabel>
                  <input aria-label="# swimmers" type="number" min="0" value={newWorkout.attendance}
                    onChange={(e) => setNewWorkout((d) => ({ ...d, attendance: e.target.value }))}
                    placeholder="# swimmers"
                    style={inputStyle}
                  />
                </div>
              </div>

              <div className="u-mb-16px" >
                <FieldLabel>Description / Sets</FieldLabel>
                <textarea aria-label="e.g. 400 warm-up, 10×200 on 3:00, 8×50 sprint, 200 cool-down"
                  value={newWorkout.description}
                  onChange={(e) => setNewWorkout((d) => ({ ...d, description: e.target.value }))}
                  placeholder="e.g. 400 warm-up, 10×200 on 3:00, 8×50 sprint, 200 cool-down"
                  rows={2}
                  style={{ ...inputStyle, resize: 'vertical', lineHeight: '1.5' }}
                />
              </div>

              {newWorkout.type === 'water' && (
                <div className="cols-5 u-gap-12px u-mb-16px" >
                  {[
                    { key: 'freeYds',   label: 'Free' },
                    { key: 'backYds',   label: 'Back' },
                    { key: 'breastYds', label: 'Breast' },
                    { key: 'flyYds',    label: 'Fly' },
                    { key: 'imYds',     label: 'IM' },
                  ].map(({ key, label }) => (
                    <div key={key}>
                      <FieldLabel>{label} (yds)</FieldLabel>
                      <input aria-label="0"
                        type="number" min="0"
                        value={newWorkout[key as keyof typeof newWorkout]}
                        onChange={(e) => setNewWorkout((d) => ({ ...d, [key]: e.target.value }))}
                        placeholder="0"
                        style={inputStyle}
                      />
                    </div>
                  ))}
                </div>
              )}

              <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                <button
                  onClick={handleAddWorkout}
                  disabled={!newWorkout.date || !newWorkout.groupId || !newWorkout.description || saving || groups.length === 0}
                  className="btn btn-primary u-fs-fs-base u-gap-8px"
                  
                >
                  <Icon name="plus" size={13} /> {saving ? 'Saving…' : 'Add Workout'}
                </button>
              </div>
            </div>
          )}

          {/* Filter bar */}
          <div className="u-gap-8px u-mb-16px" style={{ display: 'flex', alignItems: 'center' }}>
            <span className="u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
              Group
            </span>
            <button
              onClick={() => setFilterGroup('all')}
              className="u-p-4px-12px u-fs-fs-sm u-ff-font-body" style={{ borderRadius: '16px', cursor: 'pointer', border: `1px solid ${filterGroup === 'all' ? 'var(--accent-border)' : 'var(--border)'}`, background: filterGroup === 'all' ? 'var(--accent-subtle)' : 'transparent', color: filterGroup === 'all' ? 'var(--accent)' : 'var(--text-muted)', transition: 'all 0.15s' }}
            >All groups</button>
            {groups.map((g) => (
              <button
                key={g.id}
                onClick={() => setFilterGroup(g.id)}
                className="u-p-4px-12px u-fs-fs-sm u-ff-font-body" style={{ borderRadius: '16px', cursor: 'pointer', border: `1px solid ${filterGroup === g.id ? 'var(--accent-border)' : 'var(--border)'}`, background: filterGroup === g.id ? 'var(--accent-subtle)' : 'transparent', color: filterGroup === g.id ? 'var(--accent)' : 'var(--text-muted)', transition: 'all 0.15s' }}
              >{g.name}</button>
            ))}
          </div>

          {/* Workout table */}
          <div className="card u-p-0" style={{ overflowX: 'auto', overflowY: 'hidden' }}>
            {/* Table header */}
            <div className="u-gap-12px u-p-0-16px" style={{ display: 'grid', gridTemplateColumns: '100px 120px 1fr 90px 92px 80px 68px', height: '36px', minWidth: '700px', alignItems: 'center', borderBottom: '1px solid var(--border)', background: 'var(--bg-surface)' }}>
              {['Date', 'Type', 'Workout', 'Yardage', 'Sets', 'Att.', ''].map((h, i) => (
                <span key={i} className="u-fs-fs-2xs u-ff-font-body" style={{ color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', textAlign: i === 3 ? 'right' : i === 4 || i === 5 ? 'center' : 'left' }}>
                  {h}
                </span>
              ))}
            </div>
            {loading ? (
              <div className="u-p-40px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
                Loading…
              </div>
            ) : filteredWorkouts.length === 0 ? (
              <div className="u-p-40px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
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
                  onEditSets={() => openSets(w)}
                />
              ))
            )}
          </div>
        </div>
      )}

      {activeTab === 'library' && (
        <WorkoutLibraryPanel api={api} orgId={orgId} groups={groups} onScheduled={loadPractices} />
      )}
      {activeTab === 'plan' && <SeasonPlanPanel api={api} orgId={orgId} groups={groups} />}
      {activeTab === 'stats' && <TrainingAnalyticsPanel api={api} orgId={orgId} groups={groups} />}

      {setsTarget && (
        <div role="dialog" aria-modal="true" aria-label="Practice sets" onClick={() => setSetsTarget(null)}
          style={{ position: 'fixed', inset: 0, zIndex: 60, background: 'rgba(4,10,16,0.6)', overflowY: 'auto', padding: '4vh 16px' }}>
          <div onClick={(e) => e.stopPropagation()} className="card u-m-0-auto u-p-20px" style={{ maxWidth: '1100px', background: 'var(--bg-surface)' }}>
            <div className="u-gap-12px u-mb-16px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
              <div>
                <h2 className="section-title">
                  Sets for {getGroupName(setsTarget.workout.groupId)}, {new Date(setsTarget.workout.date + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}
                </h2>
                <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-muted)' }}>Saving replaces this practice&apos;s yardage with the workout total.</p>
              </div>
              <div className="u-gap-6px" style={{ display: 'flex' }}>
                {setsTarget.workout.workoutId && (
                  <button type="button" className="btn btn-secondary u-fs-fs-sm" onClick={handleClearSets}>Remove sets</button>
                )}
                <button type="button" onClick={() => setSetsTarget(null)} className="btn-icon" title="Close" aria-label="Close" style={{ color: 'var(--text-muted)' }}>
                  <Icon name="x" size={16} />
                </button>
              </div>
            </div>
            <WorkoutComposer
              api={api} orgId={orgId} initial={setsTarget.initial} showTags={false} saveLabel="Save sets"
              onCancel={() => setSetsTarget(null)}
              onSave={async (input) => {
                const { practice } = await savePracticeWorkout(api, orgId, setsTarget.workout.id, input)
                setWorkouts((ws) => ws.map((w) => w.id === practice.id ? practiceToWorkout(practice) : w))
                setSetsTarget(null)
              }}
            />
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
