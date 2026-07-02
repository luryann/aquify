import type { Role } from './roles'

export const LOCAL_DEV_ORG_ID = '00000000-0000-0000-0000-00000000a001'

export type LocalDevUser = {
  id: string
  email: string
  name: string
  role: Role
  orgId: string
  avatarInitials: string
  mfaAuthenticated?: boolean
}

export const LOCAL_DEV_USERS: Record<string, LocalDevUser> = {
  'stub-owner': {
    id: '00000000-0000-0000-0000-00000000a101',
    email: 'owner@dare.local',
    name: 'Dev Owner',
    role: 'owner',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DO',
    mfaAuthenticated: true,
  },
  'stub-admin': {
    id: '00000000-0000-0000-0000-00000000a102',
    email: 'admin@dare.local',
    name: 'Dev Admin',
    role: 'admin',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DA',
  },
  'stub-coach': {
    id: '00000000-0000-0000-0000-00000000a103',
    email: 'coach@dare.local',
    name: 'Dev Coach',
    role: 'coach',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DC',
  },
  'stub-parent': {
    id: '00000000-0000-0000-0000-00000000a104',
    email: 'parent@dare.local',
    name: 'Dev Parent',
    role: 'parent',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DP',
  },
  'stub-billing': {
    id: '00000000-0000-0000-0000-00000000a105',
    email: 'billing@dare.local',
    name: 'Dev Billing',
    role: 'billing',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DB',
  },
  'stub-readonly': {
    id: '00000000-0000-0000-0000-00000000a106',
    email: 'readonly@dare.local',
    name: 'Dev Read-Only',
    role: 'read-only',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DR',
  },
  'stub-swimmer': {
    id: '00000000-0000-0000-0000-00000000a107',
    email: 'swimmer@dare.local',
    name: 'Dev Swimmer',
    role: 'swimmer',
    orgId: LOCAL_DEV_ORG_ID,
    avatarInitials: 'DS',
  },
}
