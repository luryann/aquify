import test from 'node:test'
import assert from 'node:assert/strict'

import {
  deriveEligiblePortalRegistrationSwimmers,
  derivePortalRegistrationPrograms,
  derivePortalRegistrationSeasons,
  derivePortalRegistrationWaivers,
} from './portal-registration-wizard.ts'

test('derivePortalRegistrationSeasons keeps a current fallback and existing live seasons', () => {
  const seasons = derivePortalRegistrationSeasons(
    [
      { id: 'reg_1', org_id: 'org_1', household_id: 'hh_1', swimmer_id: 'sw_1', group_id: 'grp_1', season: '2026-27', status: 'active', registration_date: '2026-06-01' },
      { id: 'reg_2', org_id: 'org_1', household_id: 'hh_1', swimmer_id: 'sw_2', group_id: 'grp_1', season: '2025-26', status: 'inactive', registration_date: '2025-06-01' },
    ],
    new Date('2026-05-25T12:00:00Z'),
  )

  assert.deepEqual(seasons.map((season) => season.id), ['2026-27', '2025-26'])
})

test('deriveEligiblePortalRegistrationSwimmers marks already-registered swimmers for the selected program and season', () => {
  const swimmers = deriveEligiblePortalRegistrationSwimmers(
    [
      { id: 'sw_2', org_id: 'org_1', household_id: 'hh_1', first_name: 'Lily', last_name: 'Allen', date_of_birth: '2015-01-01', gender: 'female', registration_status: 'active' },
      { id: 'sw_1', org_id: 'org_1', household_id: 'hh_1', first_name: 'Marcus', last_name: 'Allen', date_of_birth: '2012-01-01', gender: 'male', registration_status: 'active' },
    ],
    [
      { id: 'reg_1', org_id: 'org_1', household_id: 'hh_1', swimmer_id: 'sw_1', group_id: 'grp_senior', season: '2026-27', status: 'active', registration_date: '2026-06-01' },
    ],
    '2026-27',
    'grp_senior',
  )

  assert.equal(swimmers[0]?.id, 'sw_2')
  assert.equal(swimmers[0]?.alreadyRegistered, false)
  assert.equal(swimmers[1]?.id, 'sw_1')
  assert.equal(swimmers[1]?.alreadyRegistered, true)
})

test('derivePortalRegistrationPrograms and waivers filter to live active records', () => {
  const programs = derivePortalRegistrationPrograms([
    { id: 'grp_2', org_id: 'org_1', code: 'jr', name: 'Junior', type: 'developmental', is_active: true, created_at: '' },
    { id: 'grp_1', org_id: 'org_1', code: 'sr', name: 'Senior', type: 'competitive', is_active: true, created_at: '' },
    { id: 'grp_3', org_id: 'org_1', code: 'old', name: 'Old', type: 'other', is_active: false, created_at: '' },
  ])
  assert.deepEqual(programs.map((program) => program.id), ['grp_2', 'grp_1'])

  const waivers = derivePortalRegistrationWaivers([
    { id: 'doc_1', org_id: 'org_1', name: 'Code of Conduct Agreement', category: 'waiver', description: 'Club conduct terms.', is_required: true, applies_to: 'household', is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
    { id: 'doc_2', org_id: 'org_1', name: 'Photo Release', category: 'registration', description: '', is_required: false, applies_to: 'swimmer', is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
    { id: 'doc_3', org_id: 'org_1', name: 'Medical Form', category: 'medical', description: '', is_required: true, applies_to: 'swimmer', is_active: true, created_at: '', updated_at: '', created_by: '', updated_by: '' },
  ])

  assert.deepEqual(waivers.map((waiver) => waiver.id), ['doc_1', 'doc_2'])
  assert.equal(waivers[0]?.waiverType, 'code of conduct agreement')
  assert.match(waivers[1]?.body ?? '', /No additional waiver copy/)
})
