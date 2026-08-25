import { strict as assert } from 'node:assert'
import { test } from 'node:test'
import {
  MAX_RANGE_DAYS,
  addDays,
  formatRangeLabel,
  isSelectableDay,
  maxSelectableDate,
  normalizeRange,
  parseISODate,
  presetRange,
  toISODate,
  validateRange,
} from './tmdtDateRange.ts'

// Mốc "hôm nay" cố định cho mọi test: 25/08/2026.
const today = parseISODate('2026-08-25')

test('maxSelectableDate là hôm qua, không phải hôm nay', () => {
  assert.equal(toISODate(maxSelectableDate(today)), '2026-08-24')
})

test('isSelectableDay chặn hôm nay và tương lai', () => {
  assert.equal(isSelectableDay(parseISODate('2026-08-24'), today, null), true)
  assert.equal(isSelectableDay(parseISODate('2026-08-25'), today, null), false)
  assert.equal(isSelectableDay(parseISODate('2026-08-26'), today, null), false)
})

test('isSelectableDay chặn ngày cách mốc đã chọn quá 6 ngày', () => {
  const anchor = '2026-08-18'
  // 18 → 24 là đúng 7 ngày tính cả hai đầu: hợp lệ.
  assert.equal(isSelectableDay(parseISODate('2026-08-24'), today, anchor), true)
  // 17 → 18 ... đi về phía trước cũng vẫn trong 7 ngày.
  assert.equal(isSelectableDay(parseISODate('2026-08-12'), today, anchor), true)
  // Ngày thứ 8 ở cả hai phía: chặn.
  assert.equal(isSelectableDay(parseISODate('2026-08-11'), today, anchor), false)
  // Phía sau bị chặn bởi ràng buộc "≤ hôm qua" trước cả ràng buộc 7 ngày.
  assert.equal(isSelectableDay(parseISODate('2026-08-25'), today, anchor), false)
})

test('presetRange cho ra đúng khoảng, luôn kết thúc ở hôm qua', () => {
  assert.deepEqual(presetRange('yesterday', today), { from: '2026-08-24', to: '2026-08-24' })
  assert.deepEqual(presetRange('3days', today), { from: '2026-08-22', to: '2026-08-24' })
  assert.deepEqual(presetRange('7days', today), { from: '2026-08-18', to: '2026-08-24' })
})

test('normalizeRange sắp lại thứ tự khi người dùng bấm ngày sau trước', () => {
  assert.deepEqual(normalizeRange('2026-08-24', '2026-08-20'), { from: '2026-08-20', to: '2026-08-24' })
  assert.deepEqual(normalizeRange('2026-08-20', '2026-08-24'), { from: '2026-08-20', to: '2026-08-24' })
})

test('validateRange trả null khi hợp lệ', () => {
  assert.equal(validateRange({ from: '2026-08-18', to: '2026-08-24' }, today), null)
  assert.equal(validateRange({ from: '2026-08-24', to: '2026-08-24' }, today), null)
})

test('validateRange chặn khoảng dài hơn 7 ngày', () => {
  const msg = validateRange({ from: '2026-08-17', to: '2026-08-24' }, today)
  assert.ok(msg && msg.includes('7'), `muốn thông báo nhắc giới hạn 7 ngày, được: ${msg}`)
})

test('validateRange chặn ngày hôm nay và tương lai', () => {
  assert.ok(validateRange({ from: '2026-08-24', to: '2026-08-25' }, today))
  assert.ok(validateRange({ from: '2026-08-25', to: '2026-08-25' }, today))
})

test('validateRange chặn chuỗi ngày rỗng hoặc sai định dạng', () => {
  assert.ok(validateRange({ from: '', to: '2026-08-24' }, today))
  assert.ok(validateRange({ from: '24/08/2026', to: '2026-08-24' }, today))
})

test('addDays không bị lệch vì múi giờ', () => {
  assert.equal(toISODate(addDays(parseISODate('2026-08-01'), -1)), '2026-07-31')
  assert.equal(toISODate(addDays(parseISODate('2026-12-31'), 1)), '2027-01-01')
})

test('formatRangeLabel đọc được cho người Việt', () => {
  assert.equal(formatRangeLabel({ from: '2026-08-24', to: '2026-08-24' }), '24/08/2026')
  assert.equal(formatRangeLabel({ from: '2026-08-18', to: '2026-08-24' }), '18/08/2026 → 24/08/2026')
})

test('MAX_RANGE_DAYS là 7', () => {
  assert.equal(MAX_RANGE_DAYS, 7)
})
