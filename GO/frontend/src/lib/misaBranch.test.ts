import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  MISA_BRANCH_OPTIONS,
  branchTotals,
  buildMisaGroups,
  canPush,
  pendingGroups,
  rememberRouting,
  type MisaGroup,
} from './misaBranch.ts'
import type { OrderRow } from '../types.ts'

// Bản sao tối giản của groupKeyFor (lib/zaloGrouping.ts) CHỈ dùng trong
// test. Bản thật được MisaPushModal truyền vào lúc chạy — ở đây chỉ cần
// một hàm có cùng hình dạng để kiểm cơ chế gom nhóm.
const testGroupKey = (r: OrderRow): string =>
  r.system === 'JIT-CHOICE' || r.sourceId.startsWith('tmdt|') ? r.sourceId : r.po

function row(over: Partial<OrderRow>): OrderRow {
  return {
    fileName: '', sourceId: '', page: '', system: 'Lotte', maKhachHang: 'MN_MT_LOT1001',
    po: 'PO-1', resultKey: '', maVanDon: '', donGia: '', status: '', statusKind: '',
    excelRows: [9], jitPeriod: '', driveUrl: '', priceMismatchCount: 0,
    priceMismatchDetails: [], shipTo: '', entryDate: '', cancelDate: '',
    totalWeightKg: '', totalPackages: 0, totalQty: 0, skus: [], totalOrders: 0,
    promoItems: [], ...over,
  }
}

function group(over: Partial<MisaGroup>): MisaGroup {
  return {
    key: 'PO-1', po: 'PO-1', system: 'Lotte', customerCode: 'MN_MT_LOT1001', shipTo: '',
    excelRows: [9], routeKey: 'Lotte', routeLabel: 'Lotte', branch: 'htla', selected: true,
    ...over,
  }
}

test('hai nhánh, đúng khoá lưu trữ và nhãn hiển thị', () => {
  assert.deepEqual(
    MISA_BRANCH_OPTIONS.map((o) => [o.value, o.label]),
    [['ha_thanh', 'Hà Thành'], ['htla', 'HTLA']],
  )
})

test('gom nhóm theo cùng khoá mà bảng kết quả và nút Zalo đang dùng', () => {
  const groups = buildMisaGroups([
    row({ po: 'PO-A', excelRows: [9, 10] }),
    row({ po: 'PO-A', excelRows: [11] }),
    row({ po: 'PO-B', excelRows: [12] }),
  ], testGroupKey)
  assert.equal(groups.length, 2)
  assert.deepEqual(groups[0].excelRows, [9, 10, 11])
  assert.deepEqual(groups[1].excelRows, [12])
})

test('gom JIT theo file chứ không theo từng trang', () => {
  const groups = buildMisaGroups([
    row({ system: 'JIT-CHOICE', sourceId: 'awb.pdf', po: 'PO-1', excelRows: [9], shipTo: 'WH6_HN' }),
    row({ system: 'JIT-CHOICE', sourceId: 'awb.pdf', po: 'PO-2', excelRows: [10], shipTo: 'WH6_HN' }),
  ], testGroupKey)
  assert.equal(groups.length, 1)
  assert.deepEqual(groups[0].excelRows, [9, 10])
  assert.equal(groups[0].shipTo, 'WH6_HN')
})

test('bỏ qua dòng không ghi được vào Excel', () => {
  // Dòng hỏng (không trích xuất được) không có excelRows — không có gì để
  // đẩy, và đưa vào modal chỉ tổ khiến người dùng tưởng nó sẽ vào sổ.
  const groups = buildMisaGroups([row({ po: 'PO-A', excelRows: [] }), row({ po: 'PO-B', excelRows: [9] })], testGroupKey)
  assert.equal(groups.length, 1)
  assert.equal(groups[0].po, 'PO-B')
})

test('giữ thông tin định tuyến của dòng đầu mỗi nhóm', () => {
  const groups = buildMisaGroups([
    row({ po: 'PO-A', system: 'BigC', maKhachHang: 'MB_GC_BIGC', excelRows: [9] }),
  ], testGroupKey)
  assert.equal(groups[0].system, 'BigC')
  assert.equal(groups[0].customerCode, 'MB_GC_BIGC')
})

test('đếm đơn và dòng theo nhánh, chỉ tính đơn đang tick', () => {
  const totals = branchTotals([
    group({ key: 'a', branch: 'htla', excelRows: [9, 10], selected: true }),
    group({ key: 'b', branch: 'htla', excelRows: [11], selected: false }),
    group({ key: 'c', branch: 'ha_thanh', excelRows: [12], selected: true }),
  ])
  assert.deepEqual(totals.htla, { orders: 1, rows: 2 })
  assert.deepEqual(totals.ha_thanh, { orders: 1, rows: 1 })
})

test('không cho đẩy khi còn đơn đã tick mà chưa có nhánh', () => {
  assert.equal(canPush([group({ branch: '' })], []), false)
})

test('không cho đẩy khi không tick đơn nào', () => {
  assert.equal(canPush([group({ selected: false })], []), false)
})

test('cho đẩy khi mọi đơn đã tick đều có nhánh', () => {
  assert.equal(canPush([group({ branch: 'htla' }), group({ key: 'b', selected: false, branch: '' })], []), true)
})

test('nhánh đã vào sổ bị loại khỏi lượt đẩy sau, nhánh lỗi thì không', () => {
  const groups = [
    group({ key: 'a', branch: 'htla' }),
    group({ key: 'b', branch: 'ha_thanh' }),
  ]
  const pending = pendingGroups(groups, ['htla'])
  assert.deepEqual(pending.map((g) => g.key), ['b'])
  assert.equal(canPush(groups, ['htla']), true)
  assert.equal(canPush(groups, ['htla', 'ha_thanh']), false)
})

test('ghi nhớ dựng map khoá định tuyến -> nhánh', () => {
  const remembered = rememberRouting([
    group({ key: 'a', routeKey: 'Lotte', branch: 'htla' }),
    group({ key: 'b', routeKey: 'BigC/MT', branch: 'ha_thanh' }),
  ])
  assert.deepEqual(remembered, { Lotte: 'htla', 'BigC/MT': 'ha_thanh' })
})

test('không ghi nhớ khoá bị đặt hai nhánh khác nhau trong cùng một lượt', () => {
  // Người dùng cố tình cho hai đơn Lotte vào hai sổ khác nhau lần này.
  // Ghi lại một trong hai là đoán bừa cho lần sau — thà để hỏi lại.
  const remembered = rememberRouting([
    group({ key: 'a', routeKey: 'Lotte', branch: 'htla' }),
    group({ key: 'b', routeKey: 'Lotte', branch: 'ha_thanh' }),
    group({ key: 'c', routeKey: 'Emart', branch: 'ha_thanh' }),
  ])
  assert.deepEqual(remembered, { Emart: 'ha_thanh' })
})

test('không ghi nhớ đơn chưa tick hoặc chưa có nhánh', () => {
  const remembered = rememberRouting([
    group({ key: 'a', routeKey: 'Lotte', branch: 'htla', selected: false }),
    group({ key: 'b', routeKey: 'Satra', branch: '' }),
  ])
  assert.deepEqual(remembered, {})
})
