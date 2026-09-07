import { strict as assert } from 'node:assert'
import { test } from 'node:test'
import { zaloTargetQueriesFor, targetByPO, type ZaloTargetPreview } from './zaloTargets.ts'
import type { OrderRow } from '../types.ts'
import type { POContentGroup } from '../components/OrderContentModal.tsx'

function row(over: Partial<OrderRow>): OrderRow {
  return {
    fileName: '',
    sourceId: '',
    page: '',
    system: '',
    maKhachHang: '',
    po: '',
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

function group(po: string, rows: OrderRow[]): POContentGroup {
  return { po, rows }
}

// Query PHẢI dựng từ dòng ĐẦU của nhóm, y hệt cách ControlPanel.handleSendZalo
// dựng ZaloJob - lấy dòng khác đi là bản xem trước hiện một nhóm mà tin lại
// bay tới nhóm khác.
test('dựng query từ dòng đầu của mỗi nhóm', () => {
  const queries = zaloTargetQueriesFor([
    group('PO1', [row({ system: 'BigC', maKhachHang: 'MN_MT_bgc06' }), row({ system: 'BigC', maKhachHang: 'MB_GC_bgc06' })]),
    group('PO2', [row({ system: 'Satra', maKhachHang: 'MN_MT_sat01' })]),
  ])

  assert.deepEqual(queries, [
    { po: 'PO1', system: 'BigC', customerCode: 'MN_MT_bgc06' },
    { po: 'PO2', system: 'Satra', customerCode: 'MN_MT_sat01' },
  ])
})

test('nhóm rỗng vẫn ra query rỗng chứ không làm hỏng cả danh sách', () => {
  const queries = zaloTargetQueriesFor([group('PO1', []), group('PO2', [row({ system: 'Lotte', maKhachHang: 'MN01' })])])

  assert.equal(queries.length, 2)
  assert.deepEqual(queries[0], { po: 'PO1', system: '', customerCode: '' })
  assert.deepEqual(queries[1], { po: 'PO2', system: 'Lotte', customerCode: 'MN01' })
})

test('không có nhóm nào thì không hỏi backend', () => {
  assert.deepEqual(zaloTargetQueriesFor([]), [])
})

function preview(over: Partial<ZaloTargetPreview>): ZaloTargetPreview {
  return { po: '', configured: false, groupName: '', key: '', suggestedKey: '', candidateKeys: [], ...over }
}

test('tra kết quả theo po', () => {
  const index = targetByPO([
    preview({ po: 'PO1', configured: true, groupName: 'Big-C MN', key: 'MNBIGC' }),
    preview({ po: 'PO2', suggestedKey: 'MNMAXIDI', candidateKeys: ['MNGCMAXIDI', 'MNMAXIDI'] }),
  ])

  assert.equal(index['PO1']?.groupName, 'Big-C MN')
  assert.equal(index['PO2']?.suggestedKey, 'MNMAXIDI')
  assert.equal(index['PO3'], undefined)
})

// JIT/TMĐT gộp theo sourceId nên `po` có thể là hash 64 ký tự hoặc
// "tmdt|<shop>"; index chỉ cần khớp đúng chuỗi đó, không diễn giải nó.
test('po dạng hash/khoá gộp vẫn tra được', () => {
  const key = 'tmdt|Blue Việt Nam'
  const index = targetByPO([preview({ po: key, configured: true, groupName: 'Nhóm TMĐT' })])

  assert.equal(index[key]?.groupName, 'Nhóm TMĐT')
})
