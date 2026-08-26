import { strict as assert } from 'node:assert'
import { test } from 'node:test'
import {
  buildZaloMessageForJITFile,
  buildZaloMessageForTMDTShop,
  tmdtShopFromGroupKey,
} from './zaloMessage.ts'
import type { OrderRow } from '../types.ts'

function row(over: Partial<OrderRow>): OrderRow {
  return {
    fileName: 'XUẤT HÀNG HN-LA MỚI.xlsx',
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

function tmdtRow(over: Partial<OrderRow>): OrderRow {
  return row({
    sourceId: 'tmdt|Blue Việt Nam',
    system: 'TMĐT-TikTok',
    maKhachHang: 'MB_TMDT_00001',
    shipTo: 'HN',
    ...over,
  })
}

test('tin TMĐT gộp mọi ngày của một shop thành một tin', () => {
  const msg = buildZaloMessageForTMDTShop(
    [
      tmdtRow({
        po: 'Blue Việt Nam · 23/08/2026',
        entryDate: '23/08/2026',
        donGia: '1500',
        totalQty: 7,
        totalOrders: 3,
        skus: ['TP1', 'TP2'],
      }),
      tmdtRow({
        po: 'Blue Việt Nam · 25/08/2026',
        entryDate: '25/08/2026',
        donGia: '400',
        totalQty: 1,
        totalOrders: 1,
        // TP2 lặp lại từ ngày trước — số MÃ phải là 2, không phải 3.
        skus: ['TP2'],
      }),
    ],
    '08:45',
  )

  assert.match(msg, /\*\*🔔 ĐƠN HÀNG TMĐT-TikTok\*\*/)
  assert.match(msg, /🏪 \{orange:Blue Việt Nam\}/)
  assert.match(msg, /🗓️ 23\/08 → 25\/08\/2026/)
  assert.match(msg, /📍 HN/)
  assert.match(msg, /Tổng số đơn: \*\*4 đơn\*\*/)
  assert.match(msg, /Tổng số mã hàng: \*\*2 mã\*\*/)
  assert.match(msg, /📦 8 sản phẩm/)
  assert.match(msg, /💰 \*\*1\.900đ\*\*/)
  assert.match(msg, /⏱️ Xử lý lúc 08:45/)
})

test('một ngày duy nhất thì in đúng một ngày, không in khoảng', () => {
  const msg = buildZaloMessageForTMDTShop(
    [tmdtRow({ entryDate: '25/08/2026', donGia: '100', totalOrders: 1, skus: ['TP1'] })],
    '',
  )
  assert.match(msg, /🗓️ 25\/08\/2026/)
  assert.doesNotMatch(msg, /→/)
  // processedAt rỗng thì không có dòng "Xử lý lúc" trống trơ ra.
  assert.doesNotMatch(msg, /Xử lý lúc/)
})

test('shop giao từ hai kho thì nêu cả hai, không giấu bớt', () => {
  const msg = buildZaloMessageForTMDTShop(
    [
      tmdtRow({ entryDate: '25/08/2026', shipTo: 'HN', totalOrders: 1 }),
      tmdtRow({ entryDate: '25/08/2026', shipTo: 'LA', totalOrders: 1 }),
    ],
    '',
  )
  assert.match(msg, /📍 HN \+ LA/)
})

test('còn mã #N/A thì tin phải nói ra', () => {
  const clean = buildZaloMessageForTMDTShop([tmdtRow({ entryDate: '25/08/2026', totalOrders: 1 })], '')
  assert.doesNotMatch(clean, /#N\/A/)

  const dirty = buildZaloMessageForTMDTShop(
    [tmdtRow({ entryDate: '25/08/2026', totalOrders: 1, statusKind: 'warning' })],
    '',
  )
  assert.match(dirty, /#N\/A/)
})

test('shop bán trên hai sàn thì tiêu đề không nhận vơ một sàn', () => {
  const msg = buildZaloMessageForTMDTShop(
    [
      tmdtRow({ entryDate: '25/08/2026', system: 'TMĐT-TikTok', totalOrders: 1 }),
      tmdtRow({ entryDate: '25/08/2026', system: 'TMĐT-Shopee', totalOrders: 1 }),
    ],
    '',
  )
  assert.match(msg, /\*\*🔔 ĐƠN HÀNG TMĐT\*\*/)
})

test('danh sách rỗng trả chuỗi rỗng thay vì tin nhắn què', () => {
  assert.equal(buildZaloMessageForTMDTShop([], '08:45'), '')
})

test('tmdtShopFromGroupKey chỉ nhận key TMĐT, không nhận vơ po của vendor', () => {
  assert.equal(tmdtShopFromGroupKey('tmdt|Blue Việt Nam'), 'Blue Việt Nam')
  assert.equal(tmdtShopFromGroupKey('260824-01017-00354'), '')
  assert.equal(tmdtShopFromGroupKey(''), '')
  // Shop tên rỗng vẫn là nhóm TMĐT, nhưng hàm này trả chuỗi rỗng nên bên
  // gọi sẽ rơi về nhánh vendor. Chấp nhận được: backend chỉ sinh nhóm từ
  // tên shop đọc được, và một tin nhãn xấu vẫn hơn là làm phức tạp thêm
  // ba chỗ gọi bằng một giá trị null.
  assert.equal(tmdtShopFromGroupKey('tmdt|'), '')
})

// Khoá lại bản sửa của nhánh JIT: số MÃ HÀNG phải đếm theo SKU thật, không
// theo excelRows. Một file 3 PO cùng bán đúng 1 mã từng bị báo "3 mã".
test('tin JIT đếm mã hàng theo SKU thật, không theo số dòng Excel', () => {
  const jit = (over: Partial<OrderRow>) =>
    row({ system: 'JIT-CHOICE', shipTo: 'WH6_HN', entryDate: '24/08/2026', ...over })
  const msg = buildZaloMessageForJITFile(
    [
      jit({ donGia: '1000', excelRows: [9, 10], skus: ['TP1'], totalQty: 2, totalWeightKg: '1 kg' }),
      jit({ donGia: '2000', excelRows: [11, 12], skus: ['TP1'], totalQty: 3, totalWeightKg: '2 kg' }),
      jit({ donGia: '3000', excelRows: [13], skus: ['TP2'], totalQty: 1, totalWeightKg: '0 kg' }),
    ],
    'Sáng',
    '',
  )
  assert.match(msg, /Tổng số đơn: \*\*3 PO\*\*/)
  assert.match(msg, /Tổng số mã hàng: \*\*2 mã\*\*/)
  assert.match(msg, /📦 6 sản phẩm/)
  assert.match(msg, /💰 \*\*6\.000đ\*\*/)
})

// Go trả `null` (KHÔNG phải []) cho mọi slice chưa gán — đã kiểm bằng
// json.Marshal trên OrderRow rỗng: `"skus":null`. Phần còn lại của file
// này vẫn luôn viết `r.promoItems ?? []` / `r.priceMismatchDetails ?? []`
// đúng vì lý do đó; nhánh đếm SKU mới bỏ mất lớp phòng vệ ấy.
//
// Ca dính đòn thật: một nhóm TMĐT mà MỌI dòng đều là #N/A → không mã nào
// vào danh sách → backend gửi null → cả tin nhắn ném lỗi, nút Gửi Zalo
// chết cứng đúng lúc dữ liệu đang có vấn đề cần báo nhất.
test('thiếu danh sách mã (null từ Go) thì đếm 0, không được ném lỗi', () => {
  const naked = { skus: undefined as unknown as string[] }
  const tmdt = buildZaloMessageForTMDTShop(
    [tmdtRow({ entryDate: '25/08/2026', totalOrders: 2, statusKind: 'warning', ...naked })],
    '',
  )
  assert.match(tmdt, /Tổng số mã hàng: \*\*0 mã\*\*/)
  assert.match(tmdt, /Tổng số đơn: \*\*2 đơn\*\*/)

  const jit = buildZaloMessageForJITFile(
    [row({ system: 'JIT-CHOICE', shipTo: 'WH6_HN', entryDate: '24/08/2026', donGia: '1000', ...naked })],
    'Sáng',
    '',
  )
  assert.match(jit, /Tổng số mã hàng: \*\*0 mã\*\*/)
})
