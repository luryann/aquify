'use client'

import { useEffect, useMemo, useState, useCallback, type FormEvent } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { useSession } from '@/lib/session'
import { ApiError, useApiClient } from '@/lib/api'
import {
  fetchMessageThreads,
  fetchMessages,
  sendMessage,
  fetchHouseholds,
  fetchSwimmers,
  type MessageThreadRecord,
  type MessageRecord,
  type HouseholdRecord,
  type SwimmerRecord,
} from '@/lib/adminData'

function formatDateTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function senderLabel(role: string): string {
  if (role === 'parent' || role === 'swimmer') return 'Family'
  if (role === 'coach') return 'Coach'
  if (role === 'owner' || role === 'admin') return 'Staff'
  return role || 'Staff'
}

export default function MessagesInboxPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''

  const [threads, setThreads] = useState<MessageThreadRecord[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<{ householdId: string; swimmerId: string } | null>(null)

  const [messages, setMessages] = useState<MessageRecord[]>([])
  const [threadLoading, setThreadLoading] = useState(false)
  const [threadError, setThreadError] = useState<string | null>(null)
  const [threadReloadKey, setThreadReloadKey] = useState(0)

  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    async function load() {
      if (!orgId) {
        setLoading(false)
        return
      }
      try {
        setLoading(true)
        setError(null)
        const [nextThreads, nextHouseholds, nextSwimmers] = await Promise.all([
          fetchMessageThreads(api, orgId),
          fetchHouseholds(api, orgId),
          fetchSwimmers(api, orgId),
        ])
        if (cancelled) return
        setThreads(nextThreads)
        setHouseholds(nextHouseholds)
        setSwimmers(nextSwimmers)
        if (nextThreads[0]) {
          setSelected({ householdId: nextThreads[0].household_id, swimmerId: nextThreads[0].swimmer_id })
        }
      } catch (err) {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load message threads.')
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [api, orgId])

  const householdNameById = useMemo(() => new Map(households.map((h) => [h.id, h.household_name])), [households])
  const swimmerNameById = useMemo(() => new Map(swimmers.map((s) => [s.id, `${s.first_name} ${s.last_name}`])), [swimmers])

  const loadThread = useCallback(() => {
    if (!orgId || !selected) return
    setThreadLoading(true)
    setThreadError(null)
    fetchMessages(api, orgId, selected.householdId, selected.swimmerId)
      .then(setMessages)
      .catch((err) => setThreadError(err instanceof ApiError ? err.message : 'Failed to load this conversation.'))
      .finally(() => setThreadLoading(false))
  }, [api, orgId, selected])

  useEffect(() => {
    loadThread()
  }, [loadThread, threadReloadKey])

  async function handleSend(e: FormEvent) {
    e.preventDefault()
    if (!orgId || !selected || !draft.trim()) return
    setSending(true)
    setSendError(null)
    try {
      await sendMessage(api, orgId, { household_id: selected.householdId, swimmer_id: selected.swimmerId, body: draft.trim() })
      setDraft('')
      setThreadReloadKey((k) => k + 1)
      const nextThreads = await fetchMessageThreads(api, orgId)
      setThreads(nextThreads)
    } catch (err) {
      setSendError(err instanceof ApiError ? err.message : 'Failed to send your reply.')
    } finally {
      setSending(false)
    }
  }

  return (
    <div style={{ padding: '24px', maxWidth: '1200px' }}>
      <div style={{ marginBottom: '24px' }}>
        <h1 className="page-title">Messages</h1>
        <p className="page-subtitle">Direct conversations between families and staff, one thread per swimmer.</p>
      </div>

      {loading ? (
        <EmptyState icon="mail" title="Loading messages" description="Fetching message threads from the API." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load messages" description={error} />
      ) : threads.length === 0 ? (
        <EmptyState icon="mail" title="No conversations yet" description="Messages from parents will appear here once a family starts one." />
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr', gap: '16px', alignItems: 'start' }}>
          <div className="card" style={{ padding: '8px', display: 'flex', flexDirection: 'column', gap: '4px' }}>
            {threads.map((t) => {
              const active = selected?.householdId === t.household_id && selected?.swimmerId === t.swimmer_id
              return (
                <button
                  key={`${t.household_id}-${t.swimmer_id}`}
                  onClick={() => setSelected({ householdId: t.household_id, swimmerId: t.swimmer_id })}
                  style={{
                    textAlign: 'left',
                    padding: '10px 12px',
                    borderRadius: 'var(--r-md)',
                    border: 'none',
                    background: active ? 'var(--accent-subtle)' : 'transparent',
                    cursor: 'pointer',
                  }}
                >
                  <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text)' }}>
                    {swimmerNameById.get(t.swimmer_id) ?? 'Unknown swimmer'}
                  </div>
                  <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginBottom: '4px' }}>
                    {householdNameById.get(t.household_id) ?? 'Unknown household'}
                  </div>
                  <div style={{ fontSize: '12px', color: 'var(--text-subtle)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {t.last_body}
                  </div>
                  <div style={{ fontSize: '11px', fontFamily: 'var(--font-body)', color: 'var(--text-muted)', marginTop: '2px' }}>
                    {formatDateTime(t.last_message_at)}
                  </div>
                </button>
              )
            })}
          </div>

          <div className="card" style={{ display: 'flex', flexDirection: 'column', minHeight: '480px' }}>
            <div style={{ padding: '14px 18px', borderBottom: '1px solid var(--border)' }}>
              <p style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)' }}>
                {selected ? (swimmerNameById.get(selected.swimmerId) ?? 'Conversation') : 'Select a conversation'}
              </p>
              {selected && (
                <p style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                  {householdNameById.get(selected.householdId) ?? ''}
                </p>
              )}
            </div>

            <div style={{ flex: 1, padding: '18px', display: 'flex', flexDirection: 'column', gap: '12px', overflowY: 'auto', maxHeight: '480px' }}>
              {threadError ? (
                <p style={{ fontSize: '13px', color: 'var(--error)' }}>{threadError}</p>
              ) : threadLoading ? (
                <p className="page-subtitle">Loading conversation…</p>
              ) : messages.length === 0 ? (
                <p className="page-subtitle">No messages in this thread yet.</p>
              ) : (
                messages.map((m) => {
                  const mine = m.sender_role !== 'parent' && m.sender_role !== 'swimmer'
                  return (
                    <div key={m.id} style={{ alignSelf: mine ? 'flex-end' : 'flex-start', maxWidth: '75%' }}>
                      <div
                        style={{
                          background: mine ? 'var(--accent)' : 'var(--bg-elevated)',
                          color: mine ? '#fff' : 'var(--text)',
                          padding: '10px 14px',
                          borderRadius: mine ? '12px 12px 2px 12px' : '12px 12px 12px 2px',
                          fontSize: '13.5px',
                          lineHeight: 1.5,
                          whiteSpace: 'pre-wrap',
                        }}
                      >
                        {m.body}
                      </div>
                      <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '4px', textAlign: mine ? 'right' : 'left' }}>
                        {senderLabel(m.sender_role)} · {formatDateTime(m.created_at)}
                      </p>
                    </div>
                  )
                })
              )}
            </div>

            <form onSubmit={(e) => void handleSend(e)} style={{ padding: '14px 18px', borderTop: '1px solid var(--border)', display: 'flex', gap: '10px' }}>
              <input
                type="text"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="Reply to this family…"
                disabled={!selected || sending}
                maxLength={4000}
                style={{ flex: 1, padding: '9px 12px', background: 'var(--bg-elevated)', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', fontSize: '13px', color: 'var(--text)', outline: 'none' }}
              />
              <button type="submit" className="btn btn-primary" disabled={!selected || sending || !draft.trim()}>
                {sending ? 'Sending…' : 'Send'}
              </button>
            </form>
            {sendError && <p style={{ padding: '0 18px 14px', fontSize: '12px', color: 'var(--error)' }}>{sendError}</p>}
          </div>
        </div>
      )}
    </div>
  )
}
