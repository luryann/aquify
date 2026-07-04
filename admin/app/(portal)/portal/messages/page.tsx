'use client'

import { useState, useEffect, useMemo, useCallback, type FormEvent } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import {
  fetchSwimmers,
  fetchMessages,
  sendMessage,
  type SwimmerRecord,
  type MessageRecord,
} from '@/lib/adminData'

function fmtTime(value: string): string {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
}

function senderLabel(role: string): string {
  if (role === 'parent' || role === 'swimmer') return 'You'
  if (role === 'coach') return 'Coach'
  if (role === 'owner' || role === 'admin') return 'Club staff'
  return role || 'Staff'
}

export default function PortalMessagesPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''

  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [selectedSwimmerId, setSelectedSwimmerId] = useState('')
  const [swimmersLoading, setSwimmersLoading] = useState(true)
  const [swimmersError, setSwimmersError] = useState<string | null>(null)

  const [messages, setMessages] = useState<MessageRecord[]>([])
  const [messagesLoading, setMessagesLoading] = useState(false)
  const [messagesError, setMessagesError] = useState<string | null>(null)
  const [reloadKey, setReloadKey] = useState(0)

  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)

  useEffect(() => {
    if (!orgId || !householdId) {
      setSwimmersLoading(false)
      setSwimmersError('Messages are unavailable because household context is missing.')
      return
    }
    setSwimmersLoading(true)
    setSwimmersError(null)
    fetchSwimmers(api, orgId)
      .then((all) => {
        const householdSwimmers = all.filter((s) => s.household_id === householdId)
        setSwimmers(householdSwimmers)
        setSelectedSwimmerId((current) => current || householdSwimmers[0]?.id || '')
      })
      .catch((error) => {
        setSwimmersError(reportUIError('portal-messages-swimmers-load', error, 'Unable to load your swimmers right now.', { orgId }))
      })
      .finally(() => setSwimmersLoading(false))
  }, [api, orgId, householdId])

  const loadMessages = useCallback(() => {
    if (!orgId || !householdId || !selectedSwimmerId) return
    setMessagesLoading(true)
    setMessagesError(null)
    fetchMessages(api, orgId, householdId, selectedSwimmerId)
      .then(setMessages)
      .catch((error) => {
        setMessagesError(reportUIError('portal-messages-thread-load', error, 'Unable to load this conversation right now.', { orgId, householdId, swimmerId: selectedSwimmerId }))
      })
      .finally(() => setMessagesLoading(false))
  }, [api, orgId, householdId, selectedSwimmerId])

  useEffect(() => {
    loadMessages()
  }, [loadMessages, reloadKey])

  const selectedSwimmer = useMemo(
    () => swimmers.find((s) => s.id === selectedSwimmerId),
    [swimmers, selectedSwimmerId],
  )

  async function handleSend(e: FormEvent) {
    e.preventDefault()
    if (!orgId || !householdId || !selectedSwimmerId || !draft.trim()) return
    setSending(true)
    setSendError(null)
    try {
      await sendMessage(api, orgId, { household_id: householdId, swimmer_id: selectedSwimmerId, body: draft.trim() })
      setDraft('')
      setReloadKey((k) => k + 1)
    } catch (err) {
      setSendError(err instanceof ApiError ? err.message : 'Failed to send your message.')
    } finally {
      setSending(false)
    }
  }

  return (
    <div>
      <div style={{ marginBottom: '28px', animation: 'slideUp 0.35s ease both' }}>
        <h1 className="page-title">
          Messages
        </h1>
        <p className="page-subtitle">
          Message your child&rsquo;s coach directly — each swimmer has their own conversation.
        </p>
      </div>

      {swimmersError ? (
        <div style={{ background: 'var(--error-bg)', borderRadius: '14px', border: '1px solid rgba(220,38,38,0.18)', padding: '20px', color: 'var(--error)', fontSize: '13px' }}>
          {swimmersError}
        </div>
      ) : swimmersLoading ? (
        <div style={{ padding: '32px', textAlign: 'center', color: 'var(--text-muted)', fontSize: '13px' }}>Loading your swimmers…</div>
      ) : swimmers.length === 0 ? (
        <div className="card" style={{ padding: '56px 32px', textAlign: 'center' }}>
          <Icon name="users" size={32} style={{ color: '#D4D1CE', marginBottom: '14px' }} />
          <p style={{ fontSize: '15px', color: 'var(--text-muted)', fontWeight: 500 }}>No swimmers on your account yet</p>
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(160px, 220px) 1fr', gap: '20px', alignItems: 'start' }}>
          {/* Swimmer picker */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
            {swimmers.map((s) => {
              const active = s.id === selectedSwimmerId
              return (
                <button
                  key={s.id}
                  onClick={() => setSelectedSwimmerId(s.id)}
                  style={{
                    textAlign: 'left',
                    padding: '10px 14px',
                    borderRadius: '10px',
                    border: `1px solid ${active ? 'rgba(235,93,30,0.3)' : 'rgba(0,0,0,0.08)'}`,
                    background: active ? 'rgba(235,93,30,0.08)' : 'var(--bg-elevated)',
                    color: active ? 'var(--text)' : 'var(--text-subtle)',
                    fontSize: '13.5px',
                    fontWeight: active ? 600 : 500,
                    cursor: 'pointer',
                    transition: 'all 120ms ease',
                  }}
                >
                  {s.first_name} {s.last_name}
                </button>
              )
            })}
          </div>

          {/* Thread */}
          <div className="card" style={{ boxShadow: '0 1px 6px rgba(0,0,0,0.04)', display: 'flex', flexDirection: 'column', minHeight: '420px' }}>
            <div style={{ padding: '14px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
              <p style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)' }}>
                {selectedSwimmer ? `${selectedSwimmer.first_name} ${selectedSwimmer.last_name}` : 'Conversation'}
              </p>
            </div>

            <div style={{ flex: 1, padding: '20px', display: 'flex', flexDirection: 'column', gap: '14px', overflowY: 'auto', maxHeight: '480px' }}>
              {messagesError ? (
                <p style={{ fontSize: '13px', color: 'var(--error)' }}>{messagesError}</p>
              ) : messagesLoading ? (
                <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>Loading conversation…</p>
              ) : messages.length === 0 ? (
                <p style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
                  No messages yet. Send the first one below to start the conversation with your coach.
                </p>
              ) : (
                messages.map((m) => {
                  const mine = m.sender_role === 'parent' || m.sender_role === 'swimmer'
                  return (
                    <div key={m.id} style={{ alignSelf: mine ? 'flex-end' : 'flex-start', maxWidth: '75%' }}>
                      <div
                        style={{
                          background: mine ? 'var(--accent)' : 'rgba(0,0,0,0.05)',
                          color: mine ? 'var(--bg-elevated)' : 'var(--text)',
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
                        {senderLabel(m.sender_role)} · {fmtTime(m.created_at)}
                      </p>
                    </div>
                  )
                })
              )}
            </div>

            <form onSubmit={(e) => void handleSend(e)} style={{ padding: '14px 20px', borderTop: '1px solid rgba(0,0,0,0.06)', display: 'flex', gap: '10px' }}>
              <input
                type="text"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="Write a message…"
                disabled={sending || !selectedSwimmerId}
                maxLength={4000}
                style={{ flex: 1, padding: '10px 14px', borderRadius: '10px', border: '1px solid rgba(0,0,0,0.12)', fontSize: '13.5px', outline: 'none' }}
              />
              <button
                type="submit"
                disabled={sending || !draft.trim() || !selectedSwimmerId}
                style={{
                  padding: '10px 18px',
                  borderRadius: '10px',
                  border: 'none',
                  background: sending || !draft.trim() ? 'var(--text-subtle)' : 'var(--text)',
                  color: 'var(--bg-elevated)',
                  fontSize: '13.5px',
                  fontWeight: 600,
                  cursor: sending || !draft.trim() ? 'not-allowed' : 'pointer',
                }}
              >
                {sending ? 'Sending…' : 'Send'}
              </button>
            </form>
            {sendError && <p style={{ padding: '0 20px 14px', fontSize: '12px', color: 'var(--error)' }}>{sendError}</p>}
          </div>
        </div>
      )}

      <style>{`
        @keyframes slideUp {
          from { opacity: 0; transform: translateY(10px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </div>
  )
}
