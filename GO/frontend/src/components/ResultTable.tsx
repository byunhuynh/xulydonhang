import { Fragment, useEffect, useState } from 'react'
import {
  FaCircle,
  FaCheck,
  FaCircleCheck,
  FaTriangleExclamation,
  FaChevronDown,
  FaChevronRight,
  FaFilePdf,
  FaMagnifyingGlassDollar,
  FaCommentDots,
} from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import type { OrderRow, PriceMismatchDetail } from '../types'
import { SectionHeader } from './SectionHeader'
import { ConfirmPrice } from '../../wailsjs/go/main/App'
import { OrderContentModal, type POContentGroup } from './OrderContentModal'
import {
  resolveEffectivePrice,
  buildPriceBasisForRow,
  buildPriceBasisForPO,
  type PriceBasis,
} from '../lib/zaloMessage'

const columns: {
  key: Exclude<
    keyof OrderRow,
    'priceMismatchDetails' | 'driveUrl' | 'shipTo' | 'entryDate' | 'cancelDate' | 'totalWeightKg' | 'totalPackages' | 'promoItems'
  >
  label: string
}[] = [
  { key: 'fileName', label: 'Tên file' },
  { key: 'page', label: 'Trang' },
  { key: 'system', label: 'Hệ thống' },
  { key: 'maKhachHang', label: 'Mã khách hàng' },
  { key: 'po', label: 'PO' },
  { key: 'donGia', label: 'Đơn giá' },
  { key: 'priceMismatchCount', label: 'Đối soát giá' },
  { key: 'status', label: 'Trạng thái' },
]

function statusMeta(row: OrderRow): { classes: string; label: string } {
  const { status, statusKind } = row
  switch (statusKind) {
    case 'failed':
      return { classes: 'bg-danger/15 text-danger', label: status.replace('❌', '').trim() }
    case 'warning':
      return { classes: 'bg-warning/15 text-warning', label: status.replace('⚠️', '').trim() }
    case 'done':
      return { classes: 'bg-success/15 text-success', label: status.replace('✅', '').trim() }
    default:
      return { classes: 'bg-white/5 text-muted', label: status }
  }
}

// priceMeta renders a dedicated reconciliation badge, independent of the
// overall processing Status column — a "Hoàn thành" row can still carry
// mismatched SKUs (that's exactly what statusKind "warning" means), so
// this makes that fact visible as its own column instead of only living
// inside the Trạng thái text.
function priceMeta(row: OrderRow): { classes: string; label: string; icon: 'ok' | 'warn' | 'none' } {
  if (row.statusKind === 'failed') {
    return { classes: 'bg-white/5 text-muted', label: '—', icon: 'none' }
  }
  if (row.priceMismatchCount > 0) {
    return {
      classes: 'bg-danger/15 text-danger',
      label: `${row.priceMismatchCount} mã sai giá`,
      icon: 'warn',
    }
  }
  return { classes: 'bg-success/15 text-success', label: 'Đúng giá', icon: 'ok' }
}

function formatMoney(value: string): string {
  const n = Number(value)
  if (Number.isNaN(n)) return value
  return n.toLocaleString('vi-VN')
}

