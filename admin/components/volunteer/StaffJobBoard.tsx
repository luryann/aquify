'use client'

import { useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { fetchHouseholds, type HouseholdRecord } from '@/lib/adminData'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  cancelVolunteerSignup,
  checkInVolunteerSignup,
  deleteVolunteerJob,
  fetchVolunteerJob,
  signUpForVolunteerJob,
  type VolunteerJobView,
  type VolunteerSignup,
} from '@/lib/volunteerData'
import { VOLUNTEER_ROLE_LABELS, formatHours, groupInOrder, localDayKey } from '@/lib/volunteer-state'
import { InlineNotice, SpotPips } from './VolunteerPrimitives'
import { useConfirm } from '@/components/ui/ConfirmDialog'

const STATUS_BADGE: Record<VolunteerSignup['status'], string> = {
  confirmed: 'badge badge-info',
  worked: 'badge badge-success',
  no_show: 'badge badge-error',
  cancelled: 'badge badge-default',
}
const STATUS_TEXT: Record<VolunteerSignup['status'], string> = {
  confirmed: 'Signed up', worked: 'Worked', no_show: 'No-show', cancelled: 'Cancelled',
}

const ATTENDANCE_ERROR = 'Couldn’t record attendance. Try again.'

function time(iso: string) {
  return new Date(iso).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
}

function dayHeading(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' })
}

/**
 * Staff view of job slots grouped by day. Expanding a job loads its roster
 * with check-in (worked / no-show), removal, and a staff "add volunteer" form.
 */
