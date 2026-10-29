'use client'

import { useCallback, useEffect, useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { fetchClubEvents, type ClubEventRecord, type ClubEventRSVP } from '@/lib/engagementData'
import { reportUIError } from '@/lib/frontend-telemetry'
import { ClubEventCard, mergeRSVP } from './ClubEventCard'
import { localToday } from '@/lib/dates'

/** Upcoming club events on the swimmer schedule; swimmers RSVP for themselves. */
export function SwimmerClubEvents({ api, orgId, swimmerId }: { api: ApiClient; orgId: string; swimmerId: string }) {
  const [events, setEvents] = useState<ClubEventRecord[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId) return
    setError(null)
    const from = localToday()
    const to = localToday(new Date(Date.now() + 60 * 86400000))
    fetchClubEvents(api, orgId, { from, to })
      .then(setEvents)
      .catch((err) => setError(reportUIError('swimmer-club-events', err, 'Club events are unavailable right now.')))
  }, [api, orgId, reloadKey])

  const onRSVP = useCallback((eventId: string, rsvp: ClubEventRSVP) => setEvents((list) => (list ? mergeRSVP(list, eventId, rsvp) : list)), [])

  if (error) {
    return (
      <p role="alert" className="u-mb-24px u-fs-fs-sm" style={{ color: 'var(--warning)' }}>
        {error} <button type="button" className="btn btn-ghost btn-sm" onClick={() => setReloadKey((k) => k + 1)}>Retry</button>
      </p>
    )
  }
  if (!events || events.length === 0) return null
  return (
    <section className="u-mb-space-8" aria-labelledby="club-events-heading">
      <h2 id="club-events-heading" className="u-ff-font-display u-fw-display-weight u-fs-fs-xl u-mb-12px" style={{ color: 'var(--text)' }}>Club events in the next 60 days</h2>
      <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
        {events.map((e) => <ClubEventCard key={e.id} event={e} api={api} orgId={orgId} onRSVP={onRSVP} swimmerOnlyId={swimmerId || '__none__'} />)}
      </div>
    </section>
  )
}
