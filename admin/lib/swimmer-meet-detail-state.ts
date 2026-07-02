import type {
  CommitmentEventSelectionRecord,
  CommitmentRecord,
} from './adminData.ts'

export type SwimmerMeetDetailState = {
  commitment: 'pending' | 'yes' | 'no'
  commitmentId: string | null
  selectedEvents: string[]
  selectedEventSelectionIds: Record<string, string>
}

export function mapSwimmerMeetCommitmentStatus(status?: string): 'pending' | 'yes' | 'no' {
  if (status === 'committed_yes') return 'yes'
  if (status === 'committed_no') return 'no'
  return 'pending'
}

export function buildSwimmerMeetDetailState(
  commitment: CommitmentRecord | undefined,
  selections: CommitmentEventSelectionRecord[],
): SwimmerMeetDetailState {
  return {
    commitment: mapSwimmerMeetCommitmentStatus(commitment?.status),
    commitmentId: commitment?.id ?? null,
    selectedEvents: selections.map((selection) => selection.meet_event_id),
    selectedEventSelectionIds: Object.fromEntries(
      selections.map((selection) => [selection.meet_event_id, selection.id]),
    ),
  }
}

export function canEditSwimmerMeetSelections(
  commitmentOpen: boolean,
  state: SwimmerMeetDetailState | undefined,
): boolean {
  return Boolean(commitmentOpen && state && state.commitment === 'yes' && state.commitmentId)
}

export function withAddedSwimmerMeetSelection(
  state: SwimmerMeetDetailState,
  selection: CommitmentEventSelectionRecord,
): SwimmerMeetDetailState {
  const selectedEvents = state.selectedEvents.includes(selection.meet_event_id)
    ? state.selectedEvents
    : [...state.selectedEvents, selection.meet_event_id]

  return {
    ...state,
    selectedEvents,
    selectedEventSelectionIds: {
      ...state.selectedEventSelectionIds,
      [selection.meet_event_id]: selection.id,
    },
  }
}

export function withRemovedSwimmerMeetSelection(
  state: SwimmerMeetDetailState,
  meetEventId: string,
): SwimmerMeetDetailState {
  const nextSelectionIds = { ...state.selectedEventSelectionIds }
  delete nextSelectionIds[meetEventId]

  return {
    ...state,
    selectedEvents: state.selectedEvents.filter((eventId) => eventId !== meetEventId),
    selectedEventSelectionIds: nextSelectionIds,
  }
}
