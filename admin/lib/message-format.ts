/** Emoji offered in the reaction picker on every messaging surface. */
export const QUICK_REACTIONS = ['👍', '❤️', '😂', '🎉', '😮', '😢']

/** Short timestamp for a message bubble, e.g. "Jul 4, 3:05 PM"; returns the input unchanged if it is not a date. */
export function formatMessageTime(value: string): string {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('en-US', { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
}
