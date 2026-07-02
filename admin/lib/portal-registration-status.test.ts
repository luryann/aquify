import test from 'node:test'
import assert from 'node:assert/strict'

import {
  buildPortalRegistrationDocumentRows,
  summarizeRegistrationWaiver,
} from './portal-registration-status.ts'

test('summarizeRegistrationWaiver returns accepted details from live registration fields', () => {
  const summary = summarizeRegistrationWaiver({
    id: 'reg_1',
    org_id: 'org_1',
    household_id: 'hh_1',
    swimmer_id: 'sw_1',
    group_id: 'grp_1',
    season: '2026-2027',
    status: 'active',
    registration_date: '2026-05-01',
    waiver_accepted: true,
    waiver_count: 2,
    waiver_types: ['medical', 'liability'],
    last_waiver_accepted_at: '2026-05-12T10:00:00Z',
  })

  assert.equal(summary.accepted, true)
  assert.equal(summary.count, 2)
  assert.deepEqual(summary.types, ['liability', 'medical'])
  assert.equal(summary.lastAcceptedAt, '2026-05-12T10:00:00Z')
})

test('buildPortalRegistrationDocumentRows filters to waiver/registration docs and resolves statuses', () => {
  const rows = buildPortalRegistrationDocumentRows({
    registration: {
      id: 'reg_1',
      org_id: 'org_1',
      household_id: 'hh_1',
      swimmer_id: 'sw_1',
      group_id: 'grp_1',
      season: '2026-2027',
      status: 'active',
      registration_date: '2026-05-01',
    },
    swimmerId: 'sw_1',
    documentTypes: [
      { id: 'dt_household', org_id: 'org_1', name: 'Club Registration Packet', category: 'registration', is_required: true, applies_to: 'household', is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
      { id: 'dt_swimmer', org_id: 'org_1', name: 'Meet Waiver', category: 'waiver', is_required: true, applies_to: 'swimmer', is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
      { id: 'dt_medical', org_id: 'org_1', name: 'Medical Form', category: 'medical', is_required: true, applies_to: 'swimmer', is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
    ],
    submissions: [
      { id: 'sub_1', org_id: 'org_1', document_type_id: 'dt_household', household_id: 'hh_1', status: 'submitted', submitted_at: '2026-05-11T00:00:00Z', created_at: '', updated_at: '', created_by: '', updated_by: '' },
      { id: 'sub_2', org_id: 'org_1', document_type_id: 'dt_swimmer', household_id: 'hh_1', swimmer_id: 'sw_1', status: 'rejected', submitted_at: '2026-05-10T00:00:00Z', created_at: '', updated_at: '', created_by: '', updated_by: '' },
    ],
    now: new Date('2026-05-17T00:00:00Z'),
  })

  assert.equal(rows.length, 2)
  assert.deepEqual(rows.map((row) => [row.name, row.effectiveStatus]), [
    ['Club Registration Packet', 'submitted'],
    ['Meet Waiver', 'rejected'],
  ])
})

test('buildPortalRegistrationDocumentRows marks expired submissions and missing swimmer docs', () => {
  const rows = buildPortalRegistrationDocumentRows({
    registration: {
      id: 'reg_1',
      org_id: 'org_1',
      household_id: 'hh_1',
      swimmer_id: 'sw_1',
      group_id: 'grp_1',
      season: '2026-2027',
      status: 'active',
      registration_date: '2026-05-01',
    },
    swimmerId: 'sw_1',
    documentTypes: [
      { id: 'dt_expired', org_id: 'org_1', name: 'Expired Waiver', category: 'waiver', is_required: true, applies_to: 'household', is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
      { id: 'dt_missing', org_id: 'org_1', name: 'Missing Registration Form', category: 'registration', is_required: true, applies_to: 'swimmer', is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
    ],
    submissions: [
      { id: 'sub_1', org_id: 'org_1', document_type_id: 'dt_expired', household_id: 'hh_1', status: 'pending', expires_at: '2026-05-01T00:00:00Z', created_at: '', updated_at: '', created_by: '', updated_by: '' },
    ],
    now: new Date('2026-05-17T00:00:00Z'),
  })

  assert.deepEqual(rows.map((row) => [row.name, row.effectiveStatus]), [
    ['Expired Waiver', 'expired'],
    ['Missing Registration Form', 'missing'],
  ])
})
