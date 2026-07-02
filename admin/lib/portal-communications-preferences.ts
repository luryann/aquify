import { ApiError } from '#lib/api'

import type { ApiClient } from './api.ts'
import {
  fetchCommunicationPreferences,
  updateCommunicationPreferences,
  type CommunicationPreferenceRecord,
} from './adminData.ts'

export type PortalCommunicationPreference = {
  id: string
  label: string
  description: string
  email: boolean
  push: boolean
}

export const DEFAULT_PORTAL_COMMUNICATION_PREFERENCES: PortalCommunicationPreference[] = [
  { id: 'meet_commitment', label: 'Meet Commitment Requests', description: 'When a coach sends a new meet invitation', email: true, push: true },
  { id: 'invoice_due', label: 'Invoice Due Reminders', description: 'When an invoice is due or overdue', email: true, push: true },
  { id: 'announcement', label: 'Club Announcements', description: 'General announcements from club staff', email: true, push: false },
  { id: 'doc_reminder', label: 'Document Reminders', description: 'When a required document is expiring', email: true, push: false },
  { id: 'entry_update', label: 'Entry Updates', description: "When your swimmer's meet entries change", email: false, push: true },
]

export function mergePortalCommunicationPreferences(
  records: CommunicationPreferenceRecord[],
): PortalCommunicationPreference[] {
  const byId = new Map(records.map((record) => [record.event_id, record]))
  return DEFAULT_PORTAL_COMMUNICATION_PREFERENCES.map((preference) => {
    const persisted = byId.get(preference.id)
    return {
      id: preference.id,
      label: persisted?.label ?? preference.label,
      description: persisted?.description ?? preference.description,
      email: persisted?.email_enabled ?? preference.email,
      push: persisted?.push_enabled ?? preference.push,
    }
  })
}

export function portalCommunicationPreferencesPayload(preferences: PortalCommunicationPreference[]) {
  return preferences.map((preference) => ({
    event_id: preference.id,
    email_enabled: preference.email,
    push_enabled: preference.push,
  }))
}

export function portalCommunicationPreferencesChanged(
  current: PortalCommunicationPreference[],
  saved: PortalCommunicationPreference[],
): boolean {
  return JSON.stringify(portalCommunicationPreferencesPayload(current)) !== JSON.stringify(portalCommunicationPreferencesPayload(saved))
}

export async function loadPortalCommunicationPreferences(api: ApiClient, orgId: string): Promise<PortalCommunicationPreference[]> {
  return mergePortalCommunicationPreferences(await fetchCommunicationPreferences(api, orgId))
}

export async function savePortalCommunicationPreferences(
  api: ApiClient,
  orgId: string,
  preferences: PortalCommunicationPreference[],
): Promise<PortalCommunicationPreference[]> {
  return mergePortalCommunicationPreferences(
    await updateCommunicationPreferences(api, orgId, portalCommunicationPreferencesPayload(preferences)),
  )
}

export function portalCommunicationPreferencesErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return error.message
  }
  return 'Unable to save communication preferences right now.'
}
