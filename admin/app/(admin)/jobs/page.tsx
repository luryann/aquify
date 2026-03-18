'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Icon } from '@/components/ui/icons'
import { Tabs } from '@/components/ui/Tabs'
import { EmptyState } from '@/components/ui/EmptyState'

// ─── Types ────────────────────────────────────────────────────────────────────

type QueueStatus = 'healthy' | 'degraded' | 'idle'
type QueueStats = {
  name: string
  displayName: string
  active: number
  pending: number
  completed: number
  failed: number
  retryPolicy: string
  lastRunAt: string | null
  status: QueueStatus
}

type DLQTask = {
  id: string
  taskType: string
  queue: string
  payload: Record<string, string>
  failedAt: string
  attempts: number
  maxAttempts: number
  lastError: string
}

type MetricCounter = {
  taskType: string
  queue: string
  total: number
  succeeded: number
  failed: number
  avgDurationMs: number
}

type RecentExecution = {
  id: string
  taskType: string
  queue: string
  status: 'succeeded' | 'failed'
  durationMs: number
  ranAt: string
}

// ─── Mock Data ─────────────────────────────────────────────────────────────────

const MOCK_QUEUES: QueueStats[] = [
  { name: 'imports',   displayName: 'Imports',    active: 0,  pending: 2,  completed: 48,  failed: 1,  retryPolicy: '5 retries, exp backoff up to 2h', lastRunAt: '2026-03-15T11:22:00Z', status: 'healthy' },
  { name: 'billing',   displayName: 'Billing',    active: 0,  pending: 0,  completed: 124, failed: 0,  retryPolicy: '3 retries, up to 1h',             lastRunAt: '2026-03-15T00:00:05Z', status: 'idle' },
  { name: 'email',     displayName: 'Email',      active: 0,  pending: 0,  completed: 312, failed: 3,  retryPolicy: '3 retries, up to 30m',            lastRunAt: '2026-03-12T07:30:44Z', status: 'healthy' },
  { name: 'reconcile', displayName: 'Reconcile',  active: 0,  pending: 0,  completed: 15,  failed: 0,  retryPolicy: '2 retries, up to 30m',            lastRunAt: '2026-03-15T00:00:10Z', status: 'idle' },
  { name: 'default',   displayName: 'Default',    active: 0,  pending: 1,  completed: 203, failed: 2,  retryPolicy: '3 retries, up to 30m',            lastRunAt: '2026-03-15T14:55:00Z', status: 'healthy' },
]

const MOCK_DLQ: DLQTask[] = [
  {
    id: 'dlq-001', taskType: 'imports:parse', queue: 'imports',
    payload: { importId: 'imp-002', filename: 'badly_formed.cl2' },
    failedAt: '2026-03-13T08:44:00Z', attempts: 5, maxAttempts: 5,
    lastError: 'unexpected EOF at line 204: record type E0 missing required fields',
  },
  {
    id: 'dlq-002', taskType: 'email:campaign_batch', queue: 'email',
    payload: { campaignId: 'camp-old-1', batchIndex: '3' },
    failedAt: '2026-03-10T14:22:00Z', attempts: 3, maxAttempts: 3,
    lastError: 'postmark API rate limit exceeded: 429 Too Many Requests',
  },
  {
    id: 'dlq-003', taskType: 'email:campaign_batch', queue: 'email',
    payload: { campaignId: 'camp-old-2', batchIndex: '1' },
    failedAt: '2026-03-09T09:15:00Z', attempts: 3, maxAttempts: 3,
    lastError: 'postmark API timeout: context deadline exceeded after 30s',
  },
  {
    id: 'dlq-004', taskType: 'integrity:nightly', queue: 'default',
    payload: { runDate: '2026-03-11' },
    failedAt: '2026-03-11T01:05:00Z', attempts: 3, maxAttempts: 3,
    lastError: 'database query timeout: results_history scan exceeded 30s',
  },
  {
    id: 'dlq-005', taskType: 'cleanup:purge_expired', queue: 'default',
    payload: { beforeDate: '2026-02-09' },
    failedAt: '2026-03-09T06:02:00Z', attempts: 3, maxAttempts: 3,
    lastError: 'connection pool exhausted: all 10 connections busy',
  },
]

const MOCK_METRICS: MetricCounter[] = [
  { taskType: 'imports:parse',         queue: 'imports',   total: 49,  succeeded: 48,  failed: 1,  avgDurationMs: 4820 },
  { taskType: 'exports:generate',      queue: 'imports',   total: 22,  succeeded: 22,  failed: 0,  avgDurationMs: 1240 },
  { taskType: 'billing:reconcile',     queue: 'reconcile', total: 15,  succeeded: 15,  failed: 0,  avgDurationMs: 890 },
  { taskType: 'email:campaign_batch',  queue: 'email',     total: 18,  succeeded: 15,  failed: 3,  avgDurationMs: 3200 },
  { taskType: 'integrity:nightly',     queue: 'default',   total: 14,  succeeded: 13,  failed: 1,  avgDurationMs: 12400 },
  { taskType: 'cleanup:purge_expired', queue: 'default',   total: 60,  succeeded: 59,  failed: 1,  avgDurationMs: 3800 },
  { taskType: 'compliance:data_export',queue: 'default',   total: 3,   succeeded: 3,   failed: 0,  avgDurationMs: 7200 },
]

