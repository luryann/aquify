'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { useToast } from '@/components/ui/Toast'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  createSeason,
  deleteSeason,
  fetchSeasons,
  updateSeason,
  type SeasonRecord,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { FIELD_INPUT_STYLE } from '@/lib/form-styles'

type SeasonStatus = SeasonRecord['status']

const STATUS_LABELS: Record<SeasonStatus, string> = {
  active: 'Active',
  upcoming: 'Upcoming',
  completed: 'Completed',
}

const STATUS_COLORS: Record<SeasonStatus, { bg: string; border: string; text: string }> = {
  active:    { bg: 'var(--success-bg)',  border: 'var(--success-border)',  text: 'var(--success)' },
  upcoming:  { bg: 'var(--info-bg)',     border: 'var(--info-border)',     text: 'var(--info)' },
  completed: { bg: 'var(--bg-elevated)', border: 'var(--border)',          text: 'var(--text-muted)' },
}

function StatusBadge({ status }: { status: SeasonStatus }) {
  const c = STATUS_COLORS[status]
  return (
    <span className="u-gap-4px u-p-4px-12px u-fs-fs-xs u-ff-font-body u-fw-600" style={{ display: 'inline-flex', alignItems: 'center', borderRadius: '16px', background: c.bg, border: `1px solid ${c.border}`, color: c.text, textTransform: 'uppercase', letterSpacing: '0.06em' }}>
      <span style={{ width: '5px', height: '5px', borderRadius: '50%', background: c.text, display: 'inline-block' }} />
      {STATUS_LABELS[status]}
    </span>
  )
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <label className="u-fs-fs-sm u-ff-font-body u-mb-8px" style={{ display: 'block', color: 'var(--text-muted)' }}>
      {children}
    </label>
  )
}

type SeasonDraft = {
  name: string
  startDate: string
  endDate: string
  status: SeasonStatus
}

function draftFromSeason(season: SeasonRecord): SeasonDraft {
  return {
    name: season.name,
    startDate: season.start_date,
    endDate: season.end_date,
    status: season.status,
  }
}

function formatSeasonDate(value: string) {
  if (!value) return '—'
  return new Date(value + 'T00:00:00').toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })
}

function StatusPicker({ value, onChange }: { value: SeasonStatus; onChange: (s: SeasonStatus) => void }) {
  return (
    <div className="u-gap-6px" style={{ display: 'flex' }}>
      {(['active', 'upcoming', 'completed'] as SeasonStatus[]).map((s) => (
        <button type="button"
          key={s}
          onClick={() => onChange(s)}
          aria-pressed={value === s} className="u-p-6px-16px u-fs-fs-sm u-ff-font-body" style={{ minHeight: '36px', borderRadius: '8px', border: `1px solid ${value === s ? 'var(--accent-border)' : 'var(--border)'}`, background: value === s ? 'var(--accent-subtle)' : 'var(--bg-elevated)', color: value === s ? 'var(--accent)' : 'var(--text-muted)', cursor: 'pointer', fontWeight: value === s ? 600 : 400, transition: 'all 0.15s' }}
        >{STATUS_LABELS[s]}</button>
      ))}
    </div>
  )
}

