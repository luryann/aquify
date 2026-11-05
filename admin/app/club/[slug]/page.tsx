import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { fetchPublicSite } from '@/lib/publicSiteData'

type Props = { params: Promise<{ slug: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) return {}
  return { title: { absolute: `${site.club.name} swim club` } }
}

/** Boilerplate team page: club name, about text, and the ways in. No swimmer data is shown. */
export default async function ClubHome({ params }: Props) {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) notFound()
  const { club } = site

  return (
    <>
      <section className="cs-hero">
        <div className="cs-wrap">
          <h1 className="cs-name">{club.name}</h1>
          {club.about ? <p className="cs-about">{club.about}</p> : null}
          <div className="cs-actions">
            <Link href={`/register?club=${encodeURIComponent(club.slug)}`} className="cs-btn cs-btn-primary">Join the team</Link>
            <Link href="/login" className="cs-btn cs-btn-ghost">Member sign in</Link>
          </div>
          <div className="cs-rope" aria-hidden="true" />
        </div>
      </section>

      <section className="cs-section">
        <div className="cs-wrap">
          <div className="cs-section-head"><h2>For team families</h2></div>
          <p className="cs-about">
            Sign in to see practice and meet schedules, respond to meet invitations, pay invoices and read messages from the coaches.
          </p>
        </div>
      </section>
    </>
  )
}
