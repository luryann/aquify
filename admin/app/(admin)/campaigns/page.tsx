'use client'

import { Suspense, useEffect, useRef, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Tabs } from '@/components/ui/Tabs'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { canAccess } from '@/lib/roles'
import { CampaignsPanel } from './CampaignsPanel'
import { SuppressionPanel } from './SuppressionPanel'

export default function CampaignsRootPage() {
  return (
    <Suspense fallback={null}>
      <CampaignsRootPageContent />
    </Suspense>
  )
}

function CampaignsRootPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { session } = useSession()
  const canSeeSuppression = canAccess(session?.user.roles ?? [], ['admin'])
  const [tab, setTab] = useState(searchParams.get('tab') === 'suppression' && canSeeSuppression ? 'suppression' : 'campaigns')

  // Session (and therefore canSeeSuppression) resolves asynchronously after
  // first render, so a `?tab=suppression` deep link taken before it resolves
  // must be re-applied once we actually know the caller is an admin.
  const deepLinkAppliedRef = useRef(false)
  useEffect(() => {
    if (deepLinkAppliedRef.current || !session) return
    deepLinkAppliedRef.current = true
    if (searchParams.get('tab') === 'suppression' && canSeeSuppression) {
      setTab('suppression')
    }
  }, [session, canSeeSuppression, searchParams])

  function changeTab(id: string) {
    setTab(id)
    router.replace(id === 'suppression' ? '/campaigns?tab=suppression' : '/campaigns')
  }

  return (
    <div className="page">
      <div className="page-heading">
        <h1 className="page-title">Email campaigns</h1>
      </div>

      <Tabs
        tabs={
          canSeeSuppression
            ? [
                { id: 'campaigns', label: 'Campaigns' },
                { id: 'suppression', label: 'Suppression list' },
              ]
            : [{ id: 'campaigns', label: 'Campaigns' }]
        }
        active={tab}
        onChange={changeTab}
      />

      {tab === 'suppression' && canSeeSuppression ? (
        <SuppressionPanel />
      ) : tab === 'suppression' ? (
        <div style={{ padding: '20px 0' }}>
          <EmptyState icon="shield" title="Not available" description="Suppression list management requires admin access." />
        </div>
      ) : (
        <CampaignsPanel />
      )}
    </div>
  )
}
