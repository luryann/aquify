'use client'

import { useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Notice } from '@/components/ui/Notice'
import { addClubAdmin, isValidReason } from '@/lib/tenancyData'
import { LABEL, ReasonField } from './shared'

type Props = { api: ApiClient; platformOrgId: string; orgId: string; onChanged: () => Promise<void>; onNotice: (message: string) => void }
const EMPTY = { email: '', first_name: '', last_name: '', reason: '' }

/** Gives the club an administrator: the usual fix when the only admin has left. */
export function AddAdminForm({ api, platformOrgId, orgId, onChanged, onNotice }: Props) {
  const [form, setForm] = useState(EMPTY)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const set = (key: keyof typeof EMPTY) => (e: React.ChangeEvent<HTMLInputElement>) => setForm((f) => ({ ...f, [key]: e.target.value }))
  const ready = form.email.includes('@') && form.first_name.trim() !== '' && form.last_name.trim() !== '' && isValidReason(form.reason)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!ready) return
    setBusy(true)
    setError(null)
    try {
      const res = await addClubAdmin(api, platformOrgId, orgId, { ...form, email: form.email.trim(), reason: form.reason.trim() })
      onNotice(res.promoted
        ? `${form.email.trim()} is now an administrator.`
        : `${form.email.trim()} is now an administrator. We emailed them to set a password.`)
      setForm(EMPTY)
      await onChanged()
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t add the administrator. Nothing was changed. Try again.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="card u-p-20px" aria-labelledby="add-admin-title">
      <h2 className="card-title" id="add-admin-title">Add an administrator</h2>
      <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-muted)' }}>
        If the email already belongs to someone at this club, they become an administrator. Otherwise we create their account and email them to set a password.
      </p>
      <form onSubmit={submit} className="u-mt-16px u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', alignItems: 'start' }}>
        <div><label style={LABEL} htmlFor="aa-first">First name</label><input id="aa-first" style={{ width: '100%' }} value={form.first_name} onChange={set('first_name')} /></div>
        <div><label style={LABEL} htmlFor="aa-last">Last name</label><input id="aa-last" style={{ width: '100%' }} value={form.last_name} onChange={set('last_name')} /></div>
        <div><label style={LABEL} htmlFor="aa-email">Email</label><input id="aa-email" type="email" style={{ width: '100%' }} value={form.email} onChange={set('email')} /></div>
        <div style={{ gridColumn: '1 / -1' }}>
          <ReasonField id="aa-reason" value={form.reason} onChange={(reason) => setForm((f) => ({ ...f, reason }))} />
        </div>
        <div style={{ gridColumn: '1 / -1', display: 'flex', justifyContent: 'flex-end' }}>
          <button type="submit" className="btn btn-primary" disabled={!ready || busy}>{busy ? 'Adding…' : 'Add administrator'}</button>
        </div>
      </form>
      {error ? <Notice tone="err" className="u-mt-12px">{error}</Notice> : null}
    </section>
  )
}
