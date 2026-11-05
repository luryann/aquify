import Link from 'next/link'

export function ClubNav({ slug }: { slug: string }) {
  return (
    <nav className="cs-nav" aria-label="Club site">
      <Link href="/login">Member sign in</Link>
      <Link href={`/register?club=${encodeURIComponent(slug)}`} className="cs-join">Join the team</Link>
    </nav>
  )
}
