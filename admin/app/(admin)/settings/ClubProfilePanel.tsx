'use client'

import { useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchOrgSettings, updateOrgSettings } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Notice } from '@/components/ui/Notice'

const LABEL: React.CSSProperties = { fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }

type Form = { legal_name: string; timezone: string; currency: string }

export function ClubProfilePanel({ canEdit }: { canEdit: boolean }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [form, setForm] = useState<Form>({ legal_name: '', timezone: '', currency: '' })
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId) return
    let cancelled = false
    fetchOrgSettings(api, orgId)
      .then((s) => {
        if (cancelled) return
        setForm({ legal_name: s.legal_name ?? '', timezone: s.timezone ?? '', currency: (s.currency ?? '').toUpperCase() })
        setLoaded(true)
      })
      .catch(() => { if (!cancelled) setLoaded(true) })
    return () => { cancelled = true }
  }, [api, orgId])

  async function save(e: React.FormEvent) {
    e.preventDefault()
    if (!canEdit) return
    setSaving(true)
    setMsg(null)
    setErr(null)
    try {
      await updateOrgSettings(api, orgId, { legal_name: form.legal_name, timezone: form.timezone, currency: form.currency })
      setMsg('Saved.')
    } catch (error) {
      setErr(uiErrorMessage(error, 'Couldn’t save club details. Check the timezone and currency and try again.'))
    } finally {
      setSaving(false)
    }
  }

  const disabled = !canEdit || !loaded
  return (
    <section className="card u-p-20px u-mb-20px" aria-labelledby="club-profile-title">
      <h2 className="card-title" id="club-profile-title">Club details</h2>
      <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>
        Your legal name appears on invoices. The timezone is used in your families’ calendar feeds. Leave currency empty to use the platform default.
      </p>
      {msg ? <Notice tone="ok" className="u-mt-12px">{msg}</Notice> : null}
      {err ? <Notice tone="err" className="u-mt-12px">{err}</Notice> : null}
      <form onSubmit={save} className="u-mt-16px u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', alignItems: 'end' }}>
        <div>
          <label style={LABEL} htmlFor="club-legal-name">Legal name</label>
          <input id="club-legal-name" style={{ width: '100%' }} maxLength={120} value={form.legal_name} disabled={disabled} onChange={(e) => setForm((f) => ({ ...f, legal_name: e.target.value }))} />
        </div>
        <div>
          <label style={LABEL} htmlFor="club-timezone">Timezone</label>
          <input id="club-timezone" style={{ width: '100%' }} value={form.timezone} disabled={disabled} placeholder="America/Los_Angeles" onChange={(e) => setForm((f) => ({ ...f, timezone: e.target.value }))} />
        </div>
        <div>
          <label style={LABEL} htmlFor="club-currency">Currency</label>
          <input id="club-currency" style={{ width: '100%' }} maxLength={3} value={form.currency} disabled={disabled} placeholder="USD" onChange={(e) => setForm((f) => ({ ...f, currency: e.target.value.toUpperCase() }))} />
        </div>
        {canEdit ? (
          <div style={{ gridColumn: '1 / -1', display: 'flex', justifyContent: 'flex-end' }}>
            <button type="submit" className="btn btn-primary" disabled={saving || !loaded}>{saving ? 'Saving…' : 'Save club details'}</button>
          </div>
        ) : null}
      </form>
    </section>
  )
}
