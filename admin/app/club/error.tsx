'use client'

import Link from 'next/link'
import './club.css'

// Catches failures in the /club/[slug] layout (API down or unreachable) so visitors
// see guidance instead of a raw application error.
export default function ClubRootError({ reset }: { error: Error; reset: () => void }) {
  return (
    <div className="cs">
      <div className="cs-wrap cs-page">
        <div className="cs-empty" role="alert">
          <strong>The club site is not available right now</strong>
          This is usually temporary. Wait a minute and try again. If it keeps happening, contact your club directly.
          <div className="cs-actions">
            <button type="button" className="cs-btn cs-btn-primary" onClick={reset}>Try again</button>
            <Link href="/" className="cs-btn cs-btn-ghost">Back to home</Link>
          </div>
        </div>
      </div>
    </div>
  )
}
