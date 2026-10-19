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

export interface MisaBuildResult {
  groups: MisaGroupSeed[]
  /** Số đơn (theo khoá nhóm) bị bỏ vì không có dòng nào trong sổ đặt hàng. */
  skipped: { key: string; po: string; system: string }[]
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
// Một đơn có excelRows là rỗng ở MỌI dòng (mười vendor ngoài BigC/JIT
// hiện chưa điền OrderRow.ExcelRows - xem internal/processing) không có
// gì trong sổ đặt hàng để tách ra mà đẩy. Trước đây những đơn này bị
// `continue` âm thầm ngay trong vòng lặp: biến mất khỏi modal không một
// lời cảnh báo, MISA vẫn báo ✅ cho phần còn lại, và lô thuần các vendor
// đó thì modal hiện "0 đơn" mà không giải thích gì. Giờ trả về riêng
// trong `skipped` để gọi nơi hiển thị BẮT BUỘC phải cảnh báo và canPush
// BẮT BUỘC phải chặn - xem canPush bên dưới.
//
// Một đơn có ÍT NHẤT một dòng hợp lệ vẫn vào `groups` như cũ, chỉ gộp
// những dòng hợp lệ (dòng thiếu excelRows của đơn đó không đóng góp gì
// vào excelRows của nhóm, nhưng không làm cả đơn bị coi là bỏ).
//
// Dòng THẤT BẠI thì bị loại hẳn từ đầu, không vào groups lẫn skipped: nó
// là một trang PDF không đọc được, không phải một đơn. Trang như vậy có
// po/system rỗng, nên trước đây mọi trang hỏng của cả lô dồn chung vào
// khoá "" và hiện lên thành cảnh báo đỏ "1 đơn không đẩy được ... ()" —
// cái ngoặc rỗng chính là po/system rỗng đó, và nó CHẶN luôn nút đẩy
// (canPush) dù mọi đơn thật đều sẵn sàng. Bảng kết quả đã tô đỏ những
// trang này rồi; modal push không có gì để nói thêm. Cùng hợp đồng với
// TestMoiDonThanhCongDeuCoExcelRows bên Go, vốn cũng chỉ đòi ExcelRows ở
// các dòng KHÔNG thất bại.
export function buildMisaGroups(rows: OrderRow[], groupKey: (row: OrderRow) => string): MisaBuildResult {
  const order: string[] = []
  const byKey = new Map<string, MisaGroupSeed>()
  const hasValidRow = new Set<string>()

  for (const row of rows) {
    if (row.statusKind === 'failed') continue
    const key = groupKey(row)
    let seed = byKey.get(key)
    if (!seed) {
      order.push(key)
      seed = {
        key,
        po: row.po,
        // Thông tin định tuyến lấy từ dòng ĐẦU của nhóm: mọi dòng trong
        // một đơn đều cùng hệ thống, cùng mã khách hàng, cùng kho giao.
        system: row.system,
        customerCode: row.maKhachHang,
        shipTo: row.shipTo,
        excelRows: [],
      }
      byKey.set(key, seed)
    }
    if (row.excelRows && row.excelRows.length > 0) {
      seed.excelRows.push(...row.excelRows)
      hasValidRow.add(key)
    }
  }

  const groups: MisaGroupSeed[] = []
  const skipped: MisaBuildResult['skipped'] = []
  for (const key of order) {
    const seed = byKey.get(key)!
    if (hasValidRow.has(key)) {
      groups.push(seed)
    } else {
      skipped.push({ key: seed.key, po: seed.po, system: seed.system })
    }
  }

  return { groups, skipped }
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

// pendingGroups là các đơn còn phải đẩy: đã tick, và CHƯA vào sổ. Trước
// đây khoá theo nhánh (nhánh ok thì loại cả nhánh); giờ MISA báo được
// từng đơn nên khoá theo từng đơn — trong một nhánh lỗi, đơn "đã tồn tại"
// bỏ tick đi thì phần còn lại đẩy lại được mà không ghi trùng đơn nào.
export function pendingGroups(groups: MisaGroup[], doneKeys: string[]): MisaGroup[] {
  const done = new Set(doneKeys)
  return groups.filter((g) => g.selected && !done.has(g.key))
}

// canPush khoá nút đẩy cho tới khi mọi đơn đang tick đều có nhánh. Không
// đoán bừa một nhánh cho khoá chưa map: đoán sai là đơn vào sổ của pháp
// nhân khác.
//
// skippedCount > 0 luôn khoá nút, bất kể các đơn còn lại sẵn sàng đến
// đâu: đẩy phần còn lại rồi để MISA báo ✅ là cách chắc chắn nhất khiến
// sổ kế toán ghi thiếu những đơn bị bỏ (không có dòng nào trong sổ đặt
// hàng) mà không ai biết - người dùng phải thấy băng cảnh báo và tự
// quyết định (chấp nhận bỏ đơn đó, hay quay lại kiểm tra sổ đặt hàng)
// trước khi được phép bấm đẩy, chứ không được đẩy một phần trong im lặng.
export function canPush(groups: MisaGroup[], doneKeys: string[], skippedCount: number): boolean {
  if (skippedCount > 0) return false
  const pending = pendingGroups(groups, doneKeys)
  if (pending.length === 0) return false
  return !pending.some((g) => !g.branch)
}

// Trạng thái từng đơn mà backend phát qua misa:orders — PHẢI khớp các
// hằng misaOrder* trong app_misa.go.
export type MisaOrderState = 'pushing' | 'ok' | 'exists' | 'invalid' | 'held' | 'failed' | 'unknown'

export interface MisaOrderStatus {
  key: string
  po: string
  branch: string
  status: MisaOrderState
  message: string
}

export const MISA_ORDER_META: Record<MisaOrderState, { label: string; tone: 'accent' | 'success' | 'warning' | 'danger' | 'muted' }> = {
  pushing: { label: 'Đang đẩy…', tone: 'accent' },
  ok: { label: 'Đã vào sổ', tone: 'success' },
  exists: { label: 'Đã tồn tại', tone: 'warning' },
  invalid: { label: 'Lỗi dữ liệu', tone: 'danger' },
  held: { label: 'Chưa ghi', tone: 'muted' },
  failed: { label: 'Không đẩy được', tone: 'danger' },
  unknown: { label: 'Chưa rõ', tone: 'warning' },
}

// doneOrderKeys là các đơn KHÔNG được đẩy lại: đã vào sổ, hoặc đang đẩy.
// "Chưa rõ" (lỗi giữa lúc ghi sổ) cố ý KHÔNG nằm đây — người dùng phải
// kiểm tra MISA rồi tự quyết; nếu đơn thật ra đã vào, MISA sẽ báo "đã
// tồn tại" chứ không ghi trùng.
export function doneOrderKeys(statuses: Record<string, MisaOrderStatus>): string[] {
  return Object.values(statuses)
    .filter((s) => s.status === 'ok' || s.status === 'pushing')
    .map((s) => s.key)
}

// shouldPreselect: lúc mở modal hoặc khi kết quả về, đơn đã tồn tại trên
// MISA được bỏ tick sẵn — đẩy lại chỉ khiến cả nhánh bị chặn lần nữa.
export function shouldPreselect(status: MisaOrderStatus | undefined): boolean {
  return !status || (status.status !== 'exists' && status.status !== 'ok')
}

// summarizeOrderStatuses đếm số đơn theo trạng thái, theo thứ tự cố định
// của MISA_ORDER_META, bỏ trạng thái không có đơn nào.
export function summarizeOrderStatuses(statuses: MisaOrderStatus[]): { status: MisaOrderState; count: number }[] {
  const counts = new Map<MisaOrderState, number>()
  for (const s of statuses) counts.set(s.status, (counts.get(s.status) ?? 0) + 1)
  return (Object.keys(MISA_ORDER_META) as MisaOrderState[])
    .filter((k) => counts.has(k))
    .map((k) => ({ status: k, count: counts.get(k)! }))
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
