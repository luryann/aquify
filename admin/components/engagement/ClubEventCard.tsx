'use client'

import { Icon } from '@/components/ui/icons'
import type { ApiClient } from '@/lib/api'
import { CLUB_EVENT_CATEGORY_LABELS, type ClubEventRecord, type ClubEventRSVP } from '@/lib/engagementData'
import { applyRSVPToSummary, findMyRSVP, formatClubEventWhen, formatFee } from '@/lib/club-events'
import { RSVPControl } from './RSVPControl'

export function mergeRSVP(list: ClubEventRecord[], eventId: string, rsvp: ClubEventRSVP): ClubEventRecord[] {
  return list.map((e) => {
    if (e.id !== eventId) return e
    const prev = (e.my_rsvps ?? []).find((r) => r.id === rsvp.id)
    const others = (e.my_rsvps ?? []).filter((r) => r.id !== rsvp.id)
    return { ...e, my_rsvps: [...others, rsvp], rsvp_summary: applyRSVPToSummary(e.rsvp_summary, prev, rsvp) }
  })
}

export function ClubEventCard({ event, api, orgId, onRSVP, swimmerOnlyId }: {
  event: ClubEventRecord
  api: ApiClient
  orgId: string
  onRSVP: (eventId: string, rsvp: ClubEventRSVP) => void
  /** Swimmer portal: only this swimmer may respond (swimmer-mode events). */
  swimmerOnlyId?: string
}) {
  const s = event.rsvp_summary
  const cancelled = event.status === 'cancelled'
  return (
    <article id={`event-${event.id}`} className="card" style={{ padding: 20, display: 'flex', flexDirection: 'column', gap: 12, opacity: cancelled ? 0.7 : 1 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div style={{ minWidth: 0 }}>
          <h2 className="section-title" style={{ textDecoration: cancelled ? 'line-through' : 'none' }}>{event.title}</h2>
          <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-subtle)', marginTop: 4, display: 'flex', gap: 12, flexWrap: 'wrap' }}>
            <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><Icon name="clock" size={12} />{formatClubEventWhen(event)}</span>
            {event.location && <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><Icon name="map-pin" size={12} />{event.location}</span>}
          </p>
        </div>
        <span className={cancelled ? 'badge badge-error' : 'badge badge-default'} style={{ alignSelf: 'flex-start' }}>
          {cancelled ? 'Cancelled' : CLUB_EVENT_CATEGORY_LABELS[event.category]}
        </span>
      </div>
      {event.description && <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text)', lineHeight: 1.55, whiteSpace: 'pre-wrap' }}>{event.description}</p>}
      {event.rsvp_enabled && !cancelled && (
        <div style={{ borderTop: '1px solid var(--border)', paddingTop: 12, display: 'flex', flexDirection: 'column', gap: 12 }}>
          <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>
            {s.yes_headcount} going{s.spots_left !== undefined && s.spots_left !== null ? ` · ${s.spots_left} spots left` : ''}
            {event.fee_cents > 0 ? ` · ${formatFee(event.fee_cents)} per person, billed to your account` : ''}
            {event.rsvp_deadline ? ` · reply by ${new Date(event.rsvp_deadline).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}` : ''}
          </p>
          {event.rsvp_mode === 'household' ? (
            swimmerOnlyId ? (
              <p style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)' }}>
                {findMyRSVP(event.my_rsvps) ? `Your family replied: ${findMyRSVP(event.my_rsvps)!.response}.` : 'A parent replies for your family.'}
              </p>
            ) : (
              <RSVPControl api={api} orgId={orgId} eventId={event.id} current={findMyRSVP(event.my_rsvps)} open={event.rsvp_open}
                feeCents={event.fee_cents} headcountLabel="People attending" onSaved={(r) => onRSVP(event.id, r)} />
            )
          ) : (
            (event.eligible_swimmers ?? []).filter((sw) => !swimmerOnlyId || sw.id === swimmerOnlyId).map((sw) => (
              <RSVPControl key={sw.id} api={api} orgId={orgId} eventId={event.id} swimmerId={sw.id} label={swimmerOnlyId ? undefined : sw.display_name}
                current={findMyRSVP(event.my_rsvps, sw.id)} open={event.rsvp_open} feeCents={event.fee_cents}
                headcountLabel="Incl. guests" onSaved={(r) => onRSVP(event.id, r)} />
            ))
          )}
        </div>
      )}
    </article>
  )
}

