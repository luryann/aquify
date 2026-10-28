'use client'

import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { WellnessPanel } from '@/components/engagement/WellnessPanel'
import './page.inline.css'

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
        <div className="card is-0a6919f" >
          Your login is not linked to a swimmer profile yet. Ask your coach to link it, then check in here.
        </div>
      )}
    </div>
  )
}
