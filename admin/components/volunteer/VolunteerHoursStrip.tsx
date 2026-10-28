'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useApiClient } from '@/lib/api'
import { useSession } from '@/lib/session'
import { reportUIError } from '@/lib/frontend-telemetry'
import { fetchHouseholdLedger, type VolunteerLedgerRow } from '@/lib/volunteerData'
import { currentLedgerRow, formatHours } from '@/lib/volunteer-state'
import { HoursMeter } from './VolunteerPrimitives'

/**
 * Compact hours-remaining indicator for the parent dashboard. Renders only
 * when the current season has an open requirement; the full view (and any
 * load error with retry) lives on /portal/volunteer.
 */
export function VolunteerHoursStrip() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''
  const [row, setRow] = useState<VolunteerLedgerRow | null>(null)

  useEffect(() => {
    if (!orgId || !householdId) return
    let cancelled = false
    fetchHouseholdLedger(api, orgId, householdId)
      .then((rows) => { if (!cancelled) setRow(currentLedgerRow(rows)) })
      .catch((err) => { reportUIError('portal.dashboard.volunteer', err, 'Couldn’t load volunteer hours. Try again.') })
    return () => { cancelled = true }
  }, [api, orgId, householdId])

  if (!row || row.required_hours <= 0 || row.status === 'bought_out' || row.remaining_hours <= 0) return null

  return (
    <Link
      href="/portal/volunteer"
      className="card u-gap-16px u-mb-32px"
      style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', color: 'var(--text)', textDecoration: 'none' }}
    >
      <span className="u-fs-fs-md" style={{ flex: '1 1 200px' }}>
        <strong>{formatHours(row.remaining_hours)}</strong> of volunteer service left in {row.season_name}
      </span>
      <span style={{ flex: '2 1 220px' }}>
        <HoursMeter compact required={row.required_hours} earned={row.earned_hours} scheduled={row.scheduled_hours} />
      </span>
      <span className="u-fs-fs-base u-fw-600" style={{ color: 'var(--accent-text, var(--accent))' }}>Find a shift</span>
    </Link>
  )
}
