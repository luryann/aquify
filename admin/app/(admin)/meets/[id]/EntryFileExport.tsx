'use client'

import { useState, type ReactNode } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { exportMeetEntryBundle } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type Props = {
  meetId: string
  meetStatus: string
  /** Entries that will be written to the files (approved or already exported). */
  approvedEntryCount: number
  onExported: () => void
  /** Secondary links shown beside the download button. */
  actions?: ReactNode
}

/** Why the files can't be downloaded yet, or null when they can. */
export function entryExportBlocker(meetStatus: string, approvedEntryCount: number): string | null {
  if (meetStatus !== 'entry_finalized' && meetStatus !== 'completed') {
    return 'Available once the meet reaches Entries finalized.'
  }
  if (approvedEntryCount === 0) {
    return 'No approved entries yet.'
  }
  return null
}

function plural(count: number, one: string, many: string): string {
  return `${count} ${count === 1 ? one : many}`
}

/** Downloads the entry bundle (entry files, reports and log) to send to the meet host. */
export function EntryFileExport({ meetId, meetStatus, approvedEntryCount, onExported, actions }: Props) {
  const { session } = useSession()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<string | null>(null)
  const blocker = entryExportBlocker(meetStatus, approvedEntryCount)

  async function handleExport() {
    if (!session?.user.orgId) return
    setBusy(true)
    setError(null)
    setResult(null)
    try {
      const file = await exportMeetEntryBundle(session.token || undefined, session.user.orgId, meetId)
      const url = URL.createObjectURL(file.blob)
      const a = document.createElement('a')
      a.href = url
      a.download = file.filename
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
      setResult(
        `Downloaded ${file.filename}: ${plural(file.athleteCount, 'swimmer', 'swimmers')}, ${plural(file.entryCount, 'entry', 'entries')}.` +
        (file.skippedRelays > 0 ? ` ${plural(file.skippedRelays, 'relay entry is', 'relay entries are')} not included; enter relays in the host’s software.` : '') +
        (file.skippedOrphans > 0 ? ` ${plural(file.skippedOrphans, 'approved entry was', 'approved entries were')} left out because the swimmer or event no longer exists. Check the Entries tab before sending.` : ''),
      )
      onExported()
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t build the entry files. Try again.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="u-mb-16px">
      <div className="md-toolbar" style={{ justifyContent: 'space-between' }}>
        <p className="u-fs-fs-sm" style={{ color: 'var(--text-muted)', margin: 0, flex: '1 1 200px' }}>
          {blocker ?? 'Downloading marks approved entries as exported.'}
        </p>
        <div className="md-actions">
          {actions}
          <button type="button" className="btn btn-primary" onClick={() => { void handleExport() }} disabled={busy || blocker !== null}>
            <Icon name="download" size={14} /> {busy ? 'Building…' : 'Entry files'}
          </button>
        </div>
      </div>
      {error ? <p role="alert" className="u-fs-fs-sm" style={{ color: 'var(--error)' }}>{error}</p> : null}
      {result ? <p role="status" className="u-fs-fs-sm" style={{ color: 'var(--success)' }}>{result}</p> : null}
    </div>
  )
}