export function StaffJobBoard({ api, orgId, jobs, onChanged, showEvent = true }: {
  api: ApiClient
  orgId: string
  jobs: VolunteerJobView[]
  onChanged: () => void
  showEvent?: boolean
}) {
  const [openId, setOpenId] = useState<string | null>(null)
  const [roster, setRoster] = useState<VolunteerSignup[]>([])
  const [rosterError, setRosterError] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [households, setHouseholds] = useState<HouseholdRecord[] | null>(null)
  const [addHousehold, setAddHousehold] = useState('')
  const [addName, setAddName] = useState('')

  async function loadRoster(jobId: string) {
    setRosterError(null)
    try {
      const job = await fetchVolunteerJob(api, orgId, jobId)
      setRoster((job.signups ?? []).filter((s) => s.status !== 'cancelled'))
    } catch (err) {
      setRosterError(reportUIError('volunteer.roster', err, 'Couldn’t load the roster. Try again.'))
    }
  }

  async function toggle(jobId: string) {
    if (openId === jobId) {
      setOpenId(null)
      return
    }
    setOpenId(jobId)
    setRoster([])
    void loadRoster(jobId)
    if (households === null) {
      fetchHouseholds(api, orgId).then(setHouseholds).catch(() => setHouseholds([]))
    }
  }

  /** Runs a staff action and refreshes the roster; resolves true when it succeeded. */
  async function run(key: string, fn: () => Promise<unknown>, fallback: string): Promise<boolean> {
    setBusy(key)
    setRosterError(null)
    try {
      await fn()
      if (openId) await loadRoster(openId)
      onChanged()
      return true
    } catch (err) {
      setRosterError(reportUIError('volunteer.staff_action', err, fallback))
      return false
    } finally {
      setBusy(null)
    }
  }

  const confirm = useConfirm()
  async function removeJob(job: VolunteerJobView) {
    if (!(await confirm({ title: `Delete ${job.title}?`, message: 'Volunteers signed up for it lose their spot.', confirmLabel: 'Delete job', destructive: true }))) return
    if (await run(`del-${job.id}`, () => deleteVolunteerJob(api, orgId, job.id), 'Couldn’t delete the job. Try again.')) setOpenId(null)
  }

  const days = groupInOrder(jobs, (j) => localDayKey(j.starts_at))

  return (
    <div className="u-gap-20px" style={{ display: 'flex', flexDirection: 'column' }}>
      {days.map((day) => (
        <section key={day.key}>
          <h3 className="section-title u-m-0-0-12px" >
            {dayHeading(day.items[0].starts_at)}
          </h3>
          <div className="card u-p-0" style={{ overflow: 'hidden' }}>
            {day.items.map((job, idx) => {
              const open = openId === job.id
              return (
                <div key={job.id} style={{ borderTop: idx === 0 ? 'none' : '1px solid var(--border)' }}>
                  <button
                    type="button"
                    onClick={() => toggle(job.id)}
                    aria-expanded={open}
                    className="job-row u-gap-12px u-p-12px-16px u-ff-font-body"
                    style={{ width: '100%', alignItems: 'center', textAlign: 'left', background: open ? 'var(--bg-elevated)' : 'transparent', border: 'none', color: 'var(--text)', cursor: 'pointer' }}
                  >
                    <span className="u-ff-font-mono u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>
                      {time(job.starts_at)}–{time(job.ends_at)}
                    </span>
                    <span style={{ minWidth: 0 }}>
                      <span className="u-fw-600 u-fs-fs-md" style={{ display: 'block' }}>{job.title}</span>
                      <span className="u-fs-fs-sm" style={{ display: 'block', color: 'var(--text-muted)' }}>
                        {VOLUNTEER_ROLE_LABELS[job.role]} · {formatHours(job.hour_credit)} credit
                        {showEvent ? ` · ${job.meet_name || job.event_name || 'Meet'}` : ''}
                        {job.requires_certification ? ' · Certified only' : ''}
                      </span>
                    </span>
                    <SpotPips filled={job.filled_count} capacity={job.capacity} />
                  </button>
                  {open ? (
                    <div style={{ padding: '4px 16px 16px', background: 'var(--bg-elevated)' }}>
                      {rosterError ? <InlineNotice tone="error">{rosterError}</InlineNotice> : null}
                      {roster.length === 0 && !rosterError ? (
                        <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)', margin: '8px 0' }}>Nobody has signed up yet.</p>
                      ) : null}
                      {roster.map((s) => (
                        <div key={s.id} className="u-gap-12px u-p-8px-0" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', borderBottom: '1px solid var(--border)' }}>
                          <span style={{ flex: '1 1 160px', minWidth: 0 }}>
                            <span className="u-fs-fs-md u-fw-600" >{s.volunteer_name}</span>
                            <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}> · {s.household_name || 'Household'}</span>
                          </span>
                          <span className={STATUS_BADGE[s.status]}>
                            {STATUS_TEXT[s.status]}{s.status === 'worked' ? ` ${formatHours(s.effective_hours)}` : ''}
                          </span>
                          {s.status === 'confirmed' ? (
                            <>
                              <button type="button" className="btn btn-secondary btn-sm" disabled={busy !== null}
                                onClick={() => run(s.id, () => checkInVolunteerSignup(api, orgId, s.id, 'worked'), ATTENDANCE_ERROR)}>Worked</button>
                              <button type="button" className="btn btn-ghost btn-sm" disabled={busy !== null}
                                onClick={() => run(s.id, () => checkInVolunteerSignup(api, orgId, s.id, 'no_show'), ATTENDANCE_ERROR)}>No-show</button>
                              <button type="button" className="btn btn-ghost btn-sm" disabled={busy !== null}
                                onClick={() => run(s.id, () => cancelVolunteerSignup(api, orgId, s.id, 'Removed by staff'), 'Couldn’t remove the volunteer. Try again.')}>Remove</button>
                            </>
                          ) : (
                            <button type="button" className="btn btn-ghost btn-sm" disabled={busy !== null}
                              onClick={() => run(s.id, () => checkInVolunteerSignup(api, orgId, s.id, 'confirmed'), 'Couldn’t undo attendance. Try again.')}>Undo</button>
                          )}
                        </div>
                      ))}
                      <form
                        onSubmit={(e) => {
                          e.preventDefault()
                          if (!addHousehold || !addName.trim()) return
                          void run('add', () => signUpForVolunteerJob(api, orgId, job.id, {
                            household_id: addHousehold, volunteer_name: addName.trim(), certified: job.requires_certification,
                          }), 'Couldn’t add the volunteer. Try again.').then((ok) => { if (ok) setAddName('') })
                        }}
                        className="u-gap-8px u-mt-12px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}
                      >
                        <select className="form-input" aria-label="Household" value={addHousehold} onChange={(e) => setAddHousehold(e.target.value)} style={{ flex: '1 1 160px', minWidth: 0 }}>
                          <option value="">Household…</option>
                          {(households ?? []).map((h) => <option key={h.id} value={h.id}>{h.household_name}</option>)}
                        </select>
                        <input className="form-input" aria-label="Volunteer name" placeholder="Volunteer name" value={addName}
                          onChange={(e) => setAddName(e.target.value)} style={{ flex: '1 1 160px', minWidth: 0 }} />
                        <button type="submit" className="btn btn-primary btn-sm" disabled={busy !== null || job.open_spots === 0 || !addHousehold || !addName.trim()}>
                          Add volunteer
                        </button>
                        <button type="button" className="btn btn-danger btn-sm u-ml-auto" disabled={busy !== null} onClick={() => removeJob(job)}>
                          Delete job
                        </button>
                      </form>
                    </div>
                  ) : null}
                </div>
              )
            })}
          </div>
        </section>
      ))}
    </div>
  )
}
