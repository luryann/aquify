'use client'

import { useRef, useState } from 'react'
import { useSession } from '@/lib/session'
import { importMeetResults, type ResultsImportSummary } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

/** Imports the host's results file so times flow into best times and cuts. */
export function ResultsImport({ meetId }: { meetId: string }) {
  const { session } = useSession()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [summary, setSummary] = useState<ResultsImportSummary | null>(null)

  async function handleFile(file: File | null) {
    if (!file || !session?.user.orgId) return
    setBusy(true)
    setError(null)
    setSummary(null)
    try {
      setSummary(await importMeetResults(session.token || undefined, session.user.orgId, meetId, file))
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t import results. Try again.'))
    } finally {
      setBusy(false)
      if (inputRef.current) inputRef.current.value = ''
    }
  }

  return (
    <div className="card" style={{ padding: '20px', gridColumn: '1 / -1' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px', flexWrap: 'wrap' }}>
        <div style={{ flex: '1 1 300px' }}>
          <h3 className="card-title" style={{ marginBottom: '6px' }}>Results</h3>
          <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', lineHeight: 1.5 }}>
            Import the host&apos;s results file (.sd3, .cl2, .hy3, .zip). Re-importing is safe.
          </p>
        </div>
        <input
          ref={inputRef}
          type="file"
          accept=".sd3,.cl2,.hy3,.zip"
          aria-label="Results file"
          style={{ display: 'none' }}
          onChange={(e) => { void handleFile(e.target.files?.[0] ?? null) }}
        />
        <button type="button" className="btn btn-primary" disabled={busy} onClick={() => inputRef.current?.click()}>
          {busy ? 'Importing…' : 'Import Results File'}
        </button>
      </div>
      {error ? <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--danger)', marginTop: '12px' }}>{error}</p> : null}
      {summary ? (
        <div style={{ marginTop: '12px', fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', lineHeight: 1.6 }}>
          <p style={{ color: 'var(--success)', fontWeight: 600 }}>
            {summary.imported} swim{summary.imported === 1 ? '' : 's'} imported
            {summary.duplicates > 0 ? ` · ${summary.duplicates} already recorded` : ''}
            {summary.skipped > 0 ? ` · ${summary.skipped} skipped (relays / no time)` : ''}
          </p>
          {summary.unmatched.length > 0 ? (
            <div style={{ marginTop: '8px' }}>
              <p style={{ color: 'var(--warning)', fontWeight: 600 }}>
                {summary.unmatched.length} swim{summary.unmatched.length === 1 ? '' : 's'} not matched to a club swimmer
              </p>
              <ul style={{ margin: '4px 0 0', paddingLeft: '20px' }}>
                {summary.unmatched.slice(0, 20).map((u, i) => (
                  <li key={`${u.name}-${u.event}-${i}`}>{u.name || 'Unknown'} — {u.event}: {u.reason}</li>
                ))}
              </ul>
              {summary.unmatched.length > 20 ? <p>…and {summary.unmatched.length - 20} more.</p> : null}
            </div>
          ) : null}
          {summary.warnings.length > 0 ? <p style={{ marginTop: '6px', color: 'var(--text-muted)' }}>File warnings: {summary.warnings.join('; ')}</p> : null}
        </div>
      ) : null}
    </div>
  )
}
