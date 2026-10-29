'use client'

import { useState, useEffect, useMemo, useRef } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { reportUIError } from '@/lib/frontend-telemetry'
import { useMessageThread } from '@/lib/useMessageThread'
import { fetchSwimmers, type SwimmerRecord } from '@/lib/adminData'
import './page.inline.css'
import { QUICK_REACTIONS, formatMessageTime } from '@/lib/message-format'

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

  const {
    messages,
    loading: messagesLoading,
    loadError: messagesError,
    draft,
    setDraft,
    sending,
    sendError,
    editingId,
    setEditingId,
    editDraft,
    setEditDraft,
    reactionPickerId,
    setReactionPickerId,
    pendingAttachment,
    setPendingAttachment,
    attachError,
    uploading,
    handleSend,
    handleFileSelect,
    handleEditSave,
    handleToggleReaction,
  } = useMessageThread({
    api,
    orgId,
    householdId,
    swimmerId: selectedSwimmerId,
    userId,
    token: session?.token,
    loadErrorScope: 'portal-messages-thread-load',
    loadErrorMessage: 'Couldn’t load this conversation. Try again.',
  })
  const fileInputRef = useRef<HTMLInputElement | null>(null)

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

  const selectedSwimmer = useMemo(
    () => swimmers.find((s) => s.id === selectedSwimmerId),
    [swimmers, selectedSwimmerId],
  )

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
                          {senderLabel(m.sender_role)} · {formatMessageTime(m.created_at)}{m.edited_at ? ' · (edited)' : ''}
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

          </div>
  )
}
