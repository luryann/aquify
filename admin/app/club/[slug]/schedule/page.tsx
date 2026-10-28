import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { fetchPublicSite } from '@/lib/publicSiteData'
import { COURSE_LABELS, formatEventDay, formatEventTime, formatMeetDates, groupByMonth, monthLabel } from '@/lib/club-site'
import { parseBackendDate } from '@/lib/dates'

type Props = { params: Promise<{ slug: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) return {}
  return {
    title: 'Schedule',
    description: `Upcoming meets and club events for ${site.club.name}.`,
    alternates: { canonical: `/club/${site.club.slug}/schedule` },
  }
}

export default async function ClubSchedule({ params }: Props) {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) notFound()
  const meetMonths = groupByMonth(site.meets, (m) => m.start_date)
  const eventMonths = groupByMonth(site.events, (e) => (e.all_day ? e.starts_at.slice(0, 10) : new Date(e.starts_at).toLocaleDateString('en-CA')))

  return (
    <div className="cs-wrap">
      <div className="cs-page"><h1>Schedule</h1></div>

      <h2 style={{ fontSize: 'clamp(2rem, 5vw, 3.2rem)', margin: '24px 0 16px' }}>Meets</h2>
      {meetMonths.length === 0 ? (
        <div className="cs-empty"><strong>No meets scheduled</strong>Published meets appear here as soon as the coaches announce them.</div>
      ) : (
        meetMonths.map((g) => (
          <div key={g.key}>
            <h3 className="cs-month">{monthLabel(g.key)}</h3>
            <div className="cs-rows">
              {g.items.map((m) => (
                <div key={m.id} className="cs-row">
                  <div className="cs-day">
                    <b>{m.start_date ? parseBackendDate(m.start_date).getDate() : ''}</b>
                    <span>{m.start_date ? parseBackendDate(m.start_date).toLocaleDateString('en-US', { month: 'short' }) : ''}</span>
                  </div>
                  <div>
                    <h3>{m.name}</h3>
                    <p>{[formatMeetDates(m), m.location, m.host_org && `hosted by ${m.host_org}`].filter(Boolean).join(', ')}</p>
                  </div>
                  <span className="cs-tag">{COURSE_LABELS[m.course] ?? m.course}</span>
                </div>
              ))}
            </div>
          </div>
        ))
      )}

      <h2 style={{ fontSize: 'clamp(2rem, 5vw, 3.2rem)', margin: '64px 0 16px' }}>Club events</h2>
      {eventMonths.length === 0 ? (
        <div className="cs-empty"><strong>No events scheduled</strong>Socials, clinics and team meetings appear here once they are on the calendar.</div>
      ) : (
        eventMonths.map((g) => (
          <div key={g.key}>
            <h3 className="cs-month">{monthLabel(g.key)}</h3>
            <div className="cs-rows">
              {g.items.map((e) => (
                <div key={e.id} className="cs-row">
                  <div className="cs-day">
                    <b>{formatEventDay(e, { day: 'numeric' })}</b>
                    <span>{formatEventDay(e, { month: 'short' })}</span>
                  </div>
                  <div>
                    <h3>{e.title}</h3>
                    <p>{[formatEventTime(e), e.location].filter(Boolean).join(', ')}</p>
                    {e.description ? <p>{e.description}</p> : null}
                  </div>
                  <span className="cs-tag">{e.category.replace(/_/g, ' ')}</span>
                </div>
              ))}
            </div>
          </div>
        ))
      )}
    </div>
  )
}
