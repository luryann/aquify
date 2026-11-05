'use client'

import { useCallback, useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchOrgSettings } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Notice } from '@/components/ui/Notice'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { createInvite, fetchInvites, inviteState, revokeInvite, type InviteRecord, type InviteRole, type InviteState } from '@/lib/tenancyData'

const LABEL: React.CSSProperties = { fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }

const ROLE_LABEL: Record<InviteRole, string> = { parent: 'Parent', swimmer: 'Swimmer', coach: 'Coach' }
const STATE_UI: Record<InviteState, { label: string; badge: string }> = {
  active: { label: 'Active', badge: 'badge-success' },
  used_up: { label: 'Used up', badge: 'badge-default' },
  expired: { label: 'Expired', badge: 'badge-default' },
  revoked: { label: 'Revoked', badge: 'badge-error' },
}

export function InvitesPanel({ canEdit }: { canEdit: boolean }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [invites, setInvites] = useState<InviteRecord[] | null>(null)
  const [slug, setSlug] = useState('')
  const [loadError, setLoadError] = useState<string | null>(null)
  const [role, setRole] = useState<InviteRole>('parent')
  const [label, setLabel] = useState('')
  const [maxUses, setMaxUses] = useState(1)
  const [ttlDays, setTtlDays] = useState(14)
  const [busy, setBusy] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const [fresh, setFresh] = useState<{ code: string; role: InviteRole } | null>(null)
  const [copied, setCopied] = useState(false)

  const load = useCallback(async () => {
    if (!orgId) return
    setLoadError(null)
    try {
      const [list, settings] = await Promise.all([fetchInvites(api, orgId), fetchOrgSettings(api, orgId)])
      setInvites(list)
      setSlug(settings.public_slug ?? '')
    } catch (error) {
      setLoadError(uiErrorMessage(error, 'Couldn’t load invites. Try again.'))
    }
  }, [api, orgId])

  useEffect(() => { void load() }, [load])

  async function create(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setFormError(null)
    setCopied(false)
    try {
      const res = await createInvite(api, orgId, { role, label: label.trim() || undefined, max_uses: maxUses, ttl_days: ttlDays })
      setFresh({ code: res.code, role: res.invite.role })
      setLabel('')
      await load()
    } catch (error) {
      setFormError(uiErrorMessage(error, 'Couldn’t create the invite. Try again.'))
    } finally {
      setBusy(false)
    }
  }

  async function revoke(id: string) {
    setFormError(null)
    try {
      await revokeInvite(api, orgId, id)
      await load()
    } catch (error) {
      setFormError(uiErrorMessage(error, 'Couldn’t revoke the invite. Try again.'))
    }
  }

  const joinLink = slug && typeof window !== 'undefined' ? `${window.location.origin}/register?club=${encodeURIComponent(slug)}` : ''

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }

  return (
    <section className="card u-p-20px u-mb-20px" aria-labelledby="invites-title">
      <h2 className="card-title" id="invites-title">Sign-up invites</h2>
      <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>
        Give people a code so they join as the role you choose. You still approve each request.
      </p>

      {!slug && invites ? (
        <p className="u-mt-12px u-fs-fs-sm" style={{ color: 'var(--warning)' }}>
          Set a website handle under Public team website first. The sign-up link needs it.
        </p>
      ) : joinLink ? (
        <p className="u-mt-12px u-fs-fs-sm">
          Sign-up link: <code>{joinLink}</code>
        </p>
      ) : null}

      {fresh ? (
        <div role="status" className="u-mt-12px u-p-12px" style={{ border: '1px solid var(--success-border)', background: 'var(--success-bg)', borderRadius: '8px' }}>
          <p className="u-fs-fs-sm u-m-0">New {ROLE_LABEL[fresh.role].toLowerCase()} code. Copy it now, it won’t be shown again.</p>
          <div className="u-mt-8px" style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
            <code className="u-fs-fs-lg" style={{ letterSpacing: '0.06em' }}>{fresh.code}</code>
            <button type="button" className="btn btn-secondary" onClick={() => void copy(fresh.code)}>{copied ? 'Copied' : 'Copy code'}</button>
            <button type="button" className="btn btn-ghost" onClick={() => setFresh(null)}>Done</button>
          </div>
        </div>
      ) : null}

      {canEdit ? (
        <form onSubmit={create} className="u-mt-16px u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', alignItems: 'end' }}>
          <div>
            <label style={LABEL} htmlFor="invite-role">Joins as</label>
            <select id="invite-role" style={{ width: '100%' }} value={role} onChange={(e) => setRole(e.target.value as InviteRole)}>
              {(Object.keys(ROLE_LABEL) as InviteRole[]).map((r) => <option key={r} value={r}>{ROLE_LABEL[r]}</option>)}
            </select>
          </div>
          <div>
            <label style={LABEL} htmlFor="invite-label">Note (optional)</label>
            <input id="invite-label" style={{ width: '100%' }} value={label} maxLength={80} onChange={(e) => setLabel(e.target.value)} placeholder="Spring 10-and-under families" />
          </div>
          <div>
            <label style={LABEL} htmlFor="invite-uses">Uses</label>
            <input id="invite-uses" type="number" min={1} max={1000} style={{ width: '100%' }} value={maxUses} onChange={(e) => setMaxUses(Math.max(1, Math.min(1000, Number(e.target.value) || 1)))} />
          </div>
          <div>
            <label style={LABEL} htmlFor="invite-days">Expires in (days)</label>
            <input id="invite-days" type="number" min={1} max={90} style={{ width: '100%' }} value={ttlDays} onChange={(e) => setTtlDays(Math.max(1, Math.min(90, Number(e.target.value) || 14)))} />
          </div>
          <div>
            <button type="submit" className="btn btn-primary" disabled={busy}>{busy ? 'Creating…' : 'Create invite'}</button>
          </div>
        </form>
      ) : (
        <p className="u-mt-12px u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>Only admins can create invites.</p>
      )}

      {formError ? <Notice tone="err" className="u-mt-12px">{formError}</Notice> : null}
      {loadError ? <div className="u-mt-12px"><LoadErrorBanner message={loadError} onRetry={() => void load()} /></div> : null}

      {invites && invites.length === 0 ? (
        <p className="u-mt-16px u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>No invites yet. Create one to bring a family or coach in.</p>
      ) : null}
      {invites && invites.length > 0 ? (
        <div className="table-scroll u-mt-16px">
          <table style={{ width: '100%' }}>
            <thead>
              <tr><th align="left">Note</th><th align="left">Joins as</th><th align="left">Used</th><th align="left">Expires</th><th align="left">Status</th><th /></tr>
            </thead>
            <tbody>
              {invites.map((i) => {
                const state = inviteState(i)
                return (
                  <tr key={i.id}>
                    <td>{i.label || <span style={{ color: 'var(--text-muted)' }}>No note</span>}</td>
                    <td>{ROLE_LABEL[i.role]}</td>
                    <td>{i.uses} of {i.max_uses}</td>
                    <td>{new Date(i.expires_at).toLocaleDateString()}</td>
                    <td><span className={`badge ${STATE_UI[state].badge}`}>{STATE_UI[state].label}</span></td>
                    <td align="right">
                      {canEdit && state === 'active' ? (
                        <button type="button" className="btn btn-ghost" onClick={() => void revoke(i.id)} aria-label={`Revoke invite ${i.label || i.id}`}>Revoke</button>
                      ) : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  )
}
