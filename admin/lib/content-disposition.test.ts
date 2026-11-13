import test from 'node:test'
import assert from 'node:assert/strict'

import { filenameFromContentDisposition } from './content-disposition.ts'

test('reads a plain token and a quoted name', () => {
  assert.equal(filenameFromContentDisposition('attachment; filename=a.zip', 'x'), 'a.zip')
  assert.equal(filenameFromContentDisposition('attachment; filename="My Meet.zip"', 'x'), 'My Meet.zip')
})

test('keeps a semicolon inside a quoted name', () => {
  assert.equal(
    filenameFromContentDisposition('attachment; filename="DARE-Entries-Fall Classic; Day 1-14Nov2026-0900.zip"', 'x'),
    'DARE-Entries-Fall Classic; Day 1-14Nov2026-0900.zip',
  )
})

test('decodes the UTF-8 extended form', () => {
  assert.equal(filenameFromContentDisposition("attachment; filename*=utf-8''Caf%C3%A9%20Open.zip", 'x'), 'Café Open.zip')
  assert.equal(
    filenameFromContentDisposition('attachment; filename="Cafe Open.zip"; filename*=UTF-8\'\'Caf%C3%A9%20Open.zip', 'x'),
    'Café Open.zip',
  )
})

test('falls back when there is no name', () => {
  assert.equal(filenameFromContentDisposition(null, 'meet-entries.zip'), 'meet-entries.zip')
  assert.equal(filenameFromContentDisposition('attachment', 'meet-entries.zip'), 'meet-entries.zip')
  assert.equal(filenameFromContentDisposition("attachment; filename*=UTF-8''%E0%A4%A", 'f.zip'), 'f.zip')
})
