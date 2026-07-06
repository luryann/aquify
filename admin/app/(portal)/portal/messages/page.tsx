'use client'

import { useState, useEffect, useMemo, useCallback, useRef, type FormEvent } from 'react'
import Link from 'next/link'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient, ApiError } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { useMessageStream } from '@/lib/useMessageStream'
import {
  fetchSwimmers,
  fetchMessages,
  sendMessage,
  editMessage,
  addReaction,
  removeReaction,
  markThreadRead,
  requestMessageUploadURL,
  type SwimmerRecord,
  type MessageRecord,
} from '@/lib/adminData'

const QUICK_REACTIONS = ['👍', '❤️', '😂', '🎉', '😮', '😢']

function fmtTime(value: string): string {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
}

function senderLabel(role: string): string {
  if (role === 'parent' || role === 'swimmer') return 'You'
  if (role === 'coach') return 'Coach'
  if (role === 'admin') return 'Club staff'
  return role || 'Staff'
}

export default function PortalMessagesPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''
  const userId = session?.user.id ?? ''

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

  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState('')
  const [reactionPickerId, setReactionPickerId] = useState<string | null>(null)
  const [pendingAttachment, setPendingAttachment] = useState<{ object_key: string; file_name: string; file_mime: string } | null>(null)
  const [attachError, setAttachError] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const selectedSwimmerIdRef = useRef(selectedSwimmerId)
  selectedSwimmerIdRef.current = selectedSwimmerId

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

  useEffect(() => {
    if (!orgId || !householdId || !selectedSwimmerId) return
    markThreadRead(api, orgId, householdId, selectedSwimmerId).catch(() => {})
  }, [api, orgId, householdId, selectedSwimmerId])

  useMessageStream({
    orgId,
    token: session?.token,
    onEvent: (event) => {
      if (event.household_id !== householdId || event.swimmer_id !== selectedSwimmerIdRef.current) return
      setMessages((prev) => {
        const idx = prev.findIndex((m) => m.id === event.message.id)
        if (idx === -1) return [...prev, event.message]
        const next = [...prev]
        next[idx] = event.message
        return next
      })
      if (event.type === 'message.created' && event.message.sender_user_id !== userId) {
        markThreadRead(api, orgId, event.household_id, event.swimmer_id).catch(() => {})
      }
    },
  })

  const selectedSwimmer = useMemo(
    () => swimmers.find((s) => s.id === selectedSwimmerId),
    [swimmers, selectedSwimmerId],
  )

  async function handleSend(e: FormEvent) {
    e.preventDefault()
    if (!orgId || !householdId || !selectedSwimmerId || (!draft.trim() && !pendingAttachment)) return
    setSending(true)
    setSendError(null)
    try {
      await sendMessage(api, orgId, {
        household_id: householdId,
        swimmer_id: selectedSwimmerId,
        body: draft.trim() || '(attachment)',
        attachments: pendingAttachment ? [pendingAttachment] : undefined,
      })
      setDraft('')
      setPendingAttachment(null)
      setReloadKey((k) => k + 1)
    } catch (err) {
      setSendError(err instanceof ApiError ? err.message : 'Failed to send your message.')
    } finally {
      setSending(false)
    }
  }

  async function handleFileSelect(file: File) {
    if (!orgId || !householdId || !selectedSwimmerId) return
    setAttachError(null)
    setUploading(true)
    try {
      const { upload_url, object_key, expected_file_name } = await requestMessageUploadURL(api, orgId, {
        household_id: householdId,
        swimmer_id: selectedSwimmerId,
        file_name: file.name,
        file_mime: file.type,
        file_size_bytes: file.size,
      })
      const putResp = await fetch(upload_url, { method: 'PUT', body: file, headers: { 'Content-Type': file.type } })
      if (!putResp.ok) throw new Error('Upload failed')
      setPendingAttachment({ object_key, file_name: expected_file_name, file_mime: file.type })
    } catch {
      setAttachError('Could not upload that image. Try a smaller file or a different format.')
    } finally {
      setUploading(false)
    }
  }

  async function handleEditSave(messageId: string) {
    if (!orgId || !editDraft.trim()) return
    try {
      const updated = await editMessage(api, orgId, messageId, editDraft.trim())
      setMessages((prev) => prev.map((m) => (m.id === messageId ? updated : m)))
    } catch {
      // Leave the bubble as-is; the user can retry.
    } finally {
      setEditingId(null)
    }
  }

  async function handleToggleReaction(message: MessageRecord, emoji: string) {
    if (!orgId || !householdId || !selectedSwimmerId) return
    const already = message.reactions?.some((r) => r.emoji === emoji && r.reacted_by_me)
    try {
      if (already) await removeReaction(api, orgId, message.id, emoji)
      else await addReaction(api, orgId, message.id, emoji)
      // Realtime (when connected) will also deliver this update, but don't
      // depend on it — refetch so the reaction shows immediately even
      // without Redis configured.
      const next = await fetchMessages(api, orgId, householdId, selectedSwimmerId)
      setMessages(next)
    } catch {
      // Leave state as-is; the user can retry.
    }
    setReactionPickerId(null)
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
        <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '6px' }}>
          Looking for team announcements or to update what you get notified about? Visit{' '}
          <Link href="/portal/communications" style={{ color: 'var(--accent)', textDecoration: 'none', fontWeight: 600 }}>Communications</Link>.
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
        <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
          {/* Swimmer picker — a horizontal chip row rather than a permanent sidebar, since most
              households have only 1–3 swimmers and this reads better on a phone. */}
          {swimmers.length > 1 && (
            <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
              {swimmers.map((s) => {
                const active = s.id === selectedSwimmerId
                return (
                  <button
                    key={s.id}
                    onClick={() => setSelectedSwimmerId(s.id)}
                    style={{
                      padding: '8px 14px',
                      borderRadius: '999px',
                      border: `1px solid ${active ? 'rgba(235,93,30,0.3)' : 'rgba(0,0,0,0.08)'}`,
                      background: active ? 'rgba(235,93,30,0.08)' : 'var(--bg-elevated)',
                      color: active ? 'var(--text)' : 'var(--text-subtle)',
                      fontSize: '13px',
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
          )}

          {/* Thread */}
          <div className="card" style={{ boxShadow: '0 1px 6px rgba(0,0,0,0.04)', display: 'flex', flexDirection: 'column', minHeight: '520px' }}>
            <div style={{ padding: '14px 20px', borderBottom: '1px solid rgba(0,0,0,0.06)' }}>
              <p style={{ fontSize: '14px', fontWeight: 600, color: 'var(--text)' }}>
                {selectedSwimmer ? `${selectedSwimmer.first_name} ${selectedSwimmer.last_name}` : 'Conversation'}
              </p>
            </div>

            <div style={{ flex: 1, padding: '20px', display: 'flex', flexDirection: 'column', gap: '14px', overflowY: 'auto', maxHeight: '560px' }}>
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
                  const canEdit = m.sender_user_id === userId
                  const isEditing = editingId === m.id
                  return (
                    <div key={m.id} style={{ alignSelf: mine ? 'flex-end' : 'flex-start', maxWidth: '75%' }}>
                      {isEditing ? (
                        <div style={{ display: 'flex', gap: '6px', alignItems: 'flex-end' }}>
                          <textarea
                            value={editDraft}
                            onChange={(e) => setEditDraft(e.target.value)}
                            rows={2}
                            style={{ minWidth: '220px', padding: '8px 10px', borderRadius: '10px', border: '1px solid rgba(235,93,30,0.3)', fontSize: '13.5px', resize: 'vertical' }}
                          />
                          <button className="btn btn-sm btn-primary" onClick={() => void handleEditSave(m.id)}>Save</button>
                          <button className="btn btn-sm btn-secondary" onClick={() => setEditingId(null)}>Cancel</button>
                        </div>
                      ) : (
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
                          {m.attachments?.map((a) => (
                            a.download_url ? (
                              // eslint-disable-next-line @next/next/no-img-element
                              <img key={a.filename} src={a.download_url} alt={a.filename} style={{ display: 'block', marginTop: '8px', maxWidth: '220px', maxHeight: '220px', borderRadius: '8px' }} />
                            ) : null
                          ))}
                        </div>
                      )}

                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginTop: '4px', justifyContent: mine ? 'flex-end' : 'flex-start' }}>
                        <p style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                          {senderLabel(m.sender_role)} · {fmtTime(m.created_at)}{m.edited_at ? ' · (edited)' : ''}
                        </p>
                        {canEdit && !isEditing && (
                          <button
                            onClick={() => { setEditingId(m.id); setEditDraft(m.body) }}
                            title="Edit"
                            style={{ background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', padding: '2px', display: 'flex' }}
                          >
                            <Icon name="edit-2" size={11} />
                          </button>
                        )}
                        <button
                          onClick={() => setReactionPickerId(reactionPickerId === m.id ? null : m.id)}
                          title="React"
                          style={{ background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', padding: '2px', display: 'flex' }}
                        >
                          <Icon name="plus" size={11} />
                        </button>
                      </div>

                      {reactionPickerId === m.id && (
                        <div style={{ display: 'flex', gap: '4px', marginTop: '4px', padding: '4px 6px', borderRadius: '10px', border: '1px solid rgba(0,0,0,0.08)', background: 'var(--bg-elevated)', width: 'fit-content' }}>
                          {QUICK_REACTIONS.map((emoji) => (
                            <button key={emoji} onClick={() => void handleToggleReaction(m, emoji)} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: '15px', padding: '2px' }}>
                              {emoji}
                            </button>
                          ))}
                        </div>
                      )}

                      {m.reactions && m.reactions.length > 0 && (
                        <div style={{ display: 'flex', gap: '4px', marginTop: '4px', flexWrap: 'wrap', justifyContent: mine ? 'flex-end' : 'flex-start' }}>
                          {m.reactions.map((r) => (
                            <button
                              key={r.emoji}
                              onClick={() => void handleToggleReaction(m, r.emoji)}
                              style={{
                                display: 'flex', alignItems: 'center', gap: '3px', padding: '2px 7px', borderRadius: '999px', fontSize: '11.5px', cursor: 'pointer',
                                border: r.reacted_by_me ? '1px solid rgba(235,93,30,0.3)' : '1px solid rgba(0,0,0,0.08)',
                                background: r.reacted_by_me ? 'rgba(235,93,30,0.08)' : 'var(--bg-elevated)',
                                color: 'var(--text)',
                              }}
                            >
                              <span>{r.emoji}</span>
                              <span style={{ fontFamily: 'var(--font-body)' }}>{r.count}</span>
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  )
                })
              )}
            </div>

            {pendingAttachment && (
              <div style={{ padding: '0 20px 8px', display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span className="badge badge-default">{pendingAttachment.file_name}</span>
                <button onClick={() => setPendingAttachment(null)} style={{ background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex' }}>
                  <Icon name="x" size={12} />
                </button>
              </div>
            )}
            {attachError && <p style={{ padding: '0 20px 8px', fontSize: '12px', color: 'var(--error)' }}>{attachError}</p>}

            <form onSubmit={(e) => void handleSend(e)} style={{ padding: '14px 20px', borderTop: '1px solid rgba(0,0,0,0.06)', display: 'flex', gap: '10px' }}>
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                style={{ display: 'none' }}
                onChange={(e) => { const f = e.target.files?.[0]; if (f) void handleFileSelect(f); e.target.value = '' }}
              />
              <button
                type="button"
                onClick={() => fileInputRef.current?.click()}
                disabled={!selectedSwimmerId || uploading}
                title="Attach an image"
                style={{ background: 'none', border: '1px solid rgba(0,0,0,0.12)', borderRadius: '10px', color: 'var(--text-muted)', cursor: 'pointer', padding: '10px', display: 'flex' }}
              >
                <Icon name="upload-cloud" size={15} />
              </button>
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
                disabled={sending || (!draft.trim() && !pendingAttachment) || !selectedSwimmerId}
                style={{
                  padding: '10px 18px',
                  borderRadius: '10px',
                  border: 'none',
                  background: sending || (!draft.trim() && !pendingAttachment) ? 'var(--text-subtle)' : 'var(--text)',
                  color: 'var(--bg-elevated)',
                  fontSize: '13.5px',
                  fontWeight: 600,
                  cursor: sending || (!draft.trim() && !pendingAttachment) ? 'not-allowed' : 'pointer',
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
