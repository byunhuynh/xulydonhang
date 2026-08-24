import test from 'node:test'
import assert from 'node:assert/strict'
import type { OrderRow } from '../types.ts'
import { groupJITFiles, skipsPriceReconciliation } from './jitFileGroups.ts'

function row(fileName: string, excelRows: number[], period = 'chiều'): OrderRow {
  return {
    fileName, excelRows, jitPeriod: period, system: 'JIT-CHOICE',
    entryDate: '24/08/2026', shipTo: 'WH6_HN', statusKind: 'done',
  } as OrderRow
}

test('groupJITFiles creates one period control per PDF and collects every Excel row', () => {
  const groups = groupJITFiles([
    row('air_waybill_WH6_HN_24082026.pdf', [9]),
    row('air_waybill_WH6_HN_24082026.pdf', [10, 11]),
    row('air_waybill_WH6_HTLA_24082026.pdf', [12], 'sáng'),
    { system: 'BigC', fileName: 'bigc.pdf' } as OrderRow,
  ])

  assert.deepEqual(groups, [
    {
      fileName: 'air_waybill_WH6_HN_24082026.pdf', orderDate: '24/08/2026',
      warehouse: 'WH6_HN', period: 'chiều', excelRows: [9, 10, 11], orderCount: 2,
    },
    {
      fileName: 'air_waybill_WH6_HTLA_24082026.pdf', orderDate: '24/08/2026',
      warehouse: 'WH6_HN', period: 'sáng', excelRows: [12], orderCount: 1,
    },
  ])
})

test('JIT rows explicitly skip price reconciliation', () => {
  assert.equal(skipsPriceReconciliation(row('air_waybill_WH6_HN_24082026.pdf', [9])), true)
  assert.equal(skipsPriceReconciliation({ system: 'BigC' } as OrderRow), false)
})
