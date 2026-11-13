'use client'

import Link from 'next/link'
import { useCallback, useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { activityLabel, fetchClubActivity, fetchPlatformOrgs } from '@/lib/tenancyData'
import { mergeActivity, type OperatorEvent } from '@/lib/platform-insights'
import { MOCK_EVENTS, MOCK_ORGS, MOCK_PLATFORM_ORG_ID, usePlatformMock } from '@/lib/platform-mock'

const WORKERS = 6

export default function ActivityPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const mock = usePlatformMock()
  const platformOrgId = mock ? MOCK_PLATFORM_ORG_ID : (session?.user.orgId ?? '')
  const [events, setEvents] = useState<OperatorEvent[] | null>(null)
  const [failed, setFailed] = useState(0)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (mock === null) return
    if (mock) {
      setFailed(0)
      setEvents(mergeActivity(MOCK_ORGS.map((org) => ({ org, events: MOCK_EVENTS[org.id] ?? [] }))))
      return
    }
    if (!platformOrgId) return
    setError(null)
    try {
      const orgs = (await fetchPlatformOrgs(api, platformOrgId)).filter((o) => o.status !== 'deleted')
      const perClub: Parameters<typeof mergeActivity>[0] = []
      let misses = 0
      let next = 0
      await Promise.all(Array.from({ length: Math.min(WORKERS, orgs.length) }, async () => {
        while (next < orgs.length) {
          const org = orgs[next++]
          try { perClub.push({ org, events: await fetchClubActivity(api, platformOrgId, org.id) }) } catch { misses += 1 }
        }
      }))
      setFailed(misses)
      setEvents(mergeActivity(perClub))
    } catch {
      setError('Couldn’t load activity. Try again.')
    }
  }, [api, platformOrgId, mock])
  useEffect(() => { void load() }, [load])

  return (
    <>
      <div className="page-header">
        <div>
          <h1 className="page-title">Activity</h1>
          <p className="page-subtitle">Everything operators have changed across all clubs, newest first. Shows the latest 50 changes per club.</p>
        </div>
      </div>

      {error ? <LoadErrorBanner message={error} onRetry={() => void load()} /> : null}
      {failed > 0 ? <LoadErrorBanner message={`${failed} ${failed === 1 ? 'club' : 'clubs'} couldn’t be loaded, so some activity may be missing.`} onRetry={() => void load()} /> : null}
      {events === null && !error ? <p className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Loading activity…</p> : null}

      {events ? (
        <section className="card u-p-0" aria-label="Operator activity">
          {events.length === 0 ? (
            <div className="op-empty"><strong>No operator activity yet.</strong>Every change you make to a club is recorded here.</div>
          ) : (
            <ul className="op-list op-feed">
              {events.map((ev) => (
                <li key={`${ev.orgId}-${ev.id}`}>
                  <div className="op-list-main">
                    <div className="op-list-head">
                      <span>{activityLabel(ev.action)}</span>
                      <Link href={`/platform/clubs/${ev.orgId}`} className="u-fs-fs-sm">{ev.orgName}</Link>
                    </div>
                    {ev.metadata?.target_email || ev.metadata?.reason ? (
                      <p className="op-list-detail u-m-0">
                        {ev.metadata?.target_email ? <>{ev.metadata.target_email}{ev.metadata?.reason ? ' · ' : ''}</> : null}
                        {ev.metadata?.reason ? <>Reason: {ev.metadata.reason}</> : null}
                      </p>
                    ) : null}
                  </div>
                  <time dateTime={ev.occurred_at}>{new Date(ev.occurred_at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })}</time>
                </li>
              ))}
            </ul>
          )}
        </section>
      ) : null}
    </>
  )
}
