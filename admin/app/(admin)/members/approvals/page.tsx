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
    <div style={{ marginTop: '32px' }}>
      <div style={{ marginBottom: '16px' }}>
        <h2 className="section-title" style={{ marginBottom: '4px' }}>
          Swimmer Link Requests
        </h2>
      </div>

      {error && (
        <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '12px 16px', marginBottom: '16px', fontSize: 'var(--fs-base)' }}>{error}</div>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
        {requests.map((req) => (
          <div key={req.id} className="card" style={{ padding: '16px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px', flexWrap: 'wrap' }}>
            <div>
              <div style={{ fontSize: 'var(--fs-md)', fontWeight: 600, color: 'var(--text)' }}>
                {req.requested_swimmer_name || 'Name not provided'}
              </div>
              {req.note && (
                <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', marginTop: '2px' }}>{req.note}</div>
              )}
              <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginTop: '4px' }}>
                Requested {new Date(req.created_at).toLocaleString()}
              </div>
            </div>
            <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
              <select
                value={selectedSwimmer[req.id] ?? ''}
                onChange={(e) => setSelectedSwimmer((s) => ({ ...s, [req.id]: e.target.value }))}
                style={{ padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', fontSize: 'var(--fs-sm)', color: 'var(--text)' }}
              >
                <option value="">Select swimmer…</option>
                {swimmers.map((sw) => (
                  <option key={sw.id} value={sw.id}>{sw.first_name} {sw.last_name}</option>
                ))}
              </select>
              <button
                onClick={() => void handleResolve(req.id)}
                disabled={!selectedSwimmer[req.id] || resolving === req.id}
                style={{ display: 'flex', alignItems: 'center', gap: '4px', padding: '8px 16px', borderRadius: '8px', border: 'none', background: !selectedSwimmer[req.id] || resolving === req.id ? 'var(--bg-elevated)' : 'var(--success)', color: !selectedSwimmer[req.id] || resolving === req.id ? 'var(--text-muted)' : '#fff', fontSize: 'var(--fs-sm)', fontWeight: 600, cursor: !selectedSwimmer[req.id] || resolving === req.id ? 'not-allowed' : 'pointer' }}
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
        <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '12px 16px', marginBottom: '16px', fontSize: 'var(--fs-base)' }}>{error}</div>
      )}

      <div className="toolbar">
        <div className="toolbar-search">
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search by name or email…" value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none' }} />
        </div>
        <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-muted)', fontFamily: 'var(--font-body)', marginLeft: 'auto' }}>{filtered.length} pending</span>
      </div>

      {loading ? (
        <EmptyState icon="users" title="Loading approvals" description="Fetching pending registration requests…" />
      ) : filtered.length === 0 ? (
        <EmptyState icon="check-circle" title="No pending approvals" description={search ? 'No matches for your search.' : 'All registration requests have been reviewed.'} />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
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
              <div key={user.id} className="card" style={{ padding: '20px' }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '4px' }}>
                      <span style={{ fontSize: 'var(--fs-md)', fontWeight: 700, color: 'var(--text)' }}>{displayName}</span>
                      <span className="badge" style={{ background: 'var(--warning-bg)', color: 'var(--warning)' }}>Pending</span>
                      {requestedRole && (
                        <span className="badge badge-default">{requestedRoleLabel(requestedRole)}</span>
                      )}
                      <button
                        onClick={() => void navigator.clipboard.writeText(user.id)}
                        title={`Copy user ID (${user.id})`}
                        aria-label="Copy user ID"
                        style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', padding: '4px', background: 'none', border: 'none', color: 'var(--text-disabled)', cursor: 'pointer', borderRadius: 'var(--r-xs)' }}
                      >
                        <Icon name="copy" size={12} />
                      </button>
                    </div>
                    <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', marginBottom: '4px', display: 'flex', gap: '16px', flexWrap: 'wrap' }}>
                      <span>{user.email}</span>
                      {user.firstName && user.lastName && (
                        <span style={{ fontFamily: 'var(--font-body)', color: 'var(--text-muted)' }}>Self-registered</span>
                      )}
                      {submittedAt && (
                        <span style={{ color: 'var(--text-muted)' }}>Submitted {new Date(submittedAt).toLocaleDateString()}</span>
                      )}
                    </div>
                    {(swimmerName || clubCode || registrationNote) && (
                      <div style={{ marginTop: '12px', display: 'grid', gap: '6px' }}>
                        {swimmerName && (
                          <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>
                            Swimmer: <strong style={{ color: 'var(--text)' }}>{swimmerName}</strong> <span style={{ color: 'var(--text-muted)' }}>(not registered yet, will need to register after approval)</span>
                          </div>
                        )}
                        {clubCode && (
                          <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)' }}>
                            Club code: <span style={{ fontFamily: 'var(--font-body)', color: 'var(--text)' }}>{clubCode}</span>
                          </div>
                        )}
                        {registrationNote && (
                          <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--text-subtle)', lineHeight: 1.5 }}>
                            Note: <span style={{ color: 'var(--text)' }}>{registrationNote}</span>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                  <div style={{ display: 'flex', gap: '8px', flexShrink: 0 }}>
                    <button
                      onClick={() => void approve(user.id)}
                      disabled={!!busyState}
                      style={{ display: 'flex', alignItems: 'center', gap: '4px', padding: '8px 16px', borderRadius: '8px', border: 'none', background: busyState ? 'var(--bg-elevated)' : 'var(--success)', color: busyState ? 'var(--text-muted)' : '#fff', fontSize: 'var(--fs-sm)', fontWeight: 600, cursor: busyState ? 'not-allowed' : 'pointer', transition: 'all 0.15s' }}
                    >
                      <Icon name="check" size={13} />
                      {busyState === 'approving' ? 'Approving…' : 'Approve'}
                    </button>
                    <button
                      onClick={() => setShowRejectId(isRejectOpen ? null : user.id)}
                      disabled={!!busyState}
                      style={{ display: 'flex', alignItems: 'center', gap: '4px', padding: '8px 16px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text-muted)', fontSize: 'var(--fs-sm)', fontWeight: 600, cursor: busyState ? 'not-allowed' : 'pointer', transition: 'all 0.15s' }}
                    >
                      <Icon name="x" size={13} />
                      {busyState === 'rejecting' ? 'Rejecting…' : 'Reject'}
                    </button>
                  </div>
                </div>

                {isRejectOpen && (
                  <div style={{ marginTop: '16px', paddingTop: '16px', borderTop: '1px solid var(--border)' }}>
                    <label style={{ fontSize: 'var(--fs-sm)', fontWeight: 600, color: 'var(--text-muted)', display: 'block', marginBottom: '6px' }}>Rejection reason (optional)</label>
                    <textarea
                      rows={2}
                      placeholder="Optional note for internal records…"
                      value={rejectNotes[user.id] ?? ''}
                      onChange={(e) => setRejectNotes((n) => ({ ...n, [user.id]: e.target.value }))}
                      style={{ width: '100%', padding: '8px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', fontSize: 'var(--fs-base)', color: 'var(--text)', outline: 'none', resize: 'vertical', boxSizing: 'border-box' }}
                    />
                    <div style={{ display: 'flex', gap: '8px', marginTop: '8px', justifyContent: 'flex-end' }}>
                      <button onClick={() => setShowRejectId(null)} style={{ padding: '6px 16px', borderRadius: '8px', border: '1px solid var(--border)', background: 'none', color: 'var(--text-muted)', fontSize: 'var(--fs-sm)', cursor: 'pointer' }}>Cancel</button>
                      <button onClick={() => void reject(user.id)} disabled={!!busyState} style={{ padding: '6px 16px', borderRadius: '8px', border: 'none', background: 'var(--error)', color: '#fff', fontSize: 'var(--fs-sm)', fontWeight: 600, cursor: busyState ? 'not-allowed' : 'pointer' }}>
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
