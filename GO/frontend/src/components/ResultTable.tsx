import { FaCircleCheck, FaCircleXmark, FaTriangleExclamation } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import type { OrderRow } from '../types'
import type { ReactNode } from 'react'

const columns: { key: keyof OrderRow; label: string }[] = [
  { key: 'fileName', label: 'Tên file' },
  { key: 'page', label: 'Trang' },
  { key: 'system', label: 'Hệ thống' },
  { key: 'maKhachHang', label: 'Mã khách hàng' },
  { key: 'po', label: 'PO' },
  { key: 'donGia', label: 'Đơn giá' },
  { key: 'status', label: 'Trạng thái' },
]

function statusMeta(row: OrderRow): { icon: ReactNode; classes: string; label: string } {
  const { status, statusKind } = row
  switch (statusKind) {
    case 'failed':
      return { icon: <FaCircleXmark />, classes: 'bg-danger/20 text-danger', label: status.replace('❌', '').trim() }
    case 'warning':
      return {
        icon: <FaTriangleExclamation />,
        classes: 'bg-warning/20 text-warning',
        label: status.replace('⚠️', '').trim(),
      }
    case 'done':
      return { icon: <FaCircleCheck />, classes: 'bg-success/20 text-success', label: status.replace('✅', '').trim() }
    default:
      return { icon: null, classes: 'bg-border text-muted', label: status }
  }
}

export function ResultTable() {
  const rows = useAppStore((s) => s.rows)

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3">
      <h2 className="mb-2 text-sm font-semibold text-muted">4. Kết quả xử lý chi tiết</h2>
      <div className="selectable flex-1 overflow-auto rounded-lg border border-border">
        <table className="w-full border-collapse font-mono text-xs">
          <thead className="sticky top-0 bg-bg">
            <tr>
              {columns.map((c) => (
                <th key={c.key} className="border-b border-border px-2 py-1.5 text-left text-muted">
                  {c.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => {
              const meta = statusMeta(row)
              return (
                <tr key={i} className="odd:bg-bg/40">
                  {columns.map((c) => (
                    <td key={c.key} className="border-b border-border px-2 py-1.5 text-ink">
                      {c.key === 'status' ? (
                        <span
                          className={`inline-flex items-center gap-1.5 rounded px-2 py-0.5 font-semibold ${meta.classes}`}
                        >
                          {meta.icon}
                          {meta.label}
                        </span>
                      ) : (
                        row[c.key]
                      )}
                    </td>
                  ))}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </section>
  )
}
