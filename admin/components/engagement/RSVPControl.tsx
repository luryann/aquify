'use client'

import { useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { respondClubEvent, type ClubEventRSVP, type RSVPResponse } from '@/lib/engagementData'
import { reportUIError } from '@/lib/frontend-telemetry'
import { RSVP_LABELS, billingNotice } from '@/lib/club-events'

/** Going / Maybe / Not going buttons with a party-size stepper for one RSVP party. */
export function RSVPControl({
  api, orgId, eventId, swimmerId, current, open, feeCents, label, headcountLabel, onSaved,
}: {
  api: ApiClient
  orgId: string
  eventId: string
  swimmerId?: string
  current?: ClubEventRSVP
  open: boolean
  feeCents: number
  label?: string
  headcountLabel: string
  onSaved: (r: ClubEventRSVP) => void
}) {
  const [party, setParty] = useState(1 + (current?.guest_count ?? 0))
  const [saving, setSaving] = useState<RSVPResponse | null>(null)
  const [message, setMessage] = useState<{ tone: 'ok' | 'error'; text: string } | null>(null)

  async function send(response: RSVPResponse) {
    setSaving(response)
    setMessage(null)
    try {
      const res = await respondClubEvent(api, orgId, eventId, {
        swimmer_id: swimmerId, response, guest_count: response === 'no' ? 0 : Math.max(0, party - 1),
      })
      onSaved(res.rsvp)
      setMessage({ tone: 'ok', text: billingNotice(res.billing, feeCents) ?? `Saved: ${RSVP_LABELS[response]}.` })
    } catch (err) {
      setMessage({ tone: 'error', text: reportUIError('club-event-rsvp', err, 'Your RSVP did not save. Try again.', { eventId }) })
    } finally {
      setSaving(null)
    }
  }

  return (
    <div className="u-gap-8px" style={{ display: 'flex', flexDirection: 'column' }}>
      <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
        {label && <span className="u-fs-fs-base u-fw-600" style={{ color: 'var(--text)', minWidth: 90 }}>{label}</span>}
        {(['yes', 'maybe', 'no'] as const).map((r) => {
          const active = current?.response === r
          return (
            <button key={r} type="button" disabled={!open || saving !== null} aria-pressed={active} onClick={() => send(r)}
              className={active ? 'btn btn-primary btn-sm' : 'btn btn-secondary btn-sm'}>
              {saving === r ? 'Saving…' : RSVP_LABELS[r]}
            </button>
          )
        })}
        <label className="u-gap-6px u-fs-fs-sm" style={{ display: 'inline-flex', alignItems: 'center', color: 'var(--text-subtle)' }}>
          {headcountLabel}
          <input type="number" min={1} max={51} value={party} disabled={!open}
            onChange={(e) => setParty(Math.max(1, Math.min(51, Number(e.target.value) || 1)))}
            className="input u-p-4px-8px" style={{ width: 64 }} />
        </label>
      </div>
      {!open && <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>RSVPs are closed{current ? `; your answer was ${RSVP_LABELS[current.response].toLowerCase()}` : ''}.</span>}
      {message && <span role={message.tone === 'error' ? 'alert' : 'status'} className="u-fs-fs-sm" style={{ color: message.tone === 'error' ? 'var(--error)' : 'var(--success)' }}>{message.text}</span>}
    </div>
  )
}
