'use client'

import Link from 'next/link'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { fetchPlatformOrgs, type PlatformOrg } from '@/lib/tenancyData'
import { attentionItems, statusCounts } from '@/lib/platform-insights'
import { MOCK_ORGS, MOCK_PLATFORM_ORG_ID, usePlatformMock } from '@/lib/platform-mock'

export default function AttentionPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const mock = usePlatformMock()
  const platformOrgId = mock ? MOCK_PLATFORM_ORG_ID : (session?.user.orgId ?? '')
  const [orgs, setOrgs] = useState<PlatformOrg[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (mock === null) return
    if (mock) { setOrgs(MOCK_ORGS); return }
    if (!platformOrgId) return
    setError(null)
    try { setOrgs(await fetchPlatformOrgs(api, platformOrgId)) } catch (e) { setError(uiErrorMessage(e, 'Couldn’t load clubs. Try again.')) }
  }, [api, platformOrgId, mock])
  useEffect(() => { void load() }, [load])

  const items = useMemo(() => (orgs ? attentionItems(orgs, platformOrgId) : []), [orgs, platformOrgId])
  const counts = useMemo(() => (orgs ? statusCounts(orgs, platformOrgId) : null), [orgs, platformOrgId])
  const readyToDelete = items.filter((i) => i.kind === 'delete_ready').length

  return (
    <>
      <div className="page-header">
        <div>
          <h1 className="page-title">Needs attention</h1>
          <p className="page-subtitle">Clubs that are stuck, paused, or waiting on you to finish closing.</p>
        </div>
      </div>

      {error ? <LoadErrorBanner message={error} onRetry={() => void load()} /> : null}
      {orgs === null && !error ? <p className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Loading clubs…</p> : null}

      {counts ? (
        <div className="op-tiles">
          <Link href="/platform" className="op-tile"><div className="op-tile-value">{counts.active}</div><div className="op-tile-label">Active clubs</div></Link>
          <Link href="/platform" className="op-tile"><div className="op-tile-value">{counts.suspended}</div><div className="op-tile-label">Paused</div></Link>
          <Link href="/platform" className="op-tile"><div className="op-tile-value">{counts.offboarding}</div><div className="op-tile-label">Closing</div></Link>
          <Link href="/platform" className="op-tile" data-alert={readyToDelete > 0}><div className="op-tile-value">{readyToDelete}</div><div className="op-tile-label">Ready to delete</div></Link>
        </div>
      ) : null}

      {orgs ? (
        <section className="card u-p-0" aria-label="Clubs that need attention">
          {items.length === 0 ? (
            <div className="op-empty"><strong>Nothing needs you right now.</strong>Paused, stuck and closing clubs show up here.</div>
          ) : (
            <ul className="op-list">
              {items.map(({ org, kind, title, detail, tone }) => (
                <li key={`${org.id}-${kind}`}>
                  <div className="op-list-main">
                    <div className="op-list-head">
                      <Link href={`/platform/clubs/${org.id}`}>{org.name}</Link>
                      <span className={`badge badge-${tone}`}>{title}</span>
                    </div>
                    <p className="op-list-detail u-m-0">{detail}</p>
                  </div>
                  <Link href={kind === 'setup_stuck' ? '/platform' : `/platform/clubs/${org.id}`} className="btn btn-ghost">{kind === 'setup_stuck' ? 'Go to Clubs' : 'Open club'}</Link>
                </li>
              ))}
            </ul>
          )}
        </section>
      ) : null}
    </>
  )
}
