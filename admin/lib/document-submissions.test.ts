import test from 'node:test'
import assert from 'node:assert/strict'

import { documentNeedsAction, getPortalDocumentAction, resolveDocumentStatus, selectLatestSubmission } from './document-submissions.ts'

test('selectLatestSubmission prefers the newest matching submission for a swimmer scope', () => {
  const latest = selectLatestSubmission([
    {
      document_type_id: 'doc-medical',
      swimmer_id: 'swm-1',
      status: 'verified',
      created_at: '2026-03-20T10:00:00Z',
      updated_at: '2026-03-20T10:30:00Z',
    },
    {
      document_type_id: 'doc-medical',
      swimmer_id: 'swm-1',
      status: 'rejected',
      created_at: '2026-03-25T10:00:00Z',
      updated_at: '2026-03-25T10:15:00Z',
    },
  ], 'doc-medical', 'swm-1')

  assert.equal(latest?.status, 'rejected')
  assert.equal(latest?.created_at, '2026-03-25T10:00:00Z')
})

test('selectLatestSubmission keeps swimmer and household submissions separate', () => {
  const latestHousehold = selectLatestSubmission([
    {
      document_type_id: 'doc-insurance',
      swimmer_id: '',
      status: 'submitted',
      created_at: '2026-03-24T12:00:00Z',
      updated_at: '2026-03-24T12:30:00Z',
    },
    {
      document_type_id: 'doc-insurance',
      swimmer_id: 'swm-9',
      status: 'pending',
      created_at: '2026-03-25T12:00:00Z',
      updated_at: '2026-03-25T12:30:00Z',
    },
  ], 'doc-insurance')

  assert.equal(latestHousehold?.status, 'submitted')
  assert.equal(latestHousehold?.swimmer_id, '')
})

test('getPortalDocumentAction keeps waiver and medical rows on the real upload path', () => {
  assert.equal(getPortalDocumentAction({ effectiveStatus: 'missing', typeCategory: 'waiver' }), 'upload')
  assert.equal(getPortalDocumentAction({ effectiveStatus: 'rejected', typeCategory: 'medical' }), 'upload')
  assert.equal(getPortalDocumentAction({ effectiveStatus: 'expired', typeCategory: 'insurance' }), 'upload')
})

test('getPortalDocumentAction only exposes a CTA for actionable document statuses', () => {
  assert.equal(getPortalDocumentAction({ effectiveStatus: 'submitted', typeCategory: 'waiver' }), 'none')
  assert.equal(getPortalDocumentAction({ effectiveStatus: 'verified', typeCategory: 'medical' }), 'none')
  assert.equal(getPortalDocumentAction({ effectiveStatus: 'pending', typeCategory: 'insurance' }), 'upload')
})

test('resolveDocumentStatus reports missing, expired and known statuses', () => {
  const now = new Date('2026-07-01T00:00:00Z')
  assert.equal(resolveDocumentStatus(null, now), 'missing')
  assert.equal(resolveDocumentStatus({ status: 'submitted', expires_at: '2026-06-30T00:00:00Z' }, now), 'expired')
  assert.equal(resolveDocumentStatus({ status: 'verified', expires_at: '2026-06-30T00:00:00Z' }, now), 'verified')
  assert.equal(resolveDocumentStatus({ status: 'rejected' }, now), 'rejected')
  assert.equal(resolveDocumentStatus({ status: 'something_new' }, now), 'pending')
})

test('documentNeedsAction is true only for statuses that still need an upload', () => {
  for (const status of ['missing', 'pending', 'rejected', 'expired']) assert.equal(documentNeedsAction(status), true)
  for (const status of ['verified', 'submitted']) assert.equal(documentNeedsAction(status), false)
})
