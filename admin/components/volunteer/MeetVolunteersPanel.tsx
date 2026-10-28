'use client'

import { useCallback, useEffect, useState } from 'react'
import { useApiClient } from '@/lib/api'
import { useSession } from '@/lib/session'
import { reportUIError } from '@/lib/frontend-telemetry'
import { fetchVolunteerJobs, type VolunteerJobSummary, type VolunteerJobView } from '@/lib/volunteerData'
import { fillPercent } from '@/lib/volunteer-state'
import { LoadErrorBanner } from './VolunteerPrimitives'
import { NewJobsForm } from './NewJobsForm'
import { StaffJobBoard } from './StaffJobBoard'

/** Volunteers tab on the admin meet detail page. */
export function MeetVolunteersPanel({ meetId, meetStartDate }: { meetId: string; meetStartDate?: string }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [jobs, setJobs] = useState<VolunteerJobView[]>([])
  const [summary, setSummary] = useState<VolunteerJobSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [reloadKey, setReloadKey] = useState(0)
  const reload = useCallback(() => setReloadKey((k) => k + 1), [])

  useEffect(() => {
    if (!orgId) return
    let cancelled = false
    setLoading(true)
    fetchVolunteerJobs(api, orgId, { meetId })
      .then((res) => {
        if (cancelled) return
        setJobs(res.jobs)
        setSummary(res.summary)
        setError(null)
      })
      .catch((err) => { if (!cancelled) setError(reportUIError('volunteer.meet_jobs', err, 'Couldn’t load volunteer jobs for this meet. Try again.')) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [api, orgId, meetId, reloadKey])

  return (
    <div>
      <div className="u-gap-12px u-mb-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap' }}>
        <p className="u-m-0 u-fs-fs-md" style={{ color: 'var(--text-muted)' }}>
          {summary && summary.total_capacity > 0
            ? `${summary.filled_spots} of ${summary.total_capacity} spots filled (${fillPercent(summary.filled_spots, summary.total_capacity)}%) across ${summary.job_count} jobs`
            : 'No volunteer jobs posted for this meet yet.'}
        </p>
        <button type="button" className="btn btn-primary btn-sm" onClick={() => setShowForm((v) => !v)}>
          {showForm ? 'Close' : 'Add jobs'}
        </button>
      </div>
      {showForm ? (
        <div className="u-mb-20px" >
          <NewJobsForm api={api} orgId={orgId} meetId={meetId} defaultDate={meetStartDate} onCreated={() => { setShowForm(false); reload() }} />
        </div>
      ) : null}
      {error ? <LoadErrorBanner message={error} onRetry={reload} /> : null}
      {!loading && !error && jobs.length > 0 ? (
        <StaffJobBoard api={api} orgId={orgId} jobs={jobs} onChanged={reload} showEvent={false} />
      ) : null}
    </div>
  )
}
