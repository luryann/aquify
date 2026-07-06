import type { Role } from './roles'

export const LOCAL_DEV_ORG_ID = '00000000-0000-0000-0000-00000000a001'
// Fixed demo household the stub parent/swimmer are linked to (mirrors the
// backend authn/localdev.go LocalDevHouseholdID) so portal household-scoped
// pages resolve under dev stub login without an OIDC session.
export const LOCAL_DEV_HOUSEHOLD_ID = '00000000-0000-0000-0000-0000000b0001'

export type LocalDevUser = {
  id: string
  email: string
  name: string
  role: Role
  orgId: string
  avatarInitials: string
  mfaAuthenticated?: boolean
  householdId?: string
  swimmerId?: string
}

export const LOCAL_DEV_USERS: Record<string, LocalDevUser> = {
  'stub-admin': {
    id: '00000000-0000-0000-0000-00000000a102',
    email: 'admin@dare.local',
    name: 'Dev Admin',
    role: 'admin',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DA',
    mfaAuthenticated: true,
  },
  'stub-coach': {
    id: '00000000-0000-0000-0000-00000000a103',
    email: 'coach@dare.local',
    name: 'Dev Coach',
    role: 'coach',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DC',
    mfaAuthenticated: true,
  },
  'stub-parent': {
    id: '00000000-0000-0000-0000-00000000a104',
    email: 'parent@dare.local',
    name: 'Dev Parent',
    role: 'parent',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DP',
    householdId: LOCAL_DEV_HOUSEHOLD_ID,
  },
  'stub-swimmer': {
    id: '00000000-0000-0000-0000-00000000a107',
    email: 'swimmer@dare.local',
    name: 'Dev Swimmer',
    role: 'swimmer',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DS',
    householdId: LOCAL_DEV_HOUSEHOLD_ID,
  },
}
