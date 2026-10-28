'use client'

import { useCallback, useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { shouldShowCollectionEmptyState } from '@/lib/collection-load-state'
import { reportUIError } from '@/lib/frontend-telemetry'
import { fetchClubEvents, type ClubEventRecord, type ClubEventRSVP } from '@/lib/engagementData'
import { ClubEventCard, mergeRSVP } from '@/components/engagement/ClubEventCard'
import './page.inline.css'

export default function PortalEventsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [events, setEvents] = useState<ClubEventRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId) return
    setLoading(true)
    setError(null)
    const from = new Date().toISOString().slice(0, 10)
    fetchClubEvents(api, orgId, { from })
      .then(setEvents)
      .catch((err) => { setEvents([]); setError(reportUIError('portal-club-events', err, 'Club events are unavailable right now.')) })
      .finally(() => setLoading(false))
  }, [api, orgId, reloadKey])

  const onRSVP = useCallback((eventId: string, rsvp: ClubEventRSVP) => setEvents((list) => mergeRSVP(list, eventId, rsvp)), [])

  return (
    <div>
      <div className="page-header">
        <div>
          <h1 className="page-title">Club events</h1>
        </div>
      </div>
      {error && (
        <div role="alert" className="card is-9b78534" >
          {error}
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setReloadKey((k) => k + 1)}>Retry</button>
        </div>
      )}
      {loading ? <div className="skeleton" style={{ height: 160, borderRadius: 12 }} /> : (
        <div className="is-8ef476f">
          {events.map((e) => <ClubEventCard key={e.id} event={e} api={api} orgId={orgId} onRSVP={onRSVP} />)}
          {shouldShowCollectionEmptyState(loading, error, events.length) && (
            <div className="card is-4588aff" >No upcoming club events.</div>
          )}
        </div>
      )}
    </div>
  )
}
