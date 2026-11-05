'use client'

import { useId, useState } from 'react'
import Link from 'next/link'
import { useApiClient } from '@/lib/api'
import { useSession } from '@/lib/session'
import { setGuardianConsent, type SwimmerRecord } from '@/lib/adminData'
import { guardianConsentState } from '@/lib/guardian-consent'
import { formatDateOnly } from '@/lib/dates'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { PRIVACY_PATH } from '@/lib/legal'

type Props = {
  swimmer: Pick<SwimmerRecord, 'id' | 'first_name' | 'date_of_birth' | 'guardian_consent_at'>
  /** Parents can give and withdraw consent; staff see the status only. */
  mode: 'parent' | 'staff'
  onChange?: (next: SwimmerRecord) => void
}

/** Parental consent for a swimmer under 13 to have their own account (COPPA). Renders nothing for older swimmers. */
export function GuardianConsentCard({ swimmer, mode, onChange }: Props) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [agreed, setAgreed] = useState(false)
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const checkId = useId()
  const state = guardianConsentState(swimmer)

  if (state === 'not-needed') return null

  async function save(granted: boolean) {
    if (!session) return
    setSaving(true)
    setErr(null)
    try {
      const saved = await setGuardianConsent(api, session.user.orgId, swimmer.id, granted)
      setAgreed(false)
      onChange?.(saved)
    } catch (error) {
      setErr(uiErrorMessage(error, granted ? 'Couldn’t save your consent. Try again.' : 'Couldn’t withdraw consent. Try again.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="card u-p-20px" aria-labelledby={`${checkId}-title`}>
      <h3 className="card-title" id={`${checkId}-title`}>Parent consent for {swimmer.first_name}’s own account</h3>

      {state === 'given' ? (
        <>
          <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>
            Consent given{swimmer.guardian_consent_at ? ` on ${formatDateOnly(swimmer.guardian_consent_at)}` : ''}. {swimmer.first_name} can have
            their own sign-in. Withdrawing consent removes that sign-in right away; their records stay with your household.
          </p>
          {mode === 'parent' ? (
            <div className="u-mt-12px">
              <button type="button" className="btn btn-secondary" disabled={saving} onClick={() => void save(false)}>
                {saving ? 'Withdrawing…' : 'Withdraw consent'}
              </button>
            </div>
          ) : null}
        </>
      ) : (
        <>
          <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>
            {swimmer.first_name} is under 13, so they can’t sign in to their own account until a parent or guardian agrees.
            Without consent, you and the coaches keep full access to everything else.
          </p>
          {mode === 'parent' ? (
            <>
              <label className="u-gap-8px u-fs-fs-base u-mt-12px" htmlFor={checkId} style={{ display: 'flex', alignItems: 'flex-start', color: 'var(--text)' }}>
                <input id={checkId} type="checkbox" checked={agreed} disabled={saving} onChange={(e) => setAgreed(e.target.checked)} style={{ marginTop: 3 }} />
                <span>
                  I’m {swimmer.first_name}’s parent or legal guardian. I’ve read the <Link href={PRIVACY_PATH} target="_blank" rel="noopener">privacy policy</Link> and
                  agree that aquify may collect and use {swimmer.first_name}’s information through their own account as it describes.
                  I can withdraw this at any time.
                </span>
              </label>
              <div className="u-mt-12px">
                <button type="button" className="btn btn-primary" disabled={!agreed || saving} onClick={() => void save(true)}>
                  {saving ? 'Saving…' : 'Give consent'}
                </button>
              </div>
            </>
          ) : (
            <p className="u-fs-fs-sm u-mt-8px" style={{ color: 'var(--text-subtle)' }}>Only a parent or guardian can give consent, in the parent portal.</p>
          )}
        </>
      )}

      {err ? <div className="u-mt-12px u-p-8px-12px u-fs-fs-sm" role="alert" style={{ background: 'var(--error-bg)', border: '1px solid var(--error-border)', borderRadius: '8px', color: 'var(--error)' }}>{err}</div> : null}
    </div>
  )
}
