// Kiểu dữ liệu và kiểm tra đầu vào cho modal khai mã thành phẩm còn thiếu.
//
// Đặt ở lib chứ không ở types.ts: types.ts đang được một phiên khác sửa
// song song, và phần logic ở đây cần test được bằng node:test.

/** Một mã chưa khai báo trong sheet "data shop", đã gom unique ở backend. */
export interface TMDTMissingCombo {
  key: string
  product: string
  variant: string
  combo: string
  lineCount: number
}

/** Bản nháp người dùng đang điền — đúng hình dạng một dòng cột A..K. */
export interface TMDTComboDraft {
  key: string
  product: string
  variant: string
  combo: string
  tp: [string, string, string, string]
  sl: [string, string, string, string]
}

export function emptyDraft(m: TMDTMissingCombo): TMDTComboDraft {
  return {
    key: m.key,
    product: m.product,
    variant: m.variant,
    combo: m.combo,
    tp: ['', '', '', ''],
    sl: ['', '', '', ''],
  }
}

/** Đã khai hay chưa. Bỏ trống hoàn toàn là hợp lệ: mục đó giữ #N/A. */
export function isDraftFilled(d: TMDTComboDraft): boolean {
  return d.tp[0].trim() !== ''
}

/**
 * draftError trả null khi bản nháp dùng được, hoặc câu thông báo tiếng
 * Việt. Kiểm ngay trên form thay vì để backend từ chối: một dòng sai ghi
 * vào "data shop" sẽ sai vĩnh viễn cho mọi lần chạy sau.
 */
export function draftError(d: TMDTComboDraft): string | null {
  if (!isDraftFilled(d)) {
    // Chưa điền mã nào. Bỏ trống hoàn toàn là chủ ý — nhưng đã gõ số lượng
    // thì phải báo, vì dòng này không được gửi đi và mục đó sẽ âm thầm
    // giữ #N/A dù người dùng tưởng đã khai.
    for (let i = 0; i < 4; i += 1) {
      if (d.sl[i].trim() !== '') return 'Đã điền số lượng nhưng thiếu mã thành phẩm.'
    }
    return null
  }
  for (let i = 0; i < 4; i += 1) {
    const tp = d.tp[i].trim()
    const sl = d.sl[i].trim()
    if (tp === '' && sl === '') continue
    if (tp === '') return `Thành phẩm ${i + 1}: đã điền số lượng nhưng thiếu mã.`
    if (sl === '') return `Thành phẩm ${i + 1}: thiếu số lượng.`
    const n = Number(sl)
    if (!Number.isFinite(n) || n <= 0) return `Thành phẩm ${i + 1}: số lượng phải là số lớn hơn 0.`
  }
  return null
}
