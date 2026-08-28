// Ràng buộc khoảng ngày của nhánh TMĐT, tách khỏi component để test được
// bằng node:test mà không cần dựng DOM.
//
// Mọi ngày biểu diễn bằng chuỗi "YYYY-MM-DD" và Date đặt ở GIỜ UTC 00:00.
// Dùng UTC chứ không dùng giờ địa phương là có chủ đích: cộng/trừ ngày ở
// giờ địa phương sẽ lệch một ngày vào các mốc chuyển giờ, và chuỗi gửi
// xuống backend phải là ngày lịch thuần chứ không mang giờ.

export interface TMDTDateRange {
  from: string
  to: string
}

/** Số ngày tối đa một lần lấy dữ liệu, tính cả hai đầu. */
export const MAX_RANGE_DAYS = 7

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/

export function toISODate(d: Date): string {
  const y = d.getUTCFullYear()
  const m = String(d.getUTCMonth() + 1).padStart(2, '0')
  const day = String(d.getUTCDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

export function parseISODate(s: string): Date {
  const [y, m, d] = s.split('-').map(Number)
  return new Date(Date.UTC(y, m - 1, d))
}

export function addDays(d: Date, n: number): Date {
  return new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate() + n))
}

/** Ngày muộn nhất được chọn: HÔM QUA. Đơn hôm nay chưa chốt nên không lấy. */
export function maxSelectableDate(today: Date): Date {
  return addDays(today, -1)
}

function daysBetween(a: Date, b: Date): number {
  return Math.round((b.getTime() - a.getTime()) / 86_400_000)
}

/**
 * isSelectableDay: day có được bấm hay không. anchor là ngày đầu người
 * dùng đã chọn (null khi chưa chọn gì). Chặn NGAY TRÊN LỊCH thay vì báo
 * lỗi sau khi bấm — người dùng thấy được giới hạn trước khi va vào nó.
 *
 * Khi đã có anchor, cửa sổ chỉ mở VỀ PHÍA TRƯỚC: anchor là NGÀY BẮT ĐẦU
 * nên ngày kết thúc phải nằm trong [anchor, anchor + MAX_RANGE_DAYS - 1].
 * Trước đây cửa sổ đối xứng ±6 ngày, nên bấm 15/08 lại mở luôn tới 09/08
 * — nửa lùi về trước đó vừa thừa vừa làm người dùng hiểu sai phạm vi
 * thật sự sẽ lấy. Chưa bấm gì thì mọi ngày quá khứ đều chọn được, kể cả
 * ngày của các tháng trước.
 */
export function isSelectableDay(day: Date, today: Date, anchor: string | null): boolean {
  if (day.getTime() > maxSelectableDate(today).getTime()) return false
  if (!anchor) return true
  const delta = daysBetween(parseISODate(anchor), day)
  return delta >= 0 && delta <= MAX_RANGE_DAYS - 1
}

export function presetRange(preset: 'yesterday' | '3days' | '7days', today: Date): TMDTDateRange {
  const to = maxSelectableDate(today)
  const span = preset === 'yesterday' ? 1 : preset === '3days' ? 3 : 7
  return { from: toISODate(addDays(to, -(span - 1))), to: toISODate(to) }
}

export function normalizeRange(a: string, b: string): TMDTDateRange {
  return a <= b ? { from: a, to: b } : { from: b, to: a }
}

/** validateRange trả null khi hợp lệ, hoặc câu thông báo tiếng Việt. */
export function validateRange(range: TMDTDateRange, today: Date): string | null {
  if (!ISO_DATE.test(range.from) || !ISO_DATE.test(range.to)) {
    return 'Chưa chọn khoảng thời gian.'
  }
  const from = parseISODate(range.from)
  const to = parseISODate(range.to)
  if (from.getTime() > to.getTime()) {
    return 'Ngày bắt đầu phải trước ngày kết thúc.'
  }
  if (to.getTime() > maxSelectableDate(today).getTime()) {
    return 'Chỉ lấy được dữ liệu đến hết ngày hôm qua.'
  }
  if (daysBetween(from, to) + 1 > MAX_RANGE_DAYS) {
    return `Khoảng thời gian tối đa ${MAX_RANGE_DAYS} ngày.`
  }
  return null
}

export function formatRangeLabel(range: TMDTDateRange): string {
  const vn = (s: string) => {
    const [y, m, d] = s.split('-')
    return `${d}/${m}/${y}`
  }
  return range.from === range.to ? vn(range.from) : `${vn(range.from)} → ${vn(range.to)}`
}
