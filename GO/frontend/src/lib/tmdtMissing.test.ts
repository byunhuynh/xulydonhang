import { strict as assert } from 'node:assert'
import { test } from 'node:test'
import { draftError, emptyDraft, isDraftFilled, type TMDTMissingCombo } from './tmdtMissing.ts'

const missing: TMDTMissingCombo = {
  key: 'sku:SP777',
  product: 'Sản phẩm mới',
  variant: 'Combo 3 Túi',
  combo: 'SP777',
  lineCount: 12,
}

test('emptyDraft mang sẵn thông tin điền trước, để trống phần người dùng phải nhập', () => {
  const d = emptyDraft(missing)
  assert.equal(d.key, 'sku:SP777')
  assert.equal(d.product, 'Sản phẩm mới')
  assert.equal(d.variant, 'Combo 3 Túi')
  assert.equal(d.combo, 'SP777')
  assert.deepEqual(d.tp, ['', '', '', ''])
  assert.deepEqual(d.sl, ['', '', '', ''])
})

test('isDraftFilled chỉ tính là đã khai khi có mã thành phẩm đầu tiên', () => {
  const d = emptyDraft(missing)
  assert.equal(isDraftFilled(d), false)
  assert.equal(isDraftFilled({ ...d, tp: ['TP777', '', '', ''], sl: ['1', '', '', ''] }), true)
})

test('draftError đòi số lượng cho mỗi mã đã điền', () => {
  const d = emptyDraft(missing)
  // Chưa điền gì: không phải lỗi — bỏ trống là giữ #N/A cho mục này.
  assert.equal(draftError(d), null)
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['', '', '', ''] }))
  assert.equal(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['1', '', '', ''] }), null)
})

test('draftError chặn số lượng không phải số dương', () => {
  const d = emptyDraft(missing)
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['0', '', '', ''] }))
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['abc', '', '', ''] }))
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['-2', '', '', ''] }))
})

test('draftError chặn số lượng điền mà thiếu mã thành phẩm tương ứng', () => {
  const d = emptyDraft(missing)
  assert.ok(draftError({ ...d, tp: ['TP777', '', '', ''], sl: ['1', '2', '', ''] }))
})

// Trường hợp riêng: KHÔNG điền mã nào nhưng có điền số lượng. isDraftFilled
// trả false nên dòng này sẽ không được gửi lên backend — nếu draftError im
// lặng thì người dùng gõ số rồi bấm "Lưu" và tưởng đã khai xong, trong khi
// mục đó âm thầm giữ #N/A.
test('draftError bắt được số lượng lạc khi chưa điền mã nào', () => {
  const d = emptyDraft(missing)
  assert.ok(draftError({ ...d, sl: ['', '2', '', ''] }))
})
