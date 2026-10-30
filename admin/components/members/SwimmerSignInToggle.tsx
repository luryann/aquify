'use client'

import { useState } from 'react'
import { useApiClient } from '@/lib/api'
import { useSession } from '@/lib/session'
import { setSwimmerPortalAccess } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type Props = {
  swimmerId: string
  /** Current stored flag (true = sign-in blocked). */
  disabled: boolean
  onChange?: (disabled: boolean) => void
}

/** Per-swimmer switch for the swimmer's own sign-in. Records are never affected. */
export function SwimmerSignInToggle({ swimmerId, disabled, onChange }: Props) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const allowed = !disabled

  async function toggle(next: boolean) {
    if (!session) return
    setSaving(true)
    setErr(null)
    try {
      const saved = await setSwimmerPortalAccess(api, session.user.orgId, swimmerId, next)
      onChange?.(saved.portal_access_disabled ?? !next)
    } catch (error) {
      setErr(uiErrorMessage(error, 'Couldn’t update sign-in. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="card u-p-20px">
      <div className="u-gap-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div>
          <h3 className="card-title">Swimmer sign-in</h3>
          <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>
            When off, this swimmer can’t sign in to their own account. Parents and coaches keep full access, and all of the swimmer’s records are kept.
          </p>
        </div>
        <label className="u-gap-8px u-fs-fs-base" style={{ display: 'flex', alignItems: 'center', color: 'var(--text)', flexShrink: 0 }}>
          <input type="checkbox" role="switch" checked={allowed} disabled={saving} onChange={(e) => void toggle(e.target.checked)} />
          Allow swimmer to sign in
        </label>
      </div>
      {err ? <div className="u-mt-12px u-p-8px-12px u-fs-fs-sm" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)' }}>{err}</div> : null}
    </div>
  )
}
