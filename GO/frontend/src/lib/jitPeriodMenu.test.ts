import { test } from 'node:test'
import assert from 'node:assert/strict'

import { JIT_PERIOD_OPTIONS, isActiveJITPeriod } from './jitPeriodMenu.ts'

test('exposes the exact JIT delivery periods expected by the backend', () => {
  assert.deepEqual(
    JIT_PERIOD_OPTIONS.map((option) => option.value),
    ['sáng', 'chiều', 'tối'],
  )
})

test('every period ships a capitalised label for the control', () => {
  assert.deepEqual(
    JIT_PERIOD_OPTIONS.map((option) => option.label),
    ['Sáng', 'Chiều', 'Tối'],
  )
})

test('recognizes only the active JIT delivery period', () => {
  assert.equal(isActiveJITPeriod('chiều', 'chiều'), true)
  assert.equal(isActiveJITPeriod('chiều', 'sáng'), false)
  assert.equal(isActiveJITPeriod('', 'sáng'), false)
})
