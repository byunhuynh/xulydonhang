import type { OrderRow } from '../types'

// Khoá lưu trữ của hai nhánh — PHẢI khớp misapush.BranchHaThanh /
// BranchHTLA phía Go. Nhãn là thứ hiện ra, khoá là thứ ghi xuống file.
export const MISA_BRANCH_OPTIONS = [
  { value: 'ha_thanh', label: 'Hà Thành' },
  { value: 'htla', label: 'HTLA' },
] as const

export interface MisaGroupSeed {
  key: string
  po: string
  system: string
  customerCode: string
  shipTo: string
  excelRows: number[]
}

export interface MisaGroup extends MisaGroupSeed {
  // routeKey/routeLabel/branch do Go phân giải (MisaResolveRoutes) — quy
  // tắc định tuyến chỉ tồn tại ở đó, không viết lại ở đây.
  routeKey: string
  routeLabel: string
  branch: string
  selected: boolean
}

// buildMisaGroups gom các dòng kết quả thành đơn vị "1 đơn".
//
// groupKey được TRUYỀN VÀO chứ không import: file này phải nạp được bằng
// `node --experimental-strip-types` (bộ test của repo), mà runner đó không
// phân giải nổi import không đuôi khi là import giá trị - và
// ./zaloGrouping có đúng một import như vậy. Chỗ gọi duy nhất
// (MisaPushModal) truyền thẳng groupKeyFor, nên vẫn chỉ có MỘT định nghĩa
// khoá nhóm cho cả bảng kết quả, nút Zalo lẫn modal này; chữ ký ép phải
// truyền một cái gì đó, không có đường quên.
//
// Dòng không có excelRows bị bỏ hẳn: không trích xuất được thì không có
// gì trong sổ đặt hàng để đẩy, và để nó hiện lên chỉ tổ khiến người dùng
// tưởng nó sẽ vào sổ.
export function buildMisaGroups(rows: OrderRow[], groupKey: (row: OrderRow) => string): MisaGroupSeed[] {
  const order: string[] = []
  const byKey = new Map<string, MisaGroupSeed>()

  for (const row of rows) {
    if (!row.excelRows || row.excelRows.length === 0) continue
    const key = groupKey(row)
    const existing = byKey.get(key)
    if (existing) {
      existing.excelRows.push(...row.excelRows)
      continue
    }
    order.push(key)
    byKey.set(key, {
      key,
      po: row.po,
      // Thông tin định tuyến lấy từ dòng ĐẦU của nhóm: mọi dòng trong
      // một đơn đều cùng hệ thống, cùng mã khách hàng, cùng kho giao.
      system: row.system,
      customerCode: row.maKhachHang,
      shipTo: row.shipTo,
      excelRows: [...row.excelRows],
    })
  }

  return order.map((key) => byKey.get(key)!)
}

// branchTotals đếm số đơn và số dòng Excel của từng nhánh, chỉ tính các
// đơn đang tick.
export function branchTotals(groups: MisaGroup[]): Record<string, { orders: number; rows: number }> {
  const totals: Record<string, { orders: number; rows: number }> = {}
  for (const option of MISA_BRANCH_OPTIONS) {
    totals[option.value] = { orders: 0, rows: 0 }
  }
  for (const g of groups) {
    if (!g.selected || !g.branch) continue
    const bucket = totals[g.branch] ?? (totals[g.branch] = { orders: 0, rows: 0 })
    bucket.orders += 1
    bucket.rows += g.excelRows.length
  }
  return totals
}

// pendingGroups là các đơn còn phải đẩy: đã tick, và thuộc nhánh CHƯA vào
// sổ thành công. Nhánh đã ghi xong bị loại hẳn - bấm đẩy lại chỉ gửi
// nhánh còn lỗi, không có đường nào ghi trùng nhánh đã vào sổ.
export function pendingGroups(groups: MisaGroup[], pushedBranches: string[]): MisaGroup[] {
  const done = new Set(pushedBranches)
  return groups.filter((g) => g.selected && !done.has(g.branch))
}

// canPush khoá nút đẩy cho tới khi mọi đơn đang tick đều có nhánh. Không
// đoán bừa một nhánh cho khoá chưa map: đoán sai là đơn vào sổ của pháp
// nhân khác.
export function canPush(groups: MisaGroup[], pushedBranches: string[]): boolean {
  const pending = groups.filter((g) => g.selected)
  if (pending.length === 0) return false
  if (pending.some((g) => !g.branch)) return false
  return pendingGroups(groups, pushedBranches).length > 0
}

// rememberRouting dựng map khoá định tuyến -> nhánh để lưu vào Cài đặt.
//
// Khoá bị đặt HAI nhánh khác nhau trong cùng một lượt thì bỏ hẳn: người
// dùng cố tình cho hai đơn cùng loại vào hai sổ khác nhau lần này, ghi
// lại một trong hai là đoán bừa cho lần sau. Thà để lần sau hỏi lại.
export function rememberRouting(groups: MisaGroup[]): Record<string, string> {
  const seen = new Map<string, string>()
  const conflicting = new Set<string>()

  for (const g of groups) {
    if (!g.selected || !g.branch || !g.routeKey) continue
    const previous = seen.get(g.routeKey)
    if (previous !== undefined && previous !== g.branch) {
      conflicting.add(g.routeKey)
      continue
    }
    seen.set(g.routeKey, g.branch)
  }

  const out: Record<string, string> = {}
  for (const [key, branch] of seen) {
    if (!conflicting.has(key)) out[key] = branch
  }
  return out
}
