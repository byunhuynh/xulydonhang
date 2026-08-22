import { Fragment, useEffect, useState } from 'react'
import {
  FaCircle,
  FaCheck,
  FaCircleCheck,
  FaTriangleExclamation,
  FaChevronDown,
  FaChevronRight,
} from 'react-icons/fa6'
import { FaUpRightFromSquare } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import type { OrderRow, PriceMismatchDetail } from '../types'
import { SectionHeader } from './SectionHeader'
import { ConfirmPrice } from '../../wailsjs/go/main/App'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const columns: { key: Exclude<keyof OrderRow, 'priceMismatchDetails'>; label: string }[] = [
  { key: 'fileName', label: 'Tên file' },
  { key: 'page', label: 'Trang' },
  { key: 'system', label: 'Hệ thống' },
  { key: 'maKhachHang', label: 'Mã khách hàng' },
  { key: 'po', label: 'PO' },
  { key: 'donGia', label: 'Đơn giá' },
  { key: 'priceMismatchCount', label: 'Đối soát giá' },
  { key: 'status', label: 'Trạng thái' },
  { key: 'driveUrl', label: 'File Drive' },
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
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  const [expandedRow, setExpandedRow] = useState<number | null>(null)
  const [resolvedChoice, setResolvedChoice] = useState<Record<string, 'po' | 'system'>>({})
  const appendLog = useAppStore((s) => s.appendLog)

  // A new batch calls resetRows() (see ControlPanel), which empties this
  // array before new rows stream in — that's the right moment to clear
  // any expand/resolved-choice state from the PREVIOUS batch's results,
  // since row index i in the new batch has no relationship to whatever
  // order used to be at that same index.
  useEffect(() => {
    if (rows.length === 0) {
      setExpandedRow(null)
      setResolvedChoice({})
    }
  }, [rows.length])

  function handleCopy(key: string, value: string) {
    navigator.clipboard.writeText(value).catch(() => {})
    setCopiedKey(key)
    setTimeout(() => setCopiedKey((cur) => (cur === key ? null : cur)), 1000)
  }

  async function handleApplyPrice(rowIndex: number, detail: PriceMismatchDetail, useInvoicePrice: boolean) {
    const price = useInvoicePrice ? detail.invoicePrice : detail.systemPrice
    const key = `${rowIndex}-${detail.excelRow}`
    try {
      await ConfirmPrice(detail.excelRow, price)
      setResolvedChoice((prev) => ({ ...prev, [key]: useInvoicePrice ? 'po' : 'system' }))
    } catch (err) {
      appendLog(`❌ Lỗi áp dụng giá cho ${detail.sku}: ${String(err)}`)
    }
  }

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3.5">
      <SectionHeader index="04" title="Kết quả xử lý chi tiết" />
      <div className="selectable flex-1 overflow-auto rounded-lg border border-border">
        <table className="w-full border-collapse font-mono text-xs">
          <thead className="sticky top-0 bg-bg">
            <tr>
              {columns.map((c) => (
                <th
                  key={c.key}
                  className="border-b border-border px-3 py-2 text-left font-sans text-[10px] font-bold uppercase tracking-wider text-muted"
                >
                  {c.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr>
                <td colSpan={columns.length} className="p-6 text-center font-sans text-muted">
                  Chưa có kết quả nào.
                </td>
              </tr>
            )}
            {rows.map((row, i) => {
              const meta = statusMeta(row)
              const price = priceMeta(row)
              return (
                <Fragment key={i}>
                  <tr className="transition-colors hover:bg-white/[0.03]">
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
                            <span className="font-semibold text-accent">{formatMoney(row[c.key])}</span>
                          ) : c.key === 'driveUrl' ? (
                            row.driveUrl ? (
                              <button
                                type="button"
                                onClick={(e) => {
                                  e.stopPropagation()
                                  BrowserOpenURL(row.driveUrl)
                                }}
                                className="inline-flex items-center gap-1.5 rounded-full border border-border px-2.5 py-0.5 font-sans font-semibold text-accent transition-colors hover:border-accent"
                              >
                                <FaUpRightFromSquare size={9} /> Mở file
                              </button>
                            ) : (
                              <span className="text-muted">—</span>
                            )
                          ) : (
                            row[c.key]
                          )}
                        </td>
                      )
                    })}
                  </tr>
                  {expandedRow === i && row.priceMismatchDetails.length > 0 && (
                    <tr key={`${i}-detail`} className="bg-bg/60">
                      <td colSpan={columns.length} className="p-0">
                        <table className="w-full border-collapse font-mono text-[11px]">
                          <thead>
                            <tr className="border-b border-border">
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Mã</th>
                              <th className="px-3 py-1.5 text-left font-sans font-semibold text-muted">Tên SP</th>
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
    </section>
  )
}
