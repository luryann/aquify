'use client'

import { useState } from 'react'
import { Table, type ColumnDef } from '@/components/ui/Table'
import { Notice } from '@/components/ui/Notice'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  verifyUSASwimmingMembers,
  verifyUSASwimmingTime,
  type SwimmerRecord,
  type USASwimmingMemberCheck,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'


type CheckRow = USASwimmingMemberCheck & { status: string }

const COLUMNS: ColumnDef<CheckRow>[] = [
  { key: 'name', label: 'Swimmer' },
  { key: 'member_id', label: 'USA Swimming ID', render: (r) => <span className="tabular">{r.member_id}</span> },
  { key: 'status', label: 'Membership' },
  { key: 'detail', label: 'Note' },
]

export function VerifyPanel({ swimmers }: { swimmers: SwimmerRecord[] }) {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [checks, setChecks] = useState<CheckRow[] | null>(null)
  const [busy, setBusy] = useState<'members' | 'time' | null>(null)
  const [err, setErr] = useState<string | null>(null)

  const [swimmerId, setSwimmerId] = useState('')
  const [event, setEvent] = useState('')
  const [time, setTime] = useState('')
  const [timeResult, setTimeResult] = useState<{ valid: boolean; label: string } | null>(null)

  async function verifyAll() {
    if (!session) return
    setBusy('members')
    setErr(null)
    try {
      const res = await verifyUSASwimmingMembers(api, session.user.orgId)
      setChecks(res.results.map((r) => ({ ...r, status: r.valid ? 'Valid' : 'Not valid' })))
    } catch (e) {
      setErr(uiErrorMessage(e, 'Couldn’t verify memberships. Member verification needs Tier 3 access from USA Swimming.'))
    } finally {
      setBusy(null)
    }
  }

  async function verifyTime(e: React.FormEvent) {
    e.preventDefault()
    if (!session) return
    setBusy('time')
    setErr(null)
    setTimeResult(null)
    try {
      const valid = await verifyUSASwimmingTime(api, session.user.orgId, { swimmer_id: swimmerId, event: event.trim(), time: time.trim() })
      const s = swimmers.find((x) => x.id === swimmerId)
      setTimeResult({ valid, label: `${s ? `${s.first_name} ${s.last_name}` : 'Swimmer'}, ${event.trim()} in ${time.trim()}` })
    } catch (e2) {
      setErr(uiErrorMessage(e2, 'Couldn’t verify that time. Time verification needs Tier 3 access from USA Swimming.'))
    } finally {
      setBusy(null)
    }
  }

  const invalid = checks?.filter((c) => !c.valid).length ?? 0

  return (
    <div className="stack-lg">
      {err ? <Notice tone="err">{err}</Notice> : null}

      <section className="card u-p-20px">
        <h2 className="card-title">Membership</h2>
        <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-muted)', maxWidth: '65ch' }}>
          Confirms that each of your {swimmers.length} swimmers with a USA Swimming ID holds a current membership.
        </p>
        <button type="button" className="btn btn-primary u-mt-12px" onClick={() => void verifyAll()} disabled={busy !== null || swimmers.length === 0}>
          {busy === 'members' ? 'Verifying…' : 'Verify all memberships'}
        </button>
        {checks ? (
          <div className="u-mt-16px">
            <p className="u-fs-fs-sm u-m-0" style={{ color: invalid ? 'var(--error)' : 'var(--text-muted)' }}>
              {invalid ? `${invalid} of ${checks.length} memberships need attention.` : `All ${checks.length} memberships are valid.`}
            </p>
            <Table columns={COLUMNS} data={checks} keyField="member_id" emptyTitle="No results returned" />
          </div>
        ) : null}
      </section>

      <section className="card u-p-20px">
        <h2 className="card-title">Swim time</h2>
        <p className="u-fs-fs-sm u-mt-4px" style={{ color: 'var(--text-muted)', maxWidth: '65ch' }}>
          Checks whether USA Swimming has a swim at this time on record for the swimmer, for example before accepting a time for a meet entry.
        </p>
        <form onSubmit={(e) => void verifyTime(e)} className="u-mt-16px u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', alignItems: 'end' }}>
          <div>
            <label className="form-label" htmlFor="vt-swimmer">Swimmer</label>
            <select id="vt-swimmer" style={{ width: '100%' }} value={swimmerId} onChange={(e) => setSwimmerId(e.target.value)} required>
              <option value="">Choose a swimmer</option>
              {swimmers.map((s) => <option key={s.id} value={s.id}>{s.first_name} {s.last_name}</option>)}
            </select>
          </div>
          <div>
            <label className="form-label" htmlFor="vt-event">Event</label>
            <input id="vt-event" style={{ width: '100%' }} value={event} onChange={(e) => setEvent(e.target.value)} placeholder="100 Free SCY" required />
          </div>
          <div>
            <label className="form-label" htmlFor="vt-time">Time</label>
            <input id="vt-time" className="tabular" style={{ width: '100%' }} value={time} onChange={(e) => setTime(e.target.value)} placeholder="00:52.34" pattern="\d{1,2}:\d{2}\.\d{2}" title="Minutes:seconds.hundredths, like 00:52.34" required />
          </div>
          <button type="submit" className="btn btn-primary" disabled={busy !== null}>{busy === 'time' ? 'Checking…' : 'Verify time'}</button>
        </form>
        {timeResult ? (
          <Notice tone={timeResult.valid ? 'ok' : 'err'} className="u-mt-16px">
            {timeResult.valid ? `USA Swimming confirms ${timeResult.label}.` : `USA Swimming has no record of ${timeResult.label}. Check the event name and time.`}
          </Notice>
        ) : null}
      </section>
    </div>
  )
}
