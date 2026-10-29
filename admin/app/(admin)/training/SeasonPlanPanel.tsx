'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import type { ApiClient } from '@/lib/api'
import { fetchSeasons, type GroupRecord, type SeasonRecord } from '@/lib/adminData'
import { formatShortDate } from '@/lib/dates'
import {
  createTrainingPhase, deleteTrainingPhase, fetchSeasonPlan, type PhaseKind, type SeasonPlan,
} from '@/lib/trainingData'
import { WeeklyBars } from '@/components/training/WeeklyBars'
import { Icon } from '@/components/ui/icons'
import { errorText } from './WorkoutComposer'

const PHASES: { kind: PhaseKind; label: string }[] = [
  { kind: 'base', label: 'Base' },
  { kind: 'build', label: 'Build' },
  { kind: 'peak', label: 'Peak' },
  { kind: 'taper', label: 'Taper' },
  { kind: 'recovery', label: 'Recovery' },
]


const field: React.CSSProperties = { width: '100%' }

export function SeasonPlanPanel({ api, orgId, groups }: { api: ApiClient; orgId: string; groups: GroupRecord[] }) {
  const [seasons, setSeasons] = useState<SeasonRecord[]>([])
  const [seasonId, setSeasonId] = useState('')
  const [groupId, setGroupId] = useState(groups[0]?.id ?? '')
  const [plan, setPlan] = useState<SeasonPlan | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const [draft, setDraft] = useState({ phase: 'base' as PhaseKind, start: '', end: '', target: '', focus: '' })
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!orgId) return
    fetchSeasons(api, orgId).then((list) => {
      setSeasons(list)
      setSeasonId((cur) => cur || list.find((s) => s.status === 'active')?.id || list[0]?.id || '')
    }).catch((err) => setError(errorText(err, 'Couldn’t load seasons. Try again.')))
  }, [api, orgId])

  useEffect(() => { setGroupId((cur) => cur || groups[0]?.id || '') }, [groups])

  const load = useCallback(async () => {
    if (!seasonId || !groupId) return
    setError(null)
    try {
      setPlan(await fetchSeasonPlan(api, orgId, seasonId, groupId))
    } catch (err) {
      setError(errorText(err, 'Couldn’t load the season plan. Try again.'))
    }
  }, [api, orgId, seasonId, groupId])

  useEffect(() => { void load() }, [load])

  const season = seasons.find((s) => s.id === seasonId)

  async function addPhase() {
    setSaving(true)
    setFormError(null)
    try {
      await createTrainingPhase(api, orgId, seasonId, {
        group_id: groupId, phase: draft.phase, name: '', start_date: draft.start, end_date: draft.end,
        weekly_yardage_target: parseInt(draft.target, 10) || 0, focus: draft.focus,
      })
      setDraft({ phase: 'build', start: '', end: '', target: '', focus: '' })
      await load()
    } catch (err) {
      setFormError(errorText(err, 'Couldn’t add this phase. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  if (seasons.length === 0 && !error) {
    return (
      <div className="card u-p-24px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
        Plan phases hang off a season. <Link href="/seasons" style={{ color: 'var(--accent-text, var(--accent))' }}>Create a season</Link> first.
      </div>
    )
  }

  return (
    <div className="u-gap-16px" style={{ display: 'flex', flexDirection: 'column' }}>
      <div className="u-gap-12px" style={{ display: 'flex', flexWrap: 'wrap' }}>
        <select aria-label="Season" value={seasonId} onChange={(e) => setSeasonId(e.target.value)} style={{ ...field, width: 'auto', minWidth: 180 }}>
          {seasons.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
        </select>
        <select aria-label="Group" value={groupId} onChange={(e) => setGroupId(e.target.value)} style={{ ...field, width: 'auto', minWidth: 160 }}>
          {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
        </select>
      </div>

      {error && (
        <div role="alert" className="u-p-12px-16px u-fs-fs-base u-gap-12px" style={{ borderRadius: '8px', border: '1px solid var(--error-border)', background: 'var(--error-bg)', color: 'var(--error)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span>{error}</span>
          <button type="button" className="btn btn-secondary u-fs-fs-sm u-p-6px-12px" onClick={load} >Retry</button>
        </div>
      )}

      {plan && (
        <section className="card u-p-16px-20px" >
          <h3 className="section-title u-m-0-0-4px" >Weekly target against planned yardage</h3>
          {plan.weeks.length === 0 ? (
            <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>Give the season dates or add a phase to see its weeks.</p>
          ) : (
            <WeeklyBars
              ariaLabel="Planned yards per week with weekly target"
              height={150}
              data={plan.weeks.map((w) => ({
                key: w.week_start, label: formatShortDate(w.week_start), value: w.planned_yards, target: w.target_yards || undefined,
                detail: `${w.phase ? PHASES.find((p) => p.kind === w.phase)?.label : 'No phase'}, ${w.practices} practices`,
              }))}
            />
          )}
        </section>
      )}

      <section className="card u-p-16px-20px" >
        <h3 className="section-title u-m-0-0-12px" >Phases</h3>
        {plan && plan.phases.length === 0 && (
          <p className="u-fs-fs-base u-mb-12px" style={{ color: 'var(--text-muted)' }}>No phases yet for this group.</p>
        )}
        <ol className="u-m-0 u-p-0 u-gap-6px" style={{ listStyle: 'none', display: 'flex', flexDirection: 'column' }}>
          {plan?.phases.map((p) => (
            <li key={p.id} className="u-gap-12px u-p-8px-0 u-fs-fs-base" style={{ display: 'grid', gridTemplateColumns: '90px minmax(0,1fr) auto auto', alignItems: 'center', borderBottom: '1px solid var(--border)' }}>
              <span className="u-fw-600" style={{ color: 'var(--text)' }}>{PHASES.find((x) => x.kind === p.phase)?.label}</span>
              <span style={{ color: 'var(--text-subtle)', minWidth: 0 }}>{formatShortDate(p.start_date)} – {formatShortDate(p.end_date)}{p.focus ? `, ${p.focus}` : ''}</span>
              <span className="u-ff-font-mono u-fs-fs-sm" style={{ color: 'var(--text)' }}>{p.weekly_yardage_target.toLocaleString('en-US')} yd/wk</span>
              <button type="button" className="btn-icon" aria-label={`Delete ${p.phase} phase`} style={{ color: 'var(--text-muted)' }}
                onClick={async () => {
                  try { await deleteTrainingPhase(api, orgId, p.id); await load() } catch (err) { setFormError(errorText(err, 'Couldn’t delete this phase. Try again.')) }
                }}>
                <Icon name="trash-2" size={13} />
              </button>
            </li>
          ))}
        </ol>
        <div className="u-gap-12px u-mt-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', alignItems: 'end' }}>
          <label className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Phase
            <select value={draft.phase} onChange={(e) => setDraft((d) => ({ ...d, phase: e.target.value as PhaseKind }))} style={{ ...field, marginTop: 4 }}>
              {PHASES.map((p) => <option key={p.kind} value={p.kind}>{p.label}</option>)}
            </select>
          </label>
          <label className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Starts
            <input type="date" value={draft.start} min={season?.start_date || undefined} max={season?.end_date || undefined} onChange={(e) => setDraft((d) => ({ ...d, start: e.target.value }))} style={{ ...field, marginTop: 4 }} />
          </label>
          <label className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Ends
            <input type="date" value={draft.end} min={draft.start || season?.start_date || undefined} max={season?.end_date || undefined} onChange={(e) => setDraft((d) => ({ ...d, end: e.target.value }))} style={{ ...field, marginTop: 4 }} />
          </label>
          <label className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Yards per week
            <input type="number" min={0} step={500} value={draft.target} onChange={(e) => setDraft((d) => ({ ...d, target: e.target.value }))} style={{ ...field, marginTop: 4 }} />
          </label>
          <label className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Focus
            <input aria-label="Aerobic base, technique" value={draft.focus} maxLength={500} placeholder="Aerobic base, technique" onChange={(e) => setDraft((d) => ({ ...d, focus: e.target.value }))} style={{ ...field, marginTop: 4 }} />
          </label>
          <button type="button" className="btn btn-primary" disabled={!draft.start || !draft.end || !seasonId || !groupId || saving} onClick={addPhase}>
            {saving ? 'Adding…' : 'Add phase'}
          </button>
        </div>
        {formError && <p role="alert" className="u-fs-fs-sm u-mt-8px" style={{ color: 'var(--error)' }}>{formError}</p>}
      </section>
    </div>
  )
}
