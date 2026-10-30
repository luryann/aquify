'use client'

import Link from 'next/link'
import { Suspense, useCallback, useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Tabs } from '@/components/ui/Tabs'
import { EmptyState } from '@/components/ui/EmptyState'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { fetchSwimmers, fetchUSASwimmingIntegration, type SwimmerRecord, type USASwimmingIntegration } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { RosterPanel } from './RosterPanel'
import { VerifyPanel } from './VerifyPanel'
import { RegisterPanel } from './RegisterPanel'

type TabId = 'roster' | 'verify' | 'register'

const TABS: { id: TabId; label: string }[] = [
  { id: 'roster', label: 'Roster check' },
  { id: 'verify', label: 'Verify' },
  { id: 'register', label: 'Register' },
]

export default function USASwimmingPage() {
  return (
    <Suspense fallback={null}>
      <USASwimmingContent />
    </Suspense>
  )
}

function USASwimmingContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [info, setInfo] = useState<USASwimmingIntegration | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])

  const requested = searchParams.get('tab') as TabId | null
  const [tab, setTab] = useState<TabId>(TABS.some((t) => t.id === requested) ? (requested as TabId) : 'roster')

  const load = useCallback(() => {
    if (!session) return
    setLoadError(null)
    Promise.all([fetchUSASwimmingIntegration(api, session.user.orgId), fetchSwimmers(api, session.user.orgId)])
      .then(([integration, rows]) => {
        setInfo(integration)
        setSwimmers(rows)
      })
      .catch((e) => setLoadError(uiErrorMessage(e, 'Couldn’t load the USA Swimming connection.')))
  }, [api, session])

  useEffect(load, [load])

  function changeTab(id: string) {
    setTab(id as TabId)
    router.replace(id === 'roster' ? '/usa-swimming' : `/usa-swimming?tab=${id}`)
  }

  const ready = !!info?.vendor_api && info.enabled && !!info.club_id

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">USA Swimming</h1>
          <p className="page-subtitle">
            {info?.last_test_at && info.last_test_ok ? 'Connected to SWIMS.' : 'Check your roster against SWIMS, verify members and times, and register new members.'}
          </p>
        </div>
        <Link href="/settings" className="btn btn-secondary">Connection settings</Link>
      </div>

      {loadError ? <LoadErrorBanner message={loadError} onRetry={load} /> : null}

      {info && !ready ? (
        <EmptyState
          icon="shield-check"
          title="Finish the SWIMS connection first"
          description={
            !info.vendor_api
              ? 'These tools need the USA Swimming vendor API. Add the token URL, client ID, client secret and club ID in Settings.'
              : !info.enabled
                ? 'The connection is saved but turned off. Enable it in Settings.'
                : 'Add your club ID in Settings. Use “Find my clubs” to look it up.'
          }
        />
      ) : null}

      {ready ? (
        <>
          <Tabs tabs={TABS} active={tab} onChange={changeTab} />
          {tab === 'roster' ? <RosterPanel /> : tab === 'verify'
            ? <VerifyPanel swimmers={swimmers.filter((s) => (s.usa_swimming_id ?? '').trim() !== '')} />
            : <RegisterPanel swimmers={swimmers} />}
        </>
      ) : null}
    </div>
  )
}
