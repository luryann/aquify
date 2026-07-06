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
    <div style={{ padding: '24px', maxWidth: '1100px' }}>
      <div className="page-header">
        <div>
          <h1 className="page-title">Users &amp; Roles</h1>
          <p className="page-subtitle">Manage logins and permission levels for this organization.</p>
        </div>
      </div>

      <Tabs
        tabs={[
          { id: 'users', label: 'Users' },
          { id: 'roles', label: 'Role Management' },
        ]}
        active={tab}
        onChange={changeTab}
      />

      {tab === 'roles' ? <RolesPanel /> : <UsersPanel />}
    </div>
  )
}
