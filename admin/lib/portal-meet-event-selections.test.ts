import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildPortalSwimmerStates,
  canEditPortalMeetSelections,
  withAddedPortalMeetSelection,
  withRemovedPortalMeetSelection,
} from './portal-meet-event-selections.ts'

test('portal meet selection state reloads persisted event selections from backend records', () => {
  const states = buildPortalSwimmerStates(
    [
      {
        id: 'sw_1',
        org_id: 'org_1',
        household_id: 'hh_1',
        first_name: 'Ava',
        last_name: 'Lee',
        date_of_birth: '2012-01-01',
        gender: 'F',
        registration_status: 'active',
      },
    ],
    [
      { id: 'cmt_1', meet_id: 'meet_1', swimmer_id: 'sw_1', status: 'committed_yes', note: 'prefers sprint free' },
    ],
    {
      cmt_1: [
        { id: 'sel_1', org_id: 'org_1', commitment_id: 'cmt_1', meet_event_id: 'ev_1', created_at: '', created_by: 'u1' },
        { id: 'sel_2', org_id: 'org_1', commitment_id: 'cmt_1', meet_event_id: 'ev_2', created_at: '', created_by: 'u1' },
      ],
    },
  )

  assert.deepEqual(states.sw_1?.selectedEvents, ['ev_1', 'ev_2'])
  assert.deepEqual(states.sw_1?.selectedEventSelectionIds, { ev_1: 'sel_1', ev_2: 'sel_2' })
  assert.equal(states.sw_1?.commitment, 'yes')
  assert.equal(states.sw_1?.note, 'prefers sprint free')
})

test('portal meet selection helpers respect commitment-open and committed-yes constraints', () => {
  const openYesState = {
    commitment: 'yes' as const,
    commitmentId: 'cmt_1',
    selectedEvents: [],
    selectedEventSelectionIds: {},
    note: '',
  }
  const closedState = {
    ...openYesState,
    commitment: 'closed' as const,
  }

  assert.equal(canEditPortalMeetSelections(true, openYesState), true)
  assert.equal(canEditPortalMeetSelections(false, openYesState), false)
  assert.equal(canEditPortalMeetSelections(true, closedState), false)
})

test('portal meet selection helpers add and remove persisted selections without local-only drift', () => {
  const initial = {
    commitment: 'yes' as const,
    commitmentId: 'cmt_1',
    selectedEvents: ['ev_1'],
    selectedEventSelectionIds: { ev_1: 'sel_1' },
    note: '',
  }

  const withAdded = withAddedPortalMeetSelection(initial, {
    id: 'sel_2',
    org_id: 'org_1',
    commitment_id: 'cmt_1',
    meet_event_id: 'ev_2',
    created_at: '',
    created_by: 'u1',
  })
  assert.deepEqual(withAdded.selectedEvents, ['ev_1', 'ev_2'])
  assert.deepEqual(withAdded.selectedEventSelectionIds, { ev_1: 'sel_1', ev_2: 'sel_2' })

  const withRemoved = withRemovedPortalMeetSelection(withAdded, 'ev_1')
  assert.deepEqual(withRemoved.selectedEvents, ['ev_2'])
  assert.deepEqual(withRemoved.selectedEventSelectionIds, { ev_2: 'sel_2' })
})
