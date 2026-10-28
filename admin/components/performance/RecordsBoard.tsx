'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Modal } from '@/components/ui/Modal'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { parseBackendDate } from '@/lib/dates'
import {
  AGE_GROUPS, AGE_GROUP_LABELS, COURSES, STROKES, STROKE_LABELS,
  createManualRecord, decideRecordEntry, deleteManualRecord, draftRecordAnnouncement, fetchPendingRecords,
  fetchRecordHistory, fetchTeamRecords, formatSwimTime, genderLabel, parseSwimTime, rebuildRecords, updateRecordEntry,
  type Course, type RecordEntry, type RecordHistory, type TeamRecord,
} from '@/lib/performanceData'
import { FIELD_STYLE, FilterRow, LoadErrorBanner, Notice, Segmented } from './PerfControls'

const STROKE_TITLES: Record<string, string> = { free: 'Freestyle', back: 'Backstroke', breast: 'Breaststroke', fly: 'Butterfly', im: 'Individual Medley' }

function formatDate(value?: string) {
  if (!value) return ''
  const d = parseBackendDate(value)
  return d ? d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' }) : value
}

type Props = { canManage?: boolean }

/** Team record board. Families get a read-only board; admins can add
 *  historical records, lock/verify entries, approve pending swims and rebuild. */
