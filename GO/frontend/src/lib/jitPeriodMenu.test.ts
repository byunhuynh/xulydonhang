import assert from 'node:assert/strict'
import test from 'node:test'
import {
  JIT_PERIOD_OPTIONS,
  isActiveJITPeriod,
  menuOpenAfterEscape,
  menuOpenAfterOutsideMouseDown,
} from './jitPeriodMenu.ts'

test('exposes the exact JIT delivery periods expected by the backend', () => {
  assert.deepEqual(JIT_PERIOD_OPTIONS, [
    { value: 'sáng', label: 'Sáng' },
    { value: 'chiều', label: 'Chiều' },
    { value: 'tối', label: 'Tối' },
  ])
})

test('recognizes only the active JIT delivery period', () => {
  assert.equal(isActiveJITPeriod('chiều', 'chiều'), true)
  assert.equal(isActiveJITPeriod('chiều', 'sáng'), false)
})

test('closes an open JIT menu when Escape is pressed', () => {
  assert.equal(menuOpenAfterEscape(true, 'Escape'), false)
  assert.equal(menuOpenAfterEscape(true, 'Enter'), true)
})

test('closes an open JIT menu for an outside mousedown only', () => {
  assert.equal(menuOpenAfterOutsideMouseDown(true, false), false)
  assert.equal(menuOpenAfterOutsideMouseDown(true, true), true)
})
