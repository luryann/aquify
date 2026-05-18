'use client'

import { useState, useMemo, useCallback } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchGroups, fetchPractices, createPractice, deletePractice, type GroupRecord, type PracticeRecord } from '@/lib/adminData'
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

const DEMO_GROUPS = [
  { id: 'group-senior', code: 'SR', name: 'Senior Group' },
  { id: 'group-junior', code: 'JR', name: 'Junior Group' },
  { id: 'group-developmental', code: 'DEV', name: 'Developmental' },
]

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
      <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', width: '36px', textTransform: 'uppercase', letterSpacing: '0.04em', flexShrink: 0 }}>
        {label}
      </span>
      <div style={{ flex: 1, height: '5px', borderRadius: '3px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
        <div style={{ height: '100%', width: `${pct}%`, background: color, borderRadius: '3px', transition: 'width 0.4s ease' }} />
      </div>
      <span style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', width: '28px', textAlign: 'right', flexShrink: 0 }}>
        {pct}%
      </span>
    </div>
  )
}

function WorkoutRow({ workout, groupName, onDelete }: { workout: Workout; groupName: string; onDelete: () => void }) {
  const typeColor = WORKOUT_TYPE_COLORS[workout.type]
  const totalYards = Object.values(workout.strokes).reduce((a, b) => a + b, 0)

  return (
    <div style={{
      display: 'grid',
      gridTemplateColumns: '100px 120px 1fr 90px 80px 36px',
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
      <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
        {new Date(workout.date + 'T00:00:00').toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}
      </span>
      <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
        <span style={{ width: '5px', height: '5px', borderRadius: '50%', background: typeColor, display: 'inline-block', flexShrink: 0 }} />
        <span style={{ fontSize: '11px', color: typeColor, fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          {WORKOUT_TYPE_LABELS[workout.type]}
        </span>
      </div>
      <div style={{ minWidth: 0 }}>
        <span style={{ fontSize: '12px', color: 'var(--text)', display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {workout.description}
        </span>
        <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
          {groupName}
        </span>
      </div>
      <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: workout.yardageTarget > 0 ? 'var(--text)' : 'var(--text-muted)', textAlign: 'right' }}>
        {totalYards > 0 ? `${totalYards.toLocaleString()} yds` : '—'}
      </span>
      <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)', textAlign: 'center' }}>
        {workout.attendance != null ? `${workout.attendance} att` : '—'}
      </span>
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

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '9px 11px',
  background: 'var(--bg-elevated)', border: '1px solid var(--border)',
  borderRadius: '7px', color: 'var(--text)', fontSize: '13px',
  fontFamily: 'var(--font-body)', outline: 'none', transition: 'border-color 0.15s',
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <label style={{
      display: 'block', fontSize: '10px', fontFamily: 'var(--font-mono)',
      color: 'var(--text-muted)', letterSpacing: '0.07em',
      textTransform: 'uppercase', marginBottom: '6px',
    }}>
      {children}
    </label>
  )
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
    try {
      const [practices, fetchedGroups] = await Promise.all([
        fetchPractices(api, orgId),
        fetchGroups(api, orgId).catch(() => [] as GroupRecord[]),
      ])
      setWorkouts(practices.map(practiceToWorkout))
      setGroups(fetchedGroups)
    } catch {
      // keep existing state
    } finally {
      setLoading(false)
    }
  }, [api, orgId])

  useEffect(() => {
    loadPractices()
  }, [loadPractices])

  // Use live groups if available, otherwise demo
  const displayGroups = groups.length > 0 ? groups : DEMO_GROUPS as unknown as GroupRecord[]

  function getGroupName(groupId: string) {
    const g = displayGroups.find((g) => g.id === groupId)
    return g ? g.name : groupId
  }

  const filteredWorkouts = useMemo(() => {
    const ws = filterGroup === 'all' ? workouts : workouts.filter((w) => w.groupId === filterGroup)
    return [...ws].sort((a, b) => b.date.localeCompare(a.date))
  }, [workouts, filterGroup])

  async function handleAddWorkout() {
    if (!newWorkout.date || !newWorkout.groupId || !newWorkout.description) return
    setSaving(true)
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
        groupId: displayGroups[0]?.id ?? '',
        type: 'water',
        description: '',
        yardageTarget: '',
        attendance: '',
        freeYds: '', backYds: '', breastYds: '', flyYds: '', imYds: '',
      })
    } catch {
      // silently fail — keep form open
    } finally {
      setSaving(false)
    }
  }

  async function handleDeleteWorkout(id: string) {
    try {
      await deletePractice(api, orgId, id)
      setWorkouts((ws) => ws.filter((w) => w.id !== id))
    } catch {
      // silently fail
    }
  }

  // Weekly stats per group
  const weeklyStats = useMemo(() => {
    const now = new Date()
    const weekStart = new Date(now)
    weekStart.setDate(now.getDate() - now.getDay())
    weekStart.setHours(0, 0, 0, 0)

    const thisWeek = workouts.filter((w) => new Date(w.date + 'T00:00:00') >= weekStart)

    return displayGroups.map((g) => {
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
  }, [workouts, displayGroups])

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
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: '28px' }}>
        <div>
          <h1 style={{ fontFamily: 'var(--font-display)', fontSize: '28px', fontWeight: 400, color: 'var(--text)', letterSpacing: '-0.01em', marginBottom: '6px' }}>
            Training
          </h1>
          <p style={{ fontSize: '13px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>
            Schedule workouts, track yardage, and monitor group training loads.
          </p>
        </div>
        {activeTab === 'schedule' && (
          <button
            onClick={() => setShowAddForm((v) => !v)}
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
      {activeTab === 'schedule' && (
        <div>
          {/* Add workout form */}
          {showAddForm && (
            <div className="card" style={{ padding: '24px', marginBottom: '20px', border: '1px solid var(--accent-border)', background: 'rgba(235,93,30,0.02)' }}>
              <p style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--accent)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '18px' }}>
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
                    {displayGroups.map((g) => (
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
                  disabled={!newWorkout.date || !newWorkout.groupId || !newWorkout.description || saving}
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
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
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
            {displayGroups.map((g) => (
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
                <span key={i} style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.07em', textAlign: i === 3 ? 'right' : i === 4 ? 'center' : 'left' }}>
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
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
              This Week
            </span>
            <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
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
                      fontSize: '11px', fontFamily: 'var(--font-mono)', fontWeight: 700,
                      color: 'var(--accent)', textTransform: 'uppercase',
                    }}>
                      {(group as unknown as { code?: string }).code ?? group.name?.slice(0, 2)}
                    </div>
                    <div>
                      <p style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>{group.name}</p>
                      <p style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                        {sessions} water · {wCount} total workouts
                      </p>
                    </div>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <p style={{ fontSize: '18px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--text)', letterSpacing: '-0.02em' }}>
                      {totalYards > 0 ? `${(totalYards / 1000).toFixed(1)}k` : '—'}
                    </p>
                    <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase' }}>yds</p>
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
                    <span style={{ fontSize: '11px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>Attendance</span>
                    <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text)' }}>
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
              <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '3px' }}>Total Yardage</p>
              <p style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--text)' }}>
                {weeklyStats.reduce((s, g) => s + g.totalYards, 0).toLocaleString()} <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontWeight: 400 }}>yds</span>
              </p>
            </div>
            <div style={{ width: '1px', height: '32px', background: 'var(--border)' }} />
            <div>
              <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '3px' }}>Workouts Logged</p>
              <p style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--text)' }}>
                {weeklyStats.reduce((s, g) => s + g.workouts, 0)}
              </p>
            </div>
            <div style={{ width: '1px', height: '32px', background: 'var(--border)' }} />
            <div>
              <p style={{ fontSize: '10px', fontFamily: 'var(--font-mono)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '3px' }}>Active Groups</p>
              <p style={{ fontSize: '20px', fontFamily: 'var(--font-mono)', fontWeight: 700, color: 'var(--text)' }}>
                {weeklyStats.filter((g) => g.workouts > 0).length}
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
