'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import { fetchAuditEvents, fetchUsers, type AuditEventRecord, type UserRecord } from '@/lib/adminData'

function displayNameForUser(user: UserRecord): string {
  const name = `${user.firstName ?? ''} ${user.lastName ?? ''}`.trim()
  return name || user.email || user.id
}

function formatDateTime(value: string) {
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

function family(action: string) {
  return action.split('.')[0] ?? action
}

const FAMILY_COLOR: Record<string, string> = {
  auth: 'var(--info)',
  billing: 'var(--warning)',
  meet: 'var(--accent)',
  performance: 'var(--success)',
  import: 'var(--text-subtle)',
  members: 'var(--text)',
  compliance: 'var(--error)',
  report: 'var(--accent)',
}

export default function AuditLogPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [events, setEvents] = useState<AuditEventRecord[]>([])
  const [actorNames, setActorNames] = useState<Record<string, string>>({})
  const [search, setSearch] = useState('')
  const [actionFilter, setActionFilter] = useState('')
  const [resourceFilter, setResourceFilter] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId) {
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const response = await fetchAuditEvents(api, session.user.orgId, { limit: '200' })
        if (!cancelled) setEvents(response.events)

        // Best-effort actor-name resolution so the log reads as "who did what"
        // instead of raw UUIDs. Failure here shouldn't block the audit log itself.
        try {
          const { users } = await fetchUsers(api, session.user.orgId, { limit: 200 })
          if (!cancelled) {
            setActorNames(Object.fromEntries(users.map((u) => [u.id, displayNameForUser(u)])))
          }
        } catch {
          // Name resolution is a display nicety; leave actorNames empty and fall
          // back to raw IDs rather than failing the whole page load.
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof ApiError ? err.message : 'Unable to load audit events.')
          setEvents([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
    }
  }, [api, session?.user.orgId])

  const filtered = useMemo(() => events.filter((event) => {
    const query = search.trim().toLowerCase()
    if (actionFilter && !event.action.startsWith(actionFilter)) return false
    if (resourceFilter && event.resource_type !== resourceFilter) return false
    if (!query) return true
    return [
      event.action,
      event.resource_type,
      event.resource_id,
      event.actor_user_id,
      actorNames[event.actor_user_id] ?? '',
      JSON.stringify(event.metadata ?? {}),
    ].join(' ').toLowerCase().includes(query)
  }), [actionFilter, actorNames, events, resourceFilter, search])

  const families = Array.from(new Set(events.map((event) => family(event.action)))).sort()
  const resources = Array.from(new Set(events.map((event) => event.resource_type))).sort()

  if (!session) {
    return <div style={{ padding: '32px' }}><EmptyState icon="shield-check" title="Sign in required" description="Authenticate to view the live audit log." /></div>
  }

  return (
    <div style={{ padding: '32px 36px 80px', maxWidth: 1260, margin: '0 auto' }}>
      <div style={{ marginBottom: '28px' }}>
        <p style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', letterSpacing: '0.08em', textTransform: 'uppercase', marginBottom: '6px' }}>
          Compliance
        </p>
        <h1 className="page-title">
          Audit Log
        </h1>
        <p className="page-subtitle">
          Immutable privileged-action history returned by the live audit event endpoint.
        </p>
      </div>

      <div style={{ display: 'flex', gap: '10px', marginBottom: '18px', flexWrap: 'wrap', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: '240px', maxWidth: '340px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search actor, action, resource…"
            style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px', outline: 'none' }}
          />
        </div>
        <select value={actionFilter} onChange={(event) => setActionFilter(event.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px' }}>
          <option value="">All action families</option>
          {families.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <select value={resourceFilter} onChange={(event) => setResourceFilter(event.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', color: 'var(--text)', fontSize: '13px' }}>
          <option value="">All resources</option>
          {resources.map((item) => <option key={item} value={item}>{item}</option>)}
        </select>
        <span style={{ marginLeft: 'auto', fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-body)' }}>{filtered.length} events</span>
      </div>

      {error ? (
        <div style={{ marginBottom: '20px', padding: '12px 16px', borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)', fontSize: '12px' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <EmptyState icon="shield-check" title="Loading audit events" description="Querying the live audit log for this organisation." />
      ) : filtered.length === 0 ? (
        <EmptyState icon="shield-check" title="No audit events found" description="Try widening the filters or generate activity in the admin console." />
      ) : (
        <div className="card" style={{ overflow: 'hidden' }}>
          {filtered.map((event, index) => {
            const actionFamily = family(event.action)
            const accent = FAMILY_COLOR[actionFamily] ?? 'var(--text-subtle)'
            const isExpanded = expanded === event.id
            return (
              <div key={event.id} style={{ borderBottom: index < filtered.length - 1 ? '1px solid var(--border)' : 'none' }}>
                <button
                  onClick={() => setExpanded(isExpanded ? null : event.id)}
                  style={{ width: '100%', padding: '16px 18px', background: 'transparent', border: 'none', textAlign: 'left', cursor: 'pointer' }}
                >
                  <div style={{ display: 'grid', gridTemplateColumns: '220px 1fr 220px', gap: '16px', alignItems: 'start' }}>
                    <div>
                      <div style={{ fontSize: '12px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginBottom: '6px' }}>{formatDateTime(event.occurred_at)}</div>
                      <div style={{ fontSize: '12px', color: 'var(--text-subtle)' }}>
                        {actorNames[event.actor_user_id] ?? `${event.actor_user_id.slice(0, 8)}…`}
                      </div>
                    </div>
                    <div>
                      <div style={{ display: 'flex', gap: '8px', alignItems: 'center', flexWrap: 'wrap', marginBottom: '6px' }}>
                        <span className="badge" style={{ color: accent, background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                          {event.action}
                        </span>
                        <span style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)' }}>
                          {event.resource_type}
                        </span>
                      </div>
                      <div style={{ fontSize: '13px', color: 'var(--text)' }}>
                        Resource <code style={{ fontFamily: 'var(--font-body)' }}>{event.resource_id}</code>
                      </div>
                    </div>
                    <div style={{ textAlign: 'right', color: 'var(--text-muted)', fontSize: '12px' }}>
                      {isExpanded ? 'Hide details' : 'View metadata'}
                    </div>
                  </div>
                </button>

                {isExpanded ? (
                  <div style={{ padding: '0 18px 16px 18px', background: 'var(--bg-surface)' }}>
                    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: '14px' }}>
                      <div className="card-elevated" style={{ padding: '12px 14px' }}>
                        <div style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>Roles</div>
                        <div style={{ fontSize: '12px', color: 'var(--text)' }}>{event.actor_roles.join(', ') || '—'}</div>
                      </div>
                      <div className="card-elevated" style={{ padding: '12px 14px' }}>
                        <div style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>Metadata</div>
                        <pre style={{ margin: 0, fontSize: '11px', color: 'var(--text-subtle)', fontFamily: 'var(--font-body)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                          {JSON.stringify(event.metadata ?? {}, null, 2)}
                        </pre>
                      </div>
                      <div className="card-elevated" style={{ padding: '12px 14px' }}>
                        <div style={{ fontSize: '10px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.08em', marginBottom: '6px' }}>State Change</div>
                        <pre style={{ margin: 0, fontSize: '11px', color: 'var(--text-subtle)', fontFamily: 'var(--font-body)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                          {JSON.stringify({ before: event.before_state ?? null, after: event.after_state ?? null }, null, 2)}
                        </pre>
                      </div>
                    </div>
                  </div>
                ) : null}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
