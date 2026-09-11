import type { MissingItem, OrderRow, POTotalsCheck } from '../types'

// Cảnh báo "đơn chưa đủ" của MỘT nhóm tin Zalo - với BigC là mọi dòng
// store của cùng một PO. totals: tổng PO tự in so với phần đã ghi (backend
// gắn cùng một giá trị lên mọi dòng của PO, lấy dòng nào cũng được).
// missing: mã chưa có trong SanPham, cộng dồn theo barcode qua các trang
// store - cùng một mã thiếu ở 10 store là MỘT việc cần làm (thêm mã vào
// SanPham), không phải 10 dòng giống nhau.
export interface POCompleteness {
  totals: POTotalsCheck | null
  missing: MissingItem[]
}

// null = nhóm này không có gì để cảnh báo.
export function poCompleteness(rows: OrderRow[]): POCompleteness | null {
  let totals: POTotalsCheck | null = null
  const byBarcode = new Map<string, MissingItem>()
  for (const row of rows) {
    if (!totals && row.poTotals) totals = row.poTotals
    for (const item of row.missingItems ?? []) {
      const existing = byBarcode.get(item.barcode)
      if (existing) {
        existing.qty += item.qty
        existing.amount += item.amount
      } else {
        byBarcode.set(item.barcode, { ...item })
      }
    }
  }
  const missing = [...byBarcode.values()]
  if (!totals && missing.length === 0) return null
  return { totals, missing }
}
