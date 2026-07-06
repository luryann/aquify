// Swimmer gender (`female|male|non_binary|undisclosed`) and meet-event gender
// (`M|F|X|O`) are two different vocabularies. Comparing them directly (e.g.
// `event.gender === swimmer.gender`) never matches for gendered events —
// always translate swimmer gender to the event vocabulary first.

/** Maps a swimmer's gender to the event-gender vocabulary it's eligible for. */
export function swimmerGenderToEventGender(swimmerGender: string): 'F' | 'M' | 'X' {
  const normalized = swimmerGender.trim().toUpperCase()
  if (normalized.startsWith('F')) return 'F'
  if (normalized.startsWith('M')) return 'M'
  return 'X' // non_binary / undisclosed / unknown → only open/mixed events
}

/** Whether a swimmer is eligible for an event's gender restriction (M/F/X/O). */
export function isSwimmerGenderEligibleForEvent(swimmerGender: string, eventGender?: string | null): boolean {
  const evGender = (eventGender ?? '').trim().toUpperCase()
  if (evGender !== 'M' && evGender !== 'F') return true // open/mixed/unset events accept any swimmer
  return swimmerGenderToEventGender(swimmerGender) === evGender
}
