import { useEffect, useState } from 'react'
import { MisaRouteOptions } from '../../wailsjs/go/main/App'
import { MISA_BRANCH_OPTIONS } from '../lib/misaBranch'
import { SegmentedControl } from './SegmentedControl'

interface MisaRoutingEditorProps {
  entries: Record<string, string>
  onChange: (entries: Record<string, string>) => void
}

interface RouteRow {
  key: string
  label: string
  branch: string
}

// MisaRoutingEditor là bảng "đơn của hệ thống này vào sổ của pháp nhân
// nào". Khác KeyValueEditor ở chỗ KHÔNG gõ tay khoá: khoá do Go sinh ra
// (misapush.RouteKey) nên gõ sai một ký tự là dòng đó không bao giờ khớp
// đơn nào, mà không có gì báo.
//
// Danh sách là hợp của bảng gieo mặc định và mọi khoá đã lưu — Go lo phần
// gộp đó trong MisaRouteOptions, ở đây chỉ hiển thị.
export function MisaRoutingEditor({ entries, onChange }: MisaRoutingEditorProps) {
  const [rows, setRows] = useState<RouteRow[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    MisaRouteOptions()
      .then((options) =>
        setRows(
          options.map((o) => ({
            key: o.key,
            label: o.label,
            // Giá trị đang sửa dở trong popup thắng giá trị Go đọc từ
            // đĩa: người dùng có thể đã bấm đổi vài dòng rồi mới chuyển
            // tab, chưa bấm Lưu.
            branch: entries[o.key] ?? o.branch ?? '',
          })),
        ),
      )
      .catch((err) => setError(String(err)))
    // Chỉ nạp một lần khi mở tab; những lần đổi sau đã nằm trong `rows`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (error) {
    return <p className="font-sans text-xs text-danger">Không tải được bảng định tuyến: {error}</p>
  }
  if (!rows) {
    return <p className="font-sans text-xs text-muted">Đang tải…</p>
  }

  function setBranch(key: string, branch: string) {
    const next = rows!.map((r) => (r.key === key ? { ...r, branch } : r))
    setRows(next)
    const result: Record<string, string> = {}
    for (const r of next) {
      if (r.branch) result[r.key] = r.branch
    }
    onChange(result)
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="px-1 font-sans text-[11px] leading-relaxed text-muted">
        Đơn của mỗi hệ thống sẽ vào sổ kế toán nào. Đổi bất cứ lúc nào — bấm Lưu là áp dụng
        cho lượt đẩy kế tiếp.
      </p>
      <div className="grid grid-cols-[1fr_auto] gap-2 px-1 font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
        <span>Hệ thống</span>
        <span>Nhánh</span>
      </div>
      {rows.map((r) => (
        <div key={r.key} className="grid grid-cols-[1fr_auto] items-center gap-2">
          <span className="font-mono text-xs text-ink">{r.label}</span>
          <SegmentedControl
            options={MISA_BRANCH_OPTIONS}
            value={r.branch}
            onChange={(branch) => setBranch(r.key, branch)}
            ariaLabel={`Nhánh kế toán cho ${r.label}`}
          />
        </div>
      ))}
    </div>
  )
}
