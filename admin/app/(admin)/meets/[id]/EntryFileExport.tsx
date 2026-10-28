'use client'

import { useState } from 'react'
import { useSession } from '@/lib/session'
import { exportMeetEntriesSD3 } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type Props = {
  meetId: string
  onExported: () => void
}

/** Downloads the SD3 entry file to send to the host club. */
export function EntryFileExport({ meetId, onExported }: Props) {
  const { session } = useSession()
  const [variant, setVariant] = useState<'extended' | 'standard'>('extended')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<string | null>(null)

  async function handleExport() {
    if (!session?.user.orgId) return
    setBusy(true)
    setError(null)
    setResult(null)
    try {
      const file = await exportMeetEntriesSD3(session.token || undefined, session.user.orgId, meetId, variant)
      const url = URL.createObjectURL(file.blob)
      const a = document.createElement('a')
      a.href = url
      a.download = file.filename
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
      setResult(
        `${file.filename} — ${file.entryCount} entr${file.entryCount === 1 ? 'y' : 'ies'}` +
        (file.skippedRelays > 0 ? `; ${file.skippedRelays} relay entr${file.skippedRelays === 1 ? 'y' : 'ies'} not included (enter relays in the host's software).` : '.'),
      )
      onExported()
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t export the entry file. Try again.'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card" style={{ padding: '16px', marginBottom: '16px', display: 'flex', alignItems: 'center', gap: '16px', flexWrap: 'wrap' }}>
      <div style={{ flex: '1 1 260px' }}>
        <h3 className="card-title" style={{ marginBottom: '4px' }}>Entry file for host</h3>
        <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', lineHeight: 1.5 }}>
          SD3 file of approved entries for the host. Marks them exported.
        </p>
        {error ? <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--danger)', marginTop: '6px' }}>{error}</p> : null}
        {result ? <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--success)', marginTop: '6px' }}>{result}</p> : null}
      </div>
      <select
        aria-label="SD3 variant"
        value={variant}
        onChange={(e) => setVariant(e.target.value as 'extended' | 'standard')}
        style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)' }}
      >
        <option value="extended">With seed times</option>
        <option value="standard">Without seed times (NT)</option>
      </select>
      <button type="button" className="btn btn-primary" onClick={() => { void handleExport() }} disabled={busy}>
        {busy ? 'Exporting…' : 'Download SD3'}
      </button>
    </div>
  )
}
