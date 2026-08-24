import test from 'node:test'
import assert from 'node:assert/strict'
import type { OrderRow } from '../types.ts'
import { upsertOrderRow } from './orderRowUpsert.ts'

function row(resultKey: string, status: string): OrderRow {
  return { resultKey, status } as OrderRow
}

test('upsertOrderRow appends an unseen key after all existing rows', () => {
  const first = row('pdf|1|PO1', 'done')
  const unseen = row('pdf|2|PO2', 'processing')

  assert.deepEqual(upsertOrderRow([first], unseen), [first, unseen])
})

test('upsertOrderRow appends a new key and replaces an existing key in place', () => {
  const first = row('pdf|2/4|PO1', 'processing')
  const second = row('pdf|3/4|PO2', 'done')
  const final = row('pdf|2/4|PO1', 'done')
  assert.deepEqual(upsertOrderRow([first, second], final), [final, second])
})
