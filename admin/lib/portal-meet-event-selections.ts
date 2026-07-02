import type {
  CommitmentEventSelectionRecord,
  CommitmentRecord,
  SwimmerRecord,
} from './adminData.ts'

export type PortalSwimmerCommitmentState = {
  commitment: 'pending' | 'yes' | 'no' | 'closed'
  commitmentId: string | null
  selectedEvents: string[]
  selectedEventSelectionIds: Record<string, string>
  note: string
}

export function mapPortalCommitmentStatus(status: string): 'pending' | 'yes' | 'no' | 'closed' {
  if (status === 'committed_yes') return 'yes'
  if (status === 'committed_no') return 'no'
  if (status === 'invited') return 'pending'
  if (status === 'coach_reviewed' || status === 'locked') return 'closed'
  return 'pending'
}

export function buildPortalSwimmerStates(
  swimmers: SwimmerRecord[],
  commitments: CommitmentRecord[],
  selectionsByCommitmentId: Record<string, CommitmentEventSelectionRecord[]>,
): Record<string, PortalSwimmerCommitmentState> {
  const initialStates: Record<string, PortalSwimmerCommitmentState> = {}

  for (const swimmer of swimmers) {
    const commitment = commitments.find((item) => item.swimmer_id === swimmer.id)
    const persistedSelections = commitment?.id ? (selectionsByCommitmentId[commitment.id] ?? []) : []
    const selectedEventSelectionIds = Object.fromEntries(
      persistedSelections.map((selection) => [selection.meet_event_id, selection.id]),
    )

    initialStates[swimmer.id] = {
      commitment: commitment ? mapPortalCommitmentStatus(commitment.status) : 'pending',
      commitmentId: commitment?.id ?? null,
      selectedEvents: persistedSelections.map((selection) => selection.meet_event_id),
      selectedEventSelectionIds,
      note: commitment?.note ?? '',
    }
  }

  return initialStates
}

export function canEditPortalMeetSelections(
  commitmentOpen: boolean,
  state: PortalSwimmerCommitmentState | undefined,
): boolean {
  return Boolean(commitmentOpen && state && state.commitment === 'yes' && state.commitmentId)
}

export function withAddedPortalMeetSelection(
  state: PortalSwimmerCommitmentState,
  selection: CommitmentEventSelectionRecord,
): PortalSwimmerCommitmentState {
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

export function withRemovedPortalMeetSelection(
  state: PortalSwimmerCommitmentState,
  meetEventId: string,
): PortalSwimmerCommitmentState {
  const nextSelectionIds = { ...state.selectedEventSelectionIds }
  delete nextSelectionIds[meetEventId]

  return {
    ...state,
    selectedEvents: state.selectedEvents.filter((eventId) => eventId !== meetEventId),
    selectedEventSelectionIds: nextSelectionIds,
  }
}
