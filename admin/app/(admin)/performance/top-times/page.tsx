'use client'

import { TopTimesBoard } from '@/components/performance/TopTimesBoard'

export default function TopTimesPage() {
  return (
    <div className="page">
      <h1 className="page-title">Top times</h1>
      <p className="page-subtitle u-mb-24px" >Fastest official swim per swimmer and event, by age on the day of the swim.</p>
      <TopTimesBoard canUseSeasons />
    </div>
  )
}
