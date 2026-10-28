'use client'

import { Suspense, useMemo, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Tabs } from '@/components/ui/Tabs'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { canAccess } from '@/lib/roles'
import { HouseholdsPanel } from './HouseholdsPanel'
import { SwimmersPanel } from './SwimmersPanel'
import { GroupsPanel } from './GroupsPanel'
import { RegistrationsPanel } from './RegistrationsPanel'
import { DocumentsPanel } from './DocumentsPanel'

type TabId = 'households' | 'swimmers' | 'groups' | 'registrations' | 'documents'

export default function MembersPage() {
  return (
    <Suspense fallback={null}>
      <MembersPageContent />
    </Suspense>
  )
}

function MembersPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { session } = useSession()
  const roles = useMemo(() => session?.user.roles ?? [], [session])
  const canSeeDocuments = canAccess(roles, ['admin'])

  const tabs = useMemo(() => {
    const list: { id: TabId; label: string }[] = [
      { id: 'households', label: 'Households' },
      { id: 'swimmers', label: 'Swimmers' },
      { id: 'groups', label: 'Groups' },
      { id: 'registrations', label: 'Registrations' },
    ]
    if (canSeeDocuments) list.push({ id: 'documents', label: 'Documents' })
    return list
  }, [canSeeDocuments])

  const requestedTab = (searchParams.get('tab') as TabId | null) ?? 'households'
  const initialTab = tabs.some((t) => t.id === requestedTab) ? requestedTab : 'households'
  const [tab, setTab] = useState<TabId>(initialTab)

  function changeTab(id: string) {
    const nextTab = id as TabId
    setTab(nextTab)
    router.replace(nextTab === 'households' ? '/members' : `/members?tab=${nextTab}`)
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Members</h1>
        </div>
      </div>

      <Tabs tabs={tabs} active={tab} onChange={changeTab} />

      {tab === 'households' ? (
        <HouseholdsPanel />
      ) : tab === 'swimmers' ? (
        <SwimmersPanel />
      ) : tab === 'groups' ? (
        <GroupsPanel />
      ) : tab === 'registrations' ? (
        <RegistrationsPanel />
      ) : tab === 'documents' && canSeeDocuments ? (
        <DocumentsPanel />
      ) : (
        <div style={{ padding: '20px 0' }}>
          <EmptyState icon="shield-check" title="Admins only" description="Only admins can view documents." />
        </div>
      )}
    </div>
  )
}
