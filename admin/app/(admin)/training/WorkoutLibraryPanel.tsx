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
import { useConfirm } from '@/components/ui/ConfirmDialog'
import { localToday } from '@/lib/dates'

type Editing = { id: string | null; initial: ComposerInitial }

const BLANK: ComposerInitial = { title: '', course: 'SCY', tags: [], text: '' }

const chip = (active: boolean): React.CSSProperties => ({
  padding: '4px 12px', borderRadius: '16px', fontSize: 'var(--fs-sm)', cursor: 'pointer',
  border: `1px solid ${active ? 'var(--accent-border)' : 'var(--border)'}`,
  background: active ? 'var(--accent-subtle)' : 'transparent',
  color: active ? 'var(--accent)' : 'var(--text-muted)', fontFamily: 'var(--font-body)',
})

function AssignForm({ groups, onAssign, onCancel }: { groups: GroupRecord[]; onAssign: (date: string, groupId: string) => Promise<void>; onCancel: () => void }) {
  const [date, setDate] = useState(localToday())
  const [groupId, setGroupId] = useState(groups[0]?.id ?? '')
  const [busy, setBusy] = useState(false)
  const field: React.CSSProperties = { padding: '8px 8px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: 'var(--fs-sm)' }
  return (
    <div className="u-gap-8px u-mt-12px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>
      <input type="date" aria-label="Practice date" value={date} onChange={(e) => setDate(e.target.value)} style={field} />
      <select aria-label="Group" value={groupId} onChange={(e) => setGroupId(e.target.value)} style={field}>
        <option value="">Whole team</option>
        {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
      </select>
      <button type="button" className="btn btn-primary u-fs-fs-sm u-p-6px-12px" disabled={!date || busy}
        onClick={async () => { setBusy(true); try { await onAssign(date, groupId) } finally { setBusy(false) } }}>
        {busy ? 'Scheduling…' : 'Schedule practice'}
      </button>
      <button type="button" className="btn btn-secondary u-fs-fs-sm u-p-6px-12px" onClick={onCancel}>Cancel</button>
    </div>
  )
}

export function WorkoutLibraryPanel({ api, orgId, groups, onScheduled }: {
  api: ApiClient
  orgId: string
  groups: GroupRecord[]
  onScheduled: () => void
}) {
  const confirm = useConfirm()
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
        <div className="card u-p-20px u-mb-20px" >
          <h2 className="section-title u-mb-16px" >
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
        <div className="u-gap-12px u-mb-16px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>
          <input type="search" aria-label="Search workouts" placeholder="Search title or tag" value={query} onChange={(e) => setQuery(e.target.value)}
            className="input-search u-p-8px-12px u-fs-fs-base" style={{ flex: '1 1 220px', maxWidth: '320px' }} />
          <button type="button" className="btn btn-primary u-gap-8px u-fs-fs-base u-ml-auto" onClick={() => setEditing({ id: null, initial: BLANK })}>
            <Icon name="plus" size={13} /> New workout
          </button>
        </div>
      )}

      {allTags.length > 0 && !editing && (
        <div className="u-gap-6px u-mb-16px" style={{ display: 'flex', flexWrap: 'wrap' }}>
          <button type="button" style={chip(tag === '')} onClick={() => setTag('')}>All tags</button>
          {allTags.map((t) => <button type="button" key={t} style={chip(tag === t)} onClick={() => setTag(tag === t ? '' : t)}>{t}</button>)}
        </div>
      )}

      {notice && <p role="status" className="u-fs-fs-base u-mb-12px" style={{ color: 'var(--text-subtle)' }}>{notice}</p>}
      {loadError && (
        <div role="alert" className="u-mb-16px u-p-12px-16px u-fs-fs-base u-gap-12px" style={{ borderRadius: '8px', border: '1px solid var(--error-border)', background: 'var(--error-bg)', color: 'var(--error)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{loadError}</span>
          <button type="button" className="btn btn-secondary u-fs-fs-sm u-p-6px-12px" onClick={load} >Retry</button>
        </div>
      )}

      {loading ? (
        <p className="u-fs-fs-base u-p-24px-0" style={{ color: 'var(--text-muted)' }}>Loading library…</p>
      ) : !loadError && workouts.length === 0 ? (
        <div className="card u-p-24px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
          Your library is empty. Write a workout once, then schedule it for any group in two clicks.
        </div>
      ) : !loadError && visible.length === 0 ? (
        <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>No workouts match this search.</p>
      ) : (
        <div className="u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 320px), 1fr))' }}>
          {visible.map((w) => (
            <article key={w.id} className="card u-p-16px-20px u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
              <div className="u-gap-12px" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
                <h3 className="section-title u-m-0" style={{ minWidth: 0, overflowWrap: 'anywhere' }}>{w.title}</h3>
                <span className="u-ff-font-mono u-fs-fs-sm" style={{ color: 'var(--text-subtle)', whiteSpace: 'nowrap' }}>
                  {formatDistance(w.totals.distance, w.totals.unit)} {w.course}
                </span>
              </div>
              <p className="u-fs-fs-sm u-m-0" style={{ color: 'var(--text-muted)' }}>
                About {formatDuration(w.totals.estimated_seconds)}, {w.sections.length} {w.sections.length === 1 ? 'section' : 'sections'}
                {w.tags.length > 0 ? `, tagged ${w.tags.join(', ')}` : ''}
              </p>
              <ZoneBar byZone={w.totals.by_zone} height={8} />
              <div className="u-gap-6px" style={{ display: 'flex', flexWrap: 'wrap', marginTop: 'auto' }}>
                <button type="button" className="btn btn-secondary u-fs-fs-sm u-p-4px-12px" onClick={() => setAssigningId(assigningId === w.id ? null : w.id)}>Schedule</button>
                <button type="button" className="btn btn-secondary u-fs-fs-sm u-p-4px-12px" onClick={() => startEdit(w)}>Edit</button>
                <button type="button" className="btn btn-secondary u-fs-fs-sm u-p-4px-12px" 
                  onClick={() => run(async () => { const copy = await duplicateWorkout(api, orgId, w.id); setWorkouts((ws) => [copy, ...ws]) }, `Duplicated ${w.title}.`, 'Couldn’t duplicate. Try again.')}>
                  Duplicate
                </button>
                <Link href={`/deck/workouts/${w.id}`} target="_blank" className="btn btn-secondary u-fs-fs-sm u-p-4px-12px" >Pool deck</Link>
                <button type="button" className="btn-icon u-ml-auto" title={`Delete ${w.title}`} aria-label={`Delete ${w.title}`} style={{ color: 'var(--text-muted)' }}
                  onClick={async () => {
                    if (!(await confirm({ title: `Delete ${w.title}?`, message: 'It is removed from the library. Scheduled practices keep their own copy.', confirmLabel: 'Delete workout', destructive: true }))) return
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
