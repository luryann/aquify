import { ApiError, type ApiClient } from '#lib/api'

import { createMeetEntry } from './adminData.ts'

export async function saveMeetEntriesBatch(
  api: ApiClient,
  orgId: string,
  meetId: string,
  swimmerId: string,
  meetEventIds: string[],
): Promise<number> {
  await Promise.all(
    meetEventIds.map((meetEventId) =>
      createMeetEntry(api, orgId, meetId, {
        swimmer_id: swimmerId,
        meet_event_id: meetEventId,
      }),
    ),
  )
  return meetEventIds.length
}

export function formatMeetEntriesSaveError(error: unknown, attemptedCount: number): string {
  const prefix = attemptedCount > 1
    ? `Failed to save all ${attemptedCount} selected entries.`
    : 'Couldn’t save the selected entry. Try again.'

  if (error instanceof ApiError && error.message.trim() !== '') {
    return `${prefix} ${error.message} Some entries may have been created before the failure.`
  }
  if (error instanceof Error && error.message.trim() !== '') {
    return `${prefix} ${error.message} Some entries may have been created before the failure.`
  }
  return `${prefix} Some entries may have been created before the failure.`
}
