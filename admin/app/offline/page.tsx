export const metadata = { title: 'Offline' }

export default function OfflinePage() {
  return (
    <main
      style={{
        minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 24,
        background: 'var(--bg)', color: 'var(--text)', fontFamily: 'var(--font-body)',
        textAlign: 'center',
      }}
    >
      <div style={{ maxWidth: 360 }}>
        <h1 style={{ fontFamily: 'var(--font-display)', fontWeight: 'var(--display-weight)', fontSize: 'var(--fs-4xl)', margin: '0 0 12px' }}>
          You are offline
        </h1>
        <p style={{ color: 'var(--text-subtle)', lineHeight: 1.5, margin: '0 0 24px' }}>
          Schedules and meet details need a connection. Reconnect and try again.
        </p>
        <a
          href="/login"
          style={{
            display: 'inline-block', background: 'var(--accent)', color: '#fff', borderRadius: 12,
            padding: '12px 20px', fontWeight: 600, textDecoration: 'none',
          }}
        >
          Try again
        </a>
      </div>
    </main>
  )
}
