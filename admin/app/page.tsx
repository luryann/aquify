import Link from 'next/link'

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

      <style>{`
        .land-root {
          min-height: 100vh;
          background: #06101A;
          display: flex;
          flex-direction: column;
          align-items: center;
          justify-content: center;
          position: relative;
          overflow: hidden;
        }
        .land-bg-grain {
          position: fixed;
          inset: 0;
          pointer-events: none;
          background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.025'/%3E%3C/svg%3E");
          background-size: 200px 200px;
          opacity: 0.35;
        }
        .land-glow {
          position: absolute;
          width: 600px; height: 600px;
          top: 50%; left: 50%;
          transform: translate(-50%, -50%);
          background: radial-gradient(circle, rgba(235,93,30,0.06) 0%, transparent 65%);
          pointer-events: none;
        }
        .land-center {
          position: relative;
          z-index: 1;
          display: flex;
          flex-direction: column;
          align-items: center;
          gap: 20px;
          animation: landReveal 0.6s cubic-bezier(0.16,1,0.3,1) both;
        }
        @keyframes landReveal {
          from { opacity: 0; transform: translateY(20px); }
          to   { opacity: 1; transform: translateY(0); }
        }
        .land-logo {
          display: flex;
          align-items: center;
          gap: 12px;
          margin-bottom: 4px;
        }
        .land-wordmark {
          font-family: var(--font-display); font-weight: var(--display-weight);
          font-style: italic;
          font-size: 48px;
          color: #EDE9E3;
          letter-spacing: var(--display-tracking);
          line-height: 1;
        }
        .land-tagline {
          font-family: var(--font-body);
          font-size: var(--fs-base);
          color: rgba(237,233,227,0.3);
          margin: 0;
        }
        .land-btn {
          display: inline-flex;
          align-items: center;
          gap: 6px;
          margin-top: 8px;
          padding: 11px 24px;
          background: var(--accent);
          border-radius: 9px;
          color: #fff;
          font-family: var(--font-body);
          font-size: var(--fs-md);
          font-weight: 600;
          text-decoration: none;
          letter-spacing: 0.01em;
          transition: background 0.18s, transform 0.12s, box-shadow 0.18s;
        }
        .land-btn:hover {
          background: var(--accent-hover);
          transform: translateY(-1px);
          box-shadow: 0 6px 20px rgba(235,93,30,0.28);
        }
        .land-footer {
          position: absolute;
          bottom: 24px;
          font-family: var(--font-body);
          font-size: var(--fs-xs);
          color: rgba(237,233,227,0.14);
          z-index: 1;
        }
      `}</style>
    </>
  )
}
