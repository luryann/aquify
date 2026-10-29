import test from 'node:test'
import assert from 'node:assert/strict'

import { formatMeetFileSize, formatMeetFileType } from './meet-files.ts'

test('formatMeetFileSize reports bytes under 1 KB and rounded KB above', () => {
  assert.equal(formatMeetFileSize(undefined), 'Unknown size')
  assert.equal(formatMeetFileSize(0), 'Unknown size')
  assert.equal(formatMeetFileSize(512), '512 B')
  assert.equal(formatMeetFileSize(1536), '2 KB')
})

test('formatMeetFileType names known reports and humanises other export types', () => {
  assert.equal(formatMeetFileType('eligibility_report'), 'Eligibility report')
  assert.equal(formatMeetFileType('entry_report'), 'Entry report')
  assert.equal(formatMeetFileType('event_report'), 'Event report')
  assert.equal(formatMeetFileType('meet_manager_export'), 'meet manager export')
})
