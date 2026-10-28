'use client'

import { useState, useEffect, useMemo, useCallback, useRef, type FormEvent } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError, uiErrorMessage } from '@/lib/frontend-telemetry'
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
import './page.inline.css'

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
      setSwimmersError('We could not find your household details. Sign out and back in, or contact your club.')
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
        setSwimmersError(reportUIError('portal-messages-swimmers-load', error, 'Couldn’t load your swimmers. Try again.', { orgId }))
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
        setMessagesError(reportUIError('portal-messages-thread-load', error, 'Couldn’t load this conversation. Try again.', { orgId, householdId, swimmerId: selectedSwimmerId }))
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
      setSendError(uiErrorMessage(err, 'Couldn’t send your message. Try again.'))
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
      setAttachError('Couldn’t upload that image. Try a smaller file or a different format. Try again.')
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
      <div className="page-heading">
        <h1 className="page-title">
          Messages
        </h1>
      </div>

      {swimmersError ? (
        <div className="is-c34062e">
          {swimmersError}
        </div>
      ) : swimmersLoading ? (
        <div className="is-5ce7d6b">Loading your swimmers…</div>
      ) : swimmers.length === 0 ? (
        <div className="card is-3c1fe43" >
          <Icon name="users" size={32} style={{ color: 'var(--text-muted)', marginBottom: '16px' }} />
          <p className="is-5e8f68c">No swimmers on your account yet</p>
        </div>
      ) : (
        <div className="is-8ef476f">
          {/* Swimmer picker — a horizontal chip row rather than a permanent sidebar, since most
              households have only 1–3 swimmers and this reads better on a phone. */}
          {swimmers.length > 1 && (
            <div className="is-51144f2">
              {swimmers.map((s) => {
                const active = s.id === selectedSwimmerId
                return (
                  <button type="button"
                    key={s.id}
                    onClick={() => setSelectedSwimmerId(s.id)}
                    style={{
                      padding: '8px 16px',
                      borderRadius: '999px',
                      border: `1px solid ${active ? 'rgba(235,93,30,0.3)' : 'var(--border)'}`,
                      background: active ? 'rgba(235,93,30,0.08)' : 'var(--bg-elevated)',
                      color: active ? 'var(--text)' : 'var(--text-subtle)',
                      fontSize: 'var(--fs-base)',
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
          <div className="card" style={{ display: 'flex', flexDirection: 'column', minHeight: '520px' }}>
            <div className="is-541f5a9">
              <p className="is-7ae0218">
                {selectedSwimmer ? `${selectedSwimmer.first_name} ${selectedSwimmer.last_name}` : 'Conversation'}
              </p>
            </div>

            <div className="is-c4a895b">
              {messagesError ? (
                <p className="is-de107ca">{messagesError}</p>
              ) : messagesLoading ? (
                <p className="is-8fb176a">Loading conversation…</p>
              ) : messages.length === 0 ? (
                <p className="is-8fb176a">
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
                        <div className="is-3be73af">
                          <textarea aria-label="Edit message"
                            value={editDraft}
                            onChange={(e) => setEditDraft(e.target.value)}
                            rows={2}
                            className="is-d36963f"
                          />
                          <button type="button" className="btn btn-sm btn-primary" onClick={() => void handleEditSave(m.id)}>Save</button>
                          <button type="button" className="btn btn-sm btn-secondary" onClick={() => setEditingId(null)}>Cancel</button>
                        </div>
                      ) : (
                        <div
                          style={{
                            background: mine ? 'var(--accent)' : 'var(--bg-wash)',
                            color: mine ? 'var(--bg-elevated)' : 'var(--text)',
                            padding: '12px 16px',
                            borderRadius: mine ? '12px 12px 2px 12px' : '12px 12px 12px 2px',
                            fontSize: 'var(--fs-base)',
                            lineHeight: 1.5,
                            whiteSpace: 'pre-wrap',
                          }}
                        >
                          {m.body}
                          {m.attachments?.map((a) => (
                            a.download_url ? (
                              // eslint-disable-next-line @next/next/no-img-element
                              <img key={a.filename} src={a.download_url} alt={a.filename} className="is-5ca87c9" />
                            ) : null
                          ))}
                        </div>
                      )}

                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginTop: '4px', justifyContent: mine ? 'flex-end' : 'flex-start' }}>
                        <p className="is-c454211">
                          {senderLabel(m.sender_role)} · {fmtTime(m.created_at)}{m.edited_at ? ' · (edited)' : ''}
                        </p>
                        {canEdit && !isEditing && (
                          <button type="button"
                            onClick={() => { setEditingId(m.id); setEditDraft(m.body) }}
                            title="Edit"
                            className="is-8031a9d"
                          >
                            <Icon name="edit-2" size={11} />
                          </button>
                        )}
                        <button type="button"
                          onClick={() => setReactionPickerId(reactionPickerId === m.id ? null : m.id)}
                          title="React"
                          className="is-8031a9d"
                        >
                          <Icon name="plus" size={11} />
                        </button>
                      </div>

                      {reactionPickerId === m.id && (
                        <div className="is-1236a88">
                          {QUICK_REACTIONS.map((emoji) => (
                            <button type="button" key={emoji} onClick={() => void handleToggleReaction(m, emoji)} className="is-72e22d5">
                              {emoji}
                            </button>
                          ))}
                        </div>
                      )}

                      {m.reactions && m.reactions.length > 0 && (
                        <div style={{ display: 'flex', gap: '4px', marginTop: '4px', flexWrap: 'wrap', justifyContent: mine ? 'flex-end' : 'flex-start' }}>
                          {m.reactions.map((r) => (
                            <button type="button"
                              key={r.emoji}
                              onClick={() => void handleToggleReaction(m, r.emoji)}
                              style={{
                                display: 'flex', alignItems: 'center', gap: '4px', padding: '2px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', cursor: 'pointer',
                                border: r.reacted_by_me ? '1px solid rgba(235,93,30,0.3)' : '1px solid var(--border)',
                                background: r.reacted_by_me ? 'rgba(235,93,30,0.08)' : 'var(--bg-elevated)',
                                color: 'var(--text)',
                              }}
                            >
                              <span>{r.emoji}</span>
                              <span className="is-b88ab11">{r.count}</span>
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
              <div className="is-0d40ec1">
                <span className="badge badge-default">{pendingAttachment.file_name}</span>
                <button type="button" onClick={() => setPendingAttachment(null)} style={{ background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex' }}>
                  <Icon name="x" size={12} />
                </button>
              </div>
            )}
            {attachError && <p className="is-5d8c782">{attachError}</p>}

            <form onSubmit={(e) => void handleSend(e)} className="is-78213b9">
              <input aria-label="Attach a file"
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
                className="is-6c0ac09"
              >
                <Icon name="upload-cloud" size={15} />
              </button>
              <input aria-label="Write a message"
                type="text"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="Write a message…"
                disabled={sending || !selectedSwimmerId}
                maxLength={4000}
                className="is-1163eba"
              />
              <button
                type="submit"
                disabled={sending || (!draft.trim() && !pendingAttachment) || !selectedSwimmerId}
                style={{
                  padding: '12px 20px',
                  borderRadius: '12px',
                  border: 'none',
                  background: sending || (!draft.trim() && !pendingAttachment) ? 'var(--text-subtle)' : 'var(--text)',
                  color: 'var(--bg-elevated)',
                  fontSize: 'var(--fs-base)',
                  fontWeight: 600,
                  cursor: sending || (!draft.trim() && !pendingAttachment) ? 'not-allowed' : 'pointer',
                }}
              >
                {sending ? 'Sending…' : 'Send'}
              </button>
            </form>
            {sendError && <p className="is-2991f33">{sendError}</p>}
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
