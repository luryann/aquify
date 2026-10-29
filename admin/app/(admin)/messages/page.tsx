'use client'

import { useEffect, useMemo, useState, useCallback, useRef, type FormEvent } from 'react'
import { EmptyState } from '@/components/ui/EmptyState'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { useMessageStream } from '@/lib/useMessageStream'
import {
  fetchMessageThreads,
  fetchMessages,
  sendMessage,
  editMessage,
  addReaction,
  removeReaction,
  markThreadRead,
  requestMessageUploadURL,
  fetchHouseholds,
  fetchSwimmers,
  type MessageThreadRecord,
  type MessageRecord,
  type HouseholdRecord,
  type SwimmerRecord,
} from '@/lib/adminData'
import { uiErrorMessage } from '@/lib/frontend-telemetry'
import { QUICK_REACTIONS } from '@/lib/message-format'

function formatDateTime(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function senderLabel(role: string): string {
  if (role === 'parent' || role === 'swimmer') return 'Family'
  if (role === 'coach') return 'Coach'
  if (role === 'admin') return 'Staff'
  return role || 'Staff'
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/)
  return ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase() || '?'
}

export default function MessagesInboxPage() {
  const { session } = useSession()
  const api = useApiClient(session?.token)
  const orgId = session?.user.orgId ?? ''
  const userId = session?.user.id ?? ''

  const [threads, setThreads] = useState<MessageThreadRecord[]>([])
  const [households, setHouseholds] = useState<HouseholdRecord[]>([])
  const [swimmers, setSwimmers] = useState<SwimmerRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<{ householdId: string; swimmerId: string } | null>(null)
  const [search, setSearch] = useState('')

  const [messages, setMessages] = useState<MessageRecord[]>([])
  const [threadLoading, setThreadLoading] = useState(false)
  const [threadError, setThreadError] = useState<string | null>(null)
  const [threadReloadKey, setThreadReloadKey] = useState(0)

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

  const selectedRef = useRef(selected)
  selectedRef.current = selected

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
        if (!cancelled) setError(uiErrorMessage(err, 'Couldn’t load message threads. Try again.'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [api, orgId])

  const householdNameById = useMemo(() => new Map(households.map((h) => [h.id, h.household_name])), [households])
  const swimmerNameById = useMemo(() => new Map(swimmers.map((s) => [s.id, `${s.first_name} ${s.last_name}`])), [swimmers])

  const visibleThreads = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return threads
    return threads.filter((t) => {
      const swimmerName = swimmerNameById.get(t.swimmer_id) ?? ''
      const householdName = householdNameById.get(t.household_id) ?? ''
      return `${swimmerName} ${householdName}`.toLowerCase().includes(q)
    })
  }, [threads, search, swimmerNameById, householdNameById])

  // Staff can open a conversation with any swimmer's family; if no thread
  // exists yet, show a placeholder until the first message creates it.
  const startConversation = useCallback((swimmerId: string) => {
    const swimmer = swimmers.find((s) => s.id === swimmerId)
    if (!swimmer) return
    const key = { householdId: swimmer.household_id, swimmerId: swimmer.id }
    setThreads((prev) => prev.some((t) => t.household_id === key.householdId && t.swimmer_id === key.swimmerId)
      ? prev
      : [{ household_id: key.householdId, swimmer_id: key.swimmerId, message_count: 0, last_body: 'New conversation', last_sender_role: '', last_message_at: '', unread_count: 0 }, ...prev])
    setSearch('')
    setSelected(key)
  }, [swimmers])

  const refreshThreads = useCallback(() => {
    if (!orgId) return
    fetchMessageThreads(api, orgId).then(setThreads).catch(() => {})
  }, [api, orgId])

  const loadThread = useCallback(() => {
    if (!orgId || !selected) return
    setThreadLoading(true)
    setThreadError(null)
    fetchMessages(api, orgId, selected.householdId, selected.swimmerId)
      .then(setMessages)
      .catch((err) => setThreadError(uiErrorMessage(err, 'Couldn’t load this conversation. Try again.')))
      .finally(() => setThreadLoading(false))
  }, [api, orgId, selected])

  useEffect(() => {
    loadThread()
  }, [loadThread, threadReloadKey])

  // Mark the opened thread read, and reflect it locally without waiting on a refetch.
  useEffect(() => {
    if (!orgId || !selected) return
    markThreadRead(api, orgId, selected.householdId, selected.swimmerId).catch(() => {})
    setThreads((prev) => prev.map((t) =>
      t.household_id === selected.householdId && t.swimmer_id === selected.swimmerId ? { ...t, unread_count: 0 } : t,
    ))
  }, [api, orgId, selected])

  useMessageStream({
    orgId,
    token: session?.token,
    onEvent: (event) => {
      refreshThreads()
      const current = selectedRef.current
      if (!current || current.householdId !== event.household_id || current.swimmerId !== event.swimmer_id) return
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

  async function handleSend(e: FormEvent) {
    e.preventDefault()
    if (!orgId || !selected || (!draft.trim() && !pendingAttachment)) return
    setSending(true)
    setSendError(null)
    try {
      await sendMessage(api, orgId, {
        household_id: selected.householdId,
        swimmer_id: selected.swimmerId,
        body: draft.trim() || '(attachment)',
        attachments: pendingAttachment ? [pendingAttachment] : undefined,
      })
      setDraft('')
      setPendingAttachment(null)
      setThreadReloadKey((k) => k + 1)
      refreshThreads()
    } catch (err) {
      setSendError(uiErrorMessage(err, 'Couldn’t send your reply. Try again.'))
    } finally {
      setSending(false)
    }
  }

  async function handleFileSelect(file: File) {
    if (!orgId || !selected) return
    setAttachError(null)
    setUploading(true)
    try {
      const { upload_url, object_key, expected_file_name } = await requestMessageUploadURL(api, orgId, {
        household_id: selected.householdId,
        swimmer_id: selected.swimmerId,
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
    if (!orgId || !selected) return
    const already = message.reactions?.some((r) => r.emoji === emoji && r.reacted_by_me)
    try {
      if (already) await removeReaction(api, orgId, message.id, emoji)
      else await addReaction(api, orgId, message.id, emoji)
      // Realtime (when connected) will also deliver this update, but don't
      // depend on it — refetch so the reaction shows immediately even
      // without Redis configured.
      const next = await fetchMessages(api, orgId, selected.householdId, selected.swimmerId)
      setMessages(next)
    } catch {
      // Leave state as-is; the user can retry.
    }
    setReactionPickerId(null)
  }

  return (
    <div className="page-fill">
      <div className="u-mb-20px" style={{ flexShrink: 0 }}>
        <div className="u-gap-16px" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', flexWrap: 'wrap' }}>
          <div>
            <h1 className="page-title">Messages</h1>
          </div>
          {!loading && !error && swimmers.length > 0 && (
            <select
              aria-label="Start a conversation"
              value=""
              onChange={(e) => { if (e.target.value) startConversation(e.target.value) }}
              className="u-p-8px-12px u-fs-fs-base" style={{ maxWidth: '280px' }}
            >
              <option value="">New message to a family…</option>
              {[...swimmers]
                .sort((a, b) => `${a.last_name} ${a.first_name}`.localeCompare(`${b.last_name} ${b.first_name}`))
                .map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.last_name}, {s.first_name} — {householdNameById.get(s.household_id) ?? 'Household'}
                  </option>
                ))}
            </select>
          )}
        </div>
      </div>

      {loading ? (
        <EmptyState icon="mail" title="Loading messages" description="Fetching message threads." />
      ) : error ? (
        <EmptyState icon="alert-circle" title="Unable to load messages" description={error} />
      ) : threads.length === 0 ? (
        <EmptyState icon="mail" title="No conversations yet" description="Messages from families appear here. Use “New message to a family” to start one." />
      ) : (
        <div className="split-list u-gap-16px" style={{ flex: 1, minHeight: 0, alignItems: 'stretch' }}>
          <div className="card" style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
            <div className="u-p-12px" style={{ borderBottom: '1px solid var(--border)', flexShrink: 0 }}>
              <div style={{ position: 'relative' }}>
                <span style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }}>
                  <Icon name="search" size={13} />
                </span>
                <input aria-label="Search conversations"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search conversations…"
                  className="u-p-8px-12px-8px-32px u-fs-fs-sm" style={{ width: '100%' }}
                />
              </div>
            </div>
            <div className="u-p-6px" style={{ overflowY: 'auto', flex: 1 }}>
              {visibleThreads.map((t) => {
                const active = selected?.householdId === t.household_id && selected?.swimmerId === t.swimmer_id
                const swimmerName = swimmerNameById.get(t.swimmer_id) ?? 'Unknown swimmer'
                return (
                  <button
                    key={`${t.household_id}-${t.swimmer_id}`}
                    onClick={() => setSelected({ householdId: t.household_id, swimmerId: t.swimmer_id })}
                    className="u-gap-12px u-p-12px u-mb-2px" style={{ width: '100%', textAlign: 'left', display: 'flex', borderRadius: 'var(--r-md)', border: active ? '1px solid var(--accent-border)' : '1px solid transparent', background: active ? 'var(--accent-subtle)' : 'transparent', cursor: 'pointer' }}
                  >
                    <div style={{ position: 'relative', flexShrink: 0 }}>
                      <div className="u-fs-fs-xs u-fw-600" style={{ width: '32px', height: '32px', borderRadius: '50%', background: 'var(--bg-elevated)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-subtle)' }}>
                        {initials(swimmerName)}
                      </div>
                      {t.unread_count > 0 && (
                        <span className="u-p-0-4px u-fs-fs-2xs u-fw-700 u-ff-font-body" style={{ position: 'absolute', top: '-4px', right: '-4px', minWidth: '16px', height: '16px', borderRadius: '999px', background: 'var(--accent)', color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                          {t.unread_count > 9 ? '9+' : t.unread_count}
                        </span>
                      )}
                    </div>
                    <div style={{ minWidth: 0, flex: 1 }}>
                      <div className="u-gap-8px" style={{ display: 'flex', justifyContent: 'space-between' }}>
                        <span className="u-fs-fs-base" style={{ fontWeight: t.unread_count > 0 ? 700 : 600, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {swimmerName}
                        </span>
                        <span className="u-fs-fs-2xs u-ff-font-body" style={{ color: 'var(--text-muted)', flexShrink: 0 }}>
                          {formatDateTime(t.last_message_at)}
                        </span>
                      </div>
                      <div className="u-fs-fs-xs u-mb-2px" style={{ color: 'var(--text-muted)' }}>
                        {householdNameById.get(t.household_id) ?? 'Unknown household'}
                      </div>
                      <div className="u-fs-fs-sm" style={{ color: 'var(--text-subtle)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {t.last_body}
                      </div>
                    </div>
                  </button>
                )
              })}
            </div>
          </div>

          <div className="card" style={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
            <div className="u-p-16px-20px" style={{ borderBottom: '1px solid var(--border)', flexShrink: 0 }}>
              <p className="u-fs-fs-md u-fw-600" style={{ color: 'var(--text)' }}>
                {selected ? (swimmerNameById.get(selected.swimmerId) ?? 'Conversation') : 'Select a conversation'}
              </p>
              {selected && (
                <p className="u-fs-fs-sm" style={{ color: 'var(--text-muted)' }}>
                  {householdNameById.get(selected.householdId) ?? ''}
                </p>
              )}
            </div>

            <div className="u-p-20px u-gap-16px" style={{ flex: 1, display: 'flex', flexDirection: 'column', overflowY: 'auto', minHeight: 0 }}>
              {threadError ? (
                <p className="u-fs-fs-base" style={{ color: 'var(--error)' }}>{threadError}</p>
              ) : threadLoading ? (
                <p className="page-subtitle">Loading conversation…</p>
              ) : messages.length === 0 ? (
                <p className="page-subtitle">No messages in this thread yet.</p>
              ) : (
                messages.map((m) => {
                  // Bubble side/color reflects staff-vs-family, not which specific
                  // staff member sent it, so multi-staff threads stay grouped.
                  const mine = m.sender_role !== 'parent' && m.sender_role !== 'swimmer'
                  const canEdit = m.sender_user_id === userId
                  const isEditing = editingId === m.id
                  return (
                    <div key={m.id} style={{ alignSelf: mine ? 'flex-end' : 'flex-start', maxWidth: '75%', position: 'relative' }} className="message-bubble-group">
                      {isEditing ? (
                        <div className="u-gap-6px" style={{ display: 'flex', alignItems: 'flex-end' }}>
                          <textarea
                            value={editDraft}
                            onChange={(e) => setEditDraft(e.target.value)}
                            rows={2}
                            className="u-p-8px-12px u-fs-fs-base" style={{ minWidth: '220px', resize: 'vertical' }}
                          />
                          <button className="btn btn-sm btn-primary" onClick={() => void handleEditSave(m.id)}>Save</button>
                          <button className="btn btn-sm btn-secondary" onClick={() => setEditingId(null)}>Cancel</button>
                        </div>
                      ) : (
                        <div
                          className="u-p-12px-16px u-fs-fs-base u-lh-1_5" style={{ background: mine ? 'var(--accent)' : 'var(--bg-elevated)', color: mine ? '#fff' : 'var(--text)', borderRadius: mine ? '12px 12px 2px 12px' : '12px 12px 12px 2px', whiteSpace: 'pre-wrap' }}
                        >
                          {m.body}
                          {m.attachments?.map((a) => (
                            a.download_url ? (
                              // eslint-disable-next-line @next/next/no-img-element
                              <img key={a.filename} src={a.download_url} alt={a.filename} className="u-mt-8px" style={{ display: 'block', maxWidth: '220px', maxHeight: '220px', borderRadius: '8px' }} />
                            ) : null
                          ))}
                        </div>
                      )}

                      <div className="u-gap-8px u-mt-4px" style={{ display: 'flex', alignItems: 'center', justifyContent: mine ? 'flex-end' : 'flex-start' }}>
                        <p className="u-fs-fs-xs" style={{ color: 'var(--text-muted)' }}>
                          {senderLabel(m.sender_role)} · {formatDateTime(m.created_at)}{m.edited_at ? ' · (edited)' : ''}
                        </p>
                        {canEdit && !isEditing && (
                          <button
                            onClick={() => { setEditingId(m.id); setEditDraft(m.body) }}
                            title="Edit"
                            className="u-p-2px" style={{ background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex' }}
                          >
                            <Icon name="edit-2" size={11} />
                          </button>
                        )}
                        <button
                          onClick={() => setReactionPickerId(reactionPickerId === m.id ? null : m.id)}
                          title="React"
                          className="u-p-2px" style={{ background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex' }}
                        >
                          <Icon name="plus" size={11} />
                        </button>
                      </div>

                      {reactionPickerId === m.id && (
                        <div className="u-gap-4px u-mt-4px u-p-4px-6px" style={{ display: 'flex', borderRadius: 'var(--r-md)', border: '1px solid var(--border)', background: 'var(--bg-elevated)', width: 'fit-content' }}>
                          {QUICK_REACTIONS.map((emoji) => (
                            <button key={emoji} onClick={() => void handleToggleReaction(m, emoji)} className="u-fs-fs-md u-p-2px" style={{ background: 'none', border: 'none', cursor: 'pointer' }}>
                              {emoji}
                            </button>
                          ))}
                        </div>
                      )}

                      {m.reactions && m.reactions.length > 0 && (
                        <div className="u-gap-4px u-mt-4px" style={{ display: 'flex', flexWrap: 'wrap', justifyContent: mine ? 'flex-end' : 'flex-start' }}>
                          {m.reactions.map((r) => (
                            <button
                              key={r.emoji}
                              onClick={() => void handleToggleReaction(m, r.emoji)}
                              className="u-gap-4px u-p-2px-8px u-fs-fs-xs" style={{ display: 'flex', alignItems: 'center', borderRadius: '999px', cursor: 'pointer', border: r.reacted_by_me ? '1px solid var(--accent-border)' : '1px solid var(--border)', background: r.reacted_by_me ? 'var(--accent-subtle)' : 'var(--bg-surface)', color: 'var(--text)' }}
                            >
                              <span>{r.emoji}</span>
                              <span className="u-ff-font-body" >{r.count}</span>
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
              <div className="u-p-0-20px-8px u-gap-8px" style={{ display: 'flex', alignItems: 'center' }}>
                <span className="badge badge-default">{pendingAttachment.file_name}</span>
                <button onClick={() => setPendingAttachment(null)} style={{ background: 'none', border: 'none', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex' }}>
                  <Icon name="x" size={12} />
                </button>
              </div>
            )}
            {attachError && <p className="u-p-0-20px-8px u-fs-fs-sm" style={{ color: 'var(--error)' }}>{attachError}</p>}

            <form onSubmit={(e) => void handleSend(e)} className="u-p-16px-20px u-gap-12px" style={{ borderTop: '1px solid var(--border)', display: 'flex', flexShrink: 0 }}>
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
                disabled={!selected || uploading}
                title="Attach an image"
                className="btn-icon u-p-8px"
                style={{ background: 'none', border: '1px solid var(--border)', borderRadius: 'var(--r-md)', color: 'var(--text-muted)', cursor: 'pointer', display: 'flex' }}
              >
                <Icon name="upload-cloud" size={15} />
              </button>
              <input aria-label="Reply to this family"
                type="text"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="Reply to this family…"
                disabled={!selected || sending}
                maxLength={4000}
                className="u-p-8px-12px u-fs-fs-base" style={{ flex: 1 }}
              />
              <button type="submit" className="btn btn-primary" disabled={!selected || sending || (!draft.trim() && !pendingAttachment)}>
                {sending ? 'Sending…' : 'Send'}
              </button>
            </form>
            {sendError && <p className="u-fs-fs-sm" style={{ padding: '0 20px 16px', color: 'var(--error)' }}>{sendError}</p>}
          </div>
        </div>
      )}
    </div>
  )
}
