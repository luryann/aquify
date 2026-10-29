'use client'

import { useEffect, useState, type FormEvent } from 'react'
import type { ApiClient } from './api'
import {
  addReaction,
  editMessage,
  fetchMessages,
  markThreadRead,
  removeReaction,
  requestMessageUploadURL,
  sendMessage,
  type MessageRecord,
} from './adminData'
import { reportUIError, uiErrorMessage } from './frontend-telemetry'
import { useMessageStream } from './useMessageStream'

type PendingAttachment = { object_key: string; file_name: string; file_mime: string }

type UseMessageThreadOptions = {
  api: ApiClient
  orgId: string
  householdId: string
  /** The swimmer whose household thread is shown; empty until one is selected. */
  swimmerId: string
  userId: string
  token: string | undefined
  /** Telemetry key and user-facing text used when the thread fails to load. */
  loadErrorScope: string
  loadErrorMessage: string
}

/**
 * State and actions for one household/swimmer message thread: load, realtime
 * updates, send, attachments, edit and reactions. Shared by the parent and
 * swimmer portals so both behave the same way.
 */
export function useMessageThread({
  api,
  orgId,
  householdId,
  swimmerId,
  userId,
  token,
  loadErrorScope,
  loadErrorMessage,
}: UseMessageThreadOptions) {
  const hasThread = Boolean(orgId && householdId && swimmerId)

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
  const [pendingAttachment, setPendingAttachment] = useState<PendingAttachment | null>(null)
  const [attachError, setAttachError] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)

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
        setLoadError(reportUIError(loadErrorScope, error, loadErrorMessage, { orgId, householdId, swimmerId }))
      })
      .finally(() => setLoading(false))
  }, [api, orgId, householdId, swimmerId, hasThread, reloadKey, loadErrorScope, loadErrorMessage])

  useEffect(() => {
    if (!hasThread) return
    markThreadRead(api, orgId, householdId, swimmerId).catch((error) => { reportUIError(`${loadErrorScope}.mark-read`, error, 'Couldn’t mark this thread read.') })
  }, [api, orgId, householdId, swimmerId, hasThread, loadErrorScope])

  useMessageStream({
    orgId,
    token,
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
        markThreadRead(api, orgId, householdId, swimmerId).catch((error) => { reportUIError(`${loadErrorScope}.mark-read`, error, 'Couldn’t mark this thread read.') })
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
      setAttachError('Couldn’t upload that image. Try a smaller file or a different format.')
    } finally {
      setUploading(false)
    }
  }

  async function handleEditSave(messageId: string) {
    if (!editDraft.trim()) return
    setSendError(null)
    try {
      const updated = await editMessage(api, orgId, messageId, editDraft.trim())
      setMessages((prev) => prev.map((m) => (m.id === messageId ? updated : m)))
      setEditingId(null)
    } catch (err) {
      // Keep the editor open with the draft so the user can retry.
      setSendError(reportUIError(`${loadErrorScope}.edit`, err, 'Couldn’t save your edit. Try again.'))
    }
  }

  async function handleToggleReaction(message: MessageRecord, emoji: string) {
    if (!hasThread) return
    const already = message.reactions?.some((r) => r.emoji === emoji && r.reacted_by_me)
    setSendError(null)
    try {
      if (already) await removeReaction(api, orgId, message.id, emoji)
      else await addReaction(api, orgId, message.id, emoji)
      // Realtime (when connected) will also deliver this update, but don't
      // depend on it — refetch so the reaction shows immediately even
      // without Redis configured.
      setMessages(await fetchMessages(api, orgId, householdId, swimmerId))
    } catch (err) {
      setSendError(reportUIError(`${loadErrorScope}.reaction`, err, 'Couldn’t update that reaction. Try again.'))
    }
    setReactionPickerId(null)
  }

  return {
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
  }
}
