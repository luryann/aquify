'use client'

import { useRef, useState } from 'react'
import { Modal } from '@/components/ui/Modal'
import { importTimeStandardsFile, type StandardsImportResult } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { formatSwimTime } from '@/lib/performanceData'
import { useSession } from '@/lib/session'

const NOTE: React.CSSProperties = { color: 'var(--text-muted)' }
const ACTION_BADGE: Record<string, string> = { create: 'badge-success', update: 'badge-accent', unchanged: 'badge-default' }

function ageLabel(min?: number, max?: number) {
  if (min == null && max == null) return 'Open'
  if (min == null) return `${max} & under`
  if (max == null) return `${min} & over`
  return min === max ? `${min}` : `${min}-${max}`
}

export function ImportStandardsModal({ open, onClose, onImported }: { open: boolean; onClose: () => void; onImported: () => void }) {
  const { session } = useSession()
  const inputRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [result, setResult] = useState<StandardsImportResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function reset() {
    setFile(null)
    setResult(null)
    setError(null)
    if (inputRef.current) inputRef.current.value = ''
  }
  function close() {
    reset()
    onClose()
  }

  async function run(preview: boolean, f: File) {
    if (!session) return
    setBusy(true)
    setError(null)
    try {
      const res = await importTimeStandardsFile(session.token, session.user.orgId, f, preview)
      setResult(res)
      if (!preview) {
        onImported()
        close()
      }
    } catch (err) {
      if (preview) setResult(null)
      setError(uiErrorMessage(err, 'Couldn’t read that file. Try again.'))
    } finally {
      setBusy(false)
    }
  }

  const hasErrors = (result?.counts.errors ?? 0) > 0
  const writes = (result?.counts.create ?? 0) + (result?.counts.update ?? 0)

  return (
    <Modal
      open={open}
      onClose={close}
      title="Import time standards"
      description="Upload a CSV of cut times. You will see a preview before anything is saved."
      width={760}
      footer={
        <>
          <button onClick={close} className="btn btn-secondary">Cancel</button>
          <button
            onClick={() => file && void run(false, file)}
            disabled={busy || !file || !result || hasErrors || writes === 0}
            className="btn btn-primary"
          >
            {busy ? 'Working…' : writes > 0 ? `Import ${writes} cut time${writes === 1 ? '' : 's'}` : 'Import'}
          </button>
        </>
      }
    >
      <div className="u-gap-16px" style={{ display: 'grid' }}>
        <div>
          <label className="form-label" htmlFor="ts-import-file">CSV file</label>
          <input
            id="ts-import-file"
            ref={inputRef}
            type="file"
            accept=".csv,text/csv"
            onChange={(e) => {
              const f = e.target.files?.[0] ?? null
              setFile(f)
              setResult(null)
              setError(null)
              if (f) void run(true, f)
            }}
          />
          <p className="u-fs-fs-xs u-mt-4px" style={NOTE}>
            <a href="/templates/time-standards-template.csv" download>Download the CSV template</a>
            {' '}· Columns: standard_name, standard_code, season_year, course, gender, age_group, event_distance, stroke, cut_time.
            Spreadsheets must be exported as CSV. Existing standards with the same code, season, event, course, gender and age group are updated.
          </p>
        </div>

        {error ? (
          <div role="alert" className="u-p-12px-16px u-fs-fs-sm" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' }}>
            {error}
          </div>
        ) : null}

        {result ? (
          <div>
            <p className="u-fs-fs-sm u-mb-12px" style={NOTE}>
              {result.counts.total} rows · {result.counts.create} new · {result.counts.update} updated · {result.counts.unchanged} unchanged
              {hasErrors ? ` · ${result.counts.errors} with errors` : ''}
            </p>
            {hasErrors ? (
              <p className="u-fs-fs-sm u-mb-12px" style={{ color: 'var(--error)' }}>
                Fix the rows with errors and choose the file again. Nothing is imported until every row is valid.
              </p>
            ) : null}
            <div style={{ maxHeight: '320px', overflow: 'auto' }}>
              <table className="table" style={{ width: '100%' }}>
                <thead>
                  <tr><th>Line</th><th>Standard</th><th>Event</th><th>Ages</th><th>Cut</th><th>Result</th></tr>
                </thead>
                <tbody>
                  {result.rows.map((row) => (
                    <tr key={row.line}>
                      <td>{row.line}</td>
                      <td>{row.standard_name} {row.standard_code && row.standard_code !== row.standard_name ? `(${row.standard_code})` : ''}</td>
                      <td>{row.event_distance ? `${row.event_distance} ${row.stroke}` : '—'} · {row.gender} · {row.course}</td>
                      <td>{ageLabel(row.age_min, row.age_max)}</td>
                      <td className="u-ff-font-body">{row.time_ms ? formatSwimTime(row.time_ms) : '—'}</td>
                      <td>
                        {row.errors.length > 0
                          ? <span style={{ color: 'var(--error)' }}>{row.errors.join('; ')}</span>
                          : <span className={`badge ${ACTION_BADGE[row.action ?? 'unchanged']}`}>{row.action}</span>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : null}
      </div>
    </Modal>
  )
}
