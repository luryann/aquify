'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { fetchOrgSettings, updateOrgSettings, type OrgSettingsRecord } from '@/lib/adminData'

const TEAM_INPUT_STYLE: React.CSSProperties = { width: '100%', padding: '9px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none', boxSizing: 'border-box' }
const TEAM_LABEL_STYLE: React.CSSProperties = { fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '5px' }

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
    if (!session) return
    setTeamSaving(true)
    setTeamMsg(null)
    setTeamErr(null)
    try {
      const saved = await updateOrgSettings(api, session.user.orgId, teamForm)
      setTeamForm({ team_code: saved.team_code ?? '', team_name: saved.team_name ?? '', lsc_code: saved.lsc_code ?? '' })
      setTeamMsg('Saved.')
    } catch (err) {
      setTeamErr(err instanceof ApiError ? err.message : 'Failed to save team identity.')
    } finally {
      setTeamSaving(false)
    }
  }

  async function handleSaveEmail(e: React.FormEvent) {
    e.preventDefault()
    if (!session) return
    setEmailSaving(true)
    setEmailMsg(null)
    setEmailErr(null)
    try {
      const saved = await updateOrgSettings(api, session.user.orgId, emailForm)
      setEmailForm({ email_sender_name: saved.email_sender_name ?? '', email_sender_address: saved.email_sender_address ?? '' })
      setEmailMsg('Saved.')
    } catch (err) {
      setEmailErr(err instanceof ApiError ? err.message : 'Failed to save email sender identity.')
    } finally {
      setEmailSaving(false)
    }
  }

  return (
    <div style={{ padding: '32px 32px 80px', maxWidth: '1000px' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Settings
        </p>
        <h1 className="page-title">
          Settings
        </h1>
        <p className="page-subtitle">
          Your club&rsquo;s identity, used across meet reports and exports.
        </p>
      </div>

      <div className="card" style={{ padding: '20px', marginBottom: '20px' }}>
        <div style={{ marginBottom: '4px' }}>
          <h2 style={{ fontSize: '15px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)' }}>Team &amp; Export Identity</h2>
          <p style={{ fontSize: '12px', color: 'var(--text-subtle)', marginTop: '4px' }}>
            Your club’s team code, name, and LSC — printed on generated meet reports.
          </p>
        </div>
        {teamMsg ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--success-bg)', border: '1px solid var(--success-border)', borderRadius: '8px', color: 'var(--success)', fontSize: '12px' }}>{teamMsg}</div> : null}
        {teamErr ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)', fontSize: '12px' }}>{teamErr}</div> : null}
        <form onSubmit={handleSaveTeam} style={{ marginTop: '16px', display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: '14px', alignItems: 'end' }}>
          <div>
            <label style={TEAM_LABEL_STYLE}>Team Code</label>
            <input style={TEAM_INPUT_STYLE} value={teamForm.team_code} disabled={!canEditTeam || !teamLoaded} onChange={(e) => setTeamForm((f) => ({ ...f, team_code: e.target.value.toUpperCase() }))} placeholder="DARE" />
          </div>
          <div>
            <label style={TEAM_LABEL_STYLE}>Team Name</label>
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
            <p style={{ gridColumn: '1 / -1', fontSize: '11px', color: 'var(--text-muted)', margin: 0 }}>Only admins can edit team identity.</p>
          )}
        </form>
      </div>

      <div className="card" style={{ padding: '20px', marginBottom: '20px' }}>
        <div style={{ marginBottom: '4px' }}>
          <h2 style={{ fontSize: '15px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)' }}>Email Sender Identity</h2>
          <p style={{ fontSize: '12px', color: 'var(--text-subtle)', marginTop: '4px' }}>
            The name and address new email campaigns send from by default — still editable per campaign.
          </p>
        </div>
        {emailMsg ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--success-bg)', border: '1px solid var(--success-border)', borderRadius: '8px', color: 'var(--success)', fontSize: '12px' }}>{emailMsg}</div> : null}
        {emailErr ? <div style={{ marginTop: '12px', padding: '8px 12px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)', fontSize: '12px' }}>{emailErr}</div> : null}
        <form onSubmit={handleSaveEmail} style={{ marginTop: '16px', display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '14px', alignItems: 'end' }}>
          <div>
            <label style={TEAM_LABEL_STYLE}>Sender Name</label>
            <input style={TEAM_INPUT_STYLE} value={emailForm.email_sender_name} disabled={!canEditTeam || !teamLoaded} onChange={(e) => setEmailForm((f) => ({ ...f, email_sender_name: e.target.value }))} placeholder="Your club's name" />
          </div>
          <div>
            <label style={TEAM_LABEL_STYLE}>Sender Address</label>
            <input style={TEAM_INPUT_STYLE} value={emailForm.email_sender_address} disabled={!canEditTeam || !teamLoaded} type="email" onChange={(e) => setEmailForm((f) => ({ ...f, email_sender_address: e.target.value }))} placeholder="noreply@yourclub.org" />
          </div>
          {canEditTeam ? (
            <div style={{ gridColumn: '1 / -1', display: 'flex', justifyContent: 'flex-end' }}>
              <button type="submit" className="btn btn-primary" disabled={emailSaving || !teamLoaded}>{emailSaving ? 'Saving…' : 'Save sender identity'}</button>
            </div>
          ) : (
            <p style={{ gridColumn: '1 / -1', fontSize: '11px', color: 'var(--text-muted)', margin: 0 }}>Only admins can edit email sender identity.</p>
          )}
        </form>
      </div>

      {canEditTeam ? (
        <Link
          href="/settings/system"
          className="card"
          style={{ padding: '16px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px', textDecoration: 'none' }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <Icon name="cpu" size={16} style={{ color: 'var(--text-muted)' }} />
            <div>
              <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>System Diagnostics</div>
              <div style={{ fontSize: '12px', color: 'var(--text-subtle)', marginTop: '2px' }}>
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
