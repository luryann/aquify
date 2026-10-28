import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { fetchPublicSite, formatSwimTime } from '@/lib/publicSiteData'
import { COURSE_LABELS, GENDER_LABELS, eventName, groupRecordsByAge, resolveRecordFilter } from '@/lib/club-site'
import { formatDateOnly } from '@/lib/dates'

type Props = { params: Promise<{ slug: string }>; searchParams: Promise<{ course?: string; gender?: string }> }

export async function generateMetadata({ params }: Pick<Props, 'params'>): Promise<Metadata> {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) return {}
  return {
    title: 'Team records',
    description: `Current team records for ${site.club.name} by course, gender and age group.`,
    alternates: { canonical: `/club/${site.club.slug}/records` },
  }
}

export default async function ClubRecords({ params, searchParams }: Props) {
  const [{ slug }, query] = await Promise.all([params, searchParams])
  const site = await fetchPublicSite(slug)
  if (!site) notFound()
  const base = `/club/${site.club.slug}/records`
  const records = site.records
  const filter = resolveRecordFilter(records, query.course, query.gender)
  const courses = Array.from(new Set(records.map((r) => r.course)))
  const genders = Array.from(new Set(records.filter((r) => r.course === filter.course).map((r) => r.gender)))
  const groups = groupRecordsByAge(records, filter)

  return (
    <div className="cs-wrap">
      <div className="cs-page"><h1>Team records</h1></div>
      {records.length === 0 ? (
        <div className="cs-empty"><strong>No records posted yet</strong>Records appear here after the coaches publish them.</div>
      ) : (
        <>
          <div className="cs-tabs" role="group" aria-label="Course">
            {courses.map((c) => (
              <Link key={c} href={`${base}?course=${c}&gender=${filter.gender}`} aria-current={c === filter.course}>{COURSE_LABELS[c] ?? c}</Link>
            ))}
          </div>
          <div className="cs-tabs" role="group" aria-label="Gender" style={{ marginTop: 0 }}>
            {genders.map((g) => (
              <Link key={g} href={`${base}?course=${filter.course}&gender=${g}`} aria-current={g === filter.gender}>{GENDER_LABELS[g] ?? g}</Link>
            ))}
          </div>
          {groups.map((g) => (
            <section key={g.ageGroup}>
              <h2 className="cs-age">{g.ageGroup}</h2>
              <div className="cs-rows">
                {g.records.map((r) => (
                  <div key={`${r.age_group}-${r.distance}-${r.stroke}`} className="cs-rec">
                    <span className="cs-rec-event">{eventName(r)}</span>
                    <span className="cs-rec-time">{formatSwimTime(r.time_ms)}</span>
                    <span className="cs-rec-who">
                      {r.swimmer_name}
                      {r.meet_name ? <span className="cs-rec-meet">{r.meet_name}</span> : null}
                    </span>
                    <span className="cs-rec-when">{formatDateOnly(r.swim_date)}</span>
                  </div>
                ))}
              </div>
            </section>
          ))}
        </>
      )}
    </div>
  )
}
