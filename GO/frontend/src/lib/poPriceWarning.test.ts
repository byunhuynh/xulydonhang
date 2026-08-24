import assert from 'node:assert/strict'
import test from 'node:test'

import { belowSystemPriceDetails } from './poPriceWarning.ts'
import type { PriceMismatchDetail } from '../types.ts'

const detail = (sku: string, invoicePrice: number, systemPrice: number): PriceMismatchDetail => ({
  sku,
  productName: `Sản phẩm ${sku}`,
  qty: 1,
  invoicePrice,
  systemPrice,
  excelRow: 10,
})

test('returns only SKUs whose PO price is lower than the system price', () => {
  const result = belowSystemPriceDetails([
    detail('LOW', 90_000, 100_000),
    detail('EQUAL', 100_000, 100_000),
    detail('HIGH', 110_000, 100_000),
    detail('LOW-2', 50_000, 70_000),
  ])

  assert.deepEqual(result.map(({ sku }) => sku), ['LOW', 'LOW-2'])
})

test('returns an empty list when every PO price is at least the system price', () => {
  assert.deepEqual(
    belowSystemPriceDetails([
      detail('EQUAL', 100_000, 100_000),
      detail('HIGH', 100_001, 100_000),
    ]),
    [],
  )
})
