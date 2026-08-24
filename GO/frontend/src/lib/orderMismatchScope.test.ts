import test from 'node:test'
import assert from 'node:assert/strict'
import type { OrderRow, PriceMismatchDetail } from '../types.ts'
import { mismatchesForPO } from './orderMismatchScope.ts'

function detail(sku: string, excelRow: number): PriceMismatchDetail {
  return { sku, excelRow, productName: sku, invoicePrice: 10, systemPrice: 9, qty: 1, promoText: '', promoDateRange: '' }
}

function row(po: string, page: string, details: PriceMismatchDetail[]): OrderRow {
  return { po, page, priceMismatchDetails: details } as OrderRow
}

test('mismatchesForPO collects every mismatched SKU across BigC pages without requiring page 1', () => {
  const rows = [
    row('PO-BIGC', '2/4', [detail('SKU-A', 11)]),
    row('PO-BIGC', '3/4', [detail('SKU-B', 27), detail('SKU-C', 28)]),
    row('OTHER', '2/2', [detail('SKU-X', 40)]),
  ]

  assert.deepEqual(
    mismatchesForPO(rows, 'PO-BIGC').map(({ rowIndex, detail: item }) => [rowIndex, item.sku, item.excelRow]),
    [[0, 'SKU-A', 11], [1, 'SKU-B', 27], [1, 'SKU-C', 28]],
  )
})
