'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import type { ApiClient } from '@/lib/api'
import type { GroupRecord } from '@/lib/adminData'
import {
  assignWorkout, createWorkout, deleteWorkout, duplicateWorkout, fetchWorkout, fetchWorkouts, updateWorkout,
  type Workout,
} from '@/lib/trainingData'
import { formatDistance, formatDuration } from '@/lib/workout-format'
import { ZoneBar } from '@/components/training/WorkoutSheet'
import { Icon } from '@/components/ui/icons'
import { WorkoutComposer, errorText, type ComposerInitial } from './WorkoutComposer'

type Editing = { id: string | null; initial: ComposerInitial }

const BLANK: ComposerInitial = { title: '', course: 'SCY', tags: [], text: '' }

const chip = (active: boolean): React.CSSProperties => ({
  padding: '4px 12px', borderRadius: '16px', fontSize: 'var(--fs-sm)', cursor: 'pointer',
  border: `1px solid ${active ? 'var(--accent-border)' : 'var(--border)'}`,
  background: active ? 'var(--accent-subtle)' : 'transparent',
  color: active ? 'var(--accent)' : 'var(--text-muted)', fontFamily: 'var(--font-body)',
})

function AssignForm({ groups, onAssign, onCancel }: { groups: GroupRecord[]; onAssign: (date: string, groupId: string) => Promise<void>; onCancel: () => void }) {
  const [date, setDate] = useState(new Date().toISOString().slice(0, 10))
  const [groupId, setGroupId] = useState(groups[0]?.id ?? '')
  const [busy, setBusy] = useState(false)
  const field: React.CSSProperties = { padding: '8px 8px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: 'var(--fs-sm)' }
  return (
    <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', alignItems: 'center', marginTop: '12px' }}>
      <input type="date" aria-label="Practice date" value={date} onChange={(e) => setDate(e.target.value)} style={field} />
      <select aria-label="Group" value={groupId} onChange={(e) => setGroupId(e.target.value)} style={field}>
        <option value="">Whole team</option>
        {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
      </select>
      <button type="button" className="btn btn-primary" style={{ fontSize: 'var(--fs-sm)', padding: '6px 12px' }} disabled={!date || busy}
        onClick={async () => { setBusy(true); try { await onAssign(date, groupId) } finally { setBusy(false) } }}>
        {busy ? 'Scheduling…' : 'Schedule practice'}
      </button>
      <button type="button" className="btn btn-secondary" style={{ fontSize: 'var(--fs-sm)', padding: '6px 12px' }} onClick={onCancel}>Cancel</button>
    </div>
  )
}

export function WorkoutLibraryPanel({ api, orgId, groups, onScheduled }: {
  api: ApiClient
  orgId: string
  groups: GroupRecord[]
  onScheduled: () => void
}) {
  const [workouts, setWorkouts] = useState<Workout[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [tag, setTag] = useState('')
  const [editing, setEditing] = useState<Editing | null>(null)
  const [assigningId, setAssigningId] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setLoadError(null)
    try {
      setWorkouts(await fetchWorkouts(api, orgId))
    } catch (error) {
      setLoadError(errorText(error, 'Couldn’t load the workout library. Try again.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId])

  useEffect(() => { if (orgId) void load() }, [load, orgId])

  const allTags = useMemo(() => [...new Set(workouts.flatMap((w) => w.tags))].sort(), [workouts])
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return workouts.filter((w) =>
      (!tag || w.tags.includes(tag)) &&
      (!q || w.title.toLowerCase().includes(q) || w.description.toLowerCase().includes(q) || w.tags.some((t) => t.includes(q))))
  }, [workouts, query, tag])

  async function run(action: () => Promise<void>, done: string, fallback: string) {
    setNotice(null)
    try {
      await action()
      setNotice(done)
    } catch (error) {
      setNotice(errorText(error, fallback))
    }
  }

  async function startEdit(w: Workout) {
    await run(async () => {
      const { text } = await fetchWorkout(api, orgId, w.id)
      setEditing({ id: w.id, initial: { title: w.title, course: w.course, tags: w.tags, text } })
    }, '', 'Couldn’t open this workout. Try again.')
  }

  return (
    <div>
      {editing ? (
        <div className="card" style={{ padding: '20px', marginBottom: '20px' }}>
          <h2 className="section-title" style={{ marginBottom: '16px' }}>
            {editing.id ? `Edit ${editing.initial.title}` : 'New library workout'}
          </h2>
          <WorkoutComposer
            api={api} orgId={orgId} initial={editing.initial}
            saveLabel={editing.id ? 'Save changes' : 'Save to library'}
            onCancel={() => setEditing(null)}
            onSave={async (input) => {
              const saved = editing.id
                ? await updateWorkout(api, orgId, editing.id, input)
                : await createWorkout(api, orgId, { ...input, is_template: true })
              setWorkouts((ws) => [saved, ...ws.filter((w) => w.id !== saved.id)])
              setEditing(null)
              setNotice(editing.id ? 'Workout saved.' : 'Saved to library.')
            }}
          />
        </div>
      ) : (
        <div style={{ display: 'flex', gap: '12px', flexWrap: 'wrap', alignItems: 'center', marginBottom: '16px' }}>
          <input type="search" aria-label="Search workouts" placeholder="Search title or tag" value={query} onChange={(e) => setQuery(e.target.value)}
            style={{ flex: '1 1 220px', maxWidth: '320px', padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: 'var(--fs-base)' }} />
          <button type="button" className="btn btn-primary" style={{ gap: '8px', fontSize: 'var(--fs-base)', marginLeft: 'auto' }} onClick={() => setEditing({ id: null, initial: BLANK })}>
            <Icon name="plus" size={13} /> New workout
          </button>
        </div>
      )}

      {allTags.length > 0 && !editing && (
        <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginBottom: '16px' }}>
          <button type="button" style={chip(tag === '')} onClick={() => setTag('')}>All tags</button>
          {allTags.map((t) => <button type="button" key={t} style={chip(tag === t)} onClick={() => setTag(tag === t ? '' : t)}>{t}</button>)}
        </div>
      )}

      {notice && <p role="status" style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginBottom: '12px' }}>{notice}</p>}
      {loadError && (
        <div role="alert" style={{ marginBottom: '16px', padding: '12px 16px', borderRadius: '8px', border: '1px solid var(--error-border)', background: 'var(--error-bg)', color: 'var(--error)', fontSize: 'var(--fs-base)', display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'center' }}>
          <span>{loadError}</span>
          <button type="button" className="btn btn-secondary" onClick={load} style={{ fontSize: 'var(--fs-sm)', padding: '6px 12px' }}>Retry</button>
        </div>
      )}

      {loading ? (
        <p style={{ color: 'var(--text-muted)', fontSize: 'var(--fs-base)', padding: '24px 0' }}>Loading library…</p>
      ) : !loadError && workouts.length === 0 ? (
        <div className="card" style={{ padding: '24px', textAlign: 'center', color: 'var(--text-muted)', fontSize: 'var(--fs-base)' }}>
          Your library is empty. Write a workout once, then schedule it for any group in two clicks.
        </div>
      ) : !loadError && visible.length === 0 ? (
        <p style={{ color: 'var(--text-muted)', fontSize: 'var(--fs-base)' }}>No workouts match this search.</p>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 320px), 1fr))', gap: '16px' }}>
          {visible.map((w) => (
            <article key={w.id} className="card" style={{ padding: '16px 20px', display: 'flex', flexDirection: 'column', gap: '12px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', alignItems: 'baseline' }}>
                <h3 className="section-title" style={{ margin: 0, minWidth: 0, overflowWrap: 'anywhere' }}>{w.title}</h3>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', whiteSpace: 'nowrap' }}>
                  {formatDistance(w.totals.distance, w.totals.unit)} {w.course}
                </span>
              </div>
              <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', margin: 0 }}>
                About {formatDuration(w.totals.estimated_seconds)}, {w.sections.length} {w.sections.length === 1 ? 'section' : 'sections'}
                {w.tags.length > 0 ? `, tagged ${w.tags.join(', ')}` : ''}
              </p>
              <ZoneBar byZone={w.totals.by_zone} height={8} />
              <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginTop: 'auto' }}>
                <button type="button" className="btn btn-secondary" style={{ fontSize: 'var(--fs-sm)', padding: '4px 12px' }} onClick={() => setAssigningId(assigningId === w.id ? null : w.id)}>Schedule</button>
                <button type="button" className="btn btn-secondary" style={{ fontSize: 'var(--fs-sm)', padding: '4px 12px' }} onClick={() => startEdit(w)}>Edit</button>
                <button type="button" className="btn btn-secondary" style={{ fontSize: 'var(--fs-sm)', padding: '4px 12px' }}
                  onClick={() => run(async () => { const copy = await duplicateWorkout(api, orgId, w.id); setWorkouts((ws) => [copy, ...ws]) }, `Duplicated ${w.title}.`, 'Couldn’t duplicate. Try again.')}>
                  Duplicate
                </button>
                <Link href={`/deck/workouts/${w.id}`} target="_blank" className="btn btn-secondary" style={{ fontSize: 'var(--fs-sm)', padding: '4px 12px' }}>Pool deck</Link>
                <button type="button" className="btn-icon" title={`Delete ${w.title}`} aria-label={`Delete ${w.title}`} style={{ color: 'var(--text-muted)', marginLeft: 'auto' }}
                  onClick={() => {
                    if (!window.confirm(`Delete “${w.title}” from the library? Scheduled practices keep their own copy.`)) return
                    void run(async () => { await deleteWorkout(api, orgId, w.id); setWorkouts((ws) => ws.filter((x) => x.id !== w.id)) }, 'Workout deleted.', 'Couldn’t delete. Try again.')
                  }}>
                  <Icon name="trash-2" size={13} />
                </button>
              </div>
              {assigningId === w.id && (
                <AssignForm groups={groups} onCancel={() => setAssigningId(null)}
                  onAssign={(date, groupId) => run(async () => {
                    await assignWorkout(api, orgId, w.id, { date, group_id: groupId || undefined })
                    setAssigningId(null)
                    onScheduled()
                  }, `Scheduled ${w.title} for ${date}.`, 'Couldn’t schedule this workout. Try again.')} />
              )}
            </article>
          ))}
        </div>
      )}
    </div>
  )
}
