import type {
  CutQualificationRecord,
  SwimmerRecord,
  TimeStandardRecord,
} from './adminData.ts'

export type SwimmerCutQualificationCard = {
  standard: TimeStandardRecord
  qualification: CutQualificationRecord
}

export function swimmerAgeOnDate(swimmer: SwimmerRecord, now = new Date()): number {
  const dob = new Date(swimmer.date_of_birth)
  let age = now.getFullYear() - dob.getFullYear()
  const monthDelta = now.getMonth() - dob.getMonth()
  if (monthDelta < 0 || (monthDelta === 0 && now.getDate() < dob.getDate())) {
    age--
  }
  return age
}

export function filterApplicableTimeStandards(
  standards: TimeStandardRecord[],
  swimmer: SwimmerRecord,
  now = new Date(),
): TimeStandardRecord[] {
  const swimmerGender = swimmer.gender.trim().toUpperCase().startsWith('F')
    ? 'F'
    : swimmer.gender.trim().toUpperCase().startsWith('M')
      ? 'M'
      : 'X'
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
