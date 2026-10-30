'use client'

import { useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchOrgSettings, setSwimmerAccountsEnabled } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

export function SwimmerSignInPanel({ canEdit }: { canEdit: boolean }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [enabled, setEnabled] = useState(true)
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
        setEnabled(s.swimmer_accounts_enabled ?? true)
        setLoaded(true)
      })
      .catch(() => { if (!cancelled) setLoaded(true) })
    return () => { cancelled = true }
  }, [api, session])

  async function toggle(next: boolean) {
    if (!session || !canEdit) return
    setSaving(true)
    setMsg(null)
    setErr(null)
    try {
      const saved = await setSwimmerAccountsEnabled(api, session.user.orgId, next)
      setEnabled(saved.swimmer_accounts_enabled ?? next)
      setMsg(next ? 'Swimmer sign-in is on.' : 'Swimmer sign-in is off.')
    } catch (error) {
      setErr(uiErrorMessage(error, 'Couldn’t update swimmer sign-in. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="card u-p-20px u-mb-20px">
      <div className="u-gap-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div>
          <h2 className="card-title">Swimmer sign-in</h2>
          <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>
            When off, swimmers can’t sign in to their own accounts. Parents, coaches and all records stay as they are. You can also turn sign-in off for one swimmer on their profile.
          </p>
        </div>
        <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', color: 'var(--text)', flexShrink: 0 }}>
          <input
            type="checkbox"
            role="switch"
            checked={enabled}
            disabled={!canEdit || !loaded || saving}
            onChange={(e) => void toggle(e.target.checked)}
          />
          Allow swimmers to sign in
        </label>
      </div>
      {msg ? <div className="u-mt-12px u-p-8px-12px u-fs-fs-sm" style={{ background: 'var(--success-bg)', border: '1px solid var(--success-border)', borderRadius: '8px', color: 'var(--success)' }}>{msg}</div> : null}
      {err ? <div className="u-mt-12px u-p-8px-12px u-fs-fs-sm" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)' }}>{err}</div> : null}
      {!canEdit ? <p className="u-fs-fs-xs u-mt-12px u-m-0" style={{ color: 'var(--text-muted)' }}>Only admins can change swimmer sign-in.</p> : null}
    </div>
  )
}