export function ResultTable() {
  const rows = useAppStore((s) => s.rows)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const adjustRowDonGia = useAppStore((s) => s.adjustRowDonGia)
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  const [expandedRow, setExpandedRow] = useState<number | null>(null)
  // Selection is keyed by PO NUMBER, not row index - BigC can produce
  // several OrderRows (one per store page) sharing the same po, and
  // ticking any one of them must select the whole PO, never just that
  // one store's row (see buildZaloMessageForPO's own doc comment for why
  // a PO is the real unit of a Zalo notification, not a row).
  const selectedPOs = useAppStore((s) => s.selectedPOs)
  const togglePOSelection = useAppStore((s) => s.togglePOSelection)
  const toggleAllPOs = useAppStore((s) => s.toggleAllPOs)
  const clearSelection = useAppStore((s) => s.clearSelection)
  const resolvedChoice = useAppStore((s) => s.resolvedChoice)
  const setResolvedChoiceKey = useAppStore((s) => s.setResolvedChoice)
  const clearResolvedChoice = useAppStore((s) => s.clearResolvedChoice)
  const [flashCount, setFlashCount] = useState<Record<number, number>>({})
  const [contentModalGroups, setContentModalGroups] = useState<POContentGroup[] | null>(null)
  const appendLog = useAppStore((s) => s.appendLog)

  // Stamps the wall-clock moment each row FIRST appears in the table -
  // the honest stand-in this feature has for Python's server-side
  // start_time, which was recorded at the moment that PO's processing
  // actually finished. Lives in appStore rather than a local ref because
  // ControlPanel must send the message with the exact timestamp the
  // preview modal here showed the user - see the store's own doc comment.
  const receivedAt = useAppStore((s) => s.receivedAt)
  const stampReceivedAt = useAppStore((s) => s.stampReceivedAt)
  const clearReceivedAt = useAppStore((s) => s.clearReceivedAt)
  useEffect(() => {
    rows.forEach((_, i) => {
      // Stamp-once is enforced by the store action, so re-running this on
      // every rows change only ever stamps the newly arrived rows.
      stampReceivedAt(i, new Date().toLocaleTimeString('vi-VN', { hour12: false }))
    })
  }, [rows, stampReceivedAt])

  // A new batch calls resetRows() (see ControlPanel), which empties this
  // array before new rows stream in — that's the right moment to clear
  // any expand/resolved-choice state from the PREVIOUS batch's results,
  // since row index i in the new batch has no relationship to whatever
  // order used to be at that same index.
  useEffect(() => {
    if (rows.length === 0) {
      setExpandedRow(null)
      clearResolvedChoice()
      setContentModalGroups(null)
      clearSelection()
      clearReceivedAt()
    }
  }, [rows.length, clearResolvedChoice, clearSelection, clearReceivedAt])

  function handleCopy(key: string, value: string) {
    navigator.clipboard.writeText(value).catch(() => {})
    setCopiedKey(key)
    setTimeout(() => setCopiedKey((cur) => (cur === key ? null : cur)), 1000)
  }

  async function handleApplyPrice(rowIndex: number, detail: PriceMismatchDetail, useInvoicePrice: boolean) {
    const price = useInvoicePrice ? detail.invoicePrice : detail.systemPrice
    const key = `${rowIndex}-${detail.excelRow}`
    const previousPrice = resolveEffectivePrice(rowIndex, detail, resolvedChoice)
    try {
      await ConfirmPrice(detail.excelRow, price)
      setResolvedChoiceKey(key, useInvoicePrice ? 'po' : 'system')
      const delta = (price - previousPrice) * detail.qty
      if (delta !== 0) {
        adjustRowDonGia(rowIndex, delta)
        setFlashCount((prev) => ({ ...prev, [rowIndex]: (prev[rowIndex] ?? 0) + 1 }))
      }
    } catch (err) {
      appendLog(`❌ Lỗi áp dụng giá cho ${detail.sku}: ${String(err)}`)
    }
  }

  // Applies the same choice to every mismatched SKU in this order in one
  // click - sequential, not concurrent: each ConfirmPrice call edits a
  // different cell in the same shared Excel file, and running them one
  // at a time (matching how a user clicking each row by hand would
  // naturally serialize) avoids relying on excelize's file-level
  // open/save being safe under concurrent writers.
  async function handleApplyAll(rowIndex: number, useInvoicePrice: boolean) {
    const row = rows[rowIndex]
    for (const detail of row.priceMismatchDetails) {
      await handleApplyPrice(rowIndex, detail, useInvoicePrice)
    }
  }

  // Every PO number present in this batch, in first-seen order - the
  // "chọn tất cả" checkbox and the toolbar's selected-count both count
  // against this set, never against rows.length (one PO can span
  // several BigC rows).
  const uniquePOs: string[] = []
  for (const row of rows) {
    if (row.po && !uniquePOs.includes(row.po)) uniquePOs.push(row.po)
  }

  function rowsForPO(po: string): number[] {
    return rows.reduce<number[]>((acc, row, idx) => {
      if (row.po === po) acc.push(idx)
      return acc
    }, [])
  }

  function openContentModalForRow(rowIndex: number) {
    const row = rows[rowIndex]
    setContentModalGroups([{ po: row.po, rows: [row] }])
  }

  function openContentModalForSelection() {
    const groups: POContentGroup[] = [...selectedPOs].map((po) => ({
      po,
      rows: rowsForPO(po).map((idx) => rows[idx]),
    }))
    setContentModalGroups(groups)
  }

  const selectedCount = selectedPOs.size

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3.5">
      <SectionHeader index="04" title="Kết quả xử lý chi tiết" />
      {selectedCount > 0 && (
        <div className="mb-2 flex items-center gap-3 rounded-lg border border-accent/35 bg-accent/[0.08] px-3.5 py-2">
          <span className="font-sans text-xs font-semibold text-accent">
            Đã chọn <span className="font-mono">{selectedCount}</span> đơn
          </span>
          <button
            type="button"
            onClick={clearSelection}
            className="font-sans text-[11px] text-muted underline decoration-dotted underline-offset-2 hover:text-ink"
          >
            Bỏ chọn hết
          </button>
          <button
            type="button"
            onClick={openContentModalForSelection}
            className="ml-auto inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 font-sans text-xs font-bold text-white transition-colors"
            style={{ backgroundColor: '#0068FF' }}
          >
            <FaCommentDots size={11} /> Xem nội dung Zalo
          </button>
        </div>
      )}
      <div className="selectable flex-1 overflow-auto rounded-lg border border-border">
        <table className="w-full border-collapse font-mono text-xs">
          <thead className="sticky top-0 bg-bg">
            <tr>
              <th className="w-9 border-b border-border px-3 py-2 text-center">
                <input
                  type="checkbox"
                  className="cursor-pointer accent-accent"
                  checked={uniquePOs.length > 0 && selectedCount === uniquePOs.length}
                  ref={(el) => {
                    if (el) el.indeterminate = selectedCount > 0 && selectedCount < uniquePOs.length
                  }}
                  onChange={(e) => toggleAllPOs(uniquePOs, e.target.checked)}
                  title="Chọn tất cả PO"
                />
              </th>
              {columns.map((c) => (
                <th
                  key={c.key}
                  className="border-b border-border px-3 py-2 text-left font-sans text-[10px] font-bold uppercase tracking-wider text-muted"
                >
                  {c.label}
                </th>
              ))}
              <th className="border-b border-border px-3 py-2 text-left font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
                Nội dung
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr>
                <td colSpan={columns.length + 2} className="p-6 text-center font-sans text-muted">
                  Chưa có kết quả nào.
                </td>
              </tr>
            )}
            {rows.map((row, i) => {
              const meta = statusMeta(row)
              const price = priceMeta(row)
              const isPOSelected = row.po !== '' && selectedPOs.has(row.po)
              return (
                <Fragment key={i}>
                  <tr className={`transition-colors hover:bg-white/[0.03] ${isPOSelected ? 'bg-accent/[0.06]' : ''}`}>
                    <td className="border-b border-border px-3 py-2 text-center" onClick={(e) => e.stopPropagation()}>
                      {row.po !== '' && (
                        <input
                          type="checkbox"
                          className="cursor-pointer accent-accent"
                          checked={isPOSelected}
                          onChange={() => togglePOSelection(row.po)}
                        />
                      )}
                    </td>
                    {columns.map((c) => {
                      const cellKey = `${i}-${c.key}`
                      const copyValue =
                        c.key === 'status'
                          ? meta.label
                          : c.key === 'priceMismatchCount'
                            ? price.label
                            : String(row[c.key] ?? '')
                      const isCopied = copiedKey === cellKey
                      return (
                        <td
                          key={c.key}
                          onClick={() => handleCopy(cellKey, copyValue)}
                          title="Nhấp để copy"
                          className={`relative cursor-pointer border-b border-border px-3 py-2 text-ink transition-colors ${
                            isCopied ? 'bg-accent/20' : 'hover:bg-accent/[0.08]'
                          }`}
                        >
                          {isCopied ? (
                            <span className="inline-flex items-center gap-1.5 font-sans font-semibold text-accent">
                              <FaCheck size={10} /> Đã copy
                            </span>
                          ) : c.key === 'status' ? (
                            <span
                              className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 font-sans font-semibold ${meta.classes}`}
                            >
                              <FaCircle size={5} />
                              {meta.label}
                            </span>
                          ) : c.key === 'priceMismatchCount' ? (
                            row.priceMismatchCount > 0 ? (
                              <button
                                type="button"
                                onClick={(e) => {
                                  e.stopPropagation()
                                  setExpandedRow((cur) => (cur === i ? null : i))
                                }}
                                className={`inline-flex cursor-pointer items-center gap-1.5 rounded-full px-2.5 py-0.5 font-sans font-semibold ${price.classes}`}
                              >
                                <FaTriangleExclamation size={11} />
                                {price.label}
                                {expandedRow === i ? <FaChevronDown size={9} /> : <FaChevronRight size={9} />}
                              </button>
                            ) : (
                              <span
                                className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 font-sans font-semibold ${price.classes}`}
                              >
                                {price.icon === 'ok' && <FaCircleCheck size={11} />}
                                {price.label}
                              </span>
                            )
                          ) : c.key === 'donGia' ? (
                            <span
                              key={flashCount[i] ?? 0}
                              className={`font-semibold text-accent ${
                                (flashCount[i] ?? 0) > 0 ? 'animate-flash-cell' : ''
                              }`}
                            >
                              {formatMoney(row[c.key])}
                            </span>
                          ) : (
                            row[c.key]
                          )}
                        </td>
                      )
                    })}
                    <td
                      className="border-b border-border px-3 py-2 text-ink"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <button
                        type="button"
                        onClick={() => openContentModalForRow(i)}
                        className="inline-flex items-center gap-1.5 rounded-full border border-border px-2.5 py-0.5 font-sans font-semibold text-accent transition-colors hover:border-accent"
                      >
                        <FaCommentDots size={9} /> Xem
                      </button>
                    </td>
                  </tr>
                  {expandedRow === i && row.priceMismatchDetails.length > 0 && (
                    <tr key={`${i}-detail`} className="bg-bg/60">
                      <td colSpan={columns.length + 2} className="p-0">
                        {row.priceMismatchDetails.length > 1 && (
                          <div className="flex items-center gap-2 border-b border-border bg-panel/60 px-3 py-2">
                            <span className="font-sans text-[10px] font-semibold uppercase tracking-wide text-muted">
                              Áp dụng cho cả {row.priceMismatchDetails.length} mã
                            </span>
                            <button
                              type="button"
                              disabled={isProcessing}
                              onClick={() => handleApplyAll(i, true)}
                              className="inline-flex items-center gap-1.5 rounded border border-border px-2.5 py-1 font-sans text-[10px] font-semibold text-ink transition-colors hover:border-ink disabled:cursor-not-allowed disabled:opacity-40"
                            >
                              <FaFilePdf size={9} /> Toàn bộ giá PO
                            </button>
                            <button
                              type="button"
                              disabled={isProcessing}
                              onClick={() => handleApplyAll(i, false)}
                              className="inline-flex items-center gap-1.5 rounded border border-accent/50 px-2.5 py-1 font-sans text-[10px] font-semibold text-accent transition-colors hover:bg-accent/10 disabled:cursor-not-allowed disabled:opacity-40"
                            >
                              <FaMagnifyingGlassDollar size={9} /> Toàn bộ giá hệ thống
                            </button>
                          </div>
                        )}
                        <table className="w-full border-collapse font-mono text-[11px]">
                          <thead>
                            <tr className="border-b border-border">
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Mã</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Tên SP</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">SL</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Giá PO</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Giá hệ thống</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Áp dụng</th>
                            </tr>
                          </thead>
                          <tbody>
                            {row.priceMismatchDetails.map((detail) => {
                              const key = `${i}-${detail.excelRow}`
                              const choice = resolvedChoice[key]
                              return (
                                <tr key={key} className="border-b border-border last:border-0">
                                  <td className="px-3 py-1.5 text-ink">{detail.sku}</td>
                                  <td className="px-3 py-1.5 text-ink">{detail.productName}</td>
                                  <td className="px-3 py-1.5 text-muted">{detail.qty}</td>
                                  <td className="px-3 py-1.5 text-accent">{detail.invoicePrice.toLocaleString('vi-VN')}</td>
                                  <td className="px-3 py-1.5 text-accent">{detail.systemPrice.toLocaleString('vi-VN')}</td>
                                  <td className="px-3 py-1.5">
                                    <div className="flex gap-1.5">
                                      <button
                                        type="button"
                                        disabled={isProcessing}
                                        title={isProcessing ? 'Đang xử lý đơn hàng, vui lòng đợi' : undefined}
                                        onClick={() => handleApplyPrice(i, detail, true)}
                                        className={`rounded px-2 py-1 font-sans text-[10px] font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
                                          choice === 'po'
                                            ? 'bg-accent text-[#0a1620]'
                                            : 'border border-border text-muted hover:border-accent hover:text-accent'
                                        }`}
                                      >
                                        Dùng giá PO
                                      </button>
                                      <button
                                        type="button"
                                        disabled={isProcessing}
                                        title={isProcessing ? 'Đang xử lý đơn hàng, vui lòng đợi' : undefined}
                                        onClick={() => handleApplyPrice(i, detail, false)}
                                        className={`rounded px-2 py-1 font-sans text-[10px] font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
                                          choice === 'system'
                                            ? 'bg-accent text-[#0a1620]'
                                            : 'border border-border text-muted hover:border-accent hover:text-accent'
                                        }`}
                                      >
                                        Dùng giá hệ thống
                                      </button>
                                    </div>
                                  </td>
                                </tr>
                              )
                            })}
                          </tbody>
                        </table>
                      </td>
                    </tr>
                  )}
                </Fragment>
              )
            })}
          </tbody>
        </table>
      </div>
      {contentModalGroups && contentModalGroups.length > 0 && (() => {
        const firstRowIndex = rowsForPO(contentModalGroups[0].po)[0]
        const combinedPriceBasis: Record<number, PriceBasis> = {}
        for (const g of contentModalGroups) {
          Object.assign(combinedPriceBasis, buildPriceBasisForPO(rows, rowsForPO(g.po), resolvedChoice))
        }
        return (
          <OrderContentModal
            groups={contentModalGroups}
            processedAt={receivedAt[firstRowIndex] ?? ''}
            priceBasisBySku={combinedPriceBasis}
            onClose={() => setContentModalGroups(null)}
          />
        )
      })()}
    </section>
  )
}
