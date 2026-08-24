// Builds the Zalo order-notification message shown to the recipient
// (store/vendor contact) — plain text only, since Zalo's own send
// surface doesn't render any markup, so no rich formatting is attempted
// here (confirmed: no client-side setup needed for that). Grouped into
// short sections (blank line between each) rather than one dense block,
// and keeps the price-mismatch detail the recipient actually needs: not
// a raw invoice-vs-system dump, but which price basis is CURRENTLY being
// used for each mismatched SKU (and for the order's own total) — since
// that choice is exactly what changes what they'll actually be charged.
import type { OrderRow, PriceMismatchDetail } from '../types'

function addLine(lines: string[], label: string, value: string | number | null | undefined) {
  if (value === '' || value === null || value === undefined) return
  lines.push(`${label}: ${value}`)
}

function formatMoney(n: number): string {
  return n.toLocaleString('vi-VN')
}

export type PriceBasis = 'po' | 'system'

function basisLabel(basis: PriceBasis): string {
  return basis === 'po' ? 'giá PO' : 'giá hệ thống'
}

function effectivePrice(detail: PriceMismatchDetail, basis: PriceBasis): number {
  return basis === 'po' ? detail.invoicePrice : detail.systemPrice
}

// resolveEffectivePrice là whichever giá đang tính vào DonGia của dòng
// này cho SKU này: giá PO nếu người dùng đã xác nhận chọn, ngược lại
// giá hệ thống (mặc định của DonGia — xem PriceMismatchDetail's doc).
// resolvedChoice key là `${rowIndex}-${excelRow}` (khớp cách
// ResultTable.tsx đã dùng, giữ nguyên qua lần refactor này).
export function resolveEffectivePrice(
  rowIndex: number,
  detail: PriceMismatchDetail,
  resolvedChoice: Record<string, PriceBasis>,
): number {
  const choice = resolvedChoice[`${rowIndex}-${detail.excelRow}`]
  return choice === 'po' ? detail.invoicePrice : detail.systemPrice
}

// buildPriceBasisForRow rút gọn resolvedChoice (key theo rowIndex, có
// thể lặp excelRow giữa các dòng khác nhau) xuống 1 map theo excelRow
// riêng của 1 dòng — đúng dạng buildZaloMessage cần.
export function buildPriceBasisForRow(
  rowIndex: number,
  row: OrderRow,
  resolvedChoice: Record<string, PriceBasis>,
): Record<number, PriceBasis> {
  const result: Record<number, PriceBasis> = {}
  for (const d of row.priceMismatchDetails ?? []) {
    result[d.excelRow] = resolvedChoice[`${rowIndex}-${d.excelRow}`] ?? 'system'
  }
  return result
}

// buildPriceBasisForPO gộp buildPriceBasisForRow của mọi dòng thuộc 1 PO
// (BigC có thể có nhiều dòng/PO) thành 1 map duy nhất cho
// buildZaloMessageForPO — an toàn gộp vì excelRow là số dòng Excel thật,
// không bao giờ trùng giữa 2 OrderRow khác nhau.
export function buildPriceBasisForPO(
  rows: OrderRow[],
  rowIndices: number[],
  resolvedChoice: Record<string, PriceBasis>,
): Record<number, PriceBasis> {
  const result: Record<number, PriceBasis> = {}
  for (const idx of rowIndices) {
    Object.assign(result, buildPriceBasisForRow(idx, rows[idx], resolvedChoice))
  }
  return result
}

