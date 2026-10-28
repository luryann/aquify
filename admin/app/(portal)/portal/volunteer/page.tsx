'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  cancelVolunteerSignup,
  fetchHouseholdLedger,
  fetchVolunteerJobs,
  fetchVolunteerSignups,
  signUpForVolunteerJob,
  type VolunteerJobView,
  type VolunteerLedgerRow,
  type VolunteerSignup,
} from '@/lib/volunteerData'
import {
  currentLedgerRow,
  formatCents,
  formatHours,
  groupInOrder,
  localDayKey,
  shiftLabel,
} from '@/lib/volunteer-state'
import { HoursMeter, InlineNotice, LoadErrorBanner, SpotPips } from '@/components/volunteer/VolunteerPrimitives'

const h2: React.CSSProperties = { fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-2xl)', margin: '0 0 12px' }

function dayHeading(iso: string) {
  return new Date(iso).toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' })
}

export default function PortalVolunteerPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''
  const canSignUp = (session?.user.roles ?? []).includes('parent')

  const [jobs, setJobs] = useState<VolunteerJobView[]>([])
  const [signups, setSignups] = useState<VolunteerSignup[]>([])
  const [ledger, setLedger] = useState<VolunteerLedgerRow[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const [notice, setNotice] = useState<{ tone: 'error' | 'success'; text: string } | null>(null)
  const [signingUp, setSigningUp] = useState<string | null>(null)
  const [name, setName] = useState('')
  const [certified, setCertified] = useState(false)
  const [busy, setBusy] = useState(false)
  const reload = useCallback(() => setReloadKey((k) => k + 1), [])

  useEffect(() => {
    if (session?.user.name) setName((n) => n || session.user.name)
  }, [session?.user.name])

  useEffect(() => {
    if (!orgId || !householdId) {
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    Promise.all([
      fetchVolunteerJobs(api, orgId, { upcoming: true }),
      fetchVolunteerSignups(api, orgId, { householdId }),
      fetchHouseholdLedger(api, orgId, householdId),
    ])
      .then(([jobRes, mine, rows]) => {
        if (cancelled) return
        setJobs(jobRes.jobs)
        setSignups(mine)
        setLedger(rows)
        setLoadError(null)
      })
      .catch((err) => { if (!cancelled) setLoadError(reportUIError('portal.volunteer', err, 'Couldn’t load volunteer shifts. Try again.')) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [api, orgId, householdId, reloadKey])

  const current = currentLedgerRow(ledger)
  const upcomingMine = useMemo(() => {
    const now = Date.now()
    return signups.filter((s) => s.status === 'confirmed' && s.job && new Date(s.job.ends_at).getTime() >= now)
  }, [signups])
  const history = useMemo(
    () => signups.filter((s) => s.status === 'worked' || s.status === 'no_show').reverse(),
    [signups],
  )
  const mineByJob = useMemo(() => new Set(upcomingMine.map((s) => s.job_id)), [upcomingMine])
  const openDays = groupInOrder(jobs, (j) => localDayKey(j.starts_at))

  async function submitSignup(job: VolunteerJobView) {
    if (!name.trim()) return
    setBusy(true)
    setNotice(null)
    try {
      await signUpForVolunteerJob(api, orgId, job.id, { volunteer_name: name.trim(), certified: job.requires_certification ? certified : undefined })
      setNotice({ tone: 'success', text: `${name.trim()} is signed up for ${job.title}.` })
      setSigningUp(null)
      setCertified(false)
      reload()
    } catch (err) {
      setNotice({ tone: 'error', text: reportUIError('portal.volunteer.signup', err, 'Couldn’t sign up for this shift. Try again.') })
    } finally {
      setBusy(false)
    }
  }

  async function cancel(s: VolunteerSignup) {
    if (!window.confirm(`Cancel ${s.volunteer_name}'s ${s.job?.title ?? 'shift'}?`)) return
    setBusy(true)
    setNotice(null)
    try {
      await cancelVolunteerSignup(api, orgId, s.id)
      setNotice({ tone: 'success', text: 'Shift cancelled. The spot is open for another family.' })
      reload()
    } catch (err) {
      setNotice({ tone: 'error', text: reportUIError('portal.volunteer.cancel', err, 'Couldn’t cancel this shift. Try again.') })
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="page">
      <div className="page-heading">
        <h1 className="page-title">Volunteer</h1>
      </div>

      {!householdId && !loading ? (
        <p className="card" style={{ fontSize: 'var(--fs-md)', color: 'var(--text-muted)' }}>Your account is not linked to a household yet, so there are no shifts to show.</p>
      ) : null}
      {loadError ? <LoadErrorBanner message={loadError} onRetry={reload} /> : null}
      {notice ? <InlineNotice tone={notice.tone}>{notice.text}</InlineNotice> : null}

      {!loading && !loadError && current ? (
        <section className="card" style={{ marginBottom: 'var(--space-8)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', gap: '12px', flexWrap: 'wrap', marginBottom: '12px' }}>
            <h2 style={{ ...h2, margin: 0 }}>
              {current.required_hours > 0
                ? current.remaining_hours > 0
                  ? `${formatHours(current.remaining_hours)} to go in ${current.season_name}`
                  : `${current.season_name} hours complete`
                : `No hours required in ${current.season_name}`}
            </h2>
          </div>
          {current.required_hours > 0 ? (
            <HoursMeter required={current.required_hours} earned={current.earned_hours} scheduled={current.scheduled_hours} />
          ) : null}
          {current.status === 'bought_out' && current.buyout ? (
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', margin: '12px 0 0' }}>
              Your remaining hours were billed as a {formatCents(current.buyout.amount_cents)} buyout. See Billing for the invoice.
            </p>
          ) : current.remaining_hours > 0 && current.buyout_cents_per_hour > 0 ? (
            <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)', margin: '12px 0 0' }}>
              Hours not worked by season end are billed at {formatCents(current.buyout_cents_per_hour)} per hour.
            </p>
          ) : null}
        </section>
      ) : null}

      {!loading && !loadError && householdId ? (
        <>
          <section style={{ marginBottom: 'var(--space-8)' }}>
            <h2 style={h2}>Your shifts</h2>
            {upcomingMine.length === 0 ? (
              <p style={{ fontSize: 'var(--fs-md)', color: 'var(--text-muted)', margin: 0 }}>You have no upcoming shifts. Pick one below.</p>
            ) : (
              <div className="card" style={{ padding: 0 }}>
                {upcomingMine.map((s, i) => (
                  <div key={s.id} style={{ display: 'flex', gap: '12px', alignItems: 'center', flexWrap: 'wrap', padding: '12px 16px', borderTop: i ? '1px solid var(--border)' : 'none' }}>
                    <div style={{ flex: '1 1 220px', minWidth: 0 }}>
                      <div style={{ fontWeight: 600, fontSize: 'var(--fs-md)' }}>{s.job?.title} · {s.volunteer_name}</div>
                      <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
                        {s.job ? shiftLabel(s.job.starts_at, s.job.ends_at) : ''}{s.job?.location ? ` · ${s.job.location}` : ''} · {formatHours(s.job?.hour_credit ?? 0)}
                      </div>
                    </div>
                    {canSignUp && s.can_cancel ? (
                      <button type="button" className="btn btn-ghost btn-sm" disabled={busy} onClick={() => cancel(s)}>Cancel shift</button>
                    ) : (
                      <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>Too close to cancel online — contact the club</span>
                    )}
                  </div>
                ))}
              </div>
            )}
          </section>

          <section style={{ marginBottom: 'var(--space-8)' }}>
            <h2 style={h2}>Shifts</h2>
            {openDays.length === 0 ? (
              <p style={{ fontSize: 'var(--fs-md)', color: 'var(--text-muted)', margin: 0 }}>No shifts are posted yet. New shifts appear here as meets are scheduled.</p>
            ) : null}
            {openDays.map((day) => (
              <div key={day.key} style={{ marginBottom: '20px' }}>
                <h3 className="card-title" style={{ color: 'var(--text-muted)', margin: '0 0 8px' }}>{dayHeading(day.items[0].starts_at)}</h3>
                <div className="card" style={{ padding: 0 }}>
                  {day.items.map((job, i) => {
                    const mine = mineByJob.has(job.id)
                    const formOpen = signingUp === job.id
                    return (
                      <div key={job.id} style={{ padding: '12px 16px', borderTop: i ? '1px solid var(--border)' : 'none' }}>
                        <div style={{ display: 'flex', gap: '12px', alignItems: 'center', flexWrap: 'wrap' }}>
                          <div style={{ flex: '1 1 220px', minWidth: 0 }}>
                            <div style={{ fontWeight: 600, fontSize: 'var(--fs-md)' }}>{job.title}</div>
                            <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
                              {shiftLabel(job.starts_at, job.ends_at)} · {job.meet_name || job.event_name} · {formatHours(job.hour_credit)}
                              {job.min_age ? ` · ${job.min_age}+` : ''}{job.requires_certification ? ' · Certification required' : ''}
                            </div>
                            {job.description ? <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', marginTop: '4px' }}>{job.description}</div> : null}
                          </div>
                          <SpotPips filled={job.filled_count} capacity={job.capacity} />
                          {canSignUp && job.open_spots > 0 && !formOpen ? (
                            <button type="button" className="btn btn-secondary btn-sm" onClick={() => { setSigningUp(job.id); setCertified(false) }}>
                              {mine ? 'Add another volunteer' : 'Sign up'}
                            </button>
                          ) : null}
                          {!mine && job.open_spots <= 0 ? <span className="badge badge-default">Full</span> : null}
                          {mine && !formOpen ? <span className="badge badge-success">You&apos;re on it</span> : null}
                        </div>
                        {formOpen ? (
                          <form
                            onSubmit={(e) => { e.preventDefault(); void submitSignup(job) }}
                            style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', alignItems: 'center', marginTop: '12px' }}
                          >
                            <input className="form-input" aria-label="Who is volunteering" placeholder="Volunteer's name" value={name}
                              onChange={(e) => setName(e.target.value)} style={{ flex: '1 1 200px', minWidth: 0 }} required />
                            {job.requires_certification ? (
                              <label style={{ display: 'flex', gap: '6px', alignItems: 'center', fontSize: 'var(--fs-base)' }}>
                                <input type="checkbox" checked={certified} onChange={(e) => setCertified(e.target.checked)} />
                                I hold the required certification
                              </label>
                            ) : null}
                            <button type="submit" className="btn btn-primary btn-sm" disabled={busy || !name.trim() || (job.requires_certification && !certified)}>
                              {busy ? 'Signing up…' : 'Confirm'}
                            </button>
                            <button type="button" className="btn btn-ghost btn-sm" onClick={() => setSigningUp(null)}>Cancel</button>
                          </form>
                        ) : null}
                      </div>
                    )
                  })}
                </div>
              </div>
            ))}
          </section>

          {history.length > 0 ? (
            <section>
              <h2 style={h2}>Past shifts</h2>
              <div className="card" style={{ padding: 0 }}>
                {history.map((s, i) => (
                  <div key={s.id} style={{ display: 'flex', gap: '12px', alignItems: 'center', flexWrap: 'wrap', padding: '12px 16px', borderTop: i ? '1px solid var(--border)' : 'none', fontSize: 'var(--fs-base)' }}>
                    <span style={{ flex: '1 1 220px' }}>{s.job?.title} · {s.volunteer_name}{s.job ? ` · ${shiftLabel(s.job.starts_at, s.job.ends_at)}` : ''}</span>
                    {s.status === 'worked'
                      ? <span className="badge badge-success">{formatHours(s.effective_hours)} credited</span>
                      : <span className="badge badge-error">No-show</span>}
                  </div>
                ))}
              </div>
            </section>
          ) : null}
        </>
      ) : null}
      {loading ? <p style={{ fontSize: 'var(--fs-md)', color: 'var(--text-muted)' }}>Loading shifts…</p> : null}
    </div>
  )
}
