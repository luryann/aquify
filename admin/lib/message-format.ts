/** Emoji offered in the reaction picker on every messaging surface. */
export const QUICK_REACTIONS = ['👍', '❤️', '😂', '🎉', '😮', '😢']

/** Short timestamp for a message bubble, e.g. "Jul 4, 3:05 PM"; returns the input unchanged if it is not a date. */
export function formatMessageTime(value: string): string {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
}

type ReactionLike = { emoji: string; count: number; reacted_by_me: boolean }

/**
 * Apply a broadcast message update to the list. The server computes
 * `reacted_by_me` relative to the acting user, so a broadcast must never
 * overwrite this viewer's own flags: keep the previous per-emoji flag
 * (false for new emoji/messages) and take everything else from the event.
 */
export function mergeStreamMessage<M extends { id: string; reactions?: ReactionLike[] | null }>(prev: M[], incoming: M): M[] {
  const idx = prev.findIndex((m) => m.id === incoming.id)
  const old = idx === -1 ? undefined : prev[idx]
  const merged: M = incoming.reactions
    ? {
        ...incoming,
        reactions: incoming.reactions.map((r) => ({
          ...r,
          reacted_by_me: old?.reactions?.find((p) => p.emoji === r.emoji)?.reacted_by_me ?? false,
        })),
      }
    : incoming
  if (idx === -1) return [...prev, merged]
  const next = [...prev]
  next[idx] = merged
  return next
}
