'use client'

import { useCallback, useEffect, useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { fetchSwimmerWellness, type SwimmerWellness } from '@/lib/engagementData'
import { reportUIError } from '@/lib/frontend-telemetry'
import { localToday } from '@/lib/wellness-scale'
import { WellnessCheckInForm } from './WellnessCheckInForm'
import { ReadinessTrend } from './ReadinessTrend'

function lastNDays(n: number): string[] {
  const out: string[] = []
  const today = new Date()
  for (let i = n - 1; i >= 0; i--) {
    const d = new Date(today)
    d.setDate(today.getDate() - i)
    out.push(localToday(d))
  }
  return out
}

/** Today's check-in form plus the 30-day readiness trend for one swimmer. */
export function WellnessPanel({ api, orgId, swimmerId, swimmerName, selfView }: {
  api: ApiClient; orgId: string; swimmerId: string; swimmerName?: string; selfView: boolean
}) {
  const [data, setData] = useState<SwimmerWellness | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  const load = useCallback(() => {
    setError(null)
    fetchSwimmerWellness(api, orgId, swimmerId, 30)
      .then(setData)
      .catch((err) => setError(reportUIError('wellness-load', err, 'Check-in history is unavailable right now.', { swimmerId })))
  }, [api, orgId, swimmerId])

  useEffect(() => { if (orgId && swimmerId) load() }, [load, orgId, swimmerId, reloadKey])

  const today = localToday()
  const todays = data?.checkins.find((c) => c.date === today) ?? null
  const days = lastNDays(30)

  return (
    <div className="u-gap-20px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 340px), 1fr))', alignItems: 'start' }}>
      <section className="card u-p-20px" >
        <h2 className="section-title u-mb-4px" >
          {todays ? 'Today’s check-in is in' : selfView ? 'How are you today?' : `How is ${swimmerName ?? 'your swimmer'} today?`}
        </h2>
        <p className="u-fs-fs-base u-mb-20px" style={{ color: 'var(--text-subtle)' }}>
          Takes 20 seconds. Coaches use it to adjust training before small problems become injuries.
        </p>
        {data || error ? (
          <WellnessCheckInForm key={todays?.id ?? 'new'} api={api} orgId={orgId} swimmerId={swimmerId} existing={todays}
            forName={selfView ? undefined : swimmerName} onSaved={() => setReloadKey((k) => k + 1)} />
        ) : <div className="skeleton" style={{ height: 320, borderRadius: 12 }} />}
      </section>
      <section className="card u-p-20px" >
        <h2 className="section-title u-mb-4px" >Last 30 days</h2>
        {error ? (
          <div role="alert" className="u-gap-12px u-fs-fs-base" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', color: 'var(--warning)' }}>
            <span>{error}</span>
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => setReloadKey((k) => k + 1)}>Retry</button>
          </div>
        ) : !data ? (
          <div className="skeleton" style={{ height: 180, borderRadius: 12 }} />
        ) : data.checkins.length === 0 ? (
          <p className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>No check-ins yet. Your first one starts the trend line.</p>
        ) : (
          <>
            <p className="u-fs-fs-base u-mb-12px" style={{ color: 'var(--text-subtle)' }}>
              Average readiness {data.averages.readiness.toFixed(1)} / 5 across {data.averages.count} check-ins · {data.averages.sleep_hours}h sleep on average
            </p>
            <ReadinessTrend checkins={data.checkins} days={days} />
          </>
        )}
      </section>
    </div>
  )
}
