import type { Metadata } from 'next'

export const metadata: Metadata = { title: 'Dashboard' }

// Stat card component (server-renderable)
function StatCard({
  label,
  value,
  delta,
  positive,
  mono,
}: {
  label: string
  value: string
  delta?: string
  positive?: boolean
  mono?: boolean
}) {
  return (
    <div className="card-elevated" style={{ padding: '20px 22px' }}>
      <p
        style={{
          fontSize: '11px',
          fontFamily: 'var(--font-mono)',
          fontWeight: 500,
          color: 'var(--text-muted)',
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
          marginBottom: '10px',
        }}
      >
        {label}
      </p>
      <p
        style={{
          fontSize: '26px',
          fontFamily: mono ? 'var(--font-mono)' : 'var(--font-display)',
          color: 'var(--text)',
          lineHeight: 1.1,
          letterSpacing: mono ? '0.02em' : '-0.01em',
          marginBottom: '6px',
        }}
      >
        {value}
      </p>
      {delta && (
        <p
          style={{
            fontSize: '11px',
            color: positive ? 'var(--success)' : 'var(--error)',
            fontFamily: 'var(--font-mono)',
          }}
        >
          {positive ? '↑' : '↓'} {delta}
        </p>
      )}
    </div>
  )
}

// Activity item
function ActivityItem({
  action,
  subject,
  time,
  actor,
}: {
  action: string
  subject: string
  time: string
  actor: string
}) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'flex-start',
        gap: '10px',
        padding: '10px 0',
        borderBottom: '1px solid var(--border)',
      }}
    >
      <div
        style={{
          width: '6px',
          height: '6px',
          borderRadius: '50%',
          background: 'var(--accent)',
          flexShrink: 0,
          marginTop: '5px',
        }}
      />
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ fontSize: '13px', color: 'var(--text)', lineHeight: 1.4 }}>
          <span style={{ color: 'var(--text-subtle)', fontWeight: 500 }}>{actor}</span>
          {' '}
          <span style={{ color: 'var(--text-muted)' }}>{action}</span>
          {' '}
          <span style={{ fontWeight: 500 }}>{subject}</span>
        </p>
      </div>
      <p
        style={{
          fontSize: '11px',
          fontFamily: 'var(--font-mono)',
          color: 'var(--text-muted)',
          flexShrink: 0,
          whiteSpace: 'nowrap',
        }}
      >
        {time}
      </p>
    </div>
  )
}

// Status row for system health
function StatusRow({ label, status, detail }: { label: string; status: 'ok' | 'warn' | 'error'; detail: string }) {
  const color = status === 'ok' ? 'var(--success)' : status === 'warn' ? 'var(--warning)' : 'var(--error)'
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '8px 0',
        borderBottom: '1px solid var(--border)',
        gap: '12px',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <div style={{ width: '6px', height: '6px', borderRadius: '50%', background: color, flexShrink: 0 }} />
        <span style={{ fontSize: '13px', color: 'var(--text)' }}>{label}</span>
      </div>
      <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>
        {detail}
      </span>
    </div>
  )
}

export default function DashboardPage() {
  const today = new Date().toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })

  return (
    <div style={{ maxWidth: '1200px', animation: 'slideUp 0.35s var(--ease-out) both' }}>
      {/* Page header */}
      <div className="page-header">
        <div>
          <h1 className="page-title">Dashboard</h1>
          <p className="page-subtitle">DARE Aquatics · Season 2026</p>
        </div>
        <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
          <span
            style={{
              fontSize: '11px',
              fontFamily: 'var(--font-mono)',
              color: 'var(--text-muted)',
              background: 'var(--bg-surface)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--r-sm)',
              padding: '4px 9px',
            }}
          >
            {today}
          </span>
        </div>
      </div>

      {/* Stat grid */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(190px, 1fr))',
          gap: '14px',
          marginBottom: '28px',
        }}
      >
        <StatCard label="Active Swimmers" value="147" delta="12 this season" positive />
        <StatCard label="Households" value="84" delta="3 this week" positive />
        <StatCard label="Upcoming Meets" value="6" />
        <StatCard label="Open Invoices" value="$14,320" mono delta="$2,100 overdue" />
        <StatCard label="Pending Imports" value="2" />
        <StatCard label="Active Groups" value="9" />
      </div>

      {/* Two-column lower section */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: '1fr 280px',
          gap: '16px',
          alignItems: 'start',
        }}
      >
        {/* Recent activity */}
        <div className="card-elevated" style={{ padding: '20px 22px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '16px' }}>
            <p
              style={{
                fontSize: '12px',
                fontWeight: 600,
                fontFamily: 'var(--font-mono)',
                color: 'var(--text-subtle)',
                letterSpacing: '0.06em',
                textTransform: 'uppercase',
              }}
            >
              Recent Activity
            </p>
            <a
              href="/audit"
              style={{
                fontSize: '12px',
                color: 'var(--accent)',
                textDecoration: 'none',
                fontFamily: 'var(--font-mono)',
              }}
            >
              view audit log →
            </a>
          </div>

          <ActivityItem actor="Coach Miller" action="approved entry for" subject="Spring Invitational" time="2m ago" />
          <ActivityItem actor="Admin" action="issued invoice to" subject="Thompson Family" time="14m ago" />
          <ActivityItem actor="System" action="completed import" subject="results.sd3" time="1h ago" />
          <ActivityItem actor="Coach Davis" action="locked commitments for" subject="State Qualifier" time="3h ago" />
          <ActivityItem actor="Admin" action="created meet" subject="Summer Classic 2026" time="5h ago" />
        </div>

        {/* System status */}
        <div className="card-elevated" style={{ padding: '20px 22px' }}>
          <p
            style={{
              fontSize: '12px',
              fontWeight: 600,
              fontFamily: 'var(--font-mono)',
              color: 'var(--text-subtle)',
              letterSpacing: '0.06em',
              textTransform: 'uppercase',
              marginBottom: '14px',
            }}
          >
            System Status
          </p>

          <StatusRow label="API" status="ok" detail="healthy" />
          <StatusRow label="Database" status="ok" detail="16ms p95" />
          <StatusRow label="Worker Queue" status="ok" detail="3 active" />
          <StatusRow label="Storage" status="ok" detail="MinIO" />
          <StatusRow label="Email" status="ok" detail="Postmark" />
          <StatusRow label="Billing" status="ok" detail="Stripe" />

          <p
            style={{
              fontSize: '11px',
              fontFamily: 'var(--font-mono)',
              color: 'var(--text-muted)',
              marginTop: '14px',
              textAlign: 'right',
            }}
          >
            updated just now
          </p>
        </div>
      </div>
    </div>
  )
}
