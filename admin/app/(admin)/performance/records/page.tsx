'use client'

import { RecordsBoard } from '@/components/performance/RecordsBoard'
import { useSession } from '@/lib/session'

export default function TeamRecordsPage() {
  const { session } = useSession()
  const isAdmin = session?.user.roles?.includes('admin') ?? false
  return (
    <div className="page">
      <h1 className="page-title">Team records</h1>
      <p className="page-subtitle" style={{ marginBottom: 24 }}>Select a record to see who held it before.</p>
      <RecordsBoard canManage={isAdmin} />
    </div>
  )
}
