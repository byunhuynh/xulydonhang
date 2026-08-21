import { useState } from 'react'
import { FaCircle, FaCheck, FaCircleCheck, FaTriangleExclamation } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import type { OrderRow } from '../types'
import { SectionHeader } from './SectionHeader'

const columns: { key: keyof OrderRow; label: string }[] = [
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
  const [copiedKey, setCopiedKey] = useState<string | null>(null)

  function handleCopy(key: string, value: string) {
    navigator.clipboard.writeText(value).catch(() => {})
    setCopiedKey(key)
    setTimeout(() => setCopiedKey((cur) => (cur === key ? null : cur)), 1000)
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
                <tr key={i} className="transition-colors hover:bg-white/[0.03]">
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
                          <span
                            className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 font-sans font-semibold ${price.classes}`}
                          >
                            {price.icon === 'ok' && <FaCircleCheck size={11} />}
                            {price.icon === 'warn' && <FaTriangleExclamation size={11} />}
                            {price.label}
                          </span>
                        ) : c.key === 'donGia' ? (
                          <span className="font-semibold text-accent">{formatMoney(row[c.key])}</span>
                        ) : (
                          row[c.key]
                        )}
                      </td>
                    )
                  })}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </section>
  )
}
