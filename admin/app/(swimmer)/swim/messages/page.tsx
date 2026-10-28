'use client'

import { useState, useEffect, useRef, type FormEvent } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError, uiErrorMessage } from '@/lib/frontend-telemetry'
import { UnlinkedSwimmerNotice } from '@/components/swimmer/UnlinkedSwimmerNotice'
import { useMessageStream } from '@/lib/useMessageStream'
import {
  fetchMessages,
  sendMessage,
  editMessage,
  addReaction,
  removeReaction,
  markThreadRead,
  requestMessageUploadURL,
  type MessageRecord,
} from '@/lib/adminData'

const QUICK_REACTIONS = ['👍', '❤️', '😂', '🎉', '😮', '😢']

function fmtTime(value: string): string {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
}

function senderLabel(role: string): string {
  if (role === 'parent') return 'Parent'
  if (role === 'swimmer') return 'You'
  if (role === 'coach') return 'Coach'
  if (role === 'admin') return 'Club staff'
  return role || 'Staff'
}

export default function SwimmerMessagesPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const householdId = session?.user.householdId ?? ''
  const swimmerId = session?.user.swimmerId ?? ''
  const userId = session?.user.id ?? ''

  const [messages, setMessages] = useState<MessageRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
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

  const hasThread = Boolean(orgId && householdId && swimmerId)

  useEffect(() => {
    if (!hasThread) {
      setLoading(false)
      return
    }
    setLoading(true)
    setLoadError(null)
    fetchMessages(api, orgId, householdId, swimmerId)
      .then(setMessages)
      .catch((error) => {
        setLoadError(reportUIError('swim-messages-thread-load', error, 'Couldn’t load your messages. Try again.', { orgId, householdId, swimmerId }))
      })
      .finally(() => setLoading(false))
  }, [api, orgId, householdId, swimmerId, hasThread, reloadKey])

  useEffect(() => {
    if (!hasThread) return
    markThreadRead(api, orgId, householdId, swimmerId).catch(() => {})
  }, [api, orgId, householdId, swimmerId, hasThread])

  useMessageStream({
    orgId,
    token: session?.token,
    enabled: hasThread,
    onEvent: (event) => {
      if (event.household_id !== householdId || event.swimmer_id !== swimmerId) return
      setMessages((prev) => {
        const idx = prev.findIndex((m) => m.id === event.message.id)
        if (idx === -1) return [...prev, event.message]
        const next = [...prev]
        next[idx] = event.message
        return next
      })
      if (event.type === 'message.created' && event.message.sender_user_id !== userId) {
        markThreadRead(api, orgId, householdId, swimmerId).catch(() => {})
      }
    },
  })

  async function handleSend(e: FormEvent) {
    e.preventDefault()
    if (!hasThread || (!draft.trim() && !pendingAttachment)) return
    setSending(true)
    setSendError(null)
    try {
      await sendMessage(api, orgId, {
        household_id: householdId,
        swimmer_id: swimmerId,
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
    if (!hasThread) return
    setAttachError(null)
    setUploading(true)
    try {
      const { upload_url, object_key, expected_file_name } = await requestMessageUploadURL(api, orgId, {
        household_id: householdId,
        swimmer_id: swimmerId,
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
    if (!editDraft.trim()) return
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
    if (!hasThread) return
    const already = message.reactions?.some((r) => r.emoji === emoji && r.reacted_by_me)
    try {
      if (already) await removeReaction(api, orgId, message.id, emoji)
      else await addReaction(api, orgId, message.id, emoji)
      // Realtime (when connected) will also deliver this update, but don't
      // depend on it — refetch so the reaction shows immediately even
      // without Redis configured.
      const next = await fetchMessages(api, orgId, householdId, swimmerId)
      setMessages(next)
    } catch {
      // Leave state as-is; the user can retry.
    }
    setReactionPickerId(null)
  }

  return (
    <div>
      <div className="page-heading">
        <h1 className="page-title">Messages</h1>
        <p className="page-subtitle">Your parent can see this conversation too.</p>
      </div>

      {!hasThread ? (
        <UnlinkedSwimmerNotice />
      ) : (
        <div className="card" style={{ display: 'flex', flexDirection: 'column', minHeight: '560px' }}>
          <div style={{ flex: 1, padding: '20px', display: 'flex', flexDirection: 'column', gap: '16px', overflowY: 'auto', maxHeight: '600px' }}>
            {loadError ? (
              <p style={{ fontSize: 'var(--fs-base)', color: 'var(--error)' }}>{loadError}</p>
            ) : loading ? (
              <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>Loading conversation…</p>
            ) : messages.length === 0 ? (
              <p style={{ fontSize: 'var(--fs-base)', color: 'var(--text-muted)' }}>
                No messages yet. Send the first one below to reach your coach.
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
                          style={{ minWidth: '220px', padding: '8px 12px', borderRadius: '12px', border: '1px solid var(--accent-border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: 'var(--fs-base)', resize: 'vertical' }}
                        />
                        <button className="btn btn-sm btn-primary" onClick={() => void handleEditSave(m.id)}>Save</button>
                        <button className="btn btn-sm btn-secondary" onClick={() => setEditingId(null)}>Cancel</button>
                      </div>
                    ) : (
                      <div
                        style={{
                          background: mine ? 'var(--accent)' : 'var(--bg-elevated)',
                          color: mine ? 'var(--bg)' : 'var(--text)',
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
                            <img key={a.filename} src={a.download_url} alt={a.filename} style={{ display: 'block', marginTop: '8px', maxWidth: '220px', maxHeight: '220px', borderRadius: '8px' }} />
                          ) : null
                        ))}
                      </div>
                    )}

                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginTop: '4px', justifyContent: mine ? 'flex-end' : 'flex-start' }}>
                      <p style={{ fontSize: 'var(--fs-xs)', color: 'var(--text-muted)' }}>
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
                      <div style={{ display: 'flex', gap: '4px', marginTop: '4px', padding: '4px 6px', borderRadius: '12px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', width: 'fit-content' }}>
                        {QUICK_REACTIONS.map((emoji) => (
                          <button key={emoji} onClick={() => void handleToggleReaction(m, emoji)} style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: 'var(--fs-md)', padding: '2px' }}>
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
                              display: 'flex', alignItems: 'center', gap: '4px', padding: '2px 8px', borderRadius: '999px', fontSize: 'var(--fs-xs)', cursor: 'pointer',
                              border: r.reacted_by_me ? '1px solid var(--accent-border)' : '1px solid var(--border)',
                              background: r.reacted_by_me ? 'var(--accent-subtle)' : 'var(--bg-surface)',
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
          {attachError && <p style={{ padding: '0 20px 8px', fontSize: 'var(--fs-sm)', color: 'var(--error)' }}>{attachError}</p>}

          <form onSubmit={(e) => void handleSend(e)} style={{ padding: '16px 20px', borderTop: '1px solid var(--border)', display: 'flex', gap: '12px' }}>
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
              disabled={uploading}
              title="Attach an image"
              style={{ background: 'none', border: '1px solid var(--border)', borderRadius: '12px', color: 'var(--text-muted)', cursor: 'pointer', padding: '12px', display: 'flex' }}
            >
              <Icon name="upload-cloud" size={15} />
            </button>
            <input
              type="text"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder="Write a message to your coach…"
              disabled={sending}
              maxLength={4000}
              style={{ flex: 1, padding: '12px 16px', borderRadius: '12px', border: '1px solid var(--border)', background: 'var(--bg-elevated)', color: 'var(--text)', fontSize: 'var(--fs-base)', outline: 'none' }}
            />
            <button type="submit" className="btn btn-primary" disabled={sending || (!draft.trim() && !pendingAttachment)}>
              {sending ? 'Sending…' : 'Send'}
            </button>
          </form>
          {sendError && <p style={{ padding: '0 20px 16px', fontSize: 'var(--fs-sm)', color: 'var(--error)' }}>{sendError}</p>}
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
