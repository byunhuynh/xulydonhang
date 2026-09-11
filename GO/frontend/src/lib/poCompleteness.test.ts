import assert from 'node:assert/strict'
import test from 'node:test'

import { poCompleteness } from './poCompleteness.ts'
import type { OrderRow } from '../types.ts'

function row(over: Partial<OrderRow>): OrderRow {
  return {
    fileName: '802_NORTHDC_QP0_3006900_2636058652325.pdf',
    sourceId: '',
    page: '2/7',
    system: 'BigC',
    maKhachHang: 'MB_GC_BIGC',
    po: '2636058652325',
    resultKey: '',
    maVanDon: '',
    donGia: '0',
    status: '',
    statusKind: 'done',
    excelRows: [],
    jitPeriod: '',
    driveUrl: '',
    priceMismatchCount: 0,
    priceMismatchDetails: [],
    shipTo: '',
    entryDate: '',
    cancelDate: '',
    totalWeightKg: '0 kg',
    totalPackages: 0,
    totalQty: 0,
    skus: [],
    totalOrders: 0,
    promoItems: [],
    ...over,
  }
}

test('a complete PO has nothing to warn about, whether Go sent null or left the fields out', () => {
  assert.equal(poCompleteness([row({ missingItems: null }), row({})]), null)
})

// Real 802_NORTHDC PO: the two NRC barcodes are missing from SanPham on
// three store pages each.
test('the same missing barcode on several store pages is one line with its quantity added up', () => {
  const totals = { printedQty: 42, printedAmount: 10335408, writtenQty: 10, writtenAmount: 3190000 }
  const nrc = (qty: number) => ({ barcode: '8936240510219', description: 'TH2 NRC H. TU NHIEN SAVE 10KG', qty, amount: qty * 223294 })
  const got = poCompleteness([
    row({ page: '3/7', missingItems: [nrc(6)], poTotals: totals }),
    row({ page: '4/7', missingItems: null, poTotals: totals }),
    row({ page: '5/7', missingItems: [nrc(10)], poTotals: totals }),
  ])
  assert.deepEqual(got, {
    totals,
    missing: [{ barcode: '8936240510219', description: 'TH2 NRC H. TU NHIEN SAVE 10KG', qty: 16, amount: 16 * 223294 }],
  })
})

test('merging does not write back into the rows the table is still showing', () => {
  const first = row({ missingItems: [{ barcode: 'X', description: 'x', qty: 1, amount: 5 }] })
  poCompleteness([first, row({ missingItems: [{ barcode: 'X', description: 'x', qty: 2, amount: 10 }] })])
  assert.equal(first.missingItems?.[0].qty, 1)
})