const MOCK_RECENT: RecentExecution[] = [
  { id: 'ex-001', taskType: 'billing:reconcile',    queue: 'reconcile', status: 'succeeded', durationMs: 841,   ranAt: '2026-03-15T00:00:10Z' },
  { id: 'ex-002', taskType: 'integrity:nightly',    queue: 'default',   status: 'succeeded', durationMs: 11820, ranAt: '2026-03-15T01:00:05Z' },
  { id: 'ex-003', taskType: 'cleanup:purge_expired',queue: 'default',   status: 'succeeded', durationMs: 3210,  ranAt: '2026-03-15T06:00:08Z' },
  { id: 'ex-004', taskType: 'imports:parse',         queue: 'imports',   status: 'succeeded', durationMs: 5640,  ranAt: '2026-03-15T11:22:44Z' },
  { id: 'ex-005', taskType: 'email:campaign_batch',  queue: 'email',     status: 'succeeded', durationMs: 2890,  ranAt: '2026-03-12T07:30:44Z' },
]

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString('en-US', {
    month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit', hour12: false,
  })
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60000).toFixed(1)}m`
}

const QUEUE_STATUS_COLORS: Record<QueueStatus, string> = {
  healthy:  'var(--success)',
  degraded: 'var(--error)',
  idle:     'var(--text-muted)',
}

const QUEUE_STATUS_GLOW: Record<QueueStatus, string> = {
  healthy:  '0 0 6px rgba(74, 222, 128, 0.5)',
  degraded: '0 0 6px rgba(248, 113, 113, 0.5)',
  idle:     'none',
}

// ─── Components ───────────────────────────────────────────────────────────────

function QueueCard({ q }: { q: QueueStats }) {
  const color = QUEUE_STATUS_COLORS[q.status]
  const hasWork = q.active > 0 || q.pending > 0
  return (
    <div style={{
      padding: '20px', borderRadius: 10,
      background: 'var(--bg-elevated)', border: '1px solid var(--border)',
    }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 14 }}>
        <div>
          <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--text)', marginBottom: 3 }}>
            {q.displayName}
          </div>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>
            queue/{q.name}
          </span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <span style={{
            width: 7, height: 7, borderRadius: '50%',
            background: color,
            boxShadow: hasWork ? QUEUE_STATUS_GLOW[q.status] : 'none',
            display: 'inline-block',
          }} />
          <span style={{ fontSize: 11, color, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.04em' }}>
            {q.status}
          </span>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 8, marginBottom: 14 }}>
        {[
          { label: 'Active',    value: q.active,    color: q.active > 0 ? 'var(--accent)' : 'var(--text)' },
          { label: 'Pending',   value: q.pending,   color: q.pending > 0 ? 'var(--warning)' : 'var(--text)' },
          { label: 'Done',      value: q.completed, color: 'var(--success)' },
          { label: 'Failed',    value: q.failed,    color: q.failed > 0 ? 'var(--error)' : 'var(--text-muted)' },
        ].map(s => (
          <div key={s.label} style={{ textAlign: 'center', padding: '8px 4px', borderRadius: 6, background: 'var(--bg)', border: '1px solid var(--border)' }}>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 18, fontWeight: 700, color: s.color }}>{s.value}</div>
            <div style={{ fontSize: 10, color: 'var(--text-muted)', marginTop: 2 }}>{s.label}</div>
          </div>
        ))}
      </div>

      <div style={{ borderTop: '1px solid var(--border)', paddingTop: 10, display: 'flex', flexDirection: 'column', gap: 4 }}>
        <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>
          Retry: <span style={{ color: 'var(--text-subtle)' }}>{q.retryPolicy}</span>
        </div>
        {q.lastRunAt && (
          <div style={{ fontSize: 11, color: 'var(--text-muted)' }}>
            Last run: <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-subtle)' }}>{formatDateTime(q.lastRunAt)}</span>
          </div>
        )}
      </div>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function JobsPage() {
  const router = useRouter()
  const [activeTab, setActiveTab] = useState('queues')
  const [retryingId, setRetryingId] = useState<string | null>(null)
  const [dlqItems, setDlqItems] = useState(MOCK_DLQ)

  const totalPending   = MOCK_QUEUES.reduce((s, q) => s + q.pending, 0)
  const totalFailed    = MOCK_QUEUES.reduce((s, q) => s + q.failed, 0)
  const dlqCount       = dlqItems.length
  const healthyQueues  = MOCK_QUEUES.filter(q => q.status === 'healthy').length

  const tabs = [
    { id: 'queues',  label: 'Queues',         count: MOCK_QUEUES.length },
    { id: 'dlq',     label: 'Dead-Letter Queue', count: dlqCount },
    { id: 'metrics', label: 'Metrics' },
  ]

  function handleRetry(id: string) {
    setRetryingId(id)
    setTimeout(() => {
      setDlqItems(prev => prev.filter(t => t.id !== id))
      setRetryingId(null)
    }, 1200)
  }

  function handleDelete(id: string) {
    setDlqItems(prev => prev.filter(t => t.id !== id))
  }

  return (
    <div style={{ padding: '32px 36px', maxWidth: 1300, margin: '0 auto' }}>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 28 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <div style={{ width: 32, height: 32, borderRadius: 8, background: 'var(--accent-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Icon name="cpu" size={16} color="var(--accent)" />
            </div>
            <h1 style={{ fontSize: 22, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--text)', margin: 0 }}>
              Background Jobs
            </h1>
          </div>
          <p style={{ fontSize: 13, color: 'var(--text-muted)', margin: 0 }}>
            {healthyQueues}/{MOCK_QUEUES.length} queues healthy · {totalPending} pending · {dlqCount} in DLQ
          </p>
        </div>
        <button
          onClick={() => router.refresh()}
          style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '8px 14px', borderRadius: 7,
            background: 'var(--bg-elevated)', border: '1px solid var(--border)',
            color: 'var(--text-subtle)', fontSize: 12, fontWeight: 500, cursor: 'pointer',
          }}
        >
          <Icon name="refresh-cw" size={13} /> Refresh
        </button>
      </div>

      {/* Alert banner if DLQ non-empty or failures */}
      {(dlqCount > 0 || totalFailed > 0) && (
        <div style={{
          marginBottom: 20, padding: '12px 16px', borderRadius: 8,
          background: 'var(--error-bg)', border: '1px solid var(--error-border)',
          display: 'flex', alignItems: 'center', gap: 10,
        }}>
          <Icon name="alert-circle" size={15} color="var(--error)" />
          <span style={{ fontSize: 13, color: 'var(--text-subtle)' }}>
            <strong style={{ color: 'var(--error)' }}>{dlqCount} task{dlqCount !== 1 ? 's' : ''}</strong> in the dead-letter queue
            {totalFailed > 0 && <> and <strong style={{ color: 'var(--error)' }}>{totalFailed} failed jobs</strong> across queues</>}.
            {' '}Review the <button onClick={() => setActiveTab('dlq')} style={{ background: 'none', border: 'none', color: 'var(--accent)', fontSize: 13, cursor: 'pointer', padding: 0 }}>Dead-Letter Queue</button> tab.
          </span>
        </div>
      )}

      {/* Tabs */}
      <div style={{ marginBottom: 24 }}>
        <Tabs tabs={tabs} active={activeTab} onChange={setActiveTab} />
      </div>

      {/* ─── Tab: Queues ─────────────────────────────────────────────── */}
      {activeTab === 'queues' && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: 14 }}>
          {MOCK_QUEUES.map(q => <QueueCard key={q.name} q={q} />)}
        </div>
      )}

      {/* ─── Tab: DLQ ────────────────────────────────────────────────── */}
      {activeTab === 'dlq' && (
        dlqItems.length === 0 ? (
          <EmptyState
            icon="check-circle"
            title="Dead-letter queue is empty"
            description="All tasks are processing normally. Nice work."
          />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {dlqItems.map(task => (
              <div key={task.id} style={{
                padding: '18px 20px', borderRadius: 10,
                background: 'var(--bg-elevated)', border: '1px solid var(--border)',
              }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 8 }}>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, fontWeight: 700, color: 'var(--text)' }}>
                        {task.taskType}
                      </span>
                      <span style={{
                        padding: '1px 6px', borderRadius: 3, fontSize: 10, fontWeight: 600,
                        background: 'var(--bg)', border: '1px solid var(--border)',
                        color: 'var(--text-muted)',
                      }}>
                        {task.queue}
                      </span>
                      <span style={{ fontSize: 11, color: 'var(--error)' }}>
                        {task.attempts}/{task.maxAttempts} attempts
                      </span>
                    </div>

                    <div style={{
                      padding: '8px 12px', borderRadius: 6, marginBottom: 10,
                      background: 'var(--error-bg)', border: '1px solid var(--error-border)',
                    }}>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--error)', lineHeight: 1.5 }}>
                        {task.lastError}
                      </span>
                    </div>

                    <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
                      {Object.entries(task.payload).map(([k, v]) => (
                        <span key={k} style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                          <span style={{ fontWeight: 600, color: 'var(--text-subtle)' }}>{k}:</span>{' '}
                          <span style={{ fontFamily: 'var(--font-mono)' }}>{v}</span>
                        </span>
                      ))}
                      <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                        Failed: <span style={{ fontFamily: 'var(--font-mono)' }}>{formatDateTime(task.failedAt)}</span>
                      </span>
                    </div>
                  </div>

                  <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
                    <button
                      onClick={() => handleRetry(task.id)}
                      disabled={retryingId === task.id}
                      style={{
                        display: 'flex', alignItems: 'center', gap: 5,
                        padding: '7px 12px', borderRadius: 6,
                        background: retryingId === task.id ? 'var(--accent-border)' : 'var(--accent)',
                        border: 'none', color: '#fff', fontSize: 12, fontWeight: 600,
                        cursor: retryingId === task.id ? 'not-allowed' : 'pointer',
                        transition: 'background 150ms',
                      }}
                    >
                      <Icon name="refresh-cw" size={12} />
                      {retryingId === task.id ? 'Retrying…' : 'Retry'}
                    </button>
                    <button
                      onClick={() => handleDelete(task.id)}
                      style={{
                        display: 'flex', alignItems: 'center', gap: 5,
                        padding: '7px 12px', borderRadius: 6,
                        background: 'var(--bg)', border: '1px solid var(--border)',
                        color: 'var(--text-muted)', fontSize: 12, cursor: 'pointer',
                      }}
                    >
                      <Icon name="trash-2" size={12} /> Delete
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )
      )}

      {/* ─── Tab: Metrics ────────────────────────────────────────────── */}
      {activeTab === 'metrics' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          {/* Counters table */}
          <div style={{ borderRadius: 10, overflow: 'hidden', border: '1px solid var(--border)', background: 'var(--bg-elevated)' }}>
            <div style={{ padding: '14px 16px', borderBottom: '1px solid var(--border)', background: 'var(--bg)' }}>
              <h3 style={{ margin: 0, fontSize: 13, fontWeight: 600, color: 'var(--text)' }}>Task Counters</h3>
            </div>
            <div>
              {/* Header */}
              <div style={{
                display: 'grid', gridTemplateColumns: '1fr 100px 80px 80px 80px 100px',
                padding: '10px 16px', borderBottom: '1px solid var(--border)',
                background: 'var(--bg)',
              }}>
                {['Task Type', 'Queue', 'Total', 'Succeeded', 'Failed', 'Avg Duration'].map(h => (
                  <span key={h} style={{ fontSize: 11, fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>{h}</span>
                ))}
              </div>
              {MOCK_METRICS.map((m, i) => (
                <div key={m.taskType} style={{
                  display: 'grid', gridTemplateColumns: '1fr 100px 80px 80px 80px 100px',
                  padding: '12px 16px',
                  borderBottom: i < MOCK_METRICS.length - 1 ? '1px solid var(--border)' : 'none',
                }}>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text)' }}>{m.taskType}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>{m.queue}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600, color: 'var(--text)' }}>{m.total}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600, color: 'var(--success)' }}>{m.succeeded}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 600, color: m.failed > 0 ? 'var(--error)' : 'var(--text-muted)' }}>{m.failed}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-subtle)' }}>{formatDuration(m.avgDurationMs)}</span>
                </div>
              ))}
            </div>
          </div>

          {/* Recent executions */}
          <div style={{ borderRadius: 10, overflow: 'hidden', border: '1px solid var(--border)', background: 'var(--bg-elevated)' }}>
            <div style={{ padding: '14px 16px', borderBottom: '1px solid var(--border)', background: 'var(--bg)' }}>
              <h3 style={{ margin: 0, fontSize: 13, fontWeight: 600, color: 'var(--text)' }}>Recent Executions</h3>
            </div>
            {MOCK_RECENT.map((e, i) => (
              <div key={e.id} style={{
                display: 'flex', alignItems: 'center', gap: 16,
                padding: '12px 16px',
                borderBottom: i < MOCK_RECENT.length - 1 ? '1px solid var(--border)' : 'none',
              }}>
                <span style={{
                  width: 7, height: 7, borderRadius: '50%', flexShrink: 0,
                  background: e.status === 'succeeded' ? 'var(--success)' : 'var(--error)',
                }} />
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text)', flex: 1 }}>
                  {e.taskType}
                </span>
                <span style={{ fontSize: 11, color: 'var(--text-muted)', width: 80 }}>{e.queue}</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: e.status === 'succeeded' ? 'var(--success)' : 'var(--error)', width: 70 }}>
                  {formatDuration(e.durationMs)}
                </span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', width: 140 }}>
                  {formatDateTime(e.ranAt)}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
