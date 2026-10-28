'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { AuthGuard } from '@/components/auth/AuthGuard'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  deleteDLQTask,
  fetchDLQTasks,
  fetchJobMetrics,
  fetchJobs,
  fetchQueueStats,
  retryDLQTask,
  type JobExecutionRecord,
  type JobMetricsTypeRecord,
  type QueueStatsRecord,
  type WorkerTaskRecord,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

function formatDateTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}

export default function JobsPage() {
  return (
    <AuthGuard requiredRoles={['admin']}>
      <JobsPageContent />
    </AuthGuard>
  )
}

function JobsPageContent() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const [tasks, setTasks] = useState<WorkerTaskRecord[]>([])
  const [queues, setQueues] = useState<QueueStatsRecord[]>([])
  const [metrics, setMetrics] = useState<Record<string, JobMetricsTypeRecord>>({})
  const [executions, setExecutions] = useState<JobExecutionRecord[]>([])
  const [dlq, setDlq] = useState<WorkerTaskRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [workingTask, setWorkingTask] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session) {
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const [jobList, queueStats, metricSnapshot, dlqTasks] = await Promise.all([
          fetchJobs(api, orgId),
          fetchQueueStats(api, orgId),
          fetchJobMetrics(api, orgId),
          fetchDLQTasks(api, orgId),
        ])
        if (cancelled) return
        setTasks(jobList)
        setQueues(queueStats)
        setMetrics(metricSnapshot.by_type)
        setExecutions(metricSnapshot.recent_executions)
        setDlq(dlqTasks)
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load background jobs. Try again.'))
          setTasks([])
          setQueues([])
          setMetrics({})
          setExecutions([])
          setDlq([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, orgId, session])

  const activeTasks = useMemo(() => tasks.filter((task) => ['pending', 'active', 'retrying'].includes(task.status)), [tasks])
  const metricRows = useMemo(() => Object.entries(metrics).sort(([a], [b]) => a.localeCompare(b)), [metrics])

  async function refreshAll() {
    try {
      const [jobList, queueStats, metricSnapshot, dlqTasks] = await Promise.all([
        fetchJobs(api, orgId),
        fetchQueueStats(api, orgId),
        fetchJobMetrics(api, orgId),
        fetchDLQTasks(api, orgId),
      ])
      setTasks(jobList)
      setQueues(queueStats)
      setMetrics(metricSnapshot.by_type)
      setExecutions(metricSnapshot.recent_executions)
      setDlq(dlqTasks)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t refresh background jobs. Try again.'))
    }
  }

  async function handleRetry(taskId: string) {
    setWorkingTask(taskId)
    setError(null)
    try {
      await retryDLQTask(api, orgId, taskId)
      await refreshAll()
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t retry failed task. Try again.'))
    } finally {
      setWorkingTask(null)
    }
  }

  async function handleDelete(taskId: string) {
    setWorkingTask(taskId)
    setError(null)
    try {
      await deleteDLQTask(api, orgId, taskId)
      await refreshAll()
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t delete failed task. Try again.'))
    } finally {
      setWorkingTask(null)
    }
  }

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="activity" title="Sign in required" description="Authenticate to inspect live job queues." /></div>
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">
            Background Jobs
          </h1>
        </div>
        <button
          onClick={() => void refreshAll()}
          style={{ padding: '8px 16px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: 'var(--fs-sm)', cursor: 'pointer' }}
        >
          Refresh
        </button>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: 'var(--fs-sm)' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <EmptyState icon="activity" title="Loading jobs" description="Reading live queue state and recent executions." />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
          <div className="cols-4" style={{ gap: '12px' }}>
            {[
              { label: 'Tracked tasks', value: String(tasks.length) },
              { label: 'Active or Pending', value: String(activeTasks.length) },
              { label: 'Queues', value: String(queues.length) },
              { label: 'Failed tasks', value: String(dlq.length) },
            ].map((stat) => (
              <div key={stat.label} className="card-elevated" style={{ padding: '16px 16px' }}>
                <div style={{ fontSize: 'var(--fs-sm)', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '6px' }}>{stat.label}</div>
                <div style={{ fontSize: 'var(--fs-2xl)', fontFamily: 'var(--font-body)', color: 'var(--text)' }}>{stat.value}</div>
              </div>
            ))}
          </div>

          <section className="card" style={{ overflow: 'hidden' }}>
            <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--border)' }}>
              <h2 className="card-title">Queue depth</h2>
            </div>
            {queues.length === 0 ? (
              <div style={{ padding: '20px' }}><EmptyState icon="activity" title="No queues reported" description="The live worker server returned no queue statistics." /></div>
            ) : (
              <div style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--border)' }}>
                      {['Queue', 'Pending', 'Active', 'Completed', 'Failed', 'Out of retries'].map((header) => (
                        <th key={header} className="col-head" style={{ padding: '12px 16px', textAlign: 'left' }}>{header}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {queues.map((queue) => (
                      <tr key={queue.name} style={{ borderBottom: '1px solid var(--border)' }}>
                        <td style={{ padding: '12px 16px', fontFamily: 'var(--font-body)', color: 'var(--text)' }}>{queue.name}</td>
                        <td style={{ padding: '12px 16px' }}>{queue.pending}</td>
                        <td style={{ padding: '12px 16px' }}>{queue.active}</td>
                        <td style={{ padding: '12px 16px' }}>{queue.completed}</td>
                        <td style={{ padding: '12px 16px' }}>{queue.failed}</td>
                        <td style={{ padding: '12px 16px' }}>{queue.dead_letter}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <section className="card" style={{ overflow: 'hidden' }}>
            <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--border)' }}>
              <h2 className="card-title">Active tasks</h2>
            </div>
            {activeTasks.length === 0 ? (
              <div style={{ padding: '20px' }}><EmptyState icon="activity" title="No active work" description="There are no pending or retrying tasks right now." /></div>
            ) : (
              <div style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--border)' }}>
                      {['Task ID', 'Type', 'Queue', 'Status', 'Enqueued', 'Last Error'].map((header) => (
                        <th key={header} className="col-head" style={{ padding: '12px 16px', textAlign: 'left' }}>{header}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {activeTasks.map((task) => (
                      <tr key={task.id} style={{ borderBottom: '1px solid var(--border)' }}>
                        <td style={{ padding: '12px 16px', fontFamily: 'var(--font-body)', fontSize: 'var(--fs-xs)', color: 'var(--text)' }}>{task.id}</td>
                        <td style={{ padding: '12px 16px', fontFamily: 'var(--font-body)', fontSize: 'var(--fs-xs)', color: 'var(--accent)' }}>{task.type}</td>
                        <td style={{ padding: '12px 16px', fontFamily: 'var(--font-body)', fontSize: 'var(--fs-xs)', color: 'var(--text-subtle)' }}>{task.queue}</td>
                        <td style={{ padding: '12px 16px' }}>{task.status}</td>
                        <td style={{ padding: '12px 16px', fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>{formatDateTime(task.enqueued_at)}</td>
                        <td style={{ padding: '12px 16px', fontSize: 'var(--fs-sm)', color: task.last_error ? 'var(--error)' : 'var(--text-muted)' }}>{task.last_error || '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <div className="cols-2" style={{ gap: '20px' }}>
            <section className="card" style={{ overflow: 'hidden' }}>
              <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--border)' }}>
                <h2 className="card-title">Metrics by type</h2>
              </div>
              {metricRows.length === 0 ? (
                <div style={{ padding: '20px' }}><EmptyState icon="bar-chart-2" title="No metrics yet" description="Worker metrics will appear here after queue activity." /></div>
              ) : (
                <div style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                    <thead>
                      <tr style={{ borderBottom: '1px solid var(--border)' }}>
                        {['Type', 'Enqueued', 'Succeeded', 'Failed', 'Out of retries'].map((header) => (
                          <th key={header} className="col-head" style={{ padding: '12px 16px', textAlign: 'left' }}>{header}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {metricRows.map(([taskType, snapshot]) => (
                        <tr key={taskType} style={{ borderBottom: '1px solid var(--border)' }}>
                          <td style={{ padding: '12px 16px', fontFamily: 'var(--font-body)', fontSize: 'var(--fs-xs)', color: 'var(--text)' }}>{taskType}</td>
                          <td style={{ padding: '12px 16px' }}>{snapshot.enqueued}</td>
                          <td style={{ padding: '12px 16px' }}>{snapshot.succeeded}</td>
                          <td style={{ padding: '12px 16px' }}>{snapshot.failed}</td>
                          <td style={{ padding: '12px 16px' }}>{snapshot.dlq}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>

            <section className="card" style={{ overflow: 'hidden' }}>
              <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--border)' }}>
                <h2 className="card-title">Recent executions</h2>
              </div>
              {executions.length === 0 ? (
                <div style={{ padding: '20px' }}><EmptyState icon="activity" title="No execution history" description="Recent worker executions will appear after jobs run." /></div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column' }}>
                  {executions.slice(0, 12).map((execution, index) => (
                    <div key={`${execution.task_id}-${index}`} style={{ padding: '12px 16px', borderBottom: index < Math.min(executions.length, 12) - 1 ? '1px solid var(--border)' : 'none' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', gap: '12px', marginBottom: '4px' }}>
                        <span style={{ fontFamily: 'var(--font-body)', fontSize: 'var(--fs-xs)', color: 'var(--accent)' }}>{execution.task_type}</span>
                        <span style={{ fontSize: 'var(--fs-xs)', color: execution.status === 'failed' ? 'var(--error)' : 'var(--text-subtle)' }}>{execution.status}</span>
                      </div>
                      <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>{execution.task_id}</div>
                      <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', marginTop: '4px' }}>
                        {formatDateTime(execution.started_at)} → {formatDateTime(execution.finished_at)}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </section>
          </div>

          <section className="card" style={{ overflow: 'hidden' }}>
            <div style={{ padding: '16px 20px', borderBottom: '1px solid var(--border)' }}>
              <h2 className="card-title">Failed tasks</h2>
            </div>
            {dlq.length === 0 ? (
              <div style={{ padding: '20px' }}><EmptyState icon="shield-check" title="No failed tasks" description="Nothing is waiting for a retry or removal." /></div>
            ) : (
              <div style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--border)' }}>
                      {['Task ID', 'Type', 'Retries', 'Last Error', 'Actions'].map((header) => (
                        <th key={header} className="col-head" style={{ padding: '12px 16px', textAlign: 'left' }}>{header}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {dlq.map((task) => (
                      <tr key={task.id} style={{ borderBottom: '1px solid var(--border)' }}>
                        <td style={{ padding: '12px 16px', fontFamily: 'var(--font-body)', fontSize: 'var(--fs-xs)', color: 'var(--text)' }}>{task.id}</td>
                        <td style={{ padding: '12px 16px', fontFamily: 'var(--font-body)', fontSize: 'var(--fs-xs)', color: 'var(--accent)' }}>{task.type}</td>
                        <td style={{ padding: '12px 16px' }}>{task.retried}/{task.max_retries}</td>
                        <td style={{ padding: '12px 16px', color: 'var(--error)', fontSize: 'var(--fs-sm)' }}>{task.last_error || '—'}</td>
                        <td style={{ padding: '12px 16px' }}>
                          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                            <button
                              onClick={() => void handleRetry(task.id)}
                              disabled={workingTask === task.id}
                              style={{ padding: '6px 12px', borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: 'var(--fs-sm)', cursor: workingTask === task.id ? 'not-allowed' : 'pointer' }}
                            >
                              Retry
                            </button>
                            <button
                              onClick={() => void handleDelete(task.id)}
                              disabled={workingTask === task.id}
                              style={{ padding: '6px 12px', borderRadius: '6px', border: '1px solid var(--error-border)', background: 'var(--error-bg)', color: 'var(--error)', fontSize: 'var(--fs-sm)', cursor: workingTask === task.id ? 'not-allowed' : 'pointer' }}
                            >
                              Delete
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
        </div>
      )}
    </div>
  )
}
