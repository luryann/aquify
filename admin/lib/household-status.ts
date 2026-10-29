export type HouseholdStatus = 'active' | 'inactive' | 'archived' | 'unknown'

export function normalizeHouseholdStatus(status: string | null | undefined): HouseholdStatus {
  switch (status) {
    case 'inactive':
    case 'archived':
      return status
    case 'active':
    case undefined:
    case null:
    case '':
      return 'active'
    default:
      // A status this build does not know about must not read as active.
      return 'unknown'
  }
}
