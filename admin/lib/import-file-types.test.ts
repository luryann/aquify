import test from 'node:test'
import assert from 'node:assert/strict'

import {
  IMPORT_FILE_TYPE_CFG,
  SUPPORTED_IMPORT_FILE_TYPES,
  normalizeImportFileType,
} from './importFileTypes.ts'

test('supported import file types include HY3, HYV, and EV3', () => {
  assert.deepEqual(SUPPORTED_IMPORT_FILE_TYPES, ['SD3', 'CL2', 'ZIP', 'HY3', 'HYV', 'EV3'])
})

test('normalizeImportFileType handles HY3, HYV, and EV3 without falling back', () => {
  assert.equal(normalizeImportFileType('hy3'), 'HY3')
  assert.equal(normalizeImportFileType('HYV'), 'HYV')
  assert.equal(normalizeImportFileType('ev3'), 'EV3')
})

test('normalizeImportFileType falls back safely for unknown values', () => {
  assert.equal(normalizeImportFileType('txt'), 'UNKNOWN')
  assert.ok(IMPORT_FILE_TYPE_CFG.HYV)
  assert.ok(IMPORT_FILE_TYPE_CFG.EV3)
  assert.ok(IMPORT_FILE_TYPE_CFG.UNKNOWN)
})
