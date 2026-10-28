'use client'

import { useEffect, useState } from 'react'
import type { ApiClient } from '@/lib/api'
import { fetchCalendarFeed, revokeCalendarFeed, rotateCalendarFeed, type CalendarFeedStatus } from '@/lib/engagementData'
import { reportUIError } from '@/lib/frontend-telemetry'

/**
 * Household calendar subscription (.ics). The secret link is shown only right
 * after it is created; afterwards the family can reset or turn it off.
 */
export function CalendarSubscribeCard({ api, orgId }: { api: ApiClient; orgId: string }) {
  const [status, setStatus] = useState<CalendarFeedStatus | null>(null)
  const [link, setLink] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!orgId) return
    fetchCalendarFeed(api, orgId).then(setStatus).catch(() => setStatus(null))
  }, [api, orgId])

  async function create() {
    setBusy(true); setError(null); setCopied(false)
    try {
      const res = await rotateCalendarFeed(api, orgId)
      setStatus(res)
      if (res.feed_path) setLink(`${window.location.origin}${res.feed_path}`)
    } catch (err) {
      setError(reportUIError('calendar-feed-rotate', err, 'Couldn’t create a calendar link. Try again.'))
    } finally { setBusy(false) }
  }

  async function turnOff() {
    setBusy(true); setError(null)
    try {
      setStatus(await revokeCalendarFeed(api, orgId))
      setLink(null)
    } catch (err) {
      setError(reportUIError('calendar-feed-revoke', err, 'Couldn’t turn off the calendar link. Try again.'))
    } finally { setBusy(false) }
  }

  async function copy() {
    if (!link) return
    try { await navigator.clipboard.writeText(link); setCopied(true) } catch { setCopied(false) }
  }

  return (
    <div className="card" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
      <p style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-lg)', color: 'var(--text)' }}>Add to your phone’s calendar</p>
      <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', lineHeight: 1.5 }}>
        Meets, your swimmers’ practices and club events stay in sync in Google, Apple or Outlook calendar.
      </p>
      {link ? (
        <>
          <input readOnly value={link} className="input" onFocus={(e) => e.currentTarget.select()} aria-label="Calendar subscription link" style={{ fontSize: 'var(--fs-sm)' }} />
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <button type="button" className="btn btn-primary btn-sm" onClick={copy}>{copied ? 'Copied' : 'Copy link'}</button>
            <a className="btn btn-secondary btn-sm" href={link.replace(/^https?:/, 'webcal:')}>Open in calendar app</a>
          </div>
          <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)' }}>Keep this link private — anyone with it can see your family’s schedule. It won’t be shown again.</p>
        </>
      ) : status?.active ? (
        <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>
          Your calendar link is on{status.feed?.last_used_at ? `; last synced ${new Date(status.feed.last_used_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}` : ''}.
        </p>
      ) : null}
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {!link && (
          <button type="button" className={status?.active ? 'btn btn-secondary btn-sm' : 'btn btn-primary btn-sm'} disabled={busy} onClick={create}>
            {status?.active ? 'Reset link' : 'Create calendar link'}
          </button>
        )}
        {status?.active && <button type="button" className="btn btn-ghost btn-sm" disabled={busy} onClick={turnOff}>Turn off</button>}
      </div>
      {error && <p role="alert" style={{ fontSize: 'var(--fs-sm)', color: 'var(--error)' }}>{error}</p>}
    </div>
  )
}
