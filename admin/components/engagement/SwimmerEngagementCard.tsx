'use client'

import Link from 'next/link'
import { useEffect, useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { fetchAttendanceSummary, fetchSwimmerWellness, type AttendanceSummary, type SwimmerWellness } from '@/lib/engagementData'

/** Attendance rate (last 90 days) and wellness snapshot on the admin swimmer page. */
export function SwimmerEngagementCard({ api, orgId, swimmerId }: { api: ApiClient; orgId: string; swimmerId: string }) {
  const [att, setAtt] = useState<AttendanceSummary | null>(null)
  const [well, setWell] = useState<SwimmerWellness | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (!orgId || !swimmerId) return
    let cancelled = false
    fetchAttendanceSummary(api, orgId, swimmerId, 90).then((a) => { if (!cancelled) setAtt(a) }).catch(() => { if (!cancelled) setFailed(true) })
    fetchSwimmerWellness(api, orgId, swimmerId, 14).then((w) => { if (!cancelled) setWell(w) }).catch(() => { /* wellness is optional here */ })
    return () => { cancelled = true }
  }, [api, orgId, swimmerId])

  const stat = (value: string, caption: string) => (
    <div style={{ minWidth: 120 }}>
      <div className="u-ff-font-display u-fw-display-weight u-fs-fs-3xl u-lh-1" style={{ color: 'var(--text)' }}>{value}</div>
      <div className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-subtle)' }}>{caption}</div>
    </div>
  )

  return (
    <div className="card u-p-16px-20px u-m-0-0-16px u-gap-24px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>
      {failed ? (
        <span className="u-fs-fs-base" style={{ color: 'var(--text-muted)' }}>Attendance is unavailable right now.</span>
      ) : !att ? (
        <div className="skeleton" style={{ height: 40, width: 160, borderRadius: 8 }} />
      ) : stat(
        att.attendance_pct === null ? '–' : `${Math.round(att.attendance_pct)}%`,
        att.attendance_pct === null ? 'Attendance · no roll calls in 90 days' : `Attendance, 90 days · ${att.present} of ${att.present + att.absent} practices${att.excused ? `, ${att.excused} excused` : ''}`,
      )}
      {well && stat(
        well.averages.count ? well.averages.readiness.toFixed(1) : '–',
        well.averages.count ? `Readiness, 14 days · ${well.averages.count} check-ins${well.flags.length ? ` · ${well.flags.length} flag${well.flags.length > 1 ? 's' : ''}` : ''}` : 'Readiness · no check-ins in 14 days',
      )}
      <Link href="/wellness" className="u-ml-auto u-fs-fs-sm" style={{ color: 'var(--accent-text, var(--accent))', textDecoration: 'none' }}>Team wellness</Link>
    </div>
  )
}
