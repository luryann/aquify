'use client'

import { useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { submitWellnessCheckIn, type WellnessCheckIn, type WellnessMetrics } from '@/lib/engagementData'
import { reportUIError } from '@/lib/frontend-telemetry'
import { SCALE_QUESTIONS, localToday, readiness, type ScaleField } from '@/lib/wellness-scale'

const DEFAULTS: WellnessMetrics = { sleep_hours: 8, sleep_quality: 3, soreness: 2, energy: 3, stress: 2, mood: 3 }

function fromCheckIn(c?: WellnessCheckIn | null): WellnessMetrics {
  if (!c) return DEFAULTS
  return { sleep_hours: c.sleep_hours, sleep_quality: c.sleep_quality, soreness: c.soreness, energy: c.energy, stress: c.stress, mood: c.mood }
}

/** Daily check-in form; resubmitting the same day replaces that day's entry. */
export function WellnessCheckInForm({
  api, orgId, swimmerId, existing, onSaved, forName,
}: {
  api: ApiClient
  orgId: string
  swimmerId?: string
  existing?: WellnessCheckIn | null
  onSaved: (c: WellnessCheckIn) => void
  forName?: string
}) {
  const [m, setM] = useState<WellnessMetrics>(() => fromCheckIn(existing))
  const [note, setNote] = useState(existing?.note ?? '')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  const set = (field: ScaleField, v: number) => { setM((p) => ({ ...p, [field]: v })); setSaved(false) }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError(null)
    try {
      const res = await submitWellnessCheckIn(api, orgId, { ...m, swimmer_id: swimmerId, date: localToday(), note: note.trim() || undefined })
      setSaved(true)
      onSaved(res.checkin)
    } catch (err) {
      setError(reportUIError('wellness-checkin-save', err, 'Your check-in did not save. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={submit} className="u-gap-20px" style={{ display: 'flex', flexDirection: 'column' }}>
      <div>
        <label htmlFor="sleep-hours" className="u-fs-fs-base u-fw-600 u-mb-8px" style={{ display: 'block', color: 'var(--text)' }}>
          Hours of sleep last night{forName ? ` (${forName})` : ''}
        </label>
        <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center' }}>
          <input id="sleep-hours" type="range" min={3} max={12} step={0.5} value={m.sleep_hours}
            onChange={(e) => { setM((p) => ({ ...p, sleep_hours: Number(e.target.value) })); setSaved(false) }}
            style={{ flex: 1, accentColor: 'var(--accent)' }} />
          <span className="u-ff-font-display u-fw-display-weight u-fs-fs-3xl" style={{ minWidth: 64, textAlign: 'right', color: 'var(--text)' }}>{m.sleep_hours}h</span>
        </div>
      </div>
      {SCALE_QUESTIONS.map((q) => (
        <fieldset key={q.field} className="u-p-0 u-m-0" style={{ border: 'none' }}>
          <legend className="u-fs-fs-base u-fw-600 u-mb-8px" style={{ color: 'var(--text)' }}>{q.label}</legend>
          <div className="u-gap-6px" style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)' }}>
            {[1, 2, 3, 4, 5].map((v) => {
              const active = m[q.field] === v
              return (
                <button key={v} type="button" aria-pressed={active} onClick={() => set(q.field, v)}
                  className="u-fs-fs-md u-fw-600" style={{ minHeight: 44, borderRadius: 8, cursor: 'pointer', border: `1px solid ${active ? 'var(--accent)' : 'var(--border-strong)'}`, background: active ? 'var(--accent-subtle)' : 'var(--bg-elevated)', color: active ? 'var(--accent)' : 'var(--text-subtle)' }}>{v}</button>
              )
            })}
          </div>
          <div className="u-fs-fs-xs u-mt-4px" style={{ display: 'flex', justifyContent: 'space-between', color: 'var(--text-muted)' }}>
            <span>{q.low}</span><span>{q.high}</span>
          </div>
        </fieldset>
      ))}
      <div>
        <label htmlFor="wellness-note" className="u-fs-fs-base u-fw-600 u-mb-6px" style={{ display: 'block', color: 'var(--text)' }}>Anything your coach should know? (optional)</label>
        <textarea id="wellness-note" className="form-textarea" maxLength={500} rows={2} value={note}
          onChange={(e) => { setNote(e.target.value); setSaved(false) }}
          placeholder="Only your coaches and your family can read this." style={{ width: '100%' }} />
      </div>
      <div className="u-gap-16px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
        <button type="submit" className="btn btn-primary" disabled={saving}>
          {saving ? 'Saving…' : existing ? 'Update check-in' : 'Save check-in'}
        </button>
        <span className="u-fs-fs-base" style={{ color: 'var(--text-subtle)' }}>Readiness {readiness(m).toFixed(1)} / 5</span>
        {saved && <span role="status" className="u-fs-fs-base" style={{ color: 'var(--success)' }}>Saved for today.</span>}
      </div>
      {error && <p role="alert" className="u-fs-fs-base" style={{ color: 'var(--error)' }}>{error}</p>}
    </form>
  )
}
