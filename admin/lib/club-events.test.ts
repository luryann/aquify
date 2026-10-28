import test from 'node:test'
import assert from 'node:assert/strict'

import { clubEventDateKey, clubEventInputFromDraft, emptyClubEventDraft, findMyRSVP, formatFee, billingNotice, applyRSVPToSummary } from './club-events.ts'

test('clubEventInputFromDraft validates and builds all-day bodies', () => {
  const d = { ...emptyClubEventDraft(), title: ' Banquet ', allDay: true, startDate: '2026-11-20', endDate: '2026-11-21', fee: '12.50', capacity: '40' }
  const body = clubEventInputFromDraft(d)
  assert.ok(typeof body !== 'string')
  assert.equal(body.title, 'Banquet')
  assert.equal(body.starts_at, '2026-11-20T00:00:00Z')
  assert.equal(body.ends_at, '2026-11-21T00:00:00Z')
  assert.equal(body.fee_cents, 1250)
  assert.equal(body.capacity, 40)
})

test('clubEventInputFromDraft rejects bad drafts', () => {
  const base = emptyClubEventDraft()
  assert.match(clubEventInputFromDraft(base) as string, /title/)
  assert.match(clubEventInputFromDraft({ ...base, title: 'x' }) as string, /start/)
  assert.match(clubEventInputFromDraft({ ...base, title: 'x', allDay: true, startDate: '2026-11-20', audience: 'groups' }) as string, /group/)
  assert.match(clubEventInputFromDraft({ ...base, title: 'x', allDay: true, startDate: '2026-11-20', rsvpEnabled: false, fee: '5' }) as string, /RSVP/)
  assert.match(clubEventInputFromDraft({ ...base, title: 'x', allDay: true, startDate: '2026-11-20', capacity: '0' }) as string, /Capacity/)
})

test('all-day events key on their UTC date regardless of timezone', () => {
  assert.equal(clubEventDateKey({ starts_at: '2026-11-20T00:00:00Z', all_day: true }), '2026-11-20')
})

test('findMyRSVP separates household and per-swimmer rows', () => {
  const rows = [
    { id: '1', event_id: 'e', household_id: 'h', response: 'yes' as const, guest_count: 0, billed_quantity: 0, updated_at: '' },
    { id: '2', event_id: 'e', household_id: 'h', swimmer_id: 's', response: 'no' as const, guest_count: 0, billed_quantity: 0, updated_at: '' },
  ]
  assert.equal(findMyRSVP(rows)?.id, '1')
  assert.equal(findMyRSVP(rows, 's')?.id, '2')
  assert.equal(findMyRSVP(undefined), undefined)
})

test('fee formatting and billing notices', () => {
  assert.equal(formatFee(2500), '$25')
  assert.equal(formatFee(1250), '$12.50')
  assert.match(billingNotice('charged', 2500) ?? '', /\$25/)
  assert.equal(billingNotice('unchanged', 2500), null)
})

test('applyRSVPToSummary moves headcount and spots when a reply changes', () => {
  const base = { yes: 0, no: 0, maybe: 0, yes_headcount: 0, spots_left: 40 }
  const rsvp = { id: 'r1', event_id: 'e', household_id: 'h', response: 'yes' as const, guest_count: 2, billed_quantity: 3, updated_at: '' }
  const yes = applyRSVPToSummary(base, undefined, rsvp)
  assert.deepEqual(yes, { yes: 1, no: 0, maybe: 0, yes_headcount: 3, spots_left: 37 })
  const no = applyRSVPToSummary(yes, rsvp, { ...rsvp, response: 'no', guest_count: 0 })
  assert.deepEqual(no, { yes: 0, no: 1, maybe: 0, yes_headcount: 0, spots_left: 40 })
})
