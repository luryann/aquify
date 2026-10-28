'use client'

import { Suspense, useMemo, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Tabs } from '@/components/ui/Tabs'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { canAccess } from '@/lib/roles'
import { OverviewPanel } from './OverviewPanel'
import { InvoicesPanel } from './InvoicesPanel'
import { LedgerPanel } from './LedgerPanel'
import { AutopayPanel } from './AutopayPanel'

type TabId = 'overview' | 'invoices' | 'ledger' | 'autopay'

export default function BillingPage() {
  return (
    <Suspense fallback={null}>
      <BillingPageContent />
    </Suspense>
  )
}

function BillingPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { session } = useSession()
  const roles = useMemo(() => session?.user.roles ?? [], [session])

  const availableTabs = useMemo(() => {
    const tabs: { id: TabId; label: string }[] = []
    if (canAccess(roles, ['admin'])) tabs.push({ id: 'overview', label: 'Overview' })
    tabs.push({ id: 'invoices', label: 'Invoices' })
    if (canAccess(roles, ['admin'])) tabs.push({ id: 'ledger', label: 'Ledger' })
    if (canAccess(roles, ['admin'])) tabs.push({ id: 'autopay', label: 'Autopay' })
    return tabs
  }, [roles])

  const requestedTab = (searchParams.get('tab') as TabId | null) ?? 'overview'
  const initialTab = availableTabs.some((t) => t.id === requestedTab) ? requestedTab : (availableTabs[0]?.id ?? 'invoices')
  const [tab, setTab] = useState<TabId>(initialTab)

  function changeTab(id: string) {
    const nextTab = id as TabId
    setTab(nextTab)
    router.replace(nextTab === 'overview' ? '/billing' : `/billing?tab=${nextTab}`)
  }

  return (
    <div className="page">
      <div className="page-heading">
        <h1 className="page-title">Billing</h1>
      </div>

      <Tabs tabs={availableTabs} active={tab} onChange={changeTab} />

      {tab === 'overview' && canAccess(roles, ['admin']) ? (
        <OverviewPanel onNavigateTab={changeTab} />
      ) : tab === 'ledger' && canAccess(roles, ['admin']) ? (
        <LedgerPanel />
      ) : tab === 'autopay' && canAccess(roles, ['admin']) ? (
        <AutopayPanel />
      ) : tab === 'invoices' ? (
        <InvoicesPanel />
      ) : (
        <div className="u-p-20px-0" >
          <EmptyState icon="shield-check" title="Not available" description="This billing view requires admin access." />
        </div>
      )}
    </div>
  )
}
