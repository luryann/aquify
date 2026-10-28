'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { Modal } from '@/components/ui/Modal'
import { useSession } from '@/lib/session'
import { rawRequestAuthHeaders, useApiClient } from '@/lib/api'
import { fetchGroups, type GroupRecord } from '@/lib/adminData'
import { reportUIError } from '@/lib/frontend-telemetry'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import {
  CLUB_EVENT_CATEGORIES, CLUB_EVENT_CATEGORY_LABELS, clubEventExportPath, createClubEvent, deleteClubEvent,
  fetchClubEventRSVPs, fetchClubEvents, updateClubEvent,
  type ClubEventRecord, type ClubEventRSVP, type ClubEventSummary,
} from '@/lib/engagementData'
import {
  RSVP_LABELS, clubEventDateKey, clubEventInputFromDraft, draftFromClubEvent, emptyClubEventDraft,
  formatClubEventWhen, formatFee, type ClubEventDraft,
} from '@/lib/club-events'
import { parseBackendDate } from '@/lib/dates'
import { useConfirm } from '@/components/ui/ConfirmDialog'

const label: React.CSSProperties = { display: 'block', fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', marginBottom: 6 }

function EventForm({ draft, setDraft, groups, rsvpModeLocked }: {
  draft: ClubEventDraft
  setDraft: (d: ClubEventDraft) => void
  groups: GroupRecord[]
  rsvpModeLocked: boolean
}) {
  const u = <K extends keyof ClubEventDraft>(k: K, v: ClubEventDraft[K]) => setDraft({ ...draft, [k]: v })
  return (
    <div className="u-gap-16px" style={{ display: 'grid' }}>
      <div>
        <label style={label} htmlFor="ev-title">Title</label>
        <input id="ev-title" className="input" value={draft.title} onChange={(e) => u('title', e.target.value)} maxLength={200} style={{ width: '100%' }} />
      </div>
      <div className="u-gap-12px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))' }}>
        <div>
          <label style={label} htmlFor="ev-cat">Type</label>
          <select id="ev-cat" className="input" value={draft.category} onChange={(e) => u('category', e.target.value as ClubEventDraft['category'])} style={{ width: '100%' }}>
            {CLUB_EVENT_CATEGORIES.map((c) => <option key={c} value={c}>{CLUB_EVENT_CATEGORY_LABELS[c]}</option>)}
          </select>
        </div>
        <div>
          <label style={label} htmlFor="ev-loc">Location</label>
          <input id="ev-loc" className="input" value={draft.location} onChange={(e) => u('location', e.target.value)} style={{ width: '100%' }} />
        </div>
      </div>
      <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', color: 'var(--text)' }}>
        <input type="checkbox" checked={draft.allDay} onChange={(e) => u('allDay', e.target.checked)} /> All-day event
      </label>
      <div className="u-gap-12px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))' }}>
        {draft.allDay ? (
          <>
            <div><label style={label} htmlFor="ev-sd">Date</label><input id="ev-sd" type="date" className="input" value={draft.startDate} onChange={(e) => u('startDate', e.target.value)} style={{ width: '100%' }} /></div>
            <div><label style={label} htmlFor="ev-ed">Last day (optional)</label><input id="ev-ed" type="date" className="input" value={draft.endDate} onChange={(e) => u('endDate', e.target.value)} style={{ width: '100%' }} /></div>
          </>
        ) : (
          <>
            <div><label style={label} htmlFor="ev-s">Starts</label><input id="ev-s" type="datetime-local" className="input" value={draft.startsAt} onChange={(e) => u('startsAt', e.target.value)} style={{ width: '100%' }} /></div>
            <div><label style={label} htmlFor="ev-e">Ends (optional)</label><input id="ev-e" type="datetime-local" className="input" value={draft.endsAt} onChange={(e) => u('endsAt', e.target.value)} style={{ width: '100%' }} /></div>
          </>
        )}
      </div>
      <div>
        <label style={label} htmlFor="ev-desc">Details</label>
        <textarea id="ev-desc" className="form-textarea" rows={3} value={draft.description} onChange={(e) => u('description', e.target.value)} style={{ width: '100%' }} />
      </div>
      <fieldset className="u-p-0 u-m-0" style={{ border: 'none' }}>
        <legend style={label}>Who is invited</legend>
        <div className="u-gap-8px" style={{ display: 'flex', flexWrap: 'wrap' }}>
          <button type="button" className={draft.audience === 'all' ? 'btn btn-primary btn-sm' : 'btn btn-secondary btn-sm'} onClick={() => u('audience', 'all')}>Whole club</button>
          <button type="button" className={draft.audience === 'groups' ? 'btn btn-primary btn-sm' : 'btn btn-secondary btn-sm'} onClick={() => u('audience', 'groups')}>Specific groups</button>
        </div>
        {draft.audience === 'groups' && (
          <div className="u-gap-6px u-mt-12px" style={{ display: 'flex', flexWrap: 'wrap' }}>
            {groups.map((g) => {
              const on = draft.groupCodes.includes(g.code)
              return (
                <button key={g.id} type="button" aria-pressed={on} className={on ? 'badge badge-accent' : 'badge badge-default'} style={{ cursor: 'pointer', border: 'none' }}
                  onClick={() => u('groupCodes', on ? draft.groupCodes.filter((c) => c !== g.code) : [...draft.groupCodes, g.code])}>
                  {g.name}
                </button>
              )
            })}
          </div>
        )}
      </fieldset>
      {draft.audience === 'all' && (
        <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', color: 'var(--text)' }}>
          <input type="checkbox" checked={draft.isPublic} onChange={(e) => u('isPublic', e.target.checked)} /> Show on the public team website
        </label>
      )}
      <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', color: 'var(--text)' }}>
        <input type="checkbox" checked={draft.rsvpEnabled} onChange={(e) => u('rsvpEnabled', e.target.checked)} /> Collect RSVPs
      </label>
      {draft.rsvpEnabled && (
        <div className="u-gap-12px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))' }}>
          <div>
            <label style={label} htmlFor="ev-mode">One reply per</label>
            <select id="ev-mode" className="input" disabled={rsvpModeLocked} value={draft.rsvpMode} onChange={(e) => u('rsvpMode', e.target.value as ClubEventDraft['rsvpMode'])} style={{ width: '100%' }}>
              <option value="household">Family</option>
              <option value="swimmer">Swimmer</option>
            </select>
            {rsvpModeLocked && <span className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>Locked once families reply.</span>}
          </div>
          <div><label style={label} htmlFor="ev-dl">Reply by (optional)</label><input id="ev-dl" type="datetime-local" className="input" value={draft.rsvpDeadline} onChange={(e) => u('rsvpDeadline', e.target.value)} style={{ width: '100%' }} /></div>
          <div><label style={label} htmlFor="ev-cap">Capacity (people)</label><input id="ev-cap" inputMode="numeric" className="input" value={draft.capacity} onChange={(e) => u('capacity', e.target.value)} placeholder="No limit" style={{ width: '100%' }} /></div>
          <div><label style={label} htmlFor="ev-fee">Fee per person ($)</label><input id="ev-fee" inputMode="decimal" className="input" value={draft.fee} onChange={(e) => u('fee', e.target.value)} placeholder="Free" style={{ width: '100%' }} /></div>
        </div>
      )}
      {draft.rsvpEnabled && Number.parseFloat(draft.fee) > 0 && (
        <p className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>Each “Going” reply creates an invoice for the party size in the family’s billing account. Changing to “Not going” cancels it if unpaid.</p>
      )}
    </div>
  )
}

function SummaryLine({ s, capacity }: { s: ClubEventSummary; capacity?: number | null }) {
  return (
    <span className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
      {s.yes_headcount}{capacity ? ` / ${capacity}` : ''} going · {s.maybe} maybe · {s.no} not going
    </span>
  )
}

export default function AdminEventsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [events, setEvents] = useState<ClubEventRecord[]>([])
  const [groups, setGroups] = useState<GroupRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [showPast, setShowPast] = useState(false)
  const [editing, setEditing] = useState<ClubEventRecord | 'new' | null>(null)
  const [draft, setDraft] = useState<ClubEventDraft>(emptyClubEventDraft)
  const [formError, setFormError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  const [roster, setRoster] = useState<{ rsvps: ClubEventRSVP[]; summary: ClubEventSummary } | null>(null)
  const [rosterError, setRosterError] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId) return
    setLoading(true)
    setError(null)
    Promise.all([fetchClubEvents(api, orgId), fetchGroups(api, orgId)])
      .then(([ev, gr]) => { setEvents(ev); setGroups(gr) })
      .catch((err) => { setEvents([]); setError(reportUIError('admin-club-events', err, 'Events could not be loaded.')) })
      .finally(() => setLoading(false))
  }, [api, orgId, reloadKey])

  const loadRoster = useCallback((id: string) => {
    setRoster(null)
    setRosterError(null)
    fetchClubEventRSVPs(api, orgId, id)
      .then(setRoster)
      .catch((err) => setRosterError(reportUIError('admin-club-event-rsvps', err, 'RSVPs could not be loaded.')))
  }, [api, orgId])

  useEffect(() => { if (selected) loadRoster(selected) }, [selected, loadRoster])

  const today = new Date().toISOString().slice(0, 10)
  const visible = useMemo(() => {
    const list = events.filter((e) => (showPast ? clubEventDateKey(e) < today : clubEventDateKey(e) >= today))
    return showPast ? [...list].reverse() : list
  }, [events, showPast, today])
  const selectedEvent = events.find((e) => e.id === selected) ?? null
  const groupName = (code: string) => groups.find((g) => g.code === code)?.name ?? code

  function openEditor(target: ClubEventRecord | 'new') {
    setEditing(target)
    setDraft(target === 'new' ? emptyClubEventDraft() : draftFromClubEvent(target))
    setFormError(null)
  }

  async function save() {
    const body = clubEventInputFromDraft(draft)
    if (typeof body === 'string') { setFormError(body); return }
    setSaving(true)
    setFormError(null)
    try {
      if (editing === 'new') await createClubEvent(api, orgId, body)
      else if (editing) await updateClubEvent(api, orgId, editing.id, body)
      setEditing(null)
      setReloadKey((k) => k + 1)
    } catch (err) {
      setFormError(reportUIError('admin-club-event-save', err, 'The event did not save.'))
    } finally {
      setSaving(false)
    }
  }

  const confirm = useConfirm()
  async function remove(e: ClubEventRecord) {
    if (!(await confirm({ title: `Delete ${e.title}?`, message: 'RSVPs are removed and unpaid event invoices are voided.', confirmLabel: 'Delete event', destructive: true }))) return
    try {
      await deleteClubEvent(api, orgId, e.id)
      if (selected === e.id) setSelected(null)
      setReloadKey((k) => k + 1)
    } catch (err) {
      setError(reportUIError('admin-club-event-delete', err, 'The event could not be deleted.'))
    }
  }

  async function exportCSV(e: ClubEventRecord) {
    try {
      const res = await fetch(clubEventExportPath(e.id), {
        credentials: 'include',
        headers: { 'X-Org-Id': orgId, ...rawRequestAuthHeaders(session?.token, 'GET') },
      })
      if (!res.ok) throw new Error(`Export failed (${res.status}).`)
      const url = URL.createObjectURL(await res.blob())
      const a = document.createElement('a')
      a.href = url
      a.download = `${e.title.replace(/[^a-z0-9]+/gi, '-').toLowerCase() || 'event'}-rsvps.csv`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err) {
      setRosterError(reportUIError('admin-club-event-export', err, 'The RSVP export failed.'))
    }
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Club events</h1>
        </div>
        <button type="button" className="btn btn-primary" onClick={() => openEditor('new')}><Icon name="plus" size={14} /> New event</button>
      </div>

      {error && (
        <div role="alert" className="card u-p-16px u-mb-16px u-gap-12px u-fs-fs-base" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', color: 'var(--warning)' }}>
          {error}
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setReloadKey((k) => k + 1)}>Retry</button>
        </div>
      )}

      <div role="tablist" className="u-gap-6px u-mb-16px" style={{ display: 'flex' }}>
        <button role="tab" aria-selected={!showPast} type="button" className={!showPast ? 'btn btn-secondary btn-sm' : 'btn btn-ghost btn-sm'} onClick={() => setShowPast(false)}>Upcoming</button>
        <button role="tab" aria-selected={showPast} type="button" className={showPast ? 'btn btn-secondary btn-sm' : 'btn btn-ghost btn-sm'} onClick={() => setShowPast(true)}>Past</button>
      </div>

      <div className="u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 380px), 1fr))', alignItems: 'start' }}>
        <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
          {loading && <div className="skeleton" style={{ height: 120, borderRadius: 12 }} />}
          {!loading && visible.map((e) => {
            const d = parseBackendDate(clubEventDateKey(e))
            const active = selected === e.id
            return (
              <button key={e.id} type="button" onClick={() => setSelected(e.id)} className="card u-gap-16px u-p-16px"
                style={{ display: 'grid', gridTemplateColumns: '52px 1fr', textAlign: 'left', cursor: 'pointer', border: `1px solid ${active ? 'var(--accent-border)' : 'var(--border)'}`, background: active ? 'var(--accent-subtle)' : undefined }}>
                <span style={{ textAlign: 'center' }}>
                  <span className="u-fs-fs-xs" style={{ display: 'block', color: 'var(--text-muted)' }}>{d.toLocaleDateString('en-US', { month: 'short' })}</span>
                  <span className="u-ff-font-display u-fw-display-weight u-fs-fs-3xl u-lh-1" style={{ display: 'block', color: 'var(--text)' }}>{d.getDate()}</span>
                </span>
                <span style={{ minWidth: 0 }}>
                  <span className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
                    <span className="u-fs-fs-md u-fw-600" style={{ color: 'var(--text)', textDecoration: e.status === 'cancelled' ? 'line-through' : 'none' }}>{e.title}</span>
                    <span className={e.status === 'cancelled' ? 'badge badge-error' : 'badge badge-default'}>{e.status === 'cancelled' ? 'Cancelled' : CLUB_EVENT_CATEGORY_LABELS[e.category]}</span>
                  </span>
                  <span className="u-fs-fs-sm u-mt-4px" style={{ display: 'block', color: 'var(--text-subtle)' }}>
                    {formatClubEventWhen(e)}{e.location ? ` · ${e.location}` : ''}
                  </span>
                  <span className="u-fs-fs-sm u-mt-4px" style={{ display: 'block', color: 'var(--text-muted)' }}>
                    {e.audience === 'all' ? 'Whole club' : e.group_codes.map(groupName).join(', ')}
                    {e.rsvp_enabled ? ` · ${e.rsvp_summary.yes_headcount}${e.capacity ? `/${e.capacity}` : ''} going` : ' · No RSVP'}
                    {e.fee_cents > 0 ? ` · ${formatFee(e.fee_cents)} pp` : ''}
                  </span>
                </span>
              </button>
            )
          })}
          {shouldShowCollectionEmptyState(loading, error, visible.length) && (
            <div className="card u-p-24px u-fs-fs-base" style={{ textAlign: 'center', color: 'var(--text-subtle)' }}>
              {showPast ? 'No past events.' : 'Nothing scheduled. Create the first event and families can RSVP from the portal.'}
            </div>
          )}
        </div>

        {selectedEvent && (
          <section className="card u-p-20px" style={{ position: 'sticky', top: 72 }} aria-label={`${selectedEvent.title} RSVPs`}>
            <div className="u-gap-12px u-mb-12px" style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap' }}>
              <h2 className="section-title">{selectedEvent.title}</h2>
              <div className="u-gap-6px" style={{ display: 'flex' }}>
                <button type="button" className="btn btn-secondary btn-sm" onClick={() => openEditor(selectedEvent)}><Icon name="edit-2" size={13} /> Edit</button>
                {selectedEvent.rsvp_enabled && <button type="button" className="btn btn-secondary btn-sm" onClick={() => exportCSV(selectedEvent)}><Icon name="download" size={13} /> CSV</button>}
                <button type="button" className="btn btn-ghost btn-sm" aria-label="Delete event" onClick={() => remove(selectedEvent)}><Icon name="trash-2" size={13} /></button>
              </div>
            </div>
            {selectedEvent.description && <p className="u-fs-fs-base u-mb-12px" style={{ color: 'var(--text-subtle)', whiteSpace: 'pre-wrap' }}>{selectedEvent.description}</p>}
            {!selectedEvent.rsvp_enabled ? (
              <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>This event does not collect RSVPs.</p>
            ) : rosterError ? (
              <p role="alert" className="u-fs-fs-base" style={{ color: 'var(--warning)' }}>{rosterError} <button type="button" className="btn btn-ghost btn-sm" onClick={() => loadRoster(selectedEvent.id)}>Retry</button></p>
            ) : !roster ? (
              <div className="skeleton" style={{ height: 100, borderRadius: 8 }} />
            ) : (
              <>
                <SummaryLine s={roster.summary} capacity={selectedEvent.capacity} />
                {roster.rsvps.length === 0 ? (
                  <p className="u-fs-fs-base u-mt-12px" style={{ color: 'var(--text-muted)' }}>No replies yet.</p>
                ) : (
                  <div className="table-scroll u-mt-12px" style={{ overflowX: 'auto' }}>
                    <table className="u-fs-fs-base" style={{ width: '100%', borderCollapse: 'collapse' }}>
                      <thead>
                        <tr className="u-fs-fs-sm" style={{ textAlign: 'left', color: 'var(--text-muted)' }}>
                          <th className="u-p-6px-8px" >{selectedEvent.rsvp_mode === 'swimmer' ? 'Swimmer' : 'Family'}</th>
                          <th className="u-p-6px-8px" >Reply</th>
                          <th className="u-p-6px-8px" >People</th>
                          <th className="u-p-6px-8px" >Note</th>
                        </tr>
                      </thead>
                      <tbody>
                        {roster.rsvps.map((r) => (
                          <tr key={r.id} style={{ borderTop: '1px solid var(--border)' }}>
                            <td className="u-p-8px" style={{ color: 'var(--text)' }}>{r.swimmer_name || r.household_name || 'Family'}{r.swimmer_name && r.household_name ? <span style={{ color: 'var(--text-muted)' }}> · {r.household_name}</span> : null}</td>
                            <td className="u-p-8px" ><span className={r.response === 'yes' ? 'badge badge-success' : r.response === 'maybe' ? 'badge badge-warning' : 'badge badge-default'}>{RSVP_LABELS[r.response]}</span></td>
                            <td className="u-p-8px" style={{ color: 'var(--text-subtle)' }}>{r.response === 'no' ? '—' : 1 + r.guest_count}{r.invoice_id ? <Icon name="dollar-sign" size={12} style={{ marginLeft: 4, color: 'var(--text-muted)' }} aria-label="Invoiced" /> : null}</td>
                            <td className="u-p-8px" style={{ color: 'var(--text-subtle)' }}>{r.note || ''}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </>
            )}
          </section>
        )}
      </div>

      <Modal
        open={editing !== null}
        onClose={() => setEditing(null)}
        title={editing === 'new' ? 'New club event' : 'Edit event'}
        width={620}
        footer={
          <div className="u-gap-8px" style={{ display: 'flex', justifyContent: 'space-between', width: '100%', flexWrap: 'wrap' }}>
            {editing && editing !== 'new' ? (
              <button type="button" className="btn btn-ghost btn-sm" onClick={() => setDraft({ ...draft, status: draft.status === 'cancelled' ? 'scheduled' : 'cancelled' })}>
                {draft.status === 'cancelled' ? 'Reinstate event' : 'Mark as cancelled'}
              </button>
            ) : <span />}
            <div className="u-gap-8px" style={{ display: 'flex' }}>
              <button type="button" className="btn btn-secondary" onClick={() => setEditing(null)}>Close</button>
              <button type="button" className="btn btn-primary" disabled={saving} onClick={save}>{saving ? 'Saving…' : editing === 'new' ? 'Create event' : 'Save changes'}</button>
            </div>
          </div>
        }
      >
        <EventForm draft={draft} setDraft={setDraft} groups={groups}
          rsvpModeLocked={editing !== null && editing !== 'new' && (editing.rsvp_summary.yes + editing.rsvp_summary.no + editing.rsvp_summary.maybe) > 0} />
        {draft.status === 'cancelled' && <p className="u-mt-12px u-fs-fs-sm" style={{ color: 'var(--warning)' }}>Saving will cancel this event and void unpaid RSVP invoices.</p>}
        {formError && <p role="alert" className="u-mt-12px u-fs-fs-base" style={{ color: 'var(--error)' }}>{formError}</p>}
      </Modal>
    </div>
  )
}
