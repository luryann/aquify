'use client'

import { Suspense, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Tabs } from '@/components/ui/Tabs'
import { UsersPanel } from './UsersPanel'
import { RolesPanel } from './RolesPanel'

export default function UsersRolesPage() {
  return (
    <Suspense fallback={null}>
      <UsersRolesPageContent />
    </Suspense>
  )
}

function UsersRolesPageContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [tab, setTab] = useState(searchParams.get('tab') === 'roles' ? 'roles' : 'users')

  function changeTab(id: string) {
    setTab(id)
    router.replace(id === 'roles' ? '/users?tab=roles' : '/users')
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Users &amp; roles</h1>
        </div>
      </div>

      <Tabs
        tabs={[
          { id: 'users', label: 'Users' },
          { id: 'roles', label: 'Role management' },
        ]}
        active={tab}
        onChange={changeTab}
      />

      {tab === 'roles' ? <RolesPanel /> : <UsersPanel />}
    </div>
  )
}
