'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import {
  fetchSwimmerLinkRequests,
  resolveSwimmerLinkRequest,
  fetchSwimmers,
  type SwimmerLinkRequestRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'

type PendingUser = {
  id: string
  email: string
  firstName: string
  lastName: string
  enabled: boolean
  username: string
  emailVerified: boolean
  attributes?: {
    org_id?: string[]
    registration_status?: string[]
    requested_role?: string[]
    registration_club_code?: string[]
    registration_note?: string[]
    registration_submitted_at?: string[]
    registration_swimmer_name?: string[]
  }
}

function orgHeaders(orgId: string) { return { 'X-Org-Id': orgId } }

function attr(user: PendingUser, key: keyof NonNullable<PendingUser['attributes']>) {
  return user.attributes?.[key]?.[0] ?? ''
}

function requestedRoleLabel(role: string): string {
  switch (role) {
    case 'parent':
      return 'Parent / Guardian'
    case 'coach':
      return 'Coach'
    default:
      return role || 'Unspecified'
  }
}

function SwimmerLinkRequestsQueue() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [requests, setRequests] = useState<SwimmerLinkRequestRecord[]>([])
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedSwimmer, setSelectedSwimmer] = useState<Record<string, string>>({})
  const [resolving, setResolving] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!orgId) { setLoading(false); return }
    setLoading(true)
    setError(null)
    try {
      const [reqs, sw] = await Promise.all([
        fetchSwimmerLinkRequests(api, orgId),
        fetchSwimmers(api, orgId),
      ])
      setRequests(reqs)
      setSwimmers(sw)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t load swimmer link requests. Try again.'))
    } finally {
      setLoading(false)
    }
  }, [api, orgId])

  useEffect(() => { void load() }, [load])

  async function handleResolve(requestId: string) {
    const swimmerId = selectedSwimmer[requestId]
    if (!swimmerId) return
    setResolving(requestId)
    try {
      await resolveSwimmerLinkRequest(api, orgId, requestId, swimmerId)
      await load()
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t link swimmer. Try again.'))
    } finally {
      setResolving(null)
    }
  }

  if (loading || requests.length === 0) return null

  return (
    <div className="u-mt-32px" >
      <div className="u-mb-16px" >
        <h2 className="section-title u-mb-4px" >
          Swimmer Link Requests
        </h2>
      </div>

      {error && (
        <div className="u-p-12px-16px u-mb-16px u-fs-fs-base" style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px' }}>{error}</div>
      )}

      <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
        {requests.map((req) => (
          <div key={req.id} className="card u-p-16px-20px u-gap-16px" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap' }}>
            <div>
              <div className="u-fs-fs-md u-fw-600" style={{ color: 'var(--text)' }}>
                {req.requested_swimmer_name || 'Name not provided'}
              </div>
              {req.note && (
                <div className="u-fs-fs-sm u-mt-2px" style={{ color: 'var(--text-subtle)' }}>{req.note}</div>
              )}
              <div className="u-fs-fs-xs u-ff-font-body u-mt-4px" style={{ color: 'var(--text-muted)' }}>
                Requested {new Date(req.created_at).toLocaleString()}
              </div>
            </div>
            <div className="u-gap-8px" style={{ display: 'flex', alignItems: 'center' }}>
              <select
                value={selectedSwimmer[req.id] ?? ''}
                onChange={(e) => setSelectedSwimmer((s) => ({ ...s, [req.id]: e.target.value }))}
                className="u-p-8px-12px u-fs-fs-sm" 
              >
                <option value="">Select swimmer…</option>
                {swimmers.map((sw) => (
                  <option key={sw.id} value={sw.id}>{sw.first_name} {sw.last_name}</option>
                ))}
              </select>
              <button
                onClick={() => void handleResolve(req.id)}
                disabled={!selectedSwimmer[req.id] || resolving === req.id}
                className="u-gap-4px u-p-8px-16px u-fs-fs-sm u-fw-600" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', border: 'none', background: !selectedSwimmer[req.id] || resolving === req.id ? 'var(--bg-elevated)' : 'var(--success)', color: !selectedSwimmer[req.id] || resolving === req.id ? 'var(--text-muted)' : '#fff', cursor: !selectedSwimmer[req.id] || resolving === req.id ? 'not-allowed' : 'pointer' }}
              >
                <Icon name="check" size={13} />
                {resolving === req.id ? 'Linking…' : 'Link'}
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

export default function ApprovalsPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const [pending, setPending] = useState<PendingUser[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [working, setWorking] = useState<Record<string, 'approving' | 'rejecting'>>({})
  const [rejectNotes, setRejectNotes] = useState<Record<string, string>>({})
  const [showRejectId, setShowRejectId] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const load = useCallback(async (orgId: string, signal?: AbortSignal) => {
    const data = await api.get<{ users: PendingUser[]; count: number }>('/api/v1/users?status=pending_approval', signal, orgHeaders(orgId))
    if (signal?.aborted) return
    setPending(data.users ?? [])
  }, [api])

  useEffect(() => {
    let cancelled = false
    const controller = new AbortController()

    async function run() {
      if (!session?.user.orgId) { setLoading(false); return }
      try {
        setLoading(true)
        setError(null)
        await load(session.user.orgId, controller.signal)
      } catch (err) {
        if (controller.signal.aborted) return
        if (!cancelled) setError(uiErrorMessage(err, 'Couldn’t load pending approvals. Try again.'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => {
      cancelled = true
      controller.abort()
    }
  }, [load, session?.user.orgId])

  async function approve(userId: string) {
    if (!session?.user.orgId) return
    setWorking((w) => ({ ...w, [userId]: 'approving' }))
    try {
      await api.post(`/api/v1/users/${userId}/approve`, {}, orgHeaders(session.user.orgId))
      await load(session.user.orgId)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t approve user. Try again.'))
    } finally {
      setWorking((w) => { const n = { ...w }; delete n[userId]; return n })
    }
  }

  async function reject(userId: string) {
    if (!session?.user.orgId) return
    setWorking((w) => ({ ...w, [userId]: 'rejecting' }))
    try {
      await api.post(`/api/v1/users/${userId}/reject`, { reason: rejectNotes[userId] || undefined }, orgHeaders(session.user.orgId))
      setShowRejectId(null)
      setRejectNotes((n) => { const r = { ...n }; delete r[userId]; return r })
      await load(session.user.orgId)
    } catch (err) {
      setError(uiErrorMessage(err, 'Couldn’t reject user. Try again.'))
    } finally {
      setWorking((w) => { const n = { ...w }; delete n[userId]; return n })
    }
  }

  const filtered = useMemo(() => {
    if (!search) return pending
    const q = search.toLowerCase()
    return pending.filter((u) =>
      u.email.toLowerCase().includes(q) ||
      u.firstName.toLowerCase().includes(q) ||
      u.lastName.toLowerCase().includes(q) ||
      `${u.firstName} ${u.lastName}`.toLowerCase().includes(q),
    )
  }, [pending, search])

  return (
    <div className="page">
      <div className="page-heading">
        <h1 className="page-title">Pending approvals</h1>
      </div>

      {error && (
        <div className="u-p-12px-16px u-mb-16px u-fs-fs-base" style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px' }}>{error}</div>
      )}

      <div className="toolbar">
        <div className="toolbar-search">
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input aria-label="Search by name or email" type="text" placeholder="Search by name or email…" value={search} onChange={(e) => setSearch(e.target.value)} className="u-p-8px-12px-8px-32px u-fs-fs-base" style={{ width: '100%' }} />
        </div>
        <span className="u-fs-fs-sm u-ff-font-body u-ml-auto" style={{ color: 'var(--text-muted)' }}>{filtered.length} pending</span>
      </div>

      {loading ? (
        <EmptyState icon="users" title="Loading approvals" description="Fetching pending registration requests…" />
      ) : filtered.length === 0 ? (
        <EmptyState icon="check-circle" title="No pending approvals" description={search ? 'No matches for your search.' : 'All registration requests have been reviewed.'} />
      ) : (
        <div className="u-gap-12px" style={{ display: 'flex', flexDirection: 'column' }}>
          {filtered.map((user) => {
            const displayName = [user.firstName, user.lastName].filter(Boolean).join(' ') || user.email
            const busyState = working[user.id]
            const isRejectOpen = showRejectId === user.id
            const requestedRole = attr(user, 'requested_role')
            const clubCode = attr(user, 'registration_club_code')
            const registrationNote = attr(user, 'registration_note')
            const submittedAt = attr(user, 'registration_submitted_at')
            const swimmerName = attr(user, 'registration_swimmer_name')
            return (
              <div key={user.id} className="card u-p-20px" >
                <div className="u-gap-16px" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div className="u-gap-12px u-mb-4px" style={{ display: 'flex', alignItems: 'center' }}>
                      <span className="u-fs-fs-md u-fw-700" style={{ color: 'var(--text)' }}>{displayName}</span>
                      <span className="badge" style={{ background: 'var(--warning-bg)', color: 'var(--warning)' }}>Pending</span>
                      {requestedRole && (
                        <span className="badge badge-default">{requestedRoleLabel(requestedRole)}</span>
                      )}
                      <button
                        onClick={() => void navigator.clipboard.writeText(user.id)}
                        title={`Copy user ID (${user.id})`}
                        aria-label="Copy user ID"
                        className="u-ml-auto u-p-4px" style={{ display: 'flex', alignItems: 'center', background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', borderRadius: 'var(--r-xs)' }}
                      >
                        <Icon name="copy" size={12} />
                      </button>
                    </div>
                    <div className="u-fs-fs-sm u-mb-4px u-gap-16px" style={{ color: 'var(--text-subtle)', display: 'flex', flexWrap: 'wrap' }}>
                      <span>{user.email}</span>
                      {user.firstName && user.lastName && (
                        <span className="u-ff-font-body" style={{ color: 'var(--text-muted)' }}>Self-registered</span>
                      )}
                      {submittedAt && (
                        <span style={{ color: 'var(--text-muted)' }}>Submitted {new Date(submittedAt).toLocaleDateString()}</span>
                      )}
                    </div>
                    {(swimmerName || clubCode || registrationNote) && (
                      <div className="u-mt-12px u-gap-6px" style={{ display: 'grid' }}>
                        {swimmerName && (
                          <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
                            Swimmer: <strong style={{ color: 'var(--text)' }}>{swimmerName}</strong> <span style={{ color: 'var(--text-muted)' }}>(not registered yet, will need to register after approval)</span>
                          </div>
                        )}
                        {clubCode && (
                          <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)' }}>
                            Club code: <span className="u-ff-font-body" style={{ color: 'var(--text)' }}>{clubCode}</span>
                          </div>
                        )}
                        {registrationNote && (
                          <div className="u-fs-fs-sm u-lh-1_5" style={{ color: 'var(--text-subtle)' }}>
                            Note: <span style={{ color: 'var(--text)' }}>{registrationNote}</span>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                  <div className="u-gap-8px" style={{ display: 'flex', flexShrink: 0 }}>
                    <button
                      onClick={() => void approve(user.id)}
                      disabled={!!busyState}
                      className="u-gap-4px u-p-8px-16px u-fs-fs-sm u-fw-600" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', border: 'none', background: busyState ? 'var(--bg-elevated)' : 'var(--success)', color: busyState ? 'var(--text-muted)' : '#fff', cursor: busyState ? 'not-allowed' : 'pointer', transition: 'all 0.15s' }}
                    >
                      <Icon name="check" size={13} />
                      {busyState === 'approving' ? 'Approving…' : 'Approve'}
                    </button>
                    <button
                      onClick={() => setShowRejectId(isRejectOpen ? null : user.id)}
                      disabled={!!busyState}
                      className="u-gap-4px u-p-8px-16px u-fs-fs-sm u-fw-600" style={{ display: 'flex', alignItems: 'center', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text-muted)', cursor: busyState ? 'not-allowed' : 'pointer', transition: 'all 0.15s' }}
                    >
                      <Icon name="x" size={13} />
                      {busyState === 'rejecting' ? 'Rejecting…' : 'Reject'}
                    </button>
                  </div>
                </div>

                {isRejectOpen && (
                  <div className="u-mt-16px u-pt-16px" style={{ borderTop: '1px solid var(--border)' }}>
                    <label className="u-fs-fs-sm u-fw-600 u-mb-6px" style={{ color: 'var(--text-muted)', display: 'block' }}>Rejection reason (optional)</label>
                    <textarea aria-label="Optional note for internal records"
                      rows={2}
                      placeholder="Optional note for internal records…"
                      value={rejectNotes[user.id] ?? ''}
                      onChange={(e) => setRejectNotes((n) => ({ ...n, [user.id]: e.target.value }))}
                      className="u-p-8px-12px u-fs-fs-base" style={{ width: '100%', resize: 'vertical' }}
                    />
                    <div className="u-gap-8px u-mt-8px" style={{ display: 'flex', justifyContent: 'flex-end' }}>
                      <button onClick={() => setShowRejectId(null)} className="u-p-6px-16px u-fs-fs-sm" style={{ borderRadius: '8px', border: '1px solid var(--border)', background: 'none', color: 'var(--text-muted)', cursor: 'pointer' }}>Cancel</button>
                      <button onClick={() => void reject(user.id)} disabled={!!busyState} className="u-p-6px-16px u-fs-fs-sm u-fw-600" style={{ borderRadius: '8px', border: 'none', background: 'var(--error)', color: '#fff', cursor: busyState ? 'not-allowed' : 'pointer' }}>
                        {busyState === 'rejecting' ? 'Rejecting…' : 'Confirm Reject'}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      <SwimmerLinkRequestsQueue />
    </div>
  )
}
