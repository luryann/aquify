export type HouseholdStatus = 'active' | 'inactive' | 'archived'

export function normalizeHouseholdStatus(status: string | null | undefined): HouseholdStatus {
  switch (status) {
    case 'inactive':
    case 'archived':
      return status
    default:
      return 'active'
  }
}
