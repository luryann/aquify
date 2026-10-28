// Meet-event eligibility, mirroring the backend's validateSwimmerEventEligibility
// (server/internal/app/meets/service.go): gender via the event vocabulary, and
// age as of the meet's first day — the USA Swimming age-up rule. Filtering on
// the client keeps families from picking events the API will reject.

import { isSwimmerGenderEligibleForEvent } from './gender.ts'

const DATE_ONLY = /^(\d{4})-(\d{2})-(\d{2})/

function dateParts(value: string): [number, number, number] | null {
  const match = DATE_ONLY.exec(value.trim())
  if (!match) return null
  return [Number(match[1]), Number(match[2]), Number(match[3])]
}

/** Age on `refDate` (YYYY-MM-DD); falls back to today when refDate is missing/invalid. */
export function swimmerAgeAsOf(dateOfBirth: string, refDate?: string | null): number | null {
  const dob = dateParts(dateOfBirth)
  if (!dob) return null
  let ref = refDate ? dateParts(refDate) : null
  if (!ref) {
    const now = new Date()
    ref = [now.getFullYear(), now.getMonth() + 1, now.getDate()]
  }
  let age = ref[0] - dob[0]
  if (ref[1] < dob[1] || (ref[1] === dob[1] && ref[2] < dob[2])) age -= 1
  return age
}

type EligibilitySwimmer = { gender: string; date_of_birth: string }
type EligibilityEvent = { gender?: string | null; min_age?: number | null; max_age?: number | null }

export function isSwimmerEligibleForEvent(
  swimmer: EligibilitySwimmer,
  event: EligibilityEvent,
  meetStartDate?: string | null,
): boolean {
  if (!isSwimmerGenderEligibleForEvent(swimmer.gender, event.gender)) return false
  if (event.min_age == null && event.max_age == null) return true
  const age = swimmerAgeAsOf(swimmer.date_of_birth, meetStartDate)
  if (age == null) return false
  if (event.min_age != null && age < event.min_age) return false
  if (event.max_age != null && age > event.max_age) return false
  return true
}
