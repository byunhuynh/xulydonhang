import { Fragment, useEffect, useRef, useState } from 'react'
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
  FaXmark,
} from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import type { OrderRow, PriceMismatchDetail } from '../types'
import { SectionHeader } from './SectionHeader'
import { ConfirmPrice, UpdateJITPeriod } from '../../wailsjs/go/main/App'
import { OrderContentModal, type POContentGroup } from './OrderContentModal'
import {
  resolveEffectivePrice,
  buildPriceBasisForRow,
  buildPriceBasisForPO,
  type PriceBasis,
} from '../lib/zaloMessage'
import { useListEntrance } from '../lib/useListEntrance'
import { mismatchesForPO } from '../lib/orderMismatchScope'
import { groupJITFiles, skipsPriceReconciliation } from '../lib/jitFileGroups'
import { groupKeyFor } from '../lib/zaloGrouping'
import { belowSystemPriceDetails } from '../lib/poPriceWarning'
import { JITPeriodMenu } from './JITPeriodMenu'
import { isJITPeriodMenuDisabled } from '../lib/jitPeriodState'

type PendingPOPriceAction =
  | { kind: 'single'; rowIndex: number; detail: PriceMismatchDetail }
  | {
      kind: 'all'
      po: string
      items: Array<{ rowIndex: number; detail: PriceMismatchDetail }>
      warningDetails: PriceMismatchDetail[]
    }

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
  if (skipsPriceReconciliation(row)) {
    return { classes: 'bg-white/5 text-muted', label: 'Không đối soát', icon: 'none' }
  }
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
  const tbodyRef = useRef<HTMLTableSectionElement>(null)
  useListEntrance(tbodyRef, '[data-row-entry]', rows.length)
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
  const jitPeriodState = useAppStore((s) => s.jitPeriodState)
  const beginJITPeriodUpdate = useAppStore((s) => s.beginJITPeriodUpdate)
  const completeJITPeriodUpdate = useAppStore((s) => s.completeJITPeriodUpdate)
  const [pendingPOPriceAction, setPendingPOPriceAction] = useState<PendingPOPriceAction | null>(null)

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
      setPendingPOPriceAction(null)
      clearSelection()
      clearReceivedAt()
    }
  }, [rows.length, clearResolvedChoice, clearSelection, clearReceivedAt])

  function handleCopy(key: string, value: string) {
    navigator.clipboard.writeText(value).catch(() => {})
    setCopiedKey(key)
    setTimeout(() => setCopiedKey((cur) => (cur === key ? null : cur)), 1000)
  }

  async function commitApplyPrice(rowIndex: number, detail: PriceMismatchDetail, useInvoicePrice: boolean) {
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

  async function handleApplyPrice(rowIndex: number, detail: PriceMismatchDetail, useInvoicePrice: boolean) {
    if (useInvoicePrice && belowSystemPriceDetails([detail]).length > 0) {
      setPendingPOPriceAction({ kind: 'single', rowIndex, detail })
      return
    }
    await commitApplyPrice(rowIndex, detail, useInvoicePrice)
  }

  // Applies one price basis to every mismatch belonging to the PO, across
  // all BigC store pages. Sequential writes protect the shared workbook.
  async function handleApplyAllForPO(po: string, useInvoicePrice: boolean) {
    const items = mismatchesForPO(rows, po)
    const warningDetails = useInvoicePrice
      ? belowSystemPriceDetails(items.map(({ detail }) => detail))
      : []
    if (warningDetails.length > 0) {
      setPendingPOPriceAction({ kind: 'all', po, items, warningDetails })
      return
    }
    for (const item of items) {
      await commitApplyPrice(item.rowIndex, item.detail, useInvoicePrice)
    }
  }

  async function confirmPendingPOPrice() {
    const pending = pendingPOPriceAction
    if (!pending) return
    setPendingPOPriceAction(null)
    if (pending.kind === 'single') {
      await commitApplyPrice(pending.rowIndex, pending.detail, true)
      return
    }
    for (const item of pending.items) {
      await commitApplyPrice(item.rowIndex, item.detail, true)
    }
  }

  const pendingWarningDetails = pendingPOPriceAction
    ? pendingPOPriceAction.kind === 'single'
      ? [pendingPOPriceAction.detail]
      : pendingPOPriceAction.warningDetails
    : []

  // Every group key present in this batch (po for most vendors, sourceId
  // for JIT - see groupKeyFor), in first-seen order - the "chọn tất cả"
  // checkbox and the toolbar's selected-count both count against this
  // set, never against rows.length (one key can span several rows: BigC
  // store rows sharing a po, or JIT pages sharing a sourceId).
  const uniqueGroupKeys: string[] = []
  for (const row of rows) {
    const key = groupKeyFor(row)
    if (key && !uniqueGroupKeys.includes(key)) uniqueGroupKeys.push(key)
  }

  function rowsForGroupKey(key: string): number[] {
    return rows.reduce<number[]>((acc, row, idx) => {
      if (groupKeyFor(row) === key) acc.push(idx)
      return acc
    }, [])
  }

  function openContentModalForRow(rowIndex: number) {
    const row = rows[rowIndex]
    setContentModalGroups([{ po: row.po, rows: [row] }])
  }

  function openContentModalForSelection() {
    const groups: POContentGroup[] = [...selectedPOs].map((key) => {
      const groupRows = rowsForGroupKey(key).map((idx) => rows[idx])
      const isJIT = groupRows[0]?.system === 'JIT-CHOICE'
      return {
        // Nhãn hiển thị của nhóm: JIT không có 1 po đại diện (mỗi trang
        // 1 po khác nhau) nên dùng tên file PDF thay thế - vẫn là po
        // thật cho mọi vendor khác.
        po: isJIT ? (groupRows[0]?.fileName ?? key) : key,
        rows: groupRows,
        period: isJIT ? (jitPeriodState.periodBySource[key] ?? groupRows[0]?.jitPeriod) : undefined,
      }
    })
    setContentModalGroups(groups)
  }

  const selectedCount = selectedPOs.size
  const jitFiles = groupJITFiles(rows)

  async function handleJITPeriodChange(sourceId: string, period: string) {
    const group = jitFiles.find((item) => item.sourceId === sourceId)
    if (!group) return
    const request = beginJITPeriodUpdate(sourceId)
    if (!request) return
    try {
      await UpdateJITPeriod(group.excelRows, group.orderDate, group.warehouse, period)
      if (completeJITPeriodUpdate(request, period)) {
        appendLog(`✅ JIT ${group.fileName}: đã đổi toàn bộ ${group.orderCount} đơn sang buổi ${period}`)
      }
    } catch (err) {
      if (completeJITPeriodUpdate(request)) {
        appendLog(`❌ Không đổi được buổi JIT cho ${group.fileName}: ${String(err)}`)
      }
    }
  }

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3.5">
      <SectionHeader index="04" title="Kết quả xử lý chi tiết" />
      {jitFiles.length > 0 && (
        <div className="mb-2 space-y-1.5 rounded-lg border border-accent/25 bg-accent/[0.05] p-2.5">
          <div className="font-sans text-[10px] font-bold uppercase tracking-wider text-muted">Buổi giao đơn JIT theo file PDF</div>
          {jitFiles.map((group) => {
            const value = jitPeriodState.periodBySource[group.sourceId] ?? group.period
            return (
              <div key={group.sourceId} className="flex items-center gap-3 rounded-md bg-bg/70 px-3 py-2">
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink" title={group.fileName}>{group.fileName}</span>
                <span className="whitespace-nowrap font-sans text-[11px] text-muted">Áp dụng {group.orderCount} đơn</span>
                <JITPeriodMenu
                  value={value}
                  disabled={isJITPeriodMenuDisabled(isProcessing, jitPeriodState)}
                  onChange={(period) => handleJITPeriodChange(group.sourceId, period)}
                  ariaLabel={`Buổi giao cho ${group.fileName}`}
                />
              </div>
            )
          })}
        </div>
      )}
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
          <thead>
            <tr>
              <th className="sticky top-0 z-10 w-9 border-b border-border bg-bg px-3 py-2 text-center">
                <input
                  type="checkbox"
                  className="cursor-pointer accent-accent"
                  checked={uniqueGroupKeys.length > 0 && selectedCount === uniqueGroupKeys.length}
                  ref={(el) => {
                    if (el) el.indeterminate = selectedCount > 0 && selectedCount < uniqueGroupKeys.length
                  }}
                  onChange={(e) => toggleAllPOs(uniqueGroupKeys, e.target.checked)}
                  title="Chọn tất cả"
                />
              </th>
              <th className="sticky top-0 z-10 w-12 border-b border-border bg-bg px-3 py-2 text-center font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
                STT
              </th>
              {columns.map((c) => (
                <th
                  key={c.key}
                  className="sticky top-0 z-10 border-b border-border bg-bg px-3 py-2 text-left font-sans text-[10px] font-bold uppercase tracking-wider text-muted"
                >
                  {c.label}
                </th>
              ))}
              <th className="sticky top-0 z-10 border-b border-border bg-bg px-3 py-2 text-left font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
                Nội dung
              </th>
            </tr>
          </thead>
          <tbody ref={tbodyRef}>
            {rows.length === 0 && (
              <tr>
                <td colSpan={columns.length + 3} className="p-6 text-center font-sans text-muted">
                  Chưa có kết quả nào.
                </td>
              </tr>
            )}
            {rows.map((row, i) => {
              const meta = statusMeta(row)
              const price = priceMeta(row)
              const rowGroupKey = groupKeyFor(row)
              const isPOSelected = rowGroupKey !== '' && selectedPOs.has(rowGroupKey)
              const poMismatches = mismatchesForPO(rows, row.po)
              return (
                <Fragment key={i}>
                  <tr
                    data-row-entry
                    className={`transition-colors hover:bg-white/[0.03] ${isPOSelected ? 'bg-accent/[0.06]' : ''}`}
                  >
                    <td className="border-b border-border px-3 py-2 text-center" onClick={(e) => e.stopPropagation()}>
                      {rowGroupKey !== '' && (
                        <input
                          type="checkbox"
                          className="cursor-pointer accent-accent"
                          checked={isPOSelected}
                          onChange={() => togglePOSelection(rowGroupKey)}
                        />
                      )}
                    </td>
                    <td className="border-b border-border px-3 py-2 text-center font-semibold text-muted">{i + 1}</td>
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
                              className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-0.5 font-sans font-semibold ${meta.classes}`}
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
                                className={`inline-flex cursor-pointer items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-0.5 font-sans font-semibold ${price.classes}`}
                              >
                                <FaTriangleExclamation size={11} />
                                {price.label}
                                {expandedRow === i ? <FaChevronDown size={9} /> : <FaChevronRight size={9} />}
                              </button>
                            ) : (
                              <span
                                className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-0.5 font-sans font-semibold ${price.classes}`}
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
                      <td colSpan={columns.length + 3} className="p-0">
                        {poMismatches.length > 1 && (
                          <div className="flex items-center gap-2 border-b border-border bg-panel/60 px-3 py-2">
                            <span className="font-sans text-[10px] font-semibold uppercase tracking-wide text-muted">
                              Áp dụng cho toàn bộ PO ({poMismatches.length} mã)
                            </span>
                            <button
                              type="button"
                              disabled={isProcessing}
                              onClick={() => handleApplyAllForPO(row.po, true)}
                              className="inline-flex items-center gap-1.5 rounded border border-border px-2.5 py-1 font-sans text-[10px] font-semibold text-ink transition-colors hover:border-ink disabled:cursor-not-allowed disabled:opacity-40"
                            >
                              <FaFilePdf size={9} /> Dùng giá PO cho toàn bộ {poMismatches.length} mã
                            </button>
                            <button
                              type="button"
                              disabled={isProcessing}
                              onClick={() => handleApplyAllForPO(row.po, false)}
                              className="inline-flex items-center gap-1.5 rounded border border-accent/50 px-2.5 py-1 font-sans text-[10px] font-semibold text-accent transition-colors hover:bg-accent/10 disabled:cursor-not-allowed disabled:opacity-40"
                            >
                              <FaMagnifyingGlassDollar size={9} /> Dùng giá hệ thống cho toàn bộ {poMismatches.length} mã
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
        // Tra theo resultKey (định danh riêng của từng dòng), KHÔNG dùng
        // lại rowsForGroupKey(g.po) - với nhóm JIT, g.po giờ là TÊN FILE
        // hiển thị (xem openContentModalForSelection), không còn là 1 po
        // thật để so khớp row.po nữa.
        const indexByResultKey = new Map(rows.map((r, idx) => [r.resultKey, idx]))
        const firstRowIndex = indexByResultKey.get(contentModalGroups[0].rows[0]?.resultKey ?? '')
        const combinedPriceBasis: Record<number, PriceBasis> = {}
        for (const g of contentModalGroups) {
          const indices = g.rows
            .map((r) => indexByResultKey.get(r.resultKey))
            .filter((idx): idx is number => idx !== undefined)
          Object.assign(combinedPriceBasis, buildPriceBasisForPO(rows, indices, resolvedChoice))
        }
        return (
          <OrderContentModal
            groups={contentModalGroups}
            processedAt={(firstRowIndex !== undefined ? receivedAt[firstRowIndex] : undefined) ?? ''}
            priceBasisBySku={combinedPriceBasis}
            onClose={() => setContentModalGroups(null)}
          />
        )
      })()}
      {pendingPOPriceAction && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="po-price-warning-title"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setPendingPOPriceAction(null)
          }}
        >
          <div className="flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-warning/50 bg-panel shadow-2xl">
            <div className="flex items-start gap-3 border-b border-border px-5 py-4">
              <FaTriangleExclamation className="mt-0.5 shrink-0 text-warning" size={20} />
              <div className="min-w-0 flex-1">
                <h2 id="po-price-warning-title" className="font-sans text-base font-bold text-warning">
                  Xác nhận áp dụng giá PO thấp hơn giá hệ thống
                </h2>
                <p className="mt-1 font-sans text-xs leading-5 text-muted">
                  Giá bán ra của {pendingWarningDetails.length} mã dưới đây thấp hơn giá hệ thống. Vui lòng kiểm tra và xác nhận trước khi áp dụng.
                </p>
              </div>
              <button
                type="button"
                aria-label="Đóng cảnh báo"
                onClick={() => setPendingPOPriceAction(null)}
                className="rounded p-1 text-muted transition-colors hover:bg-white/5 hover:text-ink"
              >
                <FaXmark size={16} />
              </button>
            </div>
            <div className="overflow-auto px-5 py-3">
              <table className="w-full border-collapse font-mono text-xs">
                <thead>
                  <tr className="border-b border-border text-left font-sans text-[10px] uppercase tracking-wide text-muted">
                    <th className="px-2 py-2">Mã</th>
                    <th className="px-2 py-2">Tên sản phẩm</th>
                    <th className="px-2 py-2 text-right">Giá PO</th>
                    <th className="px-2 py-2 text-right">Giá hệ thống</th>
                  </tr>
                </thead>
                <tbody>
                  {pendingWarningDetails.map((detail) => (
                    <tr key={`${detail.excelRow}-${detail.sku}`} className="border-b border-border last:border-0">
                      <td className="px-2 py-2 font-semibold text-ink">{detail.sku}</td>
                      <td className="px-2 py-2 text-muted">{detail.productName}</td>
                      <td className="px-2 py-2 text-right font-semibold text-danger">{detail.invoicePrice.toLocaleString('vi-VN')}đ</td>
                      <td className="px-2 py-2 text-right text-ink">{detail.systemPrice.toLocaleString('vi-VN')}đ</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="flex justify-end gap-2 border-t border-border px-5 py-4">
              <button
                type="button"
                onClick={() => setPendingPOPriceAction(null)}
                className="rounded-lg border border-border px-4 py-2 font-sans text-xs font-semibold text-muted transition-colors hover:border-ink hover:text-ink"
              >
                Hủy
              </button>
              <button
                type="button"
                onClick={confirmPendingPOPrice}
                className="rounded-lg bg-warning px-4 py-2 font-sans text-xs font-bold text-[#17120a] transition-opacity hover:opacity-90"
              >
                Xác nhận áp dụng giá PO
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  )
}
