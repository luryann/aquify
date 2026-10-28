import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { fetchPublicSite } from '@/lib/publicSiteData'
import { formatDateOnly } from '@/lib/dates'

type Props = { params: Promise<{ slug: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) return {}
  return {
    title: 'News',
    description: `Announcements and updates from ${site.club.name}.`,
    alternates: { canonical: `/club/${site.club.slug}/news` },
  }
}

export default async function ClubNews({ params }: Props) {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) notFound()
  return (
    <div className="cs-wrap">
      <div className="cs-page"><h1>News</h1></div>
      {site.announcements.length === 0 ? (
        <div className="cs-empty"><strong>No news yet</strong>Posts from the coaches show up here.</div>
      ) : (
        <div className="cs-stack">
          {site.announcements.map((a) => (
            <article key={a.id} id={a.id} className="cs-article">
              <span className="cs-date">{formatDateOnly(a.published_at)}</span>
              <h2>{a.title}</h2>
              <div className="cs-body">{a.body}</div>
            </article>
          ))}
        </div>
      )}
    </div>
  )
}
