import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { fetchPublicSite } from '@/lib/publicSiteData'
import { COURSE_LABELS, excerpt, formatEventDay, formatEventTime, formatMeetDates } from '@/lib/club-site'
import { formatDateOnly } from '@/lib/dates'
import './page.inline.css'

type Props = { params: Promise<{ slug: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) return {}
  return { title: { absolute: `${site.club.name} swim club` } }
}

export default async function ClubHome({ params }: Props) {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) notFound()
  const { club, announcements, meets, events, coaches } = site
  const base = `/club/${club.slug}`
  const nextMeet = meets[0]
  const news = announcements.slice(0, 3)
  const upcoming = events.slice(0, 5)

  return (
    <>
      <section className="cs-hero">
        <div className="cs-wrap">
          <div className="cs-hero-grid">
            <div>
              <h1 className="cs-name">{club.name}</h1>
              {club.about ? <p className="cs-about">{club.about}</p> : null}
              <div className="cs-actions">
                <Link href="/register" className="cs-btn cs-btn-primary">Join the team</Link>
                <Link href={`${base}/schedule`} className="cs-btn cs-btn-ghost">See the schedule</Link>
              </div>
            </div>
            {nextMeet ? (
              <aside className="cs-heat" aria-label="Next meet">
                <h2>{nextMeet.name}</h2>
                <p className="cs-heat-when">{formatMeetDates(nextMeet)}</p>
                <div className="cs-heat-meta">
                  <span className="cs-chip">{COURSE_LABELS[nextMeet.course] ?? nextMeet.course}</span>
                  {nextMeet.location ? <span>{nextMeet.location}</span> : null}
                </div>
                <Link href={`${base}/schedule`} className="cs-heat-link">Full schedule</Link>
              </aside>
            ) : null}
          </div>
          <div className="cs-rope" aria-hidden="true" />
        </div>
      </section>

      <section className="cs-section">
        <div className="cs-wrap">
          <div className="cs-section-head">
            <h2>Latest news</h2>
            {announcements.length > 3 ? <Link href={`${base}/news`} className="cs-more">All news</Link> : null}
          </div>
          {news.length === 0 ? (
            <div className="cs-empty"><strong>No news yet</strong>Posts from the coaches show up here.</div>
          ) : (
            <div className="cs-news-grid">
              {news.map((a) => (
                <article key={a.id} className="cs-news-card">
                  <span className="cs-date">{formatDateOnly(a.published_at)}</span>
                  <h3><Link href={`${base}/news#${a.id}`} style={{ textDecoration: 'none' }}>{a.title}</Link></h3>
                  <p>{excerpt(a.body)}</p>
                </article>
              ))}
            </div>
          )}
        </div>
      </section>

      <section className="cs-section">
        <div className="cs-wrap">
          <div className="cs-section-head">
            <h2>Coming up</h2>
            {events.length > 5 ? <Link href={`${base}/schedule`} className="cs-more">Full schedule</Link> : null}
          </div>
          {upcoming.length === 0 ? (
            <div className="cs-empty"><strong>Nothing on the calendar</strong>Club events appear here once they are scheduled.</div>
          ) : (
            <div className="cs-rows">
              {upcoming.map((e) => (
                <div key={e.id} className="cs-row">
                  <div className="cs-day">
                    <b>{formatEventDay(e, { day: 'numeric' })}</b>
                    <span>{formatEventDay(e, { month: 'short' })}</span>
                  </div>
                  <div>
                    <h3>{e.title}</h3>
                    <p>{[formatEventTime(e), e.location].filter(Boolean).join(', ')}</p>
                  </div>
                  <span className="cs-tag">{e.category.replace(/_/g, ' ')}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </section>

      {coaches.length > 0 ? (
        <section className="cs-section">
          <div className="cs-wrap">
            <h2 className="is-6c3fad3">Coaches</h2>
            <ul className="cs-coaches">
              {coaches.map((c) => (
                <li key={`${c.name}-${c.role}`}>{c.name}<small>{c.role}</small></li>
              ))}
            </ul>
          </div>
        </section>
      ) : null}

      <section className="cs-join-band">
        <div className="cs-wrap cs-join-in">
          <div>
            <h2>Swim with {club.name}</h2>
            <p>Create a family account to register swimmers, answer meet invitations and pay online.</p>
          </div>
          <Link href="/register" className="cs-btn cs-btn-light">Join the team</Link>
        </div>
      </section>
    </>
  )
}
