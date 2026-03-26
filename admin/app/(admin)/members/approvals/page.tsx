'use client'

import { useEffect, useMemo, useState } from 'react'
import { Icon } from '@/components/ui/icons'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'

type PendingUser = {
  id: string
  email: string
  firstName: string
  lastName: string
  enabled: boolean
  username: string
  emailVerified: boolean
  attributes?: { org_id?: string[]; registration_status?: string[] }
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

  function orgHeaders(orgId: string) { return { 'X-Org-Id': orgId } }

  async function load(orgId: string) {
    const data = await api.get<{ users: PendingUser[]; count: number }>('/api/v1/users?status=pending_approval', undefined, orgHeaders(orgId))
    setPending(data.users ?? [])
  }

  useEffect(() => {
    let cancelled = false

    async function run() {
      if (!session?.user.orgId) { setLoading(false); return }
      try {
        setLoading(true)
        setError(null)
        await load(session.user.orgId)
      } catch (err) {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load pending approvals.')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void run()
    return () => { cancelled = true }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, session?.user.orgId])

  async function approve(userId: string) {
    if (!session?.user.orgId) return
    setWorking((w) => ({ ...w, [userId]: 'approving' }))
    try {
      await api.post(`/api/v1/users/${userId}/approve`, {}, orgHeaders(session.user.orgId))
      await load(session.user.orgId)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to approve user.')
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
      setError(err instanceof ApiError ? err.message : 'Failed to reject user.')
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
    <div style={{ padding: '24px', maxWidth: '900px' }}>
      <div style={{ marginBottom: '24px' }}>
        <h1 style={{ fontSize: '22px', fontFamily: 'var(--font-display)', fontWeight: 600, color: 'var(--text)', lineHeight: 1.2, marginBottom: '4px' }}>Pending Approvals</h1>
        <p style={{ fontSize: '13px', color: 'var(--text-subtle)' }}>Self-registered users awaiting admin review</p>
      </div>

      {error && (
        <div style={{ background: 'var(--error-bg)', color: 'var(--error)', border: '1px solid var(--error)', borderRadius: '8px', padding: '10px 14px', marginBottom: '16px', fontSize: '13px' }}>{error}</div>
      )}

      <div style={{ display: 'flex', gap: '10px', marginBottom: '16px', alignItems: 'center' }}>
        <div style={{ position: 'relative', flex: 1, maxWidth: '360px' }}>
          <Icon name="search" size={14} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input type="text" placeholder="Search by name or email…" value={search} onChange={(e) => setSearch(e.target.value)} style={{ width: '100%', padding: '8px 12px 8px 32px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }} />
        </div>
        <span style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', marginLeft: 'auto' }}>{filtered.length} pending</span>
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
            return (
              <div key={user.id} className="card" style={{ padding: '20px' }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '16px' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '4px' }}>
                      <span style={{ fontSize: '15px', fontWeight: 700, color: 'var(--text)' }}>{displayName}</span>
                      <span style={{ fontSize: '10px', fontWeight: 700, padding: '2px 7px', borderRadius: '10px', background: 'var(--warning-bg)', color: 'var(--warning)', fontFamily: 'var(--font-mono)', textTransform: 'uppercase' }}>Pending</span>
                    </div>
                    <div style={{ fontSize: '12px', color: 'var(--text-subtle)', marginBottom: '4px', display: 'flex', gap: '16px', flexWrap: 'wrap' }}>
                      <span>{user.email}</span>
                      {user.firstName && user.lastName && (
                        <span style={{ fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>Self-registered</span>
                      )}
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                      ID: {user.id}
                    </div>
                  </div>
                  <div style={{ display: 'flex', gap: '8px', flexShrink: 0 }}>
                    <button
                      onClick={() => void approve(user.id)}
                      disabled={!!busyState}
                      style={{ display: 'flex', alignItems: 'center', gap: '5px', padding: '7px 14px', borderRadius: '8px', border: 'none', background: busyState ? 'var(--bg-elevated)' : 'var(--success)', color: busyState ? 'var(--text-muted)' : '#fff', fontSize: '12px', fontWeight: 600, cursor: busyState ? 'not-allowed' : 'pointer', transition: 'all 0.15s' }}
                    >
                      <Icon name="check" size={13} />
                      {busyState === 'approving' ? 'Approving…' : 'Approve'}
                    </button>
                    <button
                      onClick={() => setShowRejectId(isRejectOpen ? null : user.id)}
                      disabled={!!busyState}
                      style={{ display: 'flex', alignItems: 'center', gap: '5px', padding: '7px 14px', borderRadius: '8px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text-muted)', fontSize: '12px', fontWeight: 600, cursor: busyState ? 'not-allowed' : 'pointer', transition: 'all 0.15s' }}
                    >
                      <Icon name="x" size={13} />
                      {busyState === 'rejecting' ? 'Rejecting…' : 'Reject'}
                    </button>
                  </div>
                </div>

                {isRejectOpen && (
                  <div style={{ marginTop: '14px', paddingTop: '14px', borderTop: '1px solid var(--border)' }}>
                    <label style={{ fontSize: '11px', fontWeight: 600, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.05em', display: 'block', marginBottom: '6px' }}>Rejection Reason (optional)</label>
                    <textarea
                      rows={2}
                      placeholder="Optional note for internal records…"
                      value={rejectNotes[user.id] ?? ''}
                      onChange={(e) => setRejectNotes((n) => ({ ...n, [user.id]: e.target.value }))}
                      style={{ width: '100%', padding: '8px 10px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: '8px', fontSize: '13px', color: 'var(--text)', outline: 'none', resize: 'vertical', boxSizing: 'border-box' }}
                    />
                    <div style={{ display: 'flex', gap: '8px', marginTop: '8px', justifyContent: 'flex-end' }}>
                      <button onClick={() => setShowRejectId(null)} style={{ padding: '6px 14px', borderRadius: '7px', border: '1px solid var(--border)', background: 'none', color: 'var(--text-muted)', fontSize: '12px', cursor: 'pointer' }}>Cancel</button>
                      <button onClick={() => void reject(user.id)} disabled={!!busyState} style={{ padding: '6px 14px', borderRadius: '7px', border: 'none', background: 'var(--error)', color: '#fff', fontSize: '12px', fontWeight: 600, cursor: busyState ? 'not-allowed' : 'pointer' }}>
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
    </div>
  )
}
