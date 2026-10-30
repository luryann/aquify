'use client'

import { useCallback, useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { formatDateOnly } from '@/lib/dates'
import {
  COURSES, EVENT_DISTANCES, STROKES, STROKE_LABELS,
  createSwimmerGoal, deleteSwimmerGoal, fetchSwimmerGoals, formatSwimTime, parseSwimTime, type Goal,
} from '@/lib/performanceData'
import { goalTone } from '@/lib/performance-periods'
import { FIELD_STYLE, LabeledField } from './PerfControls'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

function deadlineText(g: Goal) {
  if (!g.deadline) return null
  const date = formatDateOnly(g.deadline)
  if (g.achieved) return `by ${date}`
  if (g.overdue) return `deadline passed ${date}`
  if (g.days_remaining != null) return `${g.days_remaining} day${g.days_remaining === 1 ? '' : 's'} left, ${date}`
  return `by ${date}`
}

/** Target times per event with progress from the swimmer's best time at the
 *  moment the goal was set toward the target. Used on the swimmer portal and
 *  the parent swimmer page (writes are household-scoped by the API). */
export function GoalsPanel({ swimmerId, swimmerName }: { swimmerId: string; swimmerName?: string }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [goals, setGoals] = useState<Goal[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)
  const [adding, setAdding] = useState(false)
  const [stroke, setStroke] = useState('free')
  const [distance, setDistance] = useState('100')
  const [course, setCourse] = useState('SCY')
  const [target, setTarget] = useState('')
  const [deadline, setDeadline] = useState('')
  const [saving, setSaving] = useState(false)

  const load = useCallback(async () => {
    if (!orgId || !swimmerId) return
    setLoading(true)
    setError(null)
    try {
      setGoals(await fetchSwimmerGoals(api, orgId, swimmerId))
    } catch (err) {
      setError(reportUIError('performance.goals.load', err, 'Goals could not be loaded.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId, swimmerId])
  useEffect(() => { void load() }, [load])

  const distances = EVENT_DISTANCES[stroke as keyof typeof EVENT_DISTANCES] ?? []

  const submit = async () => {
    const ms = parseSwimTime(target)
    if (!ms) { setFormError('Enter the target time as 58.12 or 1:02.34.'); return }
    setSaving(true)
    setFormError(null)
    try {
      await createSwimmerGoal(api, orgId, swimmerId, { stroke, distance: Number(distance), course, target_time_ms: ms, deadline: deadline || undefined })
      setTarget('')
      setDeadline('')
      setAdding(false)
      await load()
    } catch (err) {
      setFormError(reportUIError('performance.goals.create', err, 'The goal could not be saved.'))
    } finally {
      setSaving(false)
    }
  }

  const remove = async (g: Goal) => {
    try {
      await deleteSwimmerGoal(api, orgId, swimmerId, g.id)
      await load()
    } catch (err) {
      setError(reportUIError('performance.goals.delete', err, 'The goal could not be removed.'))
    }
  }

  return (
    <section aria-labelledby={`goals-${swimmerId}`} className="card u-p-20px-20px" >
      <div className="u-gap-12px u-mb-12px" style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', flexWrap: 'wrap' }}>
        <h2 id={`goals-${swimmerId}`} className="u-m-0 u-ff-font-display u-fw-display-weight u-fs-fs-2xl" >
          {swimmerName ? `${swimmerName}'s goals` : 'Goals'}
        </h2>
        {!adding ? <button type="button" className="btn btn-sm btn-secondary" onClick={() => setAdding(true)}>Set a goal</button> : null}
      </div>

      {error ? <LoadErrorBanner message={error} onRetry={() => void load()} /> : null}

      {adding ? (
        <div className="u-p-16px u-mb-16px" style={{ border: '1px solid var(--border)', borderRadius: 'var(--r-md)' }}>
          {formError ? <LoadErrorBanner message={formError} /> : null}
          <div className="u-gap-12px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))' }}>
            <LabeledField label="Stroke">
              <select value={stroke} onChange={(e) => { setStroke(e.target.value); setDistance(String(EVENT_DISTANCES[e.target.value as keyof typeof EVENT_DISTANCES][1] ?? 100)) }} style={FIELD_STYLE}>
                {STROKES.map((s) => <option key={s} value={s}>{STROKE_LABELS[s]}</option>)}
              </select>
            </LabeledField>
            <LabeledField label="Distance">
              <select value={distance} onChange={(e) => setDistance(e.target.value)} style={FIELD_STYLE}>
                {distances.map((d) => <option key={d} value={d}>{d}</option>)}
              </select>
            </LabeledField>
            <LabeledField label="Course">
              <select value={course} onChange={(e) => setCourse(e.target.value)} style={FIELD_STYLE}>
                {COURSES.map((c) => <option key={c}>{c}</option>)}
              </select>
            </LabeledField>
            <LabeledField label="Target time">
              <input aria-label="1:05.00" placeholder="1:05.00" value={target} onChange={(e) => setTarget(e.target.value)} style={FIELD_STYLE} />
            </LabeledField>
            <LabeledField label="Deadline (optional)">
              <input type="date" value={deadline} onChange={(e) => setDeadline(e.target.value)} style={FIELD_STYLE} />
            </LabeledField>
          </div>
          <div className="u-gap-8px u-mt-12px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <button type="button" className="btn btn-sm btn-ghost" onClick={() => { setAdding(false); setFormError(null) }}>Cancel</button>
            <button type="button" className="btn btn-sm btn-primary" disabled={saving} onClick={() => void submit()}>{saving ? 'Saving…' : 'Save goal'}</button>
          </div>
        </div>
      ) : null}

      {loading ? (
        <PageSkeleton variant="list" noHeader rows={4} />
      ) : !error && goals.length === 0 ? (
        <p className="u-fs-fs-base u-m-0" style={{ color: 'var(--text-muted)' }}>No goals yet. Pick an event and a target time to track progress after every meet.</p>
      ) : (
        <ul className="u-m-0 u-p-0 u-gap-16px" style={{ listStyle: 'none', display: 'grid' }}>
          {goals.map((g) => {
            const { fill, tone } = goalTone(g)
            const color = tone === 'success' ? 'var(--success)' : tone === 'warning' ? 'var(--warning)' : 'var(--accent)'
            const due = deadlineText(g)
            return (
              <li key={g.id}>
                <div className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', flexWrap: 'wrap' }}>
                  <span className="u-fw-600" >{g.distance} {STROKE_LABELS[g.stroke]} {g.course}</span>
                  <span style={{ fontVariantNumeric: 'tabular-nums' }}>
                    {formatSwimTime(g.best_time_ms)} <span style={{ color: 'var(--text-muted)' }}>now, goal</span> {formatSwimTime(g.target_time_ms)}
                  </span>
                </div>
                <div
                  role="meter"
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.round(fill)}
                  aria-label={`${g.distance} ${STROKE_LABELS[g.stroke]} ${g.course} goal progress`}
                  title={g.baseline_time_ms ? `Started at ${formatSwimTime(g.baseline_time_ms)}` : 'No time when the goal was set'}
                  style={{ height: '6px', borderRadius: '4px', background: 'var(--bg-surface)', margin: '6px 0 4px', overflow: 'hidden' }}
                >
                  <div style={{ width: '100%', height: '100%', borderRadius: '4px', background: color, transform: `scaleX(${fill / 100})`, transformOrigin: 'left', transition: 'transform var(--dur-slow) var(--ease-out)' }} />
                </div>
                <div className="u-gap-8px u-fs-fs-sm" style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', color: 'var(--text-muted)' }}>
                  <span>
                    {g.achieved ? <strong style={{ color: 'var(--success)' }}>Achieved</strong>
                      : g.gap_ms != null ? `${formatSwimTime(g.gap_ms)} to go` : 'No time in this event yet'}
                    {due ? <span style={{ color: g.overdue ? 'var(--warning)' : undefined }}>{` · ${due}`}</span> : null}
                  </span>
                  <button type="button" className="btn btn-ghost btn-sm u-fs-fs-sm" style={{ padding: '0 6px' }} onClick={() => void remove(g)} aria-label={`Remove ${g.distance} ${STROKE_LABELS[g.stroke]} goal`}>Remove</button>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
