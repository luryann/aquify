'use client'

import { useCallback, useEffect, useState } from 'react'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { healthChecks, type HealthCheck } from '@/lib/platform-insights'
import { MOCK_CHECKS, usePlatformMock } from '@/lib/platform-mock'

type Report = { status: string; checks: Record<string, string>; timestamp?: string }

export default function SystemPage() {
  const mock = usePlatformMock()
  const [rows, setRows] = useState<HealthCheck[] | null>(null)
  const [checkedAt, setCheckedAt] = useState<Date | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    if (mock === null) return
    setBusy(true)
    setError(null)
    try {
      if (mock) { setRows(healthChecks(MOCK_CHECKS)); setCheckedAt(new Date()); return }
      // The readiness probe is public and answers 503 with the same body when a dependency is down.
      const res = await fetch('/api/v1/health/ready', { cache: 'no-store' })
      const body = (await res.json()) as Report
      setRows(healthChecks(body.checks ?? {}))
      setCheckedAt(new Date())
    } catch {
      setError('Couldn’t reach the API. If it’s down, nothing else on this console will load either.')
    } finally {
      setBusy(false)
    }
  }, [mock])
  useEffect(() => { void load() }, [load])

  const down = rows?.filter((r) => r.state === 'down').length ?? 0

  return (
    <>
      <div className="page-header">
        <div>
          <h1 className="page-title">System status</h1>
          <p className="page-subtitle">Whether the services every club depends on are working right now.</p>
        </div>
        <button type="button" className="btn btn-secondary" onClick={() => void load()} disabled={busy}>{busy ? 'Checking…' : 'Check again'}</button>
      </div>

      {error ? <LoadErrorBanner message={error} onRetry={() => void load()} /> : null}
      {rows && !error ? (
        <>
          <p className={`badge ${down > 0 ? 'badge-error' : 'badge-success'} u-mb-16px`} role="status">
            {down > 0 ? `${down} ${down === 1 ? 'service is' : 'services are'} failing` : 'All services working'}
          </p>
          <section className="card u-p-0" aria-label="Service checks">
            <ul className="op-list">
              {rows.map((r) => (
                <li key={r.name}>
                  <div className="op-list-head"><span className="op-health-dot" data-state={r.state} aria-hidden="true" />{r.name}</div>
                  <span className="op-health-state">{r.detail}</span>
                </li>
              ))}
            </ul>
          </section>
          {checkedAt ? <p className="u-fs-fs-sm u-mt-12px" style={{ color: 'var(--text-muted)' }}>Checked {checkedAt.toLocaleTimeString()}</p> : null}
        </>
      ) : null}
    </>
  )
}
