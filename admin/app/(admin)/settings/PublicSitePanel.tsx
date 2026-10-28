'use client'

import { useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchOrgSettings, updateOrgSettings } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

const INPUT: React.CSSProperties = { width: '100%', padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const LABEL: React.CSSProperties = { fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }

type Form = {
  public_slug: string
  public_site_enabled: boolean
  public_show_coaches: boolean
  public_about: string
  public_contact_email: string
  public_contact_phone: string
}

const EMPTY: Form = { public_slug: '', public_site_enabled: false, public_show_coaches: false, public_about: '', public_contact_email: '', public_contact_phone: '' }

export function PublicSitePanel({ canEdit }: { canEdit: boolean }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [form, setForm] = useState<Form>(EMPTY)
  const [loaded, setLoaded] = useState(false)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    if (!session) return
    let cancelled = false
    fetchOrgSettings(api, session.user.orgId)
      .then((s) => {
        if (cancelled) return
        setForm({
          public_slug: s.public_slug ?? '',
          public_site_enabled: s.public_site_enabled ?? false,
          public_show_coaches: s.public_show_coaches ?? false,
          public_about: s.public_about ?? '',
          public_contact_email: s.public_contact_email ?? '',
          public_contact_phone: s.public_contact_phone ?? '',
        })
        setLoaded(true)
      })
      .catch(() => { if (!cancelled) setLoaded(true) })
    return () => { cancelled = true }
  }, [api, session])

  async function save(e: React.FormEvent) {
    e.preventDefault()
    if (!session || !canEdit) return
    setSaving(true)
    setMsg(null)
    setErr(null)
    try {
      const saved = await updateOrgSettings(api, session.user.orgId, form)
      setForm((f) => ({ ...f, public_slug: saved.public_slug ?? f.public_slug }))
      setMsg('Saved.')
    } catch (error) {
      setErr(uiErrorMessage(error, 'Couldn’t save website settings. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  const disabled = !canEdit || !loaded
  const url = form.public_slug ? `/club/${form.public_slug}` : null

  return (
    <div className="card" style={{ padding: '20px', marginBottom: '20px' }}>
      <h2 className="card-title">Public team website</h2>
      <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', marginTop: '4px' }}>
        Shows only items marked public, upcoming meets, and team records.
      </p>
      {msg ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--success-bg)', border: '1px solid var(--success-border)', borderRadius: '8px', color: 'var(--success)', fontSize: 'var(--fs-sm)' }}>{msg}</div> : null}
      {err ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)', fontSize: 'var(--fs-sm)' }}>{err}</div> : null}
      <form onSubmit={save} style={{ marginTop: '16px', display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: '16px', alignItems: 'end' }}>
        <div>
          <label style={LABEL} htmlFor="ps-slug">Web address name</label>
          <input id="ps-slug" style={INPUT} value={form.public_slug} disabled={disabled} maxLength={40} onChange={(e) => setForm((f) => ({ ...f, public_slug: e.target.value.toLowerCase() }))} placeholder="dare-aquatics" />
          <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginTop: '4px' }}>
            {url ? <>Your site lives at <a href={url} target="_blank" rel="noreferrer" style={{ color: 'var(--accent)' }}>{url}</a></> : 'Lowercase letters, digits and hyphens.'}
          </div>
        </div>
        <div>
          <label style={LABEL} htmlFor="ps-email">Contact email</label>
          <input id="ps-email" type="email" style={INPUT} value={form.public_contact_email} disabled={disabled} onChange={(e) => setForm((f) => ({ ...f, public_contact_email: e.target.value }))} placeholder="coach@yourclub.org" />
        </div>
        <div>
          <label style={LABEL} htmlFor="ps-phone">Contact phone</label>
          <input id="ps-phone" style={INPUT} value={form.public_contact_phone} disabled={disabled} onChange={(e) => setForm((f) => ({ ...f, public_contact_phone: e.target.value }))} />
        </div>
        <div style={{ gridColumn: '1 / -1' }}>
          <label style={LABEL} htmlFor="ps-about">About the club</label>
          <textarea id="ps-about" rows={3} style={{ ...INPUT, resize: 'vertical' }} value={form.public_about} disabled={disabled} maxLength={1000} onChange={(e) => setForm((f) => ({ ...f, public_about: e.target.value }))} />
        </div>
        <label style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 'var(--fs-base)', color: 'var(--text)' }}>
          <input type="checkbox" checked={form.public_site_enabled} disabled={disabled} onChange={(e) => setForm((f) => ({ ...f, public_site_enabled: e.target.checked }))} /> Publish the website
        </label>
        <label style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 'var(--fs-base)', color: 'var(--text)' }}>
          <input type="checkbox" checked={form.public_show_coaches} disabled={disabled} onChange={(e) => setForm((f) => ({ ...f, public_show_coaches: e.target.checked }))} /> List coaches by name
        </label>
        {canEdit ? (
          <div style={{ gridColumn: '1 / -1', display: 'flex', justifyContent: 'flex-end' }}>
            <button type="submit" className="btn btn-primary" disabled={saving || !loaded}>{saving ? 'Saving…' : 'Save website settings'}</button>
          </div>
        ) : (
          <p style={{ gridColumn: '1 / -1', fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', margin: 0 }}>Only admins can edit the public website.</p>
        )}
      </form>
    </div>
  )
}
