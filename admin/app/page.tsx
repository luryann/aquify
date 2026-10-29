import Link from 'next/link'
import './page.inline.css'

export default function LandingPage() {
  return (
    <>
      <div className="land-root">
        <div className="land-bg-grain" />
        <div className="land-glow" />

        <main className="land-center">
          {/* Logo */}
          <div className="land-logo">
            <svg width="44" height="44" viewBox="0 0 34 34" fill="none" aria-hidden="true">
              <rect width="34" height="34" rx="9" fill="rgba(235,93,30,0.14)" />
              <path d="M9 25V9h7c4.4 0 8 3.58 8 8s-3.6 8-8 8H9z" fill="var(--accent)" />
              <path d="M13 21v-8h3c2.2 0 4 1.79 4 4s-1.8 4-4 4h-3z" fill="#06101A" />
            </svg>
            <span className="land-wordmark">aquify</span>
          </div>

          <p className="land-tagline">Swim club operations platform</p>

          <Link href="/login" className="land-btn">
            Sign in →
          </Link>
        </main>

        <footer className="land-footer">
          <span>© {new Date().getFullYear()} aquify</span>
        </footer>
      </div>

    </>
  )
}
