'use client'

import { useState } from 'react'
import { Tabs } from '@/components/ui/Tabs'
import { RecordsBoard } from './RecordsBoard'
import { TopTimesBoard } from './TopTimesBoard'

/** Read-only team records and top times for the parent and swimmer portals. */
export function ClubRecordsView() {
  const [tab, setTab] = useState('records')
  return (
    <div>
      <h1 className="page-title">Team records &amp; top times</h1>
      <p style={{ margin: '0 0 20px', color: 'var(--text-subtle)', fontSize: 'var(--fs-md)', maxWidth: '64ch' }}>
        Age groups use each swimmer’s age on the day of the swim.
      </p>
      <Tabs tabs={[{ id: 'records', label: 'Team records' }, { id: 'top', label: 'Top times' }]} active={tab} onChange={setTab} />
      {tab === 'records' ? <RecordsBoard /> : <TopTimesBoard />}
    </div>
  )
}
