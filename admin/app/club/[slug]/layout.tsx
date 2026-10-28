import type { Metadata } from 'next'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { ClubNav } from '@/components/club/ClubNav'
import { fetchPublicSite } from '@/lib/publicSiteData'
import '../club.css'

type Props = { children: React.ReactNode; params: Promise<{ slug: string }> }

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) return { title: 'Club not found', robots: { index: false } }
  const { club } = site
  const description = club.about?.trim() || `News, meet schedule, events and team records for ${club.name}.`
  return {
    title: { default: club.name, template: `%s · ${club.name}` },
    description,
    alternates: { canonical: `/club/${club.slug}` },
    openGraph: { title: club.name, description, type: 'website', siteName: club.name },
    twitter: { card: 'summary', title: club.name, description },
  }
}

export default async function ClubLayout({ children, params }: Props) {
  const { slug } = await params
  const site = await fetchPublicSite(slug)
  if (!site) notFound()
  const { club } = site
  return (
    <div className="cs">
      <header className="cs-header">
        <div className="cs-wrap cs-header-in">
          <Link href={`/club/${club.slug}`} className="cs-brand">{club.name}</Link>
          <ClubNav slug={club.slug} />
        </div>
      </header>
      <main id="main">{children}</main>
      <footer className="cs-footer">
        <div className="cs-wrap cs-footer-in">
          <span>{club.name}</span>
          <span style={{ display: 'flex', gap: 20, flexWrap: 'wrap' }}>
            {club.contact_email ? <a href={`mailto:${club.contact_email}`}>{club.contact_email}</a> : null}
            {club.contact_phone ? <a href={`tel:${club.contact_phone.replace(/[^+\d]/g, '')}`}>{club.contact_phone}</a> : null}
            <Link href="/login">Member sign in</Link>
          </span>
        </div>
      </footer>
    </div>
  )
}