// buildZaloMessage's processedAt is the frontend's own "row just
// arrived" timestamp (stamped into appStore.receivedAt when the row first
// showed up in the table) - a fair, honest stand-in for Python's real
// server-side start_time, which the Go pipeline has no equivalent moment
// to record from. Both the preview modal and the real send read that one
// stamped value, so the message the customer receives carries exactly the
// timestamp the user reviewed.
//
// priceBasisBySku tells, for each mismatched SKU (keyed by its own
// excelRow, matching ResultTable's own resolvedChoice key), which price
// the user has currently chosen — 'system' is the correct default for
// any SKU the user hasn't touched yet, matching exactly what the
// order's own DonGia total is already computed with by default (see
// PriceMismatchDetail's own doc comment in types.go).
export function buildZaloMessage(
  row: OrderRow,
  processedAt: string,
  priceBasisBySku: Record<number, PriceBasis>,
): string {
  const orderUrl = row.po ? `https://bluedonhang.pages.dev/?po=${row.po}` : ''
  const hasMismatch = row.priceMismatchCount > 0
  const mismatches = row.priceMismatchDetails ?? []
  const promoItems = row.promoItems ?? []

  const identity: string[] = []
  addLine(identity, '🎫 Đơn hàng', row.po)
  addLine(identity, '🏬 Hệ thống', row.system)
  addLine(identity, '🏪 Cửa hàng', row.shipTo)

  const dates: string[] = []
  addLine(dates, '🗓️ Ngày đặt', row.entryDate)
  addLine(dates, '⏳ Hạn giao', row.cancelDate)

  const totals: string[] = []
  addLine(
    totals,
    '💰 Tổng tiền',
    `${formatMoney(Number(row.donGia) || 0)}đ${hasMismatch ? ' (đã tính theo giá bên dưới)' : ''}`,
  )
  addLine(totals, '📦 Số kiện', row.totalPackages)
  addLine(totals, '⚖️ Trọng lượng', row.totalWeightKg)

  const sections = [
    '🔔 THÔNG BÁO ĐƠN HÀNG',
    identity.join('\n'),
    dates.join('\n'),
    totals.join('\n'),
  ].filter((s) => s !== '')

  if (hasMismatch) {
    const mismatchLines = mismatches.map((d, idx) => {
      const basis = priceBasisBySku[d.excelRow] ?? 'system'
      const chosenPrice = effectivePrice(d, basis)
      // Names the promo behind "Giá hệ thống" whenever one was actually
      // examined for this SKU - a bare system price that happens to
      // differ from the PO's own invoice price reads as unexplained
      // otherwise; "(KM: ...)" is the same promo text already shown for
      // a MATCHED price in the system log (formatSkuLogLine's own "KM:"
      // suffix), just surfaced here too since a mismatch is exactly the
      // case where the recipient most needs to know a promo was involved.
      // The trailing "(áp dụng <date range>)" is that same promo's own
      // pricing-sheet column header - lets whoever reviews this later
      // look the exact promo up on the real sheet instead of hunting by
      // free-text description alone.
      const dateSuffix = d.promoText && d.promoDateRange ? ` (áp dụng ${d.promoDateRange})` : ''
      const promoNote = d.promoText ? ` (KM: ${d.promoText}${dateSuffix})` : ''
      return (
        `${idx + 1}. ${d.sku} - ${d.productName}\n` +
        `   Giá PO: ${formatMoney(d.invoicePrice)}đ · Giá hệ thống: ${formatMoney(d.systemPrice)}đ${promoNote}\n` +
        `   ➡️ Áp dụng ${basisLabel(basis)}: ${formatMoney(chosenPrice)}đ`
      )
    })
    sections.push(`⚠️ Có ${row.priceMismatchCount} mã đang chờ xác nhận giá:\n${mismatchLines.join('\n')}`)
  }

  if (promoItems.length > 0) {
    const promoLines = promoItems.map((p) => `• ${p.sku} - ${p.productName}: ${p.qty}`)
    sections.push(`🎁 Hàng khuyến mãi (tổng theo mã):\n${promoLines.join('\n')}`)
  }

  if (orderUrl) {
    sections.push(`🔗 Xem chi tiết đơn hàng:\n${orderUrl}`)
  }
  if (processedAt) {
    sections.push(`⏱️ Xử lý lúc ${processedAt}`)
  }

  return sections.join('\n\n')
}

// parseWeightKg/formatWeightKg mirror the Go side's coop.FormatWeightKg
// exactly (kg below 1000, tấn at/above, always one decimal place) - the
// only way to correctly SUM several rows' already-formatted weight
// strings back into one combined total without redoing the underlying
// kg math on the Go side.
function parseWeightKg(formatted: string): number {
  const n = parseFloat(formatted)
  if (Number.isNaN(n)) return 0
  return formatted.trim().endsWith('tấn') ? n * 1000 : n
}

function formatWeightKg(kg: number): string {
  if (kg >= 1000) return `${(kg / 1000).toFixed(2).replace(/0$/, '').replace(/\.$/, '.0')} tấn`
  return `${kg.toFixed(2).replace(/0$/, '').replace(/\.$/, '.0')} kg`
}

