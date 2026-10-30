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
import { PageSkeleton } from '@/components/ui/PageSkeleton'

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

type JobsSnapshot = {
  tasks: WorkerTaskRecord[]
  queues: QueueStatsRecord[]
  metrics: Record<string, JobMetricsTypeRecord>
  executions: JobExecutionRecord[]
  dlq: WorkerTaskRecord[]
}

async function loadSnapshot(api: ReturnType<typeof useApiClient>, orgId: string): Promise<JobsSnapshot> {
  const [tasks, queues, metricSnapshot, dlq] = await Promise.all([
    fetchJobs(api, orgId),
    fetchQueueStats(api, orgId),
    fetchJobMetrics(api, orgId),
    fetchDLQTasks(api, orgId),
  ])
  return { tasks, queues, metrics: metricSnapshot.by_type, executions: metricSnapshot.recent_executions, dlq }
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

  function applySnapshot(snapshot: JobsSnapshot) {
    setTasks(snapshot.tasks)
    setQueues(snapshot.queues)
    setMetrics(snapshot.metrics)
    setExecutions(snapshot.executions)
    setDlq(snapshot.dlq)
  }

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
        const snapshot = await loadSnapshot(api, orgId)
        if (cancelled) return
        applySnapshot(snapshot)
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
      applySnapshot(await loadSnapshot(api, orgId))
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
    return <div className="u-p-32px" ><EmptyState icon="activity" title="Sign in required" description="Authenticate to inspect live job queues." /></div>
  }

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">
            Background Jobs
          </h1>
        </div>
        <button type="button"
          onClick={() => void refreshAll()}
          className="u-p-8px-16px u-fs-fs-sm" style={{ borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', cursor: 'pointer' }}
        >
          Refresh
        </button>
      </div>

      {error ? (
        <div className="u-mb-20px u-p-12px-16px u-fs-fs-sm" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <PageSkeleton variant="table" noHeader rows={4} />
      ) : (
        <div className="u-gap-24px" style={{ display: 'flex', flexDirection: 'column' }}>
          <div className="cols-4 u-gap-12px" >
            {[
              { label: 'Tracked tasks', value: String(tasks.length) },
              { label: 'Active or Pending', value: String(activeTasks.length) },
              { label: 'Queues', value: String(queues.length) },
              { label: 'Failed tasks', value: String(dlq.length) },
            ].map((stat) => (
              <div key={stat.label} className="card-elevated u-p-16px-16px" >
                <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{stat.label}</div>
                <div className="u-fs-fs-2xl u-ff-font-body" style={{ color: 'var(--text)' }}>{stat.value}</div>
              </div>
            ))}
          </div>

          <section className="card" style={{ overflow: 'hidden' }}>
            <div className="u-p-16px-20px" style={{ borderBottom: '1px solid var(--border)' }}>
              <h2 className="card-title">Queue depth</h2>
            </div>
            {queues.length === 0 ? (
              <div className="u-p-20px" ><EmptyState icon="activity" title="No queues reported" description="The live worker server returned no queue statistics." /></div>
            ) : (
              <div className="table-scroll" style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--border)' }}>
                      {['Queue', 'Pending', 'Active', 'Completed', 'Failed', 'Out of retries'].map((header) => (
                        <th key={header} className="col-head u-p-12px-16px" style={{ textAlign: 'left' }}>{header}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {queues.map((queue) => (
                      <tr key={queue.name} style={{ borderBottom: '1px solid var(--border)' }}>
                        <td className="u-p-12px-16px u-ff-font-body" style={{ color: 'var(--text)' }}>{queue.name}</td>
                        <td className="u-p-12px-16px" >{queue.pending}</td>
                        <td className="u-p-12px-16px" >{queue.active}</td>
                        <td className="u-p-12px-16px" >{queue.completed}</td>
                        <td className="u-p-12px-16px" >{queue.failed}</td>
                        <td className="u-p-12px-16px" >{queue.dead_letter}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <section className="card" style={{ overflow: 'hidden' }}>
            <div className="u-p-16px-20px" style={{ borderBottom: '1px solid var(--border)' }}>
              <h2 className="card-title">Active tasks</h2>
            </div>
            {activeTasks.length === 0 ? (
              <div className="u-p-20px" ><EmptyState icon="activity" title="No active work" description="There are no pending or retrying tasks right now." /></div>
            ) : (
              <div className="table-scroll" style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--border)' }}>
                      {['Task ID', 'Type', 'Queue', 'Status', 'Enqueued', 'Last Error'].map((header) => (
                        <th key={header} className="col-head u-p-12px-16px" style={{ textAlign: 'left' }}>{header}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {activeTasks.map((task) => (
                      <tr key={task.id} style={{ borderBottom: '1px solid var(--border)' }}>
                        <td className="u-p-12px-16px u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--text)' }}>{task.id}</td>
                        <td className="u-p-12px-16px u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--accent-text, var(--accent))' }}>{task.type}</td>
                        <td className="u-p-12px-16px u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--text-subtle)' }}>{task.queue}</td>
                        <td className="u-p-12px-16px" >{task.status}</td>
                        <td className="u-p-12px-16px u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{formatDateTime(task.enqueued_at)}</td>
                        <td className="u-p-12px-16px u-fs-fs-sm" style={{ color: task.last_error ? 'var(--error)' : 'var(--text-muted)' }}>{task.last_error || '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <div className="cols-2 u-gap-20px" >
            <section className="card" style={{ overflow: 'hidden' }}>
              <div className="u-p-16px-20px" style={{ borderBottom: '1px solid var(--border)' }}>
                <h2 className="card-title">Metrics by type</h2>
              </div>
              {metricRows.length === 0 ? (
                <div className="u-p-20px" ><EmptyState icon="bar-chart-2" title="No metrics yet" description="Worker metrics will appear here after queue activity." /></div>
              ) : (
                <div className="table-scroll" style={{ overflowX: 'auto' }}>
                  <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                    <thead>
                      <tr style={{ borderBottom: '1px solid var(--border)' }}>
                        {['Type', 'Enqueued', 'Succeeded', 'Failed', 'Out of retries'].map((header) => (
                          <th key={header} className="col-head u-p-12px-16px" style={{ textAlign: 'left' }}>{header}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {metricRows.map(([taskType, snapshot]) => (
                        <tr key={taskType} style={{ borderBottom: '1px solid var(--border)' }}>
                          <td className="u-p-12px-16px u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--text)' }}>{taskType}</td>
                          <td className="u-p-12px-16px" >{snapshot.enqueued}</td>
                          <td className="u-p-12px-16px" >{snapshot.succeeded}</td>
                          <td className="u-p-12px-16px" >{snapshot.failed}</td>
                          <td className="u-p-12px-16px" >{snapshot.dlq}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>

            <section className="card" style={{ overflow: 'hidden' }}>
              <div className="u-p-16px-20px" style={{ borderBottom: '1px solid var(--border)' }}>
                <h2 className="card-title">Recent executions</h2>
              </div>
              {executions.length === 0 ? (
                <div className="u-p-20px" ><EmptyState icon="activity" title="No execution history" description="Recent worker executions will appear after jobs run." /></div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column' }}>
                  {executions.slice(0, 12).map((execution, index) => (
                    <div key={`${execution.task_id}-${index}`} className="u-p-12px-16px" style={{ borderBottom: index < Math.min(executions.length, 12) - 1 ? '1px solid var(--border)' : 'none' }}>
                      <div className="u-gap-12px u-mb-4px" style={{ display: 'flex', justifyContent: 'space-between' }}>
                        <span className="u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--accent-text, var(--accent))' }}>{execution.task_type}</span>
                        <span className="u-fs-fs-xs" style={{ color: execution.status === 'failed' ? 'var(--error)' : 'var(--text-subtle)' }}>{execution.status}</span>
                      </div>
                      <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>{execution.task_id}</div>
                      <div className="u-fs-fs-xs u-mt-4px" style={{ color: 'var(--text-muted)' }}>
                        {formatDateTime(execution.started_at)} → {formatDateTime(execution.finished_at)}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </section>
          </div>

          <section className="card" style={{ overflow: 'hidden' }}>
            <div className="u-p-16px-20px" style={{ borderBottom: '1px solid var(--border)' }}>
              <h2 className="card-title">Failed tasks</h2>
            </div>
            {dlq.length === 0 ? (
              <div className="u-p-20px" ><EmptyState icon="shield-check" title="No failed tasks" description="Nothing is waiting for a retry or removal." /></div>
            ) : (
              <div className="table-scroll" style={{ overflowX: 'auto' }}>
                <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                  <thead>
                    <tr style={{ borderBottom: '1px solid var(--border)' }}>
                      {['Task ID', 'Type', 'Retries', 'Last Error', 'Actions'].map((header) => (
                        <th key={header} className="col-head u-p-12px-16px" style={{ textAlign: 'left' }}>{header}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {dlq.map((task) => (
                      <tr key={task.id} style={{ borderBottom: '1px solid var(--border)' }}>
                        <td className="u-p-12px-16px u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--text)' }}>{task.id}</td>
                        <td className="u-p-12px-16px u-ff-font-body u-fs-fs-xs" style={{ color: 'var(--accent-text, var(--accent))' }}>{task.type}</td>
                        <td className="u-p-12px-16px" >{task.retried}/{task.max_retries}</td>
                        <td className="u-p-12px-16px u-fs-fs-sm" style={{ color: 'var(--error)' }}>{task.last_error || '—'}</td>
                        <td className="u-p-12px-16px" >
                          <div className="u-gap-8px" style={{ display: 'flex', flexWrap: 'wrap' }}>
                            <button type="button"
                              onClick={() => void handleRetry(task.id)}
                              disabled={workingTask === task.id}
                              className="u-p-6px-12px u-fs-fs-sm" style={{ borderRadius: '6px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', cursor: workingTask === task.id ? 'not-allowed' : 'pointer' }}
                            >
                              Retry
                            </button>
                            <button type="button"
                              onClick={() => void handleDelete(task.id)}
                              disabled={workingTask === task.id}
                              className="u-p-6px-12px u-fs-fs-sm" style={{ borderRadius: '6px', border: '1px solid var(--error-border)', background: 'var(--error-bg)', color: 'var(--error)', cursor: workingTask === task.id ? 'not-allowed' : 'pointer' }}
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