function SeasonCard({
  season,
  accent,
  saving,
  onSave,
  onDelete,
}: {
  season: SeasonRecord
  accent?: boolean
  saving: boolean
  onSave: (draft: SeasonDraft) => Promise<boolean>
  onDelete: () => Promise<void>
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<SeasonDraft>(() => draftFromSeason(season))

  function startEditing() {
    setDraft(draftFromSeason(season))
    setEditing(true)
  }

  async function handleSave() {
    const saved = await onSave(draft)
    if (saved) setEditing(false)
  }

  function u<K extends keyof SeasonDraft>(key: K, value: SeasonDraft[K]) {
    setDraft((d) => ({ ...d, [key]: value }))
  }

  const name = editing ? draft.name : season.name
  const startDate = editing ? draft.startDate : season.start_date
  const endDate = editing ? draft.endDate : season.end_date
  const status = editing ? draft.status : season.status

  return (
    <div className="card u-p-24px" style={{ border: accent ? '1px solid var(--accent-border)' : '1px solid var(--border)', background: accent ? 'rgba(235,93,30,0.03)' : undefined, position: 'relative' }}>
      {/* Header */}
      <div className="u-mb-20px" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
        <div>
          {editing ? (
            <input
              value={draft.name}
              onChange={(e) => u('name', e.target.value)}
              style={{ ...FIELD_INPUT_STYLE, fontWeight: 'var(--display-weight)', width: '240px' }}
            />
          ) : (
            <h2 className="section-title">
              {name || <span style={{ color: 'var(--text-muted)' }}>Unnamed season</span>}
            </h2>
          )}
        </div>
        <div className="u-gap-12px" style={{ display: 'flex', alignItems: 'center' }}>
          {!editing && <StatusBadge status={status} />}
          <button type="button"
            onClick={() => editing ? setEditing(false) : startEditing()}
            className="btn btn-ghost u-gap-4px u-fs-fs-sm"
            
          >
            <Icon name={editing ? 'x' : 'edit-2'} size={12} />
            {editing ? 'Cancel' : 'Edit'}
          </button>
        </div>
      </div>

      {/* Fields */}
      <div className="cols-2 u-gap-16px u-mb-16px" >
        <div>
          <FieldLabel>Start date</FieldLabel>
          {editing ? (
            <input type="date" value={draft.startDate} onChange={(e) => u('startDate', e.target.value)}
              style={FIELD_INPUT_STYLE}
            />
          ) : (
            <p className="u-fs-fs-md u-ff-font-body" style={{ color: startDate ? 'var(--text)' : 'var(--text-muted)' }}>
              {formatSeasonDate(startDate)}
            </p>
          )}
        </div>
        <div>
          <FieldLabel>End date</FieldLabel>
          {editing ? (
            <input type="date" value={draft.endDate} onChange={(e) => u('endDate', e.target.value)}
              style={FIELD_INPUT_STYLE}
            />
          ) : (
            <p className="u-fs-fs-md u-ff-font-body" style={{ color: endDate ? 'var(--text)' : 'var(--text-muted)' }}>
              {formatSeasonDate(endDate)}
            </p>
          )}
        </div>
      </div>

      {editing && (
        <div className="u-mb-16px" >
          <FieldLabel>Status</FieldLabel>
          <StatusPicker value={draft.status} onChange={(s) => u('status', s)} />
        </div>
      )}

      {/* Duration bar (display mode) */}
      {!editing && startDate && endDate && (
        <div className="u-mb-16px" >
          <div className="u-mb-4px" style={{ display: 'flex', justifyContent: 'space-between' }}>
            <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
              {Math.round((new Date(endDate + 'T00:00:00').getTime() - new Date(startDate + 'T00:00:00').getTime()) / (1000 * 60 * 60 * 24 * 7))} weeks
            </span>
            {status === 'active' && (() => {
              const now = Date.now()
              const start = new Date(startDate + 'T00:00:00').getTime()
              const end = new Date(endDate + 'T00:00:00').getTime()
              const pct = Math.min(100, Math.max(0, Math.round((now - start) / (end - start) * 100)))
              return (
                <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--accent-text, var(--accent))' }}>
                  {pct}% complete
                </span>
              )
            })()}
          </div>
          {status === 'active' && (() => {
            const now = Date.now()
            const start = new Date(startDate + 'T00:00:00').getTime()
            const end = new Date(endDate + 'T00:00:00').getTime()
            const pct = Math.min(100, Math.max(0, Math.round((now - start) / (end - start) * 100)))
            return (
              <div style={{ height: '4px', borderRadius: '2px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
                <div style={{ height: '100%', width: `${pct}%`, background: 'var(--accent)', borderRadius: '2px', transition: 'width 0.6s ease' }} />
              </div>
            )
          })()}
        </div>
      )}

      {/* Save / delete actions */}
      {editing && (
        <div className="u-mt-8px" style={{ display: 'flex', justifyContent: 'space-between' }}>
          <button type="button" onClick={() => { void onDelete() }} className="btn btn-ghost u-fs-fs-sm u-gap-4px" disabled={saving} style={{ color: 'var(--danger, #dc2626)' }}>
            <Icon name="trash-2" size={12} /> Delete
          </button>
          <button type="button" onClick={() => { void handleSave() }} className="btn btn-primary u-fs-fs-base" disabled={saving} >
            <Icon name="check" size={13} /> {saving ? 'Saving…' : 'Save Changes'}
          </button>
        </div>
      )}
    </div>
  )
}

const EMPTY_DRAFT: SeasonDraft = { name: '', startDate: '', endDate: '', status: 'upcoming' }

export default function SeasonsPage() {
  const toast = useToast()
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [seasons, setSeasons] = useState<SeasonRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [actionError, setActionError] = useState('')
  const [saving, setSaving] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)

  const [creating, setCreating] = useState(false)
  const [createDraft, setCreateDraft] = useState<SeasonDraft>(EMPTY_DRAFT)

  const [goals, setGoals] = useState('')
  const [goalsSaved, setGoalsSaved] = useState(false)
  const goalsSavedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return () => {
      if (goalsSavedTimerRef.current) clearTimeout(goalsSavedTimerRef.current)
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    async function load() {
      if (!orgId) {
        setLoading(false)
        return
      }
      setLoading(true)
      setLoadError('')
      try {
        const list = await fetchSeasons(api, orgId)
        if (cancelled) return
        setSeasons(list)
      } catch (err) {
        if (cancelled) return
        setLoadError(uiErrorMessage(err, 'Couldn’t load seasons. Try again.'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => { cancelled = true }
  }, [api, orgId, reloadKey])

  const activeSeason = useMemo(
    () => seasons.find((s) => s.status === 'active') ?? seasons[0] ?? null,
    [seasons],
  )

  useEffect(() => {
    setGoals(activeSeason?.goals ?? '')
  }, [activeSeason?.id, activeSeason?.goals])

  const replaceSeason = useCallback((updated: SeasonRecord) => {
    setSeasons((current) => current.map((s) => (s.id === updated.id ? updated : s)))
  }, [])

  async function handleSaveSeason(season: SeasonRecord, draft: SeasonDraft): Promise<boolean> {
    setSaving(true)
    setActionError('')
    try {
      const updated = await updateSeason(api, orgId, season.id, {
        name: draft.name,
        start_date: draft.startDate,
        end_date: draft.endDate,
        status: draft.status,
      })
      replaceSeason(updated)
      toast.successWithAction('Season saved', {
        label: 'Undo',
        onClick: () => {
          void updateSeason(api, orgId, season.id, { name: season.name, start_date: season.start_date, end_date: season.end_date, status: season.status })
            .then(replaceSeason)
            .catch((err) => setActionError(uiErrorMessage(err, 'Couldn’t undo. Try again.')))
        },
      })
      return true
    } catch (err) {
      setActionError(uiErrorMessage(err, 'Couldn’t save season. Try again.'))
      return false
    } finally {
      setSaving(false)
    }
  }

  async function handleDeleteSeason(season: SeasonRecord) {
    setSaving(true)
    setActionError('')
    try {
      await deleteSeason(api, orgId, season.id)
      setSeasons((current) => current.filter((s) => s.id !== season.id))
    } catch (err) {
      setActionError(uiErrorMessage(err, 'Couldn’t delete season. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  async function handleCreateSeason() {
    if (!createDraft.name.trim()) {
      setActionError('Season name is required.')
      return
    }
    setSaving(true)
    setActionError('')
    try {
      const created = await createSeason(api, orgId, {
        name: createDraft.name.trim(),
        start_date: createDraft.startDate,
        end_date: createDraft.endDate,
        status: createDraft.status,
      })
      setSeasons((current) => [created, ...current])
      setCreating(false)
      setCreateDraft(EMPTY_DRAFT)
    } catch (err) {
      setActionError(uiErrorMessage(err, 'Couldn’t create season. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  async function handleSaveGoals() {
    if (!activeSeason) return
    setSaving(true)
    setActionError('')
    try {
      const updated = await updateSeason(api, orgId, activeSeason.id, { goals })
      replaceSeason(updated)
      setGoalsSaved(true)
      if (goalsSavedTimerRef.current) clearTimeout(goalsSavedTimerRef.current)
      goalsSavedTimerRef.current = setTimeout(() => setGoalsSaved(false), 2000)
    } catch (err) {
      setActionError(uiErrorMessage(err, 'Couldn’t save goals. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  const timelineSeasons = seasons.filter((s) => s.start_date && s.end_date)

  return (
    <div className="page">
      {/* Page header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">
            Seasons
          </h1>
        </div>
        <button type="button" onClick={() => { setCreating((c) => !c); setActionError('') }} className={`btn ${creating ? 'btn-secondary' : 'btn-primary'} u-fs-fs-base u-gap-6px`}>
          <Icon name={creating ? 'x' : 'plus'} size={13} /> {creating ? 'Cancel' : 'New season'}
        </button>
      </div>

      {/* Errors */}
      {(loadError || actionError) && (
        <div className="card u-p-16px-20px u-mb-20px u-gap-12px" style={{ border: '1px solid var(--danger-border, #fecaca)', background: 'var(--danger-bg, #fef2f2)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <p className="u-fs-fs-base" style={{ color: 'var(--danger, #dc2626)' }}>{loadError || actionError}</p>
          {loadError && (
            <button type="button" onClick={() => setReloadKey((k) => k + 1)} className="btn btn-secondary u-fs-fs-sm" style={{ flexShrink: 0 }}>
              Retry
            </button>
          )}
        </div>
      )}

      {/* Create form */}
      {creating && (
        <div className="card u-p-24px u-mb-24px" style={{ border: '1px solid var(--accent-border)' }}>
          <div className="cols-2 u-gap-16px u-mb-16px" >
            <div style={{ gridColumn: '1 / -1' }}>
              <FieldLabel>Name</FieldLabel>
              <input aria-label="2026–2027 Season" value={createDraft.name} onChange={(e) => setCreateDraft((d) => ({ ...d, name: e.target.value }))} placeholder="2026–2027 Season" style={FIELD_INPUT_STYLE} />
            </div>
            <div>
              <FieldLabel>Start date</FieldLabel>
              <input type="date" value={createDraft.startDate} onChange={(e) => setCreateDraft((d) => ({ ...d, startDate: e.target.value }))} style={FIELD_INPUT_STYLE} />
            </div>
            <div>
              <FieldLabel>End date</FieldLabel>
              <input type="date" value={createDraft.endDate} onChange={(e) => setCreateDraft((d) => ({ ...d, endDate: e.target.value }))} style={FIELD_INPUT_STYLE} />
            </div>
          </div>
          <div className="u-mb-16px" >
            <FieldLabel>Status</FieldLabel>
            <StatusPicker value={createDraft.status} onChange={(s) => setCreateDraft((d) => ({ ...d, status: s }))} />
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <button type="button" onClick={() => { void handleCreateSeason() }} className="btn btn-primary u-fs-fs-base" disabled={saving} >
              <Icon name="check" size={13} /> {saving ? 'Creating…' : 'Create season'}
            </button>
          </div>
        </div>
      )}

      {/* Loading */}
      {loading && (
        <div className="card u-p-40px u-mb-24px" style={{ textAlign: 'center' }}>
          <p className="page-subtitle">Loading seasons…</p>
        </div>
      )}

      {/* Empty state */}
      {!loading && !loadError && seasons.length === 0 && !creating && (
        <div className="card u-p-40px u-mb-24px" style={{ textAlign: 'center' }}>
          <p className="u-fs-fs-md u-mb-6px" style={{ color: 'var(--text)' }}>No seasons yet</p>
          <p className="page-subtitle">Create your first season to start planning dates and goals.</p>
        </div>
      )}

      {/* Season cards */}
      {!loading && seasons.length > 0 && (
        <div className="cols-2 u-gap-20px u-mb-24px" >
          {seasons.map((season) => (
            <SeasonCard
              key={season.id}
              season={season}
              accent={season.status === 'active'}
              saving={saving}
              onSave={(draft) => handleSaveSeason(season, draft)}
              onDelete={() => handleDeleteSeason(season)}
            />
          ))}
        </div>
      )}

      {/* Season Goals */}
      {!loading && activeSeason && (
        <div className="card u-p-24px u-mb-24px" >
          <div className="u-mb-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <div>
              <h2 className="card-title">
                {activeSeason.name} goals
              </h2>
            </div>
            <button type="button"
              onClick={() => { void handleSaveGoals() }}
              className="btn btn-secondary u-fs-fs-sm u-gap-6px"
              disabled={saving}
              style={{ flexShrink: 0 }}
            >
              {goalsSaved ? (
                <><Icon name="check" size={12} /> Saved</>
              ) : (
                <><Icon name="check" size={12} /> {saving ? 'Saving…' : 'Save Goals'}</>
              )}
            </button>
          </div>
          <textarea
            value={goals}
            onChange={(e) => setGoals(e.target.value)}
            placeholder={'e.g.\n• Qualify 10+ swimmers for sectionals\n• Improve team average in 200 fly by 3%\n• Launch new age group development track\n• Increase meet participation rate to 85%'}
            rows={8}
            className="u-p-12px-16px u-fs-fs-base u-ff-font-body u-lh-1_6" style={{ width: '100%', resize: 'vertical' }}
          />
        </div>
      )}

      {/* Timeline overview */}
      {!loading && timelineSeasons.length > 0 && (
        <div className="card u-p-24px" >
          <h2 className="card-title u-mb-20px" >
            Timeline
          </h2>

          {timelineSeasons.map((s, i) => {
            const startDate = new Date(s.start_date + 'T00:00:00')
            const endDate = new Date(s.end_date + 'T00:00:00')
            const startYear = startDate.getFullYear()
            const endYear = endDate.getFullYear()
            const months = Math.round((endDate.getTime() - startDate.getTime()) / (1000 * 60 * 60 * 24 * 30))
            const color = s.status === 'active' ? 'var(--accent)' : s.status === 'upcoming' ? 'var(--info)' : 'var(--text-muted)'

            return (
              <div key={s.id} style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: '8px 16px', marginBottom: i === timelineSeasons.length - 1 ? '0' : '14px' }}>
                <div className="u-fs-fs-sm u-ff-font-body" style={{ width: '120px', color: 'var(--text-subtle)', flexShrink: 0 }}>
                  {startYear} – {endYear}
                </div>
                <div style={{ flex: '1 1 140px', height: '8px', borderRadius: '4px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
                  <div style={{
                    height: '100%',
                    width: s.status === 'active'
                      ? `${Math.min(100, Math.max(5, Math.round((Date.now() - startDate.getTime()) / (endDate.getTime() - startDate.getTime()) * 100)))}%`
                      : s.status === 'upcoming' ? '5%' : '100%',
                    background: color,
                    borderRadius: '4px',
                    opacity: s.status === 'upcoming' ? 0.4 : 1,
                    transition: 'width 0.6s ease',
                  }} />
                </div>
                <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', width: '160px', flexShrink: 0 }}>
                  <span className="u-fs-fs-sm u-fw-500" style={{ color: 'var(--text)' }}>{s.name}</span>
                  <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
                    {months}mo
                  </span>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
