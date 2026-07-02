import type {
  DocumentTypeRecord,
  GroupRecord,
  RegistrationRecord,
  SwimmerRecord,
} from './adminData'

export type PortalRegistrationSeasonOption = {
  id: string
  label: string
}

export type PortalRegistrationProgramOption = {
  id: string
  name: string
  description?: string
  type: string
  group: GroupRecord
}

export type PortalRegistrationWaiverOption = {
  id: string
  title: string
  required: boolean
  body: string
  appliesTo: string
  waiverType: string
}

export function derivePortalRegistrationSeasons(
  registrations: RegistrationRecord[],
  now = new Date(),
): PortalRegistrationSeasonOption[] {
  const currentYear = now.getUTCMonth() >= 7 ? now.getUTCFullYear() : now.getUTCFullYear() - 1
  const fallback = `${currentYear}-${String((currentYear + 1) % 100).padStart(2, '0')}`
  const seasons = new Set<string>([fallback])
  for (const registration of registrations) {
    if (registration.season.trim()) seasons.add(registration.season.trim())
  }
  return [...seasons]
    .sort()
    .reverse()
    .map((season) => ({ id: season, label: season }))
}

export function derivePortalRegistrationPrograms(groups: GroupRecord[]): PortalRegistrationProgramOption[] {
  return [...groups]
    .filter((group) => group.is_active)
    .sort((a, b) => a.name.localeCompare(b.name))
    .map((group) => ({
      id: group.id,
      name: group.name || group.code,
      description: group.description?.trim() || undefined,
      type: group.type,
      group,
    }))
}

export function deriveEligiblePortalRegistrationSwimmers(
  swimmers: SwimmerRecord[],
  registrations: RegistrationRecord[],
  season: string,
  groupId: string,
): Array<SwimmerRecord & { alreadyRegistered: boolean }> {
  const registeredSwimmerIds = new Set(
    registrations
      .filter((registration) => registration.season === season && registration.group_id === groupId)
      .map((registration) => registration.swimmer_id),
  )

  return [...swimmers]
    .sort((a, b) => `${a.first_name} ${a.last_name}`.localeCompare(`${b.first_name} ${b.last_name}`))
    .map((swimmer) => ({
      ...swimmer,
      alreadyRegistered: registeredSwimmerIds.has(swimmer.id),
    }))
}

function buildWaiverType(name: string): string {
  return name.trim().toLowerCase()
}

export function derivePortalRegistrationWaivers(documentTypes: DocumentTypeRecord[]): PortalRegistrationWaiverOption[] {
  return [...documentTypes]
    .filter((type) => type.is_active && (type.category === 'waiver' || type.category === 'registration'))
    .sort((a, b) => {
      if (a.is_required !== b.is_required) return a.is_required ? -1 : 1
      return a.name.localeCompare(b.name)
    })
    .map((type) => ({
      id: type.id,
      title: type.name,
      required: type.is_required,
      body: type.description?.trim() || 'No additional waiver copy has been published for this requirement yet.',
      appliesTo: type.applies_to,
      waiverType: buildWaiverType(type.name),
    }))
}
