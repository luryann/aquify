import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildSwimmerMeetDetailState,
  canEditSwimmerMeetSelections,
  withAddedSwimmerMeetSelection,
  withRemovedSwimmerMeetSelection,
} from './swimmer-meet-detail-state.ts'

test('swimmer meet detail state reloads persisted commitment event selections from backend records', () => {
  const state = buildSwimmerMeetDetailState(
    { id: 'cmt_1', meet_id: 'meet_1', swimmer_id: 'sw_1', status: 'committed_yes', notes: 'ready' },
    [
      { id: 'sel_1', org_id: 'org_1', commitment_id: 'cmt_1', meet_event_id: 'ev_1', created_at: '', created_by: 'u1' },
      { id: 'sel_2', org_id: 'org_1', commitment_id: 'cmt_1', meet_event_id: 'ev_2', created_at: '', created_by: 'u1' },
    ],
  )

  assert.equal(state.commitment, 'yes')
  assert.equal(state.commitmentId, 'cmt_1')
  assert.deepEqual(state.selectedEvents, ['ev_1', 'ev_2'])
  assert.deepEqual(state.selectedEventSelectionIds, { ev_1: 'sel_1', ev_2: 'sel_2' })
})

test('swimmer meet detail selection edits require an open window and a committed-yes state', () => {
  const editableState = {
    commitment: 'yes' as const,
    commitmentId: 'cmt_1',
    selectedEvents: [],
    selectedEventSelectionIds: {},
  }

  assert.equal(canEditSwimmerMeetSelections(true, editableState), true)
  assert.equal(canEditSwimmerMeetSelections(false, editableState), false)
  assert.equal(canEditSwimmerMeetSelections(true, { ...editableState, commitment: 'no' as const }), false)
  assert.equal(canEditSwimmerMeetSelections(true, { ...editableState, commitmentId: null }), false)
})

test('swimmer meet detail selection helpers keep persisted selection ids aligned with selected event ids', () => {
  const initial = {
    commitment: 'yes' as const,
    commitmentId: 'cmt_1',
    selectedEvents: ['ev_1'],
    selectedEventSelectionIds: { ev_1: 'sel_1' },
  }

  const withAdded = withAddedSwimmerMeetSelection(initial, {
    id: 'sel_2',
    org_id: 'org_1',
    commitment_id: 'cmt_1',
    meet_event_id: 'ev_2',
    created_at: '',
    created_by: 'u1',
  })
  assert.deepEqual(withAdded.selectedEvents, ['ev_1', 'ev_2'])
  assert.deepEqual(withAdded.selectedEventSelectionIds, { ev_1: 'sel_1', ev_2: 'sel_2' })

  const withRemoved = withRemovedSwimmerMeetSelection(withAdded, 'ev_1')
  assert.deepEqual(withRemoved.selectedEvents, ['ev_2'])
  assert.deepEqual(withRemoved.selectedEventSelectionIds, { ev_2: 'sel_2' })
})
