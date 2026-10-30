'use client'

import Link from 'next/link'
import { useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchUSASwimmingClubs,
  fetchUSASwimmingIntegration,
  syncUSASwimmingTimes,
  testUSASwimmingIntegration,
  updateUSASwimmingIntegration,
  type USASwimmingClub,
  type USASwimmingIntegration,
  type USASwimmingSyncResult,
} from '@/lib/adminData'
import { Notice } from '@/components/ui/Notice'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

const INPUT: React.CSSProperties = { width: '100%' }

export function IntegrationsPanel({ canEdit }: { canEdit: boolean }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [info, setInfo] = useState<USASwimmingIntegration | null>(null)
  const [baseUrl, setBaseUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [enabled, setEnabled] = useState(false)
  const [tokenUrl, setTokenUrl] = useState('')
  const [clientId, setClientId] = useState('')
  const [clubId, setClubId] = useState('')
  const [clubs, setClubs] = useState<USASwimmingClub[] | null>(null)
  const [busy, setBusy] = useState<'save' | 'test' | 'sync' | 'clubs' | null>(null)
  const [msg, setMsg] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)
  const [sync, setSync] = useState<USASwimmingSyncResult | null>(null)

  function apply(i: USASwimmingIntegration) {
    setInfo(i)
    setBaseUrl(i.base_url)
    setEnabled(i.enabled)
    setTokenUrl(i.token_url ?? '')
    setClientId(i.client_id ?? '')
    setClubId(i.club_id ?? '')
    setApiKey('')
  }

  useEffect(() => {
    if (!session || !canEdit) return
    let cancelled = false
    fetchUSASwimmingIntegration(api, session.user.orgId)
      .then((i) => { if (!cancelled) apply(i) })
      .catch((e) => { if (!cancelled) setErr(uiErrorMessage(e, 'Couldn’t load the USA Swimming connection. Try again.')) })
    return () => { cancelled = true }
  }, [api, session, canEdit])

  async function run(kind: 'save' | 'test' | 'sync' | 'clubs', fn: () => Promise<void>, fallback: string) {
    setBusy(kind)
    setMsg(null)
    setErr(null)
    try {
      await fn()
    } catch (e) {
      setErr(uiErrorMessage(e, fallback))
    } finally {
      setBusy(null)
    }
  }

  function save(e: React.FormEvent) {
    e.preventDefault()
    if (!session) return
    void run('save', async () => {
      const body: { base_url: string; enabled: boolean; api_key?: string; token_url: string; client_id: string; club_id: string } = {
        base_url: baseUrl.trim(), enabled, token_url: tokenUrl.trim(), client_id: clientId.trim(), club_id: clubId.trim(),
      }
      if (apiKey.trim()) body.api_key = apiKey.trim()
      apply(await updateUSASwimmingIntegration(api, session.user.orgId, body))
      setMsg('Saved.')
    }, 'Couldn’t save the connection. Try again.')
  }

  function test() {
    if (!session) return
    void run('test', async () => {
      const res = await testUSASwimmingIntegration(api, session.user.orgId)
      apply(await fetchUSASwimmingIntegration(api, session.user.orgId))
      if (res.ok) setMsg(res.message)
      else setErr(`Connection failed: ${res.message}`)
    }, 'Couldn’t run the connection test. Save the base URL and API key first.')
  }

  function runSync() {
    if (!session) return
    void run('sync', async () => {
      const res = await syncUSASwimmingTimes(api, session.user.orgId)
      setSync(res)
      apply(await fetchUSASwimmingIntegration(api, session.user.orgId))
      setMsg(res.summary)
    }, 'Couldn’t sync times. Enable and test the connection first.')
  }

  function loadClubs() {
    if (!session) return
    void run('clubs', async () => { setClubs(await fetchUSASwimmingClubs(api, session.user.orgId)) }, 'Couldn’t load your clubs from USA Swimming. Test the connection first.')
  }

  if (!canEdit) return null
  const vendor = tokenUrl.trim() !== ''
  const failed = sync?.swimmers.filter((s) => s.status !== 'synced') ?? []

  return (
    <div className="card u-p-20px">
      <h2 className="card-title">USA Swimming connection</h2>
      <p className="u-fs-fs-sm u-mt-4px u-m-0" style={{ color: 'var(--text-muted)' }}>
        Connect to USA Swimming’s vendor API to check your SWIMS roster against Aquify. Add a token URL, client ID and client secret from USA Swimming to use it; leave the token URL blank for a plain API key. Times already recorded are never duplicated.
        {info?.stub_available ? ' Development mode: enter “stub” as the base URL to use sample data.' : ''}
      </p>
      {msg ? <Notice tone="ok" className="u-mt-12px">{msg}</Notice> : null}
      {err ? <Notice tone="err" className="u-mt-12px">{err}</Notice> : null}
      <form onSubmit={save} className="u-mt-16px u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', alignItems: 'end' }}>
        <div>
          <label className="form-label" htmlFor="usa-base">API base URL</label>
          <input id="usa-base" style={INPUT} value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder="https://" disabled={!info} />
        </div>
        <div>
          <label className="form-label" htmlFor="usa-token">Token URL</label>
          <input id="usa-token" style={INPUT} value={tokenUrl} onChange={(e) => setTokenUrl(e.target.value)} placeholder="https://" disabled={!info} />
        </div>
        <div>
          <label className="form-label" htmlFor="usa-client">Client ID</label>
          <input id="usa-client" autoComplete="off" style={INPUT} value={clientId} onChange={(e) => setClientId(e.target.value)} disabled={!info || !vendor} />
        </div>
        <div>
          <label className="form-label" htmlFor="usa-club">Club ID</label>
          <input id="usa-club" style={INPUT} value={clubId} onChange={(e) => setClubId(e.target.value)} disabled={!info || !vendor} />
        </div>
        <div>
          <label className="form-label" htmlFor="usa-key">{vendor ? 'Client secret' : 'API key'}</label>
          <input id="usa-key" type="password" autoComplete="off" style={INPUT} value={apiKey} onChange={(e) => setApiKey(e.target.value)}
            placeholder={info?.has_api_key ? `Saved (ends in ${info.api_key_hint ?? '••••'}); leave blank to keep` : vendor ? 'Paste secret' : 'Paste key'} disabled={!info} />
        </div>
        <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', color: 'var(--text)' }}>
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} disabled={!info} /> Enable connection
        </label>
        <div className="u-gap-8px" style={{ display: 'flex', justifyContent: 'flex-end', flexWrap: 'wrap' }}>
          <button type="button" className="btn btn-secondary" onClick={test} disabled={!info?.configured || busy !== null}>{busy === 'test' ? 'Testing…' : 'Test connection'}</button>
          {info?.vendor_api ? (
            <>
              <button type="button" className="btn btn-secondary" onClick={loadClubs} disabled={!info.enabled || busy !== null}>{busy === 'clubs' ? 'Loading…' : 'Find my clubs'}</button>
              <Link href="/usa-swimming" className="btn btn-secondary" aria-disabled={!info.enabled || !info.club_id}>Open USA Swimming tools</Link>
            </>
          ) : (
            <button type="button" className="btn btn-secondary" onClick={runSync} disabled={!info?.enabled || busy !== null}>{busy === 'sync' ? 'Syncing…' : 'Sync times now'}</button>
          )}
          <button type="submit" className="btn btn-primary" disabled={!info || busy !== null}>{busy === 'save' ? 'Saving…' : 'Save connection'}</button>
        </div>
      </form>
      {info?.last_test_at ? (
        <p className="u-fs-fs-xs u-mt-12px u-m-0" style={{ color: info.last_test_ok ? 'var(--text-muted)' : 'var(--error)' }}>
          Last test {new Date(info.last_test_at).toLocaleString()}: {info.last_test_ok ? 'connected' : info.last_test_message}
        </p>
      ) : null}
      {info?.last_sync_at ? (
        <p className="u-fs-fs-xs u-m-0" style={{ color: 'var(--text-muted)' }}>
          Last sync {new Date(info.last_sync_at).toLocaleString()}: {info.last_sync_message}
        </p>
      ) : null}
      {clubs ? (
        <div className="u-mt-12px u-fs-fs-sm">
          {clubs.length === 0 ? <p className="u-m-0" style={{ color: 'var(--text-muted)' }}>These credentials have no clubs yet. Ask USA Swimming to activate your vendor access for the club.</p> : (
            <ul className="u-m-0" style={{ color: 'var(--text-muted)' }}>
              {clubs.map((c) => (
                <li key={c.club_id}>{c.name}{c.code ? ` (${c.code})` : ''}: <button type="button" className="btn btn-secondary" onClick={() => setClubId(c.club_id)}>Use this club</button></li>
              ))}
            </ul>
          )}
        </div>
      ) : null}
      {failed.length > 0 ? (
        <ul className="u-fs-fs-sm u-mt-12px" style={{ color: 'var(--text-muted)' }}>
          {failed.map((s) => (
            <li key={s.swimmer_id}>{s.name}: {s.status === 'not_found' ? 'USA Swimming ID not found' : (s.error ?? 'error')}</li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
