'use client'

import { useEffect, useMemo, useState } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { displayNameForUser, fetchAuditEvents, fetchUsers, type AuditEventRecord } from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { PageSkeleton } from '@/components/ui/PageSkeleton'

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

const FAMILY_LABEL: Record<string, string> = {
  auth: 'Sign-ins',
  billing: 'Billing',
  meet: 'Meets',
  performance: 'Performance',
  import: 'Imports',
  members: 'Members',
  compliance: 'Compliance',
  report: 'Reports',
  comms: 'Messages & Announcements',
  season: 'Seasons',
  practice: 'Practices',
}

function familyLabel(value: string) {
  return FAMILY_LABEL[value] ?? value
}

const RESOURCE_LABEL: Record<string, string> = {
  keycloak_user: 'Sign-in account',
  communication_preferences: 'Notification preferences',
  message: 'Message',
  practice_attendance: 'Practice attendance',
  report: 'Report',
  season: 'Season',
  swimmer: 'Swimmer',
}

function resourceLabel(value: string) {
  return RESOURCE_LABEL[value] ?? value
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
        // Best-effort actor-name resolution so the log reads as "who did what"
        // instead of raw UUIDs. Failure here shouldn't block the audit log itself.
        const usersRequest = fetchUsers(api, session.user.orgId, { limit: 200 }).catch(() => null)
        const response = await fetchAuditEvents(api, session.user.orgId, { limit: '200' })
        if (!cancelled) setEvents(response.events)
        const resolved = await usersRequest
        if (!cancelled && resolved) {
          setActorNames(Object.fromEntries(resolved.users.map((u) => [u.id, displayNameForUser(u)])))
        }
      } catch (err) {
        if (!cancelled) {
          setError(uiErrorMessage(err, 'Couldn’t load audit events. Try again.'))
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

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    return events.filter((event) => {
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
    })
  }, [actionFilter, actorNames, events, resourceFilter, search])

  const families = Array.from(new Set(events.map((event) => family(event.action)))).sort()
  const resources = Array.from(new Set(events.map((event) => event.resource_type))).sort()

  if (!session) {
    return <div className="u-p-32px" ><EmptyState icon="shield-check" title="Sign in required" description="Authenticate to view the live audit log." /></div>
  }

  return (
    <div className="page">
      <div className="page-heading">
        <h1 className="page-title">
          Audit Log
        </h1>
      </div>

      <div className="u-gap-12px u-mb-20px" style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, minWidth: '240px', maxWidth: '340px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input aria-label="Search actor, action, resource"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search actor, action, resource…"
            className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }}
          />
        </div>
        <select value={actionFilter} onChange={(event) => setActionFilter(event.target.value)} className="u-p-8px-12px u-fs-fs-base" >
          <option value="">All categories</option>
          {families.map((item) => <option key={item} value={item}>{familyLabel(item)}</option>)}
        </select>
        <select value={resourceFilter} onChange={(event) => setResourceFilter(event.target.value)} className="u-p-8px-12px u-fs-fs-base" >
          <option value="">All types</option>
          {resources.map((item) => <option key={item} value={item}>{resourceLabel(item)}</option>)}
        </select>
        <span className="u-ml-auto u-fs-fs-sm u-ff-font-body" style={{ color: 'var(--text-muted)' }}>{filtered.length} events</span>
      </div>

      {error ? (
        <div className="u-mb-20px u-p-12px-16px u-fs-fs-sm" style={{ borderRadius: '8px', background: 'var(--error-bg)', border: '1px solid var(--error-border)', color: 'var(--error)' }}>
          {error}
        </div>
      ) : null}

      {loading ? (
        <PageSkeleton variant="table" noHeader rows={4} />
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
                  className="u-p-16px-20px" style={{ width: '100%', background: 'transparent', border: 'none', textAlign: 'left', cursor: 'pointer' }}
                >
                  <div className="audit-row">
                    <div>
                      <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>{formatDateTime(event.occurred_at)}</div>
                      <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
                        {actorNames[event.actor_user_id] ?? `${event.actor_user_id.slice(0, 8)}…`}
                      </div>
                    </div>
                    <div style={{ minWidth: 0, overflowWrap: 'anywhere' }}>
                      <div className="u-gap-8px u-mb-6px" style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
                        <span className="badge" style={{ color: accent, background: 'var(--bg-surface)', border: '1px solid var(--border)' }}>
                          {event.action}
                        </span>
                        <span className="u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-muted)' }}>
                          {resourceLabel(event.resource_type)}
                        </span>
                      </div>
                      <div className="u-fs-fs-base" style={{ color: 'var(--text)' }}>
                        Resource <code className="u-ff-font-body" >{event.resource_id}</code>
                      </div>
                    </div>
                    <div className="u-fs-fs-sm" style={{ textAlign: 'right', color: 'var(--text-muted)' }}>
                      {isExpanded ? 'Hide details' : 'View metadata'}
                    </div>
                  </div>
                </button>

                {isExpanded ? (
                  <div style={{ padding: '0 20px 16px 20px', background: 'var(--bg-surface)' }}>
                    <div className="u-gap-16px" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
                      <div className="card-elevated u-p-12px-16px" >
                        <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>Roles</div>
                        <div className="u-fs-fs-sm" style={{ color: 'var(--text)' }}>{event.actor_roles.join(', ') || '—'}</div>
                      </div>
                      <div className="card-elevated u-p-12px-16px" >
                        <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>Metadata</div>
                        <pre className="u-m-0 u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-subtle)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                          {JSON.stringify(event.metadata ?? {}, null, 2)}
                        </pre>
                      </div>
                      <div className="card-elevated u-p-12px-16px" >
                        <div className="u-fs-fs-sm u-ff-font-body u-mb-6px" style={{ color: 'var(--text-muted)' }}>State change</div>
                        <pre className="u-m-0 u-fs-fs-xs u-ff-font-body" style={{ color: 'var(--text-subtle)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
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
