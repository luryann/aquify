'use client'

import { useCallback, useEffect, useState } from 'react'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { Notice } from '@/components/ui/Notice'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { fetchPaymentAccount, paymentAccountSummary, startPaymentOnboarding, type PaymentAccountStatus } from '@/lib/tenancyData'

export function PaymentAccountPanel({ canEdit }: { canEdit: boolean }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [status, setStatus] = useState<PaymentAccountStatus | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!orgId) return
    setLoadError(null)
    try {
      setStatus(await fetchPaymentAccount(api, orgId))
    } catch (error) {
      setLoadError(uiErrorMessage(error, 'Couldn’t load your payment account. Try again.'))
    }
  }, [api, orgId])

  useEffect(() => { void load() }, [load])

  async function connect() {
    setBusy(true)
    setActionError(null)
    try {
      const url = await startPaymentOnboarding(api, orgId, `${window.location.origin}/settings?payments=return`)
      window.location.assign(url)
    } catch (error) {
      setActionError(uiErrorMessage(error, 'Couldn’t open Stripe. Try again.'))
      setBusy(false)
    }
  }

  const ready = status?.can_take_payments && status.connected
  const label = !status?.connected ? 'Connect Stripe' : ready ? 'Open Stripe setup' : 'Continue setup in Stripe'

  return (
    <section className="card u-p-20px u-mb-20px" aria-labelledby="payment-account-title">
      <h2 className="card-title" id="payment-account-title">Online payments</h2>
      <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>
        Your club takes card payments through its own Stripe account. Payments and payouts go straight to you.
      </p>
      {loadError ? (
        <div className="u-mt-12px"><LoadErrorBanner message={loadError} onRetry={() => void load()} /></div>
      ) : status ? (
        <div className="u-mt-12px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: '12px', justifyContent: 'space-between' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flex: '1 1 320px' }}>
            <span className={`badge ${ready ? 'badge-success' : status.connected ? 'badge-warning' : 'badge-default'}`}>
              {ready ? 'On' : status.connected ? 'Setup needed' : 'Off'}
            </span>
            <span className="u-fs-fs-sm">{paymentAccountSummary(status)}</span>
          </div>
          {canEdit ? (
            <button type="button" className="btn btn-primary" onClick={() => void connect()} disabled={busy}>{busy ? 'Opening Stripe…' : label}</button>
          ) : (
            <span className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>Only admins can change payment settings.</span>
          )}
        </div>
      ) : (
        <p className="u-mt-12px u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Loading…</p>
      )}
      {actionError ? <Notice tone="err" className="u-mt-12px">{actionError}</Notice> : null}
    </section>
  )
}
