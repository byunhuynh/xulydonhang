import { test } from 'node:test'
import assert from 'node:assert/strict'

import { createBatchProgress, formatBatchProgress, progressPercent } from './batchProgress.ts'

test('formatBatchProgress reads as file count first, percent second', () => {
  assert.equal(formatBatchProgress({ done: 5, total: 8 }), '5/8 file · 62%')
  assert.equal(formatBatchProgress({ done: 0, total: 8 }), '0/8 file · 0%')
  assert.equal(formatBatchProgress({ done: 8, total: 8 }), '8/8 file · 100%')
})

test('formatBatchProgress stays empty until a batch declares its size', () => {
  assert.equal(formatBatchProgress(createBatchProgress()), '')
  assert.equal(formatBatchProgress({ done: 3, total: 0 }), '')
})

test('progressPercent never leaves the 0..100 range on a malformed event', () => {
  assert.equal(progressPercent({ done: -2, total: 8 }), 0)
  assert.equal(progressPercent({ done: 99, total: 8 }), 100)
})
