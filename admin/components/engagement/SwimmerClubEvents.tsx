'use client'

import { useCallback, useEffect, useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { fetchClubEvents, type ClubEventRecord, type ClubEventRSVP } from '@/lib/engagementData'
import { reportUIError } from '@/lib/frontend-telemetry'
import { ClubEventCard, mergeRSVP } from './ClubEventCard'

/** Upcoming club events on the swimmer schedule; swimmers RSVP for themselves. */
export function SwimmerClubEvents({ api, orgId, swimmerId }: { api: ApiClient; orgId: string; swimmerId: string }) {
  const [events, setEvents] = useState<ClubEventRecord[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId) return
    setError(null)
    const from = new Date().toISOString().slice(0, 10)
    const to = new Date(Date.now() + 60 * 86400000).toISOString().slice(0, 10)
    fetchClubEvents(api, orgId, { from, to })
      .then(setEvents)
      .catch((err) => setError(reportUIError('swimmer-club-events', err, 'Club events are unavailable right now.')))
  }, [api, orgId, reloadKey])

  const onRSVP = useCallback((eventId: string, rsvp: ClubEventRSVP) => setEvents((list) => (list ? mergeRSVP(list, eventId, rsvp) : list)), [])

  if (error) {
    return (
      <p role="alert" style={{ marginBottom: 24, fontSize: 'var(--fs-sm)', color: 'var(--warning)' }}>
        {error} <button type="button" className="btn btn-ghost btn-sm" onClick={() => setReloadKey((k) => k + 1)}>Retry</button>
      </p>
    )
  }
  if (!events || events.length === 0) return null
  return (
    <section style={{ marginBottom: 'var(--space-8)' }} aria-labelledby="club-events-heading">
      <h2 id="club-events-heading" style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-xl)', color: 'var(--text)', marginBottom: 12 }}>Club events in the next 60 days</h2>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        {events.map((e) => <ClubEventCard key={e.id} event={e} api={api} orgId={orgId} onRSVP={onRSVP} swimmerOnlyId={swimmerId || '__none__'} />)}
      </div>
    </section>
  )
}
