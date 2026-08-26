import type { OrderRow } from '../types'
import { TMDT_SOURCE_PREFIX } from './zaloMessage'

// groupKeyFor quyết định đơn vị "1 tin Zalo" của 1 dòng: PO cho mọi
// vendor khác (không đổi), nhưng sourceId (định danh file PDF, khớp
// đúng cách jitFileGroups.ts đã gộp buổi giao) cho JIT - vì 1 PDF JIT có
// NHIỀU trang, MỖI trang 1 PO khác nhau, gộp theo po như cũ sẽ ra 1 tin
// riêng cho từng trang thay vì 1 tin cho cả file như yêu cầu thực tế.
// Dùng chung ở cả ResultTable.tsx (chọn dòng/tick chọn) lẫn
// ControlPanel.tsx (build job gửi) - PHẢI cùng 1 định nghĩa ở cả 2 nơi,
// khác nhau sẽ khiến dòng được chọn (UI) và dòng thực sự gửi (job) lệch
// nhau.

export function groupKeyFor(row: OrderRow): string {
  // TMĐT gom theo SHOP (mọi ngày của shop đó về một tin) — cùng một lý do
  // như JIT gom theo file: bảng kết quả hiện nhiều dòng cho dễ đối chiếu
  // (mỗi shop mỗi ngày một dòng), nhưng người nhận chỉ cần MỘT tin cho cả
  // đợt. Gom theo po như vendor sẽ ra mỗi ngày một tin.
  if (row.system === 'JIT-CHOICE' || row.sourceId.startsWith(TMDT_SOURCE_PREFIX)) {
    return row.sourceId
  }
  return row.po
}
