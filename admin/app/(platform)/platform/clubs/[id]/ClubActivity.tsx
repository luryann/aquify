import { activityLabel, type ClubActivityEvent } from '@/lib/tenancyData'

/** What operators have done to this club. The club's administrators see the same entries in their audit log. */
export function ClubActivity({ events }: { events: ClubActivityEvent[] }) {
  return (
    <section className="card u-p-20px" aria-labelledby="activity-title">
      <h2 className="card-title" id="activity-title">Operator activity</h2>
      {events.length === 0 ? (
        <p className="u-fs-fs-sm" style={{ color: 'var(--text-muted)', margin: '8px 0 0' }}>Nothing yet. Every change you make to this club is recorded here.</p>
      ) : (
        <ol style={{ listStyle: 'none', padding: 0, margin: '12px 0 0', display: 'grid', gap: '12px' }}>
          {events.map((ev) => {
            const who = ev.metadata?.target_email
            const reason = ev.metadata?.reason
            return (
              <li key={ev.id} style={{ display: 'grid', gap: '2px' }}>
                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap', alignItems: 'baseline', justifyContent: 'space-between' }}>
                  <span><strong>{activityLabel(ev.action)}</strong>{who ? <> · {who}</> : null}</span>
                  <time className="u-fs-fs-sm" dateTime={ev.occurred_at} style={{ color: 'var(--text-muted)' }}>{new Date(ev.occurred_at).toLocaleString()}</time>
                </div>
                {reason ? <span className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>Reason: {reason}</span> : null}
              </li>
            )
          })}
        </ol>
      )}
    </section>
  )
}
