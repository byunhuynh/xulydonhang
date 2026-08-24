import assert from 'node:assert/strict'
import test from 'node:test'
import {
  beginJITPeriodUpdate,
  completeJITPeriodUpdate,
  createJITPeriodState,
  isJITPeriodMenuDisabled,
  resetJITPeriodState,
} from './jitPeriodState.ts'

test('reset clears overrides and ignores a prior-generation completion', () => {
  const oldStart = beginJITPeriodUpdate(createJITPeriodState(), 'source-old')
  assert.ok(oldStart.request)
  const oldComplete = completeJITPeriodUpdate(oldStart.state, oldStart.request, 'sáng')
  assert.equal(oldComplete.accepted, true)
  assert.deepEqual(oldComplete.state.periodBySource, { 'source-old': 'sáng' })

  const reset = resetJITPeriodState(oldComplete.state)
  assert.deepEqual(reset.periodBySource, {})
  const currentStart = beginJITPeriodUpdate(reset, 'source-current')
  assert.ok(currentStart.request)

  const staleComplete = completeJITPeriodUpdate(currentStart.state, oldStart.request, 'tối')
  assert.equal(staleComplete.accepted, false)
  assert.deepEqual(staleComplete.state, currentStart.state)
})

test('one pending update rejects a second source and disables every menu', () => {
  const first = beginJITPeriodUpdate(createJITPeriodState(), 'source-a')
  assert.ok(first.request)
  const second = beginJITPeriodUpdate(first.state, 'source-b')

  assert.equal(second.request, null)
  assert.deepEqual(second.state, first.state)
  assert.equal(isJITPeriodMenuDisabled(false, first.state), true)
  assert.equal(isJITPeriodMenuDisabled(true, createJITPeriodState()), true)
  assert.equal(isJITPeriodMenuDisabled(false, createJITPeriodState()), false)
})
