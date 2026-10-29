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
  const familyRSVP = findMyRSVP(event.my_rsvps)
  return (
    <article id={`event-${event.id}`} className="card u-p-20px u-gap-12px" style={{ display: 'flex', flexDirection: 'column', opacity: cancelled ? 0.7 : 1 }}>
      <div className="u-gap-12px" style={{ display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap' }}>
        <div style={{ minWidth: 0 }}>
          <h2 className="section-title" style={{ textDecoration: cancelled ? 'line-through' : 'none' }}>{event.title}</h2>
          <p className="u-fs-fs-base u-mt-4px u-gap-12px" style={{ color: 'var(--text-subtle)', display: 'flex', flexWrap: 'wrap' }}>
            <span className="u-gap-4px" style={{ display: 'inline-flex', alignItems: 'center' }}><Icon name="clock" size={12} />{formatClubEventWhen(event)}</span>
            {event.location && <span className="u-gap-4px" style={{ display: 'inline-flex', alignItems: 'center' }}><Icon name="map-pin" size={12} />{event.location}</span>}
          </p>
        </div>
        <span className={cancelled ? 'badge badge-error' : 'badge badge-default'} style={{ alignSelf: 'flex-start' }}>
          {cancelled ? 'Cancelled' : CLUB_EVENT_CATEGORY_LABELS[event.category]}
        </span>
      </div>
      {event.description && <p className="u-fs-fs-base u-lh-1_55" style={{ color: 'var(--text)', whiteSpace: 'pre-wrap' }}>{event.description}</p>}
      {event.rsvp_enabled && !cancelled && (
        <div className="u-gap-12px" style={{ borderTop: '1px solid var(--border)', paddingTop: 12, display: 'flex', flexDirection: 'column' }}>
          <p className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
            {s.yes_headcount} going{s.spots_left !== undefined && s.spots_left !== null ? ` · ${s.spots_left} spots left` : ''}
            {event.fee_cents > 0 ? ` · ${formatFee(event.fee_cents)} per person, billed to your account` : ''}
            {event.rsvp_deadline ? ` · reply by ${new Date(event.rsvp_deadline).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}` : ''}
          </p>
          {event.rsvp_mode === 'household' ? (
            swimmerOnlyId ? (
              <p className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>
                {familyRSVP ? `Your family replied: ${familyRSVP.response}.` : 'A parent replies for your family.'}
              </p>
            ) : (
              <RSVPControl api={api} orgId={orgId} eventId={event.id} current={familyRSVP} open={event.rsvp_open}
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

