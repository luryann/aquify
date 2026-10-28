'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { PublicSitePanel } from './PublicSitePanel'
import { fetchOrgSettings, updateOrgSettings, type OrgSettingsRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

const TEAM_INPUT_STYLE: React.CSSProperties = { width: '100%', padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const TEAM_LABEL_STYLE: React.CSSProperties = { fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }

export default function SettingsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)

  const [teamForm, setTeamForm] = useState<{ team_code: string; team_name: string; lsc_code: string }>({ team_code: '', team_name: '', lsc_code: '' })
  const [teamLoaded, setTeamLoaded] = useState(false)
  const [teamSaving, setTeamSaving] = useState(false)
  const [teamMsg, setTeamMsg] = useState<string | null>(null)
  const [teamErr, setTeamErr] = useState<string | null>(null)

  const [emailForm, setEmailForm] = useState<{ email_sender_name: string; email_sender_address: string }>({ email_sender_name: '', email_sender_address: '' })
  const [emailSaving, setEmailSaving] = useState(false)
  const [emailMsg, setEmailMsg] = useState<string | null>(null)
  const [emailErr, setEmailErr] = useState<string | null>(null)

  const canEditTeam = useMemo(() => {
    const roles = session?.user.roles ?? (session?.user.role ? [session.user.role] : [])
    return roles.includes('admin')
  }, [session])

  useEffect(() => {
    if (!session) return
    let cancelled = false
    fetchOrgSettings(api, session.user.orgId)
      .then((s: OrgSettingsRecord) => {
        if (cancelled) return
        setTeamForm({ team_code: s.team_code ?? '', team_name: s.team_name ?? '', lsc_code: s.lsc_code ?? '' })
        setEmailForm({ email_sender_name: s.email_sender_name ?? '', email_sender_address: s.email_sender_address ?? '' })
        setTeamLoaded(true)
      })
      .catch(() => { if (!cancelled) setTeamLoaded(true) })
    return () => { cancelled = true }
  }, [api, session])

  async function handleSaveTeam(e: React.FormEvent) {
    e.preventDefault()
    if (!session || !canEditTeam) return
    setTeamSaving(true)
    setTeamMsg(null)
    setTeamErr(null)
    try {
      const saved = await updateOrgSettings(api, session.user.orgId, teamForm)
      setTeamForm({ team_code: saved.team_code ?? '', team_name: saved.team_name ?? '', lsc_code: saved.lsc_code ?? '' })
      setTeamMsg('Saved.')
    } catch (err) {
      setTeamErr(uiErrorMessage(err, 'Couldn’t save team identity. Try again.'))
    } finally {
      setTeamSaving(false)
    }
  }

  async function handleSaveEmail(e: React.FormEvent) {
    e.preventDefault()
    if (!session || !canEditTeam) return
    setEmailSaving(true)
    setEmailMsg(null)
    setEmailErr(null)
    try {
      const saved = await updateOrgSettings(api, session.user.orgId, emailForm)
      setEmailForm({ email_sender_name: saved.email_sender_name ?? '', email_sender_address: saved.email_sender_address ?? '' })
      setEmailMsg('Saved.')
    } catch (err) {
      setEmailErr(uiErrorMessage(err, 'Couldn’t save email sender identity. Try again.'))
    } finally {
      setEmailSaving(false)
    }
  }

  return (
    <div className="page">
      <div className="page-heading">
        <h1 className="page-title">
          Settings
        </h1>
      </div>

      <div className="card" style={{ padding: '20px', marginBottom: '20px' }}>
        <div style={{ marginBottom: '4px' }}>
          <h2 className="card-title">Team &amp; export identity</h2>
        </div>
        {teamMsg ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--success-bg)', border: '1px solid var(--success-border)', borderRadius: '8px', color: 'var(--success)', fontSize: 'var(--fs-sm)' }}>{teamMsg}</div> : null}
        {teamErr ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)', fontSize: 'var(--fs-sm)' }}>{teamErr}</div> : null}
        <form className="cols-3" onSubmit={handleSaveTeam} style={{ marginTop: '16px', gap: '16px', alignItems: 'end' }}>
          <div>
            <label style={TEAM_LABEL_STYLE}>Team code</label>
            <input style={TEAM_INPUT_STYLE} value={teamForm.team_code} disabled={!canEditTeam || !teamLoaded} onChange={(e) => setTeamForm((f) => ({ ...f, team_code: e.target.value.toUpperCase() }))} placeholder="DARE" />
          </div>
          <div>
            <label style={TEAM_LABEL_STYLE}>Team name</label>
            <input style={TEAM_INPUT_STYLE} value={teamForm.team_name} disabled={!canEditTeam || !teamLoaded} onChange={(e) => setTeamForm((f) => ({ ...f, team_name: e.target.value }))} placeholder="DARE Aquatics" />
          </div>
          <div>
            <label style={TEAM_LABEL_STYLE}>LSC Code</label>
            <input style={TEAM_INPUT_STYLE} value={teamForm.lsc_code} disabled={!canEditTeam || !teamLoaded} onChange={(e) => setTeamForm((f) => ({ ...f, lsc_code: e.target.value.toUpperCase() }))} placeholder="PC" maxLength={4} />
          </div>
          {canEditTeam ? (
            <div style={{ gridColumn: '1 / -1', display: 'flex', justifyContent: 'flex-end' }}>
              <button type="submit" className="btn btn-primary" disabled={teamSaving || !teamLoaded}>{teamSaving ? 'Saving…' : 'Save identity'}</button>
            </div>
          ) : (
            <p style={{ gridColumn: '1 / -1', fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', margin: 0 }}>Only admins can edit team identity.</p>
          )}
        </form>
      </div>

      <div className="card" style={{ padding: '20px', marginBottom: '20px' }}>
        <div style={{ marginBottom: '4px' }}>
          <h2 className="card-title">Email sender identity</h2>
          <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', marginTop: '4px' }}>
            Default sender for new campaigns.
          </p>
        </div>
        {emailMsg ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--success-bg)', border: '1px solid var(--success-border)', borderRadius: '8px', color: 'var(--success)', fontSize: 'var(--fs-sm)' }}>{emailMsg}</div> : null}
        {emailErr ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)', fontSize: 'var(--fs-sm)' }}>{emailErr}</div> : null}
        <form className="cols-2" onSubmit={handleSaveEmail} style={{ marginTop: '16px', gap: '16px', alignItems: 'end' }}>
          <div>
            <label style={TEAM_LABEL_STYLE}>Sender name</label>
            <input style={TEAM_INPUT_STYLE} value={emailForm.email_sender_name} disabled={!canEditTeam || !teamLoaded} onChange={(e) => setEmailForm((f) => ({ ...f, email_sender_name: e.target.value }))} placeholder="Your club's name" />
          </div>
          <div>
            <label style={TEAM_LABEL_STYLE}>Sender address</label>
            <input style={TEAM_INPUT_STYLE} value={emailForm.email_sender_address} disabled={!canEditTeam || !teamLoaded} type="email" onChange={(e) => setEmailForm((f) => ({ ...f, email_sender_address: e.target.value }))} placeholder="noreply@yourclub.org" />
          </div>
          {canEditTeam ? (
            <div style={{ gridColumn: '1 / -1', display: 'flex', justifyContent: 'flex-end' }}>
              <button type="submit" className="btn btn-primary" disabled={emailSaving || !teamLoaded}>{emailSaving ? 'Saving…' : 'Save sender identity'}</button>
            </div>
          ) : (
            <p style={{ gridColumn: '1 / -1', fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', margin: 0 }}>Only admins can edit email sender identity.</p>
          )}
        </form>
      </div>

      <PublicSitePanel canEdit={canEditTeam} />

      {canEditTeam ? (
        <Link
          href="/settings/system"
          className="card"
          style={{ padding: '16px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px', textDecoration: 'none' }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <Icon name="cpu" size={16} style={{ color: 'var(--text-muted)' }} />
            <div>
              <div style={{ fontSize: 'var(--fs-base)', fontWeight: 600, color: 'var(--text)' }}>System diagnostics</div>
              <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', marginTop: '2px' }}>
                Raw backend configuration for engineering and deployment troubleshooting.
              </div>
            </div>
          </div>
          <Icon name="chevron-right" size={14} style={{ color: 'var(--text-muted)' }} />
        </Link>
      ) : null}
    </div>
  )
}
