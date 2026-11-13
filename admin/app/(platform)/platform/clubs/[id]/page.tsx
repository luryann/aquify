'use client'

import Link from 'next/link'
import { useParams } from 'next/navigation'
import { useCallback, useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Notice } from '@/components/ui/Notice'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import {
  fetchClubActivity, fetchClubOverview, fetchClubUsers, operatorPaymentSummary, orgStatusLabel,
  type ClubActivityEvent, type ClubOverview, type ClubUser, type OrgStatus,
} from '@/lib/tenancyData'
import { AddAdminForm } from './AddAdminForm'
import { ClubActivity } from './ClubActivity'
import { ClubDetailsForm } from './ClubDetailsForm'
import { ClubPeople } from './ClubPeople'

const STATUS_BADGE: Record<OrgStatus, string> = {
  provisioning: 'badge-warning', active: 'badge-success', suspended: 'badge-error', offboarding: 'badge-warning', deleted: 'badge-default',
}
const TERM: React.CSSProperties = { fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)' }
const ROLE_COUNT_LABEL: Array<[keyof NonNullable<ClubOverview['role_counts']>, string, string]> = [
  ['admin', 'admin', 'admins'], ['coach', 'coach', 'coaches'], ['parent', 'parent', 'parents'], ['swimmer', 'swimmer', 'swimmers'],
]

function peopleSummary(counts: ClubOverview['role_counts']): string {
  const parts = ROLE_COUNT_LABEL.map(([key, one, many]) => {
    const n = counts[key] ?? 0
    return `${n} ${n === 1 ? one : many}`
  })
  return parts.join(', ')
}

/** One club for support: who it is, who is in it, and the recovery actions clubs ask for. */
export default function ClubDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const platformOrgId = session?.user.orgId ?? ''
  const [overview, setOverview] = useState<ClubOverview | null>(null)
  const [users, setUsers] = useState<ClubUser[] | null>(null)
  const [activity, setActivity] = useState<ClubActivityEvent[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [editing, setEditing] = useState(false)

  const load = useCallback(async () => {
    if (!platformOrgId || !id) return
    setLoadError(null)
    try {
      const [o, u, a] = await Promise.all([
        fetchClubOverview(api, platformOrgId, id),
        fetchClubUsers(api, platformOrgId, id),
        fetchClubActivity(api, platformOrgId, id),
      ])
      setOverview(o)
      setUsers(u)
      setActivity(a)
    } catch (error) {
      setLoadError(uiErrorMessage(error, 'Couldn’t load this club. Try again.'))
    }
  }, [api, platformOrgId, id])

  useEffect(() => { void load() }, [load])

  const org = overview?.org

  return (
    <>
      <div className="page-header">
        <div>
          <Link href="/platform" className="btn btn-ghost btn-sm u-mb-8px">Back to clubs</Link>
          <h1 className="page-title">
            {org?.name ?? 'Club'}
            {org ? <span className={`badge ${STATUS_BADGE[org.status]} u-ml-8px`} style={{ marginLeft: '12px', verticalAlign: 'middle' }}>{orgStatusLabel(org.status)}</span> : null}
          </h1>
          {org ? <p className="page-subtitle">Created {new Date(org.created_at).toLocaleDateString()}</p> : null}
        </div>
        {org && !editing ? <button type="button" className="btn btn-secondary" onClick={() => setEditing(true)}>Edit details</button> : null}
      </div>

      {notice ? <Notice tone="ok" className="u-mb-16px">{notice}</Notice> : null}
      {loadError ? <LoadErrorBanner message={loadError} onRetry={() => void load()} /> : null}
      {!overview && !loadError ? <p className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Loading club…</p> : null}

      {org && overview ? (
        <>
          {editing ? (
            <ClubDetailsForm api={api} platformOrgId={platformOrgId} org={org} onSaved={load} onNotice={setNotice} onClose={() => setEditing(false)} />
          ) : null}

          <section className="card u-p-20px u-mb-20px" aria-label="Club facts">
            <dl className="u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', margin: 0 }}>
              <div><dt style={TERM}>Web handle</dt><dd style={{ margin: 0 }}><code>{org.slug ?? '—'}</code></dd></div>
              <div><dt style={TERM}>Plan</dt><dd style={{ margin: 0 }}>{org.plan || 'Not set'}</dd></div>
              <div><dt style={TERM}>People</dt><dd style={{ margin: 0 }}>{peopleSummary(overview.role_counts)}</dd></div>
              <div><dt style={TERM}>Online payments</dt><dd style={{ margin: 0 }}>{operatorPaymentSummary(overview.payments)}</dd></div>
              <div>
                <dt style={TERM}>Web addresses</dt>
                <dd style={{ margin: 0 }}>
                  {overview.domains.length === 0 ? 'None' : overview.domains.map((d) => (
                    <div key={d.domain}><code>{d.domain}</code> <span className={`badge ${d.verified ? 'badge-success' : 'badge-warning'}`}>{d.verified ? 'Verified' : 'Waiting on DNS'}</span></div>
                  ))}
                </dd>
              </div>
            </dl>
          </section>

          <div style={{ display: 'grid', gap: '20px' }}>
            {users ? <ClubPeople api={api} platformOrgId={platformOrgId} orgId={org.id} users={users} onChanged={load} onNotice={setNotice} /> : null}
            <AddAdminForm api={api} platformOrgId={platformOrgId} orgId={org.id} onChanged={load} onNotice={setNotice} />
            <ClubActivity events={activity ?? []} />
          </div>
        </>
      ) : null}
    </>
  )
}
