'use client'

import { REASON_MAX, isValidReason } from '@/lib/tenancyData'

export const LABEL: React.CSSProperties = { fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '4px' }
export const HINT: React.CSSProperties = { fontSize: 'var(--fs-xs)', color: 'var(--text-muted)' }

/** The reason every support action records in the audit log, which the club's own admins can read. */
export function ReasonField({ id, value, onChange }: { id: string; value: string; onChange: (v: string) => void }) {
  const invalid = value !== '' && !isValidReason(value)
  return (
    <div>
      <label style={LABEL} htmlFor={id}>Why are you doing this?</label>
      <textarea id={id} rows={2} style={{ width: '100%', resize: 'vertical' }} maxLength={REASON_MAX} value={value}
        aria-invalid={invalid} aria-describedby={`${id}-hint`} onChange={(e) => onChange(e.target.value)} />
      <span id={`${id}-hint`} style={{ ...HINT, color: invalid ? 'var(--error)' : 'var(--text-muted)' }}>
        Saved in the audit log. The club’s administrators can see it.
      </span>
    </div>
  )
}
