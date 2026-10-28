'use client'

import { useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchSwimmers, type SwimmerRecord } from '@/lib/adminData'
import { reportUIError } from '@/lib/frontend-telemetry'
import { WellnessPanel } from '@/components/engagement/WellnessPanel'
import './page.inline.css'

export default function PortalWellnessPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [swimmers, setSwimmers] = useState<SwimmerRecord[] | null>(null)
  const [selected, setSelected] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    if (!orgId) return
    setError(null)
    fetchSwimmers(api, orgId)
      .then((list) => { setSwimmers(list); setSelected((cur) => cur || list[0]?.id || '') })
      .catch((err) => setError(reportUIError('portal-wellness-swimmers', err, 'Your swimmers could not be loaded.')))
  }, [api, orgId, reloadKey])

  const current = swimmers?.find((s) => s.id === selected)
  const name = (s: SwimmerRecord) => s.preferred_name || s.first_name

  return (
    <div>
      <div className="page-header">
        <div>
          <h1 className="page-title">Wellness check-in</h1>
          <p className="page-subtitle">For swimmers without their own login.</p>
        </div>
      </div>
      {error && (
        <div role="alert" className="card is-9b78534" >
          {error}
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setReloadKey((k) => k + 1)}>Retry</button>
        </div>
      )}
      {swimmers && swimmers.length === 0 && !error && (
        <div className="card is-ef1427e" >No swimmers are linked to your household yet.</div>
      )}
      {swimmers && swimmers.length > 1 && (
        <div role="tablist" aria-label="Swimmer" className="is-00afff9">
          {swimmers.map((s) => (
            <button key={s.id} role="tab" aria-selected={s.id === selected} type="button" onClick={() => setSelected(s.id)}
              className={s.id === selected ? 'btn btn-primary btn-sm' : 'btn btn-secondary btn-sm'}>{name(s)}</button>
          ))}
        </div>
      )}
      {current && <WellnessPanel key={current.id} api={api} orgId={orgId} swimmerId={current.id} swimmerName={name(current)} selfView={false} />}
    </div>
  )
}