export function RecordsBoard({ canManage = false }: Props) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [course, setCourse] = useState<Course>('SCY')
  const [gender, setGender] = useState<'F' | 'M'>('F')
  const [ageGroup, setAgeGroup] = useState<string>('OPEN')
  const [records, setRecords] = useState<TeamRecord[]>([])
  const [pending, setPending] = useState<RecordEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [selected, setSelected] = useState<TeamRecord | null>(null)
  const [adding, setAdding] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    if (!orgId) return
    setLoading(true)
    setError(null)
    try {
      const [next, nextPending] = await Promise.all([
        fetchTeamRecords(api, orgId, { course, gender, age_group: ageGroup }),
        canManage ? fetchPendingRecords(api, orgId) : Promise.resolve([] as RecordEntry[]),
      ])
      setRecords(next)
      setPending(nextPending)
    } catch (err) {
      setError(reportUIError('performance.records.load', err, 'Team records could not be loaded.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId, course, gender, ageGroup, canManage])

  useEffect(() => { void load() }, [load])

  const byStroke = useMemo(() => {
    const groups = new Map<string, TeamRecord[]>()
    for (const r of records) {
      if (!r.record) continue
      const list = groups.get(r.stroke) ?? []
      list.push(r)
      groups.set(r.stroke, list)
    }
    return STROKES.filter((s) => groups.has(s)).map((s) => ({ stroke: s, rows: groups.get(s)!.sort((a, b) => a.distance - b.distance) }))
  }, [records])

  const decide = async (entry: RecordEntry, decision: 'approve' | 'reject') => {
    setBusy(true)
    try {
      await decideRecordEntry(api, orgId, entry.id, decision)
      setNotice(decision === 'approve' ? `Approved ${entry.swimmer_name}'s ${formatSwimTime(entry.time_ms)} as the new record.` : `Rejected ${entry.swimmer_name}'s ${formatSwimTime(entry.time_ms)}.`)
      await load()
    } catch (err) {
      setError(reportUIError('performance.records.decide', err, 'The record decision could not be saved.'))
    } finally {
      setBusy(false)
    }
  }

  const rebuild = async () => {
    setBusy(true)
    try {
      const s = await rebuildRecords(api, orgId)
      setNotice(`Rebuilt from results: ${s.entries_created} record swims found${s.slots_skipped_locked ? `, ${s.slots_skipped_locked} locked records left as they were` : ''}.`)
      await load()
    } catch (err) {
      setError(reportUIError('performance.records.rebuild', err, 'Records could not be rebuilt.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <FilterRow>
        <Segmented label="Course" value={course} onChange={setCourse} options={COURSES.map((c) => ({ value: c, label: c }))} />
        <Segmented label="Gender" value={gender} onChange={setGender} options={[{ value: 'F', label: ageGroup === 'OPEN' ? 'Women' : 'Girls' }, { value: 'M', label: ageGroup === 'OPEN' ? 'Men' : 'Boys' }]} />
        <select aria-label="Age group" value={ageGroup} onChange={(e) => setAgeGroup(e.target.value)} style={FIELD_STYLE}>
          {AGE_GROUPS.map((g) => <option key={g} value={g}>{AGE_GROUP_LABELS[g]}</option>)}
        </select>
        {canManage ? (
          <div className="u-gap-8px u-ml-auto" style={{ display: 'flex', flexWrap: 'wrap' }}>
            <button type="button" className="btn btn-secondary btn-sm" disabled={busy} onClick={() => void rebuild()}>Rebuild from results</button>
            <button type="button" className="btn btn-primary btn-sm" onClick={() => setAdding(true)}>Add historical record</button>
          </div>
        ) : null}
      </FilterRow>

      {error ? <LoadErrorBanner message={error} onRetry={() => void load()} /> : null}
      {notice ? <Notice tone="success">{notice}</Notice> : null}

      {canManage && pending.length > 0 ? (
        <section aria-labelledby="pending-records" className="u-mb-space-8 u-p-16px-16px" style={{ border: '1px solid var(--warning-border)', borderRadius: 'var(--r-md)', background: 'var(--warning-bg)' }}>
          <h2 id="pending-records" className="u-m-0-0-4px u-fs-fs-md u-fw-600" >{pending.length} faster {pending.length === 1 ? 'swim beats a locked record' : 'swims beat locked records'}</h2>
          <p className="u-m-0-0-12px u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>Approve a swim to make it the record, or reject it if the result is wrong.</p>
          <div className="u-gap-8px" style={{ display: 'grid' }}>
            {pending.map((e) => (
              <div key={e.id} className="u-gap-12px u-fs-fs-base" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>
                <span style={{ flex: '1 1 240px' }}>
                  <strong>{e.swimmer_name}</strong> {formatSwimTime(e.time_ms)} in {genderLabel(e.gender, e.age_group)} {AGE_GROUP_LABELS[e.age_group]} {e.distance} {e.course} {STROKE_LABELS[e.stroke]}
                  <span style={{ color: 'var(--text-muted)' }}> · {formatDate(e.swim_date)}</span>
                </span>
                <button type="button" className="btn btn-sm btn-primary" disabled={busy} onClick={() => void decide(e, 'approve')}>Approve</button>
                <button type="button" className="btn btn-sm btn-ghost" disabled={busy} onClick={() => void decide(e, 'reject')}>Reject</button>
              </div>
            ))}
          </div>
        </section>
      ) : null}

      {loading ? (
        <div className="u-fs-fs-base u-p-24px-0" style={{ color: 'var(--text-muted)' }}>Loading records…</div>
      ) : !error && byStroke.length === 0 ? (
        <EmptyState
          icon="clock"
          title={`No ${genderLabel(gender, ageGroup).toLowerCase()} ${AGE_GROUP_LABELS[ageGroup].toLowerCase()} ${course} records yet`}
          description={canManage ? 'Records appear as official results are imported. Add pre-platform records with "Add historical record".' : 'Records appear here as official meet results come in.'}
        />
      ) : (
        <div className="u-gap-24px" style={{ display: 'grid' }}>
          {byStroke.map(({ stroke, rows }) => (
            <section key={stroke} aria-label={`${STROKE_LABELS[stroke]} records`}>
              <h2 className="section-title" style={{ margin: '0 0 8px' }}>{STROKE_TITLES[stroke]}</h2>
              <div style={{ borderTop: '1px solid var(--border)' }}>
                {rows.map((r) => <RecordRow key={`${r.distance}-${r.stroke}`} row={r} canManage={canManage} onOpen={() => setSelected(r)} />)}
              </div>
            </section>
          ))}
        </div>
      )}

      {selected ? (
        <HistoryModal
          record={selected}
          canManage={canManage}
          onClose={() => setSelected(null)}
          onChanged={(msg) => { setNotice(msg); void load() }}
        />
      ) : null}
      {adding ? (
        <AddRecordModal
          defaults={{ course, gender, ageGroup }}
          onClose={() => setAdding(false)}
          onSave={async (input) => {
            await createManualRecord(api, orgId, input)
            setAdding(false)
            setNotice(`Added ${input.swimmer_name}'s ${formatSwimTime(input.time_ms)} to the record history.`)
            await load()
          }}
        />
      ) : null}
    </div>
  )
}

function RecordRow({ row, canManage, onOpen }: { row: TeamRecord; canManage: boolean; onOpen: () => void }) {
  const r = row.record!
  return (
    <button
      type="button"
      onClick={onOpen}
      className="perf-record-row"
      style={{
        display: 'grid', gridTemplateColumns: '40px minmax(84px, 120px) minmax(0, 1fr)', alignItems: 'baseline', gap: '4px 16px',
        width: '100%', textAlign: 'left', padding: '12px 4px', border: 'none', borderBottom: '1px solid var(--border)', background: 'transparent', color: 'var(--text)', cursor: 'pointer',
      }}
      aria-label={`${row.label}: ${formatSwimTime(r.time_ms)} by ${r.swimmer_name}. Show record history.`}
    >
      <span className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>{row.distance}</span>
      <span className="u-ff-font-display u-fw-display-weight u-fs-fs-3xl u-lh-1" style={{ color: 'var(--accent-text, var(--accent))', fontVariantNumeric: 'tabular-nums' }}>{formatSwimTime(r.time_ms)}</span>
      <span style={{ minWidth: 0 }}>
        <span className="u-fw-600 u-fs-fs-md" style={{ display: 'block' }}>{r.swimmer_name}{r.swimmer_age != null ? <span className="u-fw-400" style={{ color: 'var(--text-muted)' }}> ({r.swimmer_age})</span> : null}</span>
        <span className="u-fs-fs-sm" style={{ display: 'block', color: 'var(--text-muted)' }}>
          {formatDate(r.swim_date)}{r.meet_name ? `, ${r.meet_name}` : ''}
          {row.previous ? `. Previously ${row.previous.swimmer_name}, ${formatSwimTime(row.previous.time_ms)}` : ''}
        </span>
        {r.locked || r.verified || (canManage && row.pending_count > 0) ? (
          <span className="u-gap-6px u-mt-6px" style={{ display: 'flex', flexWrap: 'wrap' }}>
            {r.locked ? <span className="badge badge-default">Locked</span> : null}
            {r.verified ? <span className="badge badge-success">Verified</span> : null}
            {canManage && row.pending_count > 0 ? <span className="badge badge-warning">{row.pending_count} pending</span> : null}
          </span>
        ) : null}
      </span>
    </button>
  )
}

function HistoryModal({ record, canManage, onClose, onChanged }: { record: TeamRecord; canManage: boolean; onClose: () => void; onChanged: (msg: string) => void }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [history, setHistory] = useState<RecordHistory | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    setError(null)
    try {
      setHistory(await fetchRecordHistory(api, orgId, { course: record.course, gender: record.gender, age_group: record.age_group, distance: record.distance, stroke: record.stroke }))
    } catch (err) {
      setError(reportUIError('performance.records.history', err, 'Record history could not be loaded.'))
    }
  }, [api, orgId, record])
  useEffect(() => { void load() }, [load])

  const act = async (fn: () => Promise<unknown>, msg: string) => {
    setBusy(true)
    try {
      await fn()
      onChanged(msg)
      await load()
    } catch (err) {
      setError(reportUIError('performance.records.action', err, 'That change could not be saved.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} title={record.label} description="Every swim that has held this record, newest first." width={560}>
      {error ? <LoadErrorBanner message={error} onRetry={() => void load()} /> : null}
      {!history ? <div className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>Loading history…</div> : (
        <ol className="u-m-0 u-p-0 u-gap-12px" style={{ listStyle: 'none', display: 'grid' }}>
          {history.progression.map((e, i) => (
            <li key={e.id} className="u-p-12px-12px" style={{ borderRadius: 'var(--r-sm)', border: '1px solid var(--border)', background: i === 0 ? 'var(--accent-subtle)' : 'transparent' }}>
              <div className="u-gap-8px" style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', alignItems: 'baseline' }}>
                <span className="u-ff-font-display u-fw-display-weight u-fs-fs-2xl" style={{ color: i === 0 ? 'var(--accent)' : 'var(--text)' }}>{formatSwimTime(e.time_ms)}</span>
                <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>{formatDate(e.swim_date)}</span>
              </div>
              <div className="u-fs-fs-base" >{e.swimmer_name}{e.meet_name ? <span style={{ color: 'var(--text-muted)' }}>, {e.meet_name}</span> : null}</div>
              <div className="u-gap-6px u-mt-6px" style={{ display: 'flex', flexWrap: 'wrap' }}>
                {e.source === 'manual' ? <span className="badge badge-default">Historical</span> : null}
                {e.locked ? <span className="badge badge-default">Locked</span> : null}
                {e.verified ? <span className="badge badge-success">Verified</span> : null}
              </div>
              {canManage ? (
                <div className="u-gap-6px u-mt-8px" style={{ display: 'flex', flexWrap: 'wrap' }}>
                  <button type="button" className="btn btn-sm btn-secondary" disabled={busy} onClick={() => void act(() => updateRecordEntry(api, orgId, e.id, { locked: !e.locked }), e.locked ? 'Record unlocked.' : 'Record locked. Faster swims now need approval.')}>{e.locked ? 'Unlock' : 'Lock'}</button>
                  <button type="button" className="btn btn-sm btn-secondary" disabled={busy} onClick={() => void act(() => updateRecordEntry(api, orgId, e.id, { verified: !e.verified }), e.verified ? 'Verification removed.' : 'Record verified.')}>{e.verified ? 'Unverify' : 'Verify'}</button>
                  {i === 0 ? <button type="button" className="btn btn-sm btn-ghost" disabled={busy} onClick={() => void act(() => draftRecordAnnouncement(api, orgId, e.id), 'Announcement draft created. Review and publish it from Announcements.')}>Draft announcement</button> : null}
                  {e.source === 'manual'
                    ? <button type="button" className="btn btn-sm btn-danger" disabled={busy} onClick={() => void act(() => deleteManualRecord(api, orgId, e.id), 'Historical record deleted.')}>Delete</button>
                    : <button type="button" className="btn btn-sm btn-ghost" disabled={busy} onClick={() => void act(() => decideRecordEntry(api, orgId, e.id, 'reject'), 'Swim removed from the record history.')}>Remove swim</button>}
                </div>
              ) : null}
            </li>
          ))}
        </ol>
      )}
      {canManage && history && history.pending.length > 0 ? (
        <p className="u-fs-fs-base u-mt-12px" style={{ color: 'var(--warning)' }}>{history.pending.length} faster swim{history.pending.length === 1 ? '' : 's'} awaiting approval in the queue above.</p>
      ) : null}
    </Modal>
  )
}

function AddRecordModal({ defaults, onClose, onSave }: {
  defaults: { course: Course; gender: 'F' | 'M'; ageGroup: string }
  onClose: () => void
  onSave: (input: Parameters<typeof createManualRecord>[2]) => Promise<void>
}) {
  const [course, setCourse] = useState<string>(defaults.course)
  const [gender, setGender] = useState<string>(defaults.gender)
  const [ageGroup, setAgeGroup] = useState<string>(defaults.ageGroup)
  const [stroke, setStroke] = useState('free')
  const [distance, setDistance] = useState('100')
  const [time, setTime] = useState('')
  const [name, setName] = useState('')
  const [date, setDate] = useState('')
  const [meet, setMeet] = useState('')
  const [locked, setLocked] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const submit = async () => {
    const ms = parseSwimTime(time)
    if (!ms) { setError('Enter the time as 58.12 or 1:02.34.'); return }
    if (!name.trim()) { setError('Enter the swimmer who set the record.'); return }
    if (!date) { setError('Enter the date the record was set.'); return }
    setSaving(true)
    setError(null)
    try {
      await onSave({ course, gender, age_group: ageGroup, stroke, distance: Number(distance), time_ms: ms, swimmer_name: name.trim(), swim_date: date, meet_name: meet.trim() || undefined, locked, verified: true })
    } catch (err) {
      setError(reportUIError('performance.records.create', err, 'The record could not be saved.'))
      setSaving(false)
    }
  }

  const label = (text: string, control: React.ReactNode) => (
    <label className="u-gap-4px u-fs-fs-sm" style={{ display: 'grid', color: 'var(--text-subtle)' }}>{text}{control}</label>
  )

  return (
    <Modal
      open
      onClose={onClose}
      title="Add historical record"
      description="For records set before the club used this platform. Historical records count toward the record history like any imported swim."
      width={520}
      footer={<>
        <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
        <button type="button" className="btn btn-primary" disabled={saving} onClick={() => void submit()}>{saving ? 'Saving…' : 'Add record'}</button>
      </>}
    >
      {error ? <LoadErrorBanner message={error} /> : null}
      <div className="u-gap-12px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))' }}>
        {label('Course', <select value={course} onChange={(e) => setCourse(e.target.value)} style={FIELD_STYLE}>{COURSES.map((c) => <option key={c}>{c}</option>)}</select>)}
        {label('Gender', <select value={gender} onChange={(e) => setGender(e.target.value)} style={FIELD_STYLE}><option value="F">Girls / Women</option><option value="M">Boys / Men</option></select>)}
        {label('Age group', <select value={ageGroup} onChange={(e) => setAgeGroup(e.target.value)} style={FIELD_STYLE}>{AGE_GROUPS.map((g) => <option key={g} value={g}>{AGE_GROUP_LABELS[g]}</option>)}</select>)}
        {label('Stroke', <select value={stroke} onChange={(e) => setStroke(e.target.value)} style={FIELD_STYLE}>{STROKES.map((s) => <option key={s} value={s}>{STROKE_LABELS[s]}</option>)}</select>)}
        {label('Distance', <input inputMode="numeric" value={distance} onChange={(e) => setDistance(e.target.value.replace(/\D/g, ''))} style={FIELD_STYLE} />)}
        {label('Time', <input aria-label="58.12" placeholder="58.12" value={time} onChange={(e) => setTime(e.target.value)} style={FIELD_STYLE} />)}
        {label('Swimmer', <input value={name} onChange={(e) => setName(e.target.value)} style={FIELD_STYLE} />)}
        {label('Date set', <input type="date" value={date} onChange={(e) => setDate(e.target.value)} style={FIELD_STYLE} />)}
        {label('Meet (optional)', <input value={meet} onChange={(e) => setMeet(e.target.value)} style={FIELD_STYLE} />)}
      </div>
      <label className="u-gap-8px u-fs-fs-base u-mt-16px" style={{ display: 'flex', alignItems: 'center' }}>
        <input type="checkbox" checked={locked} onChange={(e) => setLocked(e.target.checked)} />
        Lock this record so faster swims need admin approval
      </label>
    </Modal>
  )
}
