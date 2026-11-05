'use client'

import { useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import {
  addOrgDomain, cancelOffboarding, coolingOffDaysLeft, exportOrgData, orgStatusLabel, purgeOrg, setOrgSuspended,
  startOffboarding, verifyOrgDomain, type DomainChallenge, type OrgStatus, type PlatformOrg,
} from '@/lib/tenancyData'

const LABEL: React.CSSProperties = { fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }
const STATUS_BADGE: Record<OrgStatus, string> = {
  provisioning: 'badge-warning', active: 'badge-success', suspended: 'badge-error', offboarding: 'badge-warning', deleted: 'badge-default',
}

type Props = {
  org: PlatformOrg
  isPlatform: boolean
  api: ApiClient
  platformOrgId: string
  onChanged: () => Promise<void>
  onNotice: (message: string) => void
  onError: (message: string | null) => void
}

type Panel = 'domain' | 'close' | null

/** One club in the operator list, with its own pause/close/domain panels. */
export function ClubRow({ org, isPlatform, api, platformOrgId, onChanged, onNotice, onError }: Props) {
  const [panel, setPanel] = useState<Panel>(null)
  const [confirmingPause, setConfirmingPause] = useState(false)
  const [domain, setDomain] = useState('')
  const [challenge, setChallenge] = useState<DomainChallenge | null>(null)
  const [confirmSlug, setConfirmSlug] = useState('')
  const [exportUrl, setExportUrl] = useState<string | null>(null)

  // Runs one operator action: clears the previous error, reports failures, refreshes the list on success.
  async function run(failure: string, action: () => Promise<void>, refresh = true) {
    onError(null)
    try {
      await action()
      if (refresh) await onChanged()
    } catch (error) {
      onError(uiErrorMessage(error, failure))
    }
  }

  const togglePanel = (next: Exclude<Panel, null>) => {
    setPanel(panel === next ? null : next)
    setChallenge(null)
    setExportUrl(null)
    setConfirmSlug('')
  }

  const canToggle = !isPlatform && (org.status === 'active' || org.status === 'suspended')
  const daysLeft = coolingOffDaysLeft(org.offboarding_at)

  return (
    <tr>
      <td>{org.name}{isPlatform ? <span className="badge badge-default u-ml-8px">Platform</span> : null}</td>
      <td><code>{org.slug ?? '—'}</code></td>
      <td><span className={`badge ${STATUS_BADGE[org.status]}`}>{orgStatusLabel(org.status)}</span></td>
      <td>{new Date(org.created_at).toLocaleDateString()}</td>
      <td align="right">
        <div style={{ display: 'inline-flex', gap: '8px', flexWrap: 'wrap', justifyContent: 'flex-end' }}>
          {!isPlatform && org.status === 'active' ? <button type="button" className="btn btn-ghost" onClick={() => togglePanel('domain')}>Web address</button> : null}
          {!isPlatform && org.status !== 'provisioning' && org.status !== 'deleted' ? <button type="button" className="btn btn-ghost" onClick={() => togglePanel('close')}>Close club</button> : null}
          {canToggle && !confirmingPause ? (
            <button type="button" className={org.status === 'active' ? 'btn btn-ghost' : 'btn btn-secondary'}
              onClick={() => org.status === 'active'
                ? setConfirmingPause(true)
                : void run(`Couldn’t resume ${org.name}. Try again.`, () => setOrgSuspended(api, platformOrgId, org.id, false).then(() => undefined))}>
              {org.status === 'active' ? 'Pause club' : 'Resume club'}
            </button>
          ) : null}
          {confirmingPause ? (
            <>
              <span className="u-fs-fs-sm" style={{ alignSelf: 'center' }}>Everyone at {org.name} loses access until you resume.</span>
              <button type="button" className="btn btn-danger"
                onClick={() => void run(`Couldn’t pause ${org.name}. Try again.`, async () => { await setOrgSuspended(api, platformOrgId, org.id, true); setConfirmingPause(false) })}>
                Pause {org.name}
              </button>
              <button type="button" className="btn btn-ghost" onClick={() => setConfirmingPause(false)}>Keep active</button>
            </>
          ) : null}
        </div>

        {panel === 'close' ? (
          <div className="u-mt-12px u-fs-fs-sm" style={{ textAlign: 'left', maxWidth: '520px', marginLeft: 'auto' }}>
            {org.status !== 'offboarding' ? (
              <>
                <p className="u-m-0">Closing {org.name} stops everyone’s access now. You can still reverse it for 14 days, then its data can be deleted for good.</p>
                <button type="button" className="btn btn-danger u-mt-8px" onClick={() => void run('That step didn’t finish. Try again.', () => startOffboarding(api, platformOrgId, org.id).then(() => undefined))}>
                  Start closing {org.name}
                </button>
              </>
            ) : (
              <>
                <p className="u-m-0">{daysLeft > 0 ? `${daysLeft} days left before the data can be deleted.` : 'The waiting period is over. Download the export first, then delete.'}</p>
                <div className="u-mt-8px" style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                  <button type="button" className="btn btn-secondary" onClick={() => void run('Couldn’t create the export. Try again.', async () => setExportUrl(await exportOrgData(api, platformOrgId, org.id)), false)}>Create data export</button>
                  <button type="button" className="btn btn-ghost" onClick={() => void run('That step didn’t finish. Try again.', async () => { await cancelOffboarding(api, platformOrgId, org.id); setPanel(null) })}>Keep the club</button>
                </div>
                {exportUrl ? <p className="u-mt-8px u-mb-0"><a href={exportUrl} rel="noopener noreferrer">Download the export</a> (link works for 24 hours).</p> : null}
                {daysLeft === 0 ? (
                  <div className="u-mt-12px">
                    <label style={LABEL} htmlFor={`purge-${org.id}`}>Type the handle <code>{org.slug}</code> to delete everything</label>
                    <div style={{ display: 'flex', gap: '8px' }}>
                      <input id={`purge-${org.id}`} style={{ flex: 1 }} value={confirmSlug} onChange={(e) => setConfirmSlug(e.target.value)} autoComplete="off" />
                      <button type="button" className="btn btn-danger" disabled={confirmSlug.trim() !== org.slug}
                        onClick={() => void run('Nothing was deleted. Try again.', async () => {
                          const res = await purgeOrg(api, platformOrgId, org.id, confirmSlug.trim())
                          onNotice(`${org.name} was deleted: ${res.users_deleted} sign-ins and ${res.files_deleted} files removed. Its audit history is kept.`)
                          setPanel(null)
                        })}>
                        Delete permanently
                      </button>
                    </div>
                  </div>
                ) : null}
              </>
            )}
          </div>
        ) : null}

        {panel === 'domain' ? (
          <div className="u-mt-12px" style={{ textAlign: 'left', maxWidth: '520px', marginLeft: 'auto' }}>
            {!challenge ? (
              <div style={{ display: 'flex', gap: '8px', alignItems: 'end' }}>
                <div style={{ flex: 1 }}>
                  <label style={LABEL} htmlFor={`domain-${org.id}`}>Domain</label>
                  <input id={`domain-${org.id}`} style={{ width: '100%' }} value={domain} placeholder="swim.yourclub.org" onChange={(e) => setDomain(e.target.value)} />
                </div>
                <button type="button" className="btn btn-primary" disabled={domain.trim() === ''}
                  onClick={() => void run('Couldn’t add that domain.', async () => setChallenge(await addOrgDomain(api, platformOrgId, org.id, domain.trim())), false)}>
                  Add domain
                </button>
              </div>
            ) : (
              <div className="u-fs-fs-sm">
                <p className="u-m-0">Add this DNS record at your domain’s provider, then check it.</p>
                <p className="u-mt-8px u-mb-0"><strong>Type</strong> {challenge.challenge.type}<br /><strong>Name</strong> <code>{challenge.challenge.name}</code><br /><strong>Value</strong> <code>{challenge.challenge.value}</code></p>
                <button type="button" className="btn btn-primary u-mt-8px"
                  onClick={() => void run('The DNS record isn’t visible yet. DNS changes can take a few minutes. Try again shortly.', async () => {
                    await verifyOrgDomain(api, platformOrgId, org.id, challenge.domain)
                    onNotice(`${challenge.domain} is verified.`)
                    setChallenge(null)
                    setDomain('')
                    setPanel(null)
                  }, false)}>
                  Check DNS record
                </button>
              </div>
            )}
          </div>
        ) : null}
      </td>
    </tr>
  )
}
