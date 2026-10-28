'use client'

import { useEffect, useRef } from 'react'
import type { MessageRecord } from './adminData'

// A manual fetch-based SSE reader rather than the native EventSource API.
// EventSource cannot set custom headers, so it can't carry the Authorization
// bearer token or X-Org-Id header this app's API requires on every other
// call — using fetch() here reuses the exact same auth transport instead of
// inventing a token-in-URL scheme or a cookie-auth carve-out in the backend's
// org-scope middleware.
export type MessageStreamEvent = {
  type: 'message.created' | 'message.updated' | 'message.reaction_added' | 'message.reaction_removed'
  household_id: string
  swimmer_id: string
  message: MessageRecord
}

type UseMessageStreamOptions = {
  orgId: string | undefined
  token: string | undefined
  onEvent: (event: MessageStreamEvent) => void
  enabled?: boolean
}

const RECONNECT_MIN_MS = 2000
const RECONNECT_MAX_MS = 30000

export function useMessageStream({ orgId, token, onEvent, enabled = true }: UseMessageStreamOptions) {
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent

  useEffect(() => {
    if (!enabled || !orgId) return

    let stopped = false
    let controller: AbortController | null = null
    let reconnectDelay = RECONNECT_MIN_MS
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null

    async function connect() {
      if (stopped) return
      controller = new AbortController()

      try {
        const headers: Record<string, string> = { Accept: 'text/event-stream', 'X-Org-Id': orgId! }
        if (token) headers['Authorization'] = `Bearer ${token}`

        const res = await fetch('/api/v1/messages/stream', {
          headers,
          credentials: 'include',
          signal: controller.signal,
        })
        // 503 means this deployment has no realtime broadcaster (no Redis);
        // that will not change by retrying, so stay on polling/refresh.
        if (res.status === 503) {
          stopped = true
          return
        }
        if (!res.ok || !res.body) {
          throw new Error(`stream unavailable: ${res.status}`)
        }

        reconnectDelay = RECONNECT_MIN_MS
        const reader = res.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''

        while (!stopped) {
          const { value, done } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })

          let boundary = buffer.indexOf('\n\n')
          while (boundary !== -1) {
            const frame = buffer.slice(0, boundary)
            buffer = buffer.slice(boundary + 2)
            boundary = buffer.indexOf('\n\n')

            const dataLine = frame.split('\n').find((line) => line.startsWith('data:'))
            if (!dataLine) continue
            const payload = dataLine.slice('data:'.length).trim()
            if (!payload) continue
            try {
              onEventRef.current(JSON.parse(payload) as MessageStreamEvent)
            } catch {
              // Ignore malformed frames rather than tearing down the stream.
            }
          }
        }
      } catch {
        // Connection dropped or failed to open — fall through to reconnect.
      }

      if (!stopped) {
        reconnectTimer = setTimeout(() => {
          reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_MS)
          void connect()
        }, reconnectDelay)
      }
    }

    void connect()

    return () => {
      stopped = true
      controller?.abort()
      if (reconnectTimer) clearTimeout(reconnectTimer)
    }
  }, [orgId, token, enabled])
}
