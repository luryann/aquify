'use client'

import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { WellnessPanel } from '@/components/engagement/WellnessPanel'

export default function SwimmerWellnessPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const swimmerId = session?.user.swimmerId ?? ''

  return (
    <div>
      <div className="page-heading">
        <h1 className="page-title">Daily check-in</h1>
      </div>
      {swimmerId ? (
        <WellnessPanel api={api} orgId={orgId} swimmerId={swimmerId} selfView />
      ) : (
        <div className="card" style={{ padding: 24, fontSize: 'var(--fs-md)', color: 'var(--text-subtle)' }}>
          Your login is not linked to a swimmer profile yet. Ask your coach to link it, then check in here.
        </div>
      )}
    </div>
  )
}
