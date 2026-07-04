'use client'

import { useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { createSwimmerLinkRequest } from '@/lib/adminData'

// Swimmer portal colours — resolved from the [data-portal="swim"] token overrides in globals.css
const C = {
  text:       'var(--text)',
  subtle:     'var(--text-subtle)',
  warn:       'var(--warning)',
  warnBg:     'var(--warning-bg)',
  warnBorder: 'var(--warning-border)',
  accent:     'var(--accent)',
  success:    'var(--accent)',
  successBg:  'var(--accent-subtle)',
  successBorder: 'var(--accent-border)',
  inputBg:     'rgba(0,0,0,0.25)',
  inputBorder: 'var(--warning-border)',
}

// Renders the "no swimmer linked" warning shown across the swimmer portal,
// plus a self-service form so the swimmer can notify their club admin
// instead of hitting a dead end.
export function UnlinkedSwimmerNotice() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [name, setName] = useState('')
  const [note, setNote] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit() {
    if (!orgId || submitting) return
    setSubmitting(true)
    setError(null)
    try {
      await createSwimmerLinkRequest(api, orgId, { requested_swimmer_name: name, note })
      setSubmitted(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to send your request right now.')
    } finally {
      setSubmitting(false)
    }
  }

  if (submitted) {
    return (
      <div
        style={{
          marginBottom: '24px',
          background: C.successBg,
          border: `1px solid ${C.successBorder}`,
          borderRadius: '12px',
          padding: '14px 16px',
          display: 'flex',
          alignItems: 'flex-start',
          gap: '10px',
        }}
      >
        <Icon name="check-circle" size={16} style={{ color: C.success, marginTop: '1px', flexShrink: 0 }} />
        <p style={{ margin: 0, fontSize: '13px', color: C.text, lineHeight: 1.5 }}>
          Request sent — an admin will link your account soon.
        </p>
      </div>
    )
  }

  return (
    <div
      style={{
        marginBottom: '24px',
        background: C.warnBg,
        border: `1px solid ${C.warnBorder}`,
        borderRadius: '12px',
        padding: '14px 16px',
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: '10px' }}>
        <Icon name="alert-circle" size={16} style={{ color: C.warn, marginTop: '1px', flexShrink: 0 }} />
        <p style={{ margin: 0, fontSize: '13px', color: C.text, lineHeight: 1.5 }}>
          Your account isn&apos;t linked to a swimmer profile yet, so times and meet commitments can&apos;t be shown.
          Let your club admin know below, or ask them directly.
        </p>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginLeft: '26px', maxWidth: '360px' }}>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Your full name"
          style={{
            padding: '8px 10px', background: C.inputBg, border: `1px solid ${C.inputBorder}`,
            borderRadius: '7px', color: C.text, fontSize: '12.5px', outline: 'none',
          }}
        />
        <input
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="Anything else your admin should know? (optional)"
          style={{
            padding: '8px 10px', background: C.inputBg, border: `1px solid ${C.inputBorder}`,
            borderRadius: '7px', color: C.text, fontSize: '12.5px', outline: 'none',
          }}
        />
        {error && <p style={{ margin: 0, fontSize: '12px', color: 'var(--error)' }}>{error}</p>}
        <button
          onClick={handleSubmit}
          disabled={submitting || !name.trim()}
          className="btn btn-primary"
          style={{ alignSelf: 'flex-start' }}
        >
          {submitting ? 'Sending…' : 'Notify my club admin'}
        </button>
      </div>
    </div>
  )
}
