import './page.inline.css'
export const metadata = { title: 'Offline' }

export default function OfflinePage() {
  return (
    <main
      className="is-03c1948"
    >
      <div className="auth-arrive" style={{ maxWidth: 360 }}>
        <h1 className="is-7c3311f">
          You are offline
        </h1>
        <p className="is-3cc2d04">
          Schedules and meet details need a connection. Reconnect and try again.
        </p>
        <a
          href="/login"
          className="is-61cba8d"
        >
          Try again
        </a>
      </div>
    </main>
  )
}
