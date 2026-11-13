'use client'

import { useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Notice } from '@/components/ui/Notice'
import { isValidReason, updatePlatformOrg, type PlatformOrg } from '@/lib/tenancyData'
import { HINT, LABEL, ReasonField } from './shared'

type Props = { api: ApiClient; platformOrgId: string; org: PlatformOrg; onSaved: () => Promise<void>; onNotice: (message: string) => void; onClose: () => void }

/** Edit the club's registry name and plan. The handle is fixed: sign-up links and calendar feeds use it. */
export function ClubDetailsForm({ api, platformOrgId, org, onSaved, onNotice, onClose }: Props) {
  const [name, setName] = useState(org.name)
  const [plan, setPlan] = useState(org.plan ?? '')
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const changed = name.trim() !== org.name || plan.trim() !== (org.plan ?? '')
  const ready = name.trim() !== '' && changed && isValidReason(reason)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!ready) return
    setBusy(true)
    setError(null)
    try {
      await updatePlatformOrg(api, platformOrgId, org.id, {
        ...(name.trim() !== org.name ? { name: name.trim() } : {}),
        ...(plan.trim() !== (org.plan ?? '') ? { plan: plan.trim() } : {}),
        reason: reason.trim(),
      })
      onNotice('Club details saved.')
      await onSaved()
      onClose()
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t save the details. Nothing was changed. Try again.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="card u-p-20px u-mb-20px" aria-labelledby="edit-club-title" style={{ display: 'grid', gap: '16px', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', alignItems: 'start' }}>
      <h2 className="card-title" id="edit-club-title" style={{ gridColumn: '1 / -1' }}>Edit club details</h2>
      <div>
        <label style={LABEL} htmlFor="ed-name">Club name</label>
        <input id="ed-name" style={{ width: '100%' }} value={name} maxLength={120} onChange={(e) => setName(e.target.value)} />
        <span style={HINT}>Shown in this console. The club sets its own team name in Settings.</span>
      </div>
      <div>
        <label style={LABEL} htmlFor="ed-plan">Plan</label>
        <input id="ed-plan" style={{ width: '100%' }} value={plan} maxLength={40} onChange={(e) => setPlan(e.target.value)} />
      </div>
      <div>
        <span style={LABEL}>Web handle</span>
        <code>{org.slug ?? '—'}</code>
        <div style={HINT}>The handle can’t be changed. Sign-up links and calendar feeds use it.</div>
      </div>
      <div style={{ gridColumn: '1 / -1' }}><ReasonField id="ed-reason" value={reason} onChange={setReason} /></div>
      <div style={{ gridColumn: '1 / -1', display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
        <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
        <button type="submit" className="btn btn-primary" disabled={!ready || busy}>{busy ? 'Saving…' : 'Save details'}</button>
      </div>
      {error ? <div style={{ gridColumn: '1 / -1' }}><Notice tone="err">{error}</Notice></div> : null}
    </form>
  )
}
