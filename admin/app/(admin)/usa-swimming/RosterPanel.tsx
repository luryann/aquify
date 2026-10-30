'use client'

import { useState } from 'react'
import { Table, type ColumnDef } from '@/components/ui/Table'
import { LoadErrorBanner } from '@/components/ui/LoadErrorBanner'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  compareUSASwimmingRoster,
  fetchUSASwimmingRegistrationLink,
  type USASwimmingRosterComparison,
  type USASwimmingRosterRow,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type Row = USASwimmingRosterRow & { rowKey: string }

const COLUMNS: ColumnDef<Row>[] = [
  { key: 'name', label: 'Name' },
  { key: 'member_id', label: 'USA Swimming ID', render: (r) => <span className="tabular">{r.member_id ?? ''}</span> },
  { key: 'differences', label: 'Differences', render: (r) => (r.differences?.length ? r.differences.join(', ') : '') },
]

export function RosterPanel() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [data, setData] = useState<USASwimmingRosterComparison | null>(null)
  const [link, setLink] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [checkedAt, setCheckedAt] = useState<Date | null>(null)

  async function run() {
    if (!session) return
    setBusy(true)
    setErr(null)
    try {
      const [cmp, regLink] = await Promise.all([
        compareUSASwimmingRoster(api, session.user.orgId),
        fetchUSASwimmingRegistrationLink(api, session.user.orgId).catch(() => null), // optional extra
      ])
      setData(cmp)
      setLink(regLink)
      setCheckedAt(new Date())
    } catch (e) {
      setErr(uiErrorMessage(e, 'Couldn’t compare rosters. Check the connection and try again.'))
    } finally {
      setBusy(false)
    }
  }

  const groups: { title: string; empty: string; rows: USASwimmingRosterRow[] }[] = data
    ? [
        { title: 'In SWIMS, not in Aquify', empty: 'Everyone on the SWIMS roster is in Aquify.', rows: data.only_in_swims ?? [] },
        { title: 'In Aquify, not in SWIMS', empty: 'Every Aquify swimmer with a USA Swimming ID is on the SWIMS roster.', rows: data.only_in_aquify ?? [] },
        { title: 'Details differ', empty: 'No last name or birth date differences.', rows: data.mismatched ?? [] },
      ]
    : []

  return (
    <div className="stack-lg">
      <div className="card u-p-20px">
        <div className="row" style={{ justifyContent: 'space-between' }}>
          <p className="u-m-0" style={{ color: 'var(--text-muted)', maxWidth: '65ch' }}>
            Compares the SWIMS club roster with Aquify swimmers by USA Swimming ID. Nothing is changed; fix any differences in Members or in SWIMS.
          </p>
          <button type="button" className="btn btn-primary" onClick={() => void run()} disabled={busy}>
            {busy ? 'Comparing…' : data ? 'Compare again' : 'Compare rosters'}
          </button>
        </div>
        {data ? (
          <p className="u-fs-fs-sm u-mt-12px u-m-0" style={{ color: 'var(--text-muted)' }}>
            {data.matched} of {data.swims_count} SWIMS members match an Aquify swimmer.
            {checkedAt ? ` Checked ${checkedAt.toLocaleTimeString()}.` : ''}
            {link ? <> Members missing from SWIMS can register at <a href={link} target="_blank" rel="noreferrer">your club’s registration page</a>.</> : null}
          </p>
        ) : null}
      </div>

      {err ? <LoadErrorBanner message={err} onRetry={() => void run()} /> : null}

      {groups.map((g) => (
        <section key={g.title}>
          <h2 className="section-title">{g.title} ({g.rows.length})</h2>
          <Table
            columns={COLUMNS}
            data={g.rows.map((r, i) => ({ ...r, rowKey: `${r.member_id ?? r.swimmer_id ?? ''}-${i}` }))}
            keyField="rowKey"
            emptyTitle={g.empty}
          />
        </section>
      ))}
    </div>
  )
}
