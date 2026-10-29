'use client'

import { useRef } from 'react'
import { Icon } from '@/components/ui/icons'
import { useSession } from '@/lib/session'
import { useApiClient } from '@/lib/api'
import { UnlinkedSwimmerNotice } from '@/components/swimmer/UnlinkedSwimmerNotice'
import { useMessageThread } from '@/lib/useMessageThread'
import './page.inline.css'
import { QUICK_REACTIONS, formatMessageTime } from '@/lib/message-format'

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

  const {
    hasThread,
    messages,
    loading,
    loadError,
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
    swimmerId,
    userId,
    token: session?.token,
    loadErrorScope: 'swim-messages-thread-load',
    loadErrorMessage: 'Couldn’t load your messages. Try again.',
  })
  const fileInputRef = useRef<HTMLInputElement | null>(null)

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
          <div className="is-e566a3d">
            {loadError ? (
              <p className="is-de107ca">{loadError}</p>
            ) : loading ? (
              <p className="is-8fb176a">Loading conversation…</p>
            ) : messages.length === 0 ? (
              <p className="is-8fb176a">
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
                      <div className="is-3be73af">
                        <textarea aria-label="Edit message"
                          value={editDraft}
                          onChange={(e) => setEditDraft(e.target.value)}
                          rows={2}
                          className="is-ef385dc"
                        />
                        <button type="button" className="btn btn-sm btn-primary" onClick={() => void handleEditSave(m.id)}>Save</button>
                        <button type="button" className="btn btn-sm btn-secondary" onClick={() => setEditingId(null)}>Cancel</button>
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
                              border: r.reacted_by_me ? '1px solid var(--accent-border)' : '1px solid var(--border)',
                              background: r.reacted_by_me ? 'var(--accent-subtle)' : 'var(--bg-surface)',
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
              disabled={uploading}
              title="Attach an image"
              className="is-4a025ad"
            >
              <Icon name="upload-cloud" size={15} />
            </button>
            <input aria-label="Write a message"
              type="text"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder="Write a message to your coach…"
              disabled={sending}
              maxLength={4000}
              className="is-d884681"
            />
            <button type="submit" className="btn btn-primary" disabled={sending || (!draft.trim() && !pendingAttachment)}>
              {sending ? 'Sending…' : 'Send'}
            </button>
          </form>
          {sendError && <p className="is-2991f33">{sendError}</p>}
        </div>
      )}

          </div>
  )
}
