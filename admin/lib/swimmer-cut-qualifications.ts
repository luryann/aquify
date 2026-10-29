import type {
  CutQualificationRecord,
  SwimmerRecord,
  TimeStandardRecord,
} from './adminData.ts'
import { localToday } from './dates.ts'
import { swimmerAgeAsOf } from './event-eligibility.ts'
import { swimmerGenderToEventGender } from './gender.ts'

export type SwimmerCutQualificationCard = {
  standard: TimeStandardRecord
  qualification: CutQualificationRecord
}

export function swimmerAgeOnDate(swimmer: SwimmerRecord, now = new Date()): number {
  // Calendar-date arithmetic shared with event eligibility, so a date-only DOB
  // never shifts across a timezone and both checks agree at a birthday boundary.
  return swimmerAgeAsOf(swimmer.date_of_birth, localToday(now)) ?? Number.NaN
}

export function filterApplicableTimeStandards(
  standards: TimeStandardRecord[],
  swimmer: SwimmerRecord,
  now = new Date(),
): TimeStandardRecord[] {
  const swimmerGender = swimmerGenderToEventGender(swimmer.gender)
  const age = swimmerAgeOnDate(swimmer, now)

  return standards.filter((standard) => {
    if (!standard.is_active) return false
    if (standard.gender && standard.gender !== 'X' && standard.gender !== swimmerGender) return false
    if (typeof standard.age_min === 'number' && age < standard.age_min) return false
    if (typeof standard.age_max === 'number' && age > standard.age_max) return false
    return true
  })
}

export function sortCutQualificationCards(cards: SwimmerCutQualificationCard[]): SwimmerCutQualificationCard[] {
  return [...cards].sort((a, b) => {
    if (a.qualification.qualified !== b.qualification.qualified) {
      return a.qualification.qualified ? -1 : 1
    }
    if (a.standard.course !== b.standard.course) {
      return a.standard.course.localeCompare(b.standard.course)
    }
    if (a.standard.event_distance !== b.standard.event_distance) {
      return a.standard.event_distance - b.standard.event_distance
    }
    return a.standard.stroke.localeCompare(b.standard.stroke)
  })
}
