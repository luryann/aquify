'use client'

import '../club.css'

export default function ClubError({ reset }: { error: Error; reset: () => void }) {
  return (
    <div className="cs">
      <div className="cs-wrap cs-page">
        <div className="cs-empty">
          <strong>The club site could not load</strong>
          Check your connection and try again.
          <div className="cs-actions">
            <button type="button" className="cs-btn cs-btn-primary" onClick={reset}>Try again</button>
          </div>
        </div>
      </div>
    </div>
  )
}