// buildZaloMessageForPO mirrors the real xulydonhang.py mechanism
// confirmed for BigC (xulydonhang.py:9508-9616 + write_to_dondathang_bigc
// :4925-4964): a single PO can produce several OrderRows in this port
// (one per BigC store page), but the real app writes exactly ONE
// message.txt block per PO number - opened once, added to by every
// store, closed once with a PO-wide aggregate total. Every OTHER vendor
// already has exactly one OrderRow per PO, so passing a 1-row array here
// degrades to the same output buildZaloMessage would produce alone.
//
// Price-mismatch and promo lines are merged BY SKU across every row in
// the group rather than repeated once per store (confirmed as the
// intended business rule, not a Python-parity requirement): a price
// mismatch is a property of the shared price/CTKM sheet, not of any one
// store, so the same SKU mismatched in two stores is one line with the
// quantities added together, not two identical-looking lines.
export function buildZaloMessageForPO(
  rows: OrderRow[],
  processedAt: string,
  priceBasisBySku: Record<number, PriceBasis>,
): string {
  if (rows.length === 0) return ''
  const first = rows[0]
  const orderUrl = first.po ? `https://bluedonhang.pages.dev/?po=${first.po}` : ''

  const totalDonGia = rows.reduce((sum, r) => sum + (Number(r.donGia) || 0), 0)
  const totalPackages = rows.reduce((sum, r) => sum + (r.totalPackages || 0), 0)
  const totalWeightKg = formatWeightKg(rows.reduce((sum, r) => sum + parseWeightKg(r.totalWeightKg || '0 kg'), 0))

  const mismatchBySku = new Map<string, PriceMismatchDetail>()
  for (const r of rows) {
    for (const d of r.priceMismatchDetails ?? []) {
      const existing = mismatchBySku.get(d.sku)
      if (existing) existing.qty += d.qty
      else mismatchBySku.set(d.sku, { ...d })
    }
  }
  const mismatches = [...mismatchBySku.values()]

  const promoBySku = new Map<string, { sku: string; productName: string; qty: number }>()
  for (const r of rows) {
    for (const p of r.promoItems ?? []) {
      const existing = promoBySku.get(p.sku)
      if (existing) existing.qty += p.qty
      else promoBySku.set(p.sku, { ...p })
    }
  }
  const promoItems = [...promoBySku.values()]

  const identity: string[] = []
  addLine(identity, '🎫 Đơn hàng', first.po)
  addLine(identity, '🏬 Hệ thống', first.system)
  addLine(identity, '🏪 Cửa hàng', first.shipTo)

  const dates: string[] = []
  addLine(dates, '🗓️ Ngày đặt', first.entryDate)
  addLine(dates, '⏳ Hạn giao', first.cancelDate)

  const hasMismatch = mismatches.length > 0
  const totals: string[] = []
  addLine(
    totals,
    '💰 Tổng tiền',
    `${formatMoney(totalDonGia)}đ${hasMismatch ? ' (đã tính theo giá bên dưới)' : ''}`,
  )
  addLine(totals, '📦 Số kiện', totalPackages)
  addLine(totals, '⚖️ Trọng lượng', totalWeightKg)

  const sections = [
    '🔔 THÔNG BÁO ĐƠN HÀNG',
    identity.join('\n'),
    dates.join('\n'),
    totals.join('\n'),
  ].filter((s) => s !== '')

  if (hasMismatch) {
    const mismatchLines = mismatches.map((d, idx) => {
      const basis = priceBasisBySku[d.excelRow] ?? 'system'
      const chosenPrice = effectivePrice(d, basis)
      const dateSuffix = d.promoText && d.promoDateRange ? ` (áp dụng ${d.promoDateRange})` : ''
      const promoNote = d.promoText ? ` (KM: ${d.promoText}${dateSuffix})` : ''
      return (
        `${idx + 1}. ${d.sku} - ${d.productName}\n` +
        `   Giá PO: ${formatMoney(d.invoicePrice)}đ · Giá hệ thống: ${formatMoney(d.systemPrice)}đ${promoNote}\n` +
        `   ➡️ Áp dụng ${basisLabel(basis)}: ${formatMoney(chosenPrice)}đ`
      )
    })
    sections.push(`⚠️ Có ${mismatches.length} mã đang chờ xác nhận giá:\n${mismatchLines.join('\n')}`)
  }

  if (promoItems.length > 0) {
    const promoLines = promoItems.map((p) => `• ${p.sku} - ${p.productName}: ${p.qty}`)
    sections.push(`🎁 Hàng khuyến mãi (tổng theo mã):\n${promoLines.join('\n')}`)
  }

  if (orderUrl) {
    sections.push(`🔗 Xem chi tiết đơn hàng:\n${orderUrl}`)
  }
  if (processedAt) {
    sections.push(`⏱️ Xử lý lúc ${processedAt}`)
  }

  return sections.join('\n\n')
}
