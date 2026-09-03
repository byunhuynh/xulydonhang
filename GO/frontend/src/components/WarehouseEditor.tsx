import { useEffect, useState } from 'react'
import { WarehouseOptions } from '../../wailsjs/go/main/App'

interface WarehouseEditorProps {
  entries: Record<string, string>
  onChange: (entries: Record<string, string>) => void
}

interface WarehouseRow {
  key: string
  label: string
  code: string
  fallback: string
}

// WarehouseEditor là bảng "mỗi nhánh vendor ghi mã kho nào vào cột V".
// Giống MisaRoutingEditor ở chỗ KHÔNG cho gõ khoá — khoá do Go sinh
// (warehouse.Branches), gõ sai một ký tự là nhánh đó không bao giờ khớp
// mà chẳng có gì báo. Khác ở chỗ giá trị là mã kho tự do nên là ô nhập
// chữ, không phải chọn một trong hai như nhánh kế toán.
export function WarehouseEditor({ entries, onChange }: WarehouseEditorProps) {
  const [rows, setRows] = useState<WarehouseRow[] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    WarehouseOptions()
      .then((options) =>
        setRows(
          options.map((o) => ({
            key: o.key,
            label: o.label,
            // Giá trị đang sửa dở trong popup thắng giá trị Go đọc từ
            // đĩa: người dùng có thể đã sửa vài dòng rồi mới chuyển tab,
            // chưa bấm Lưu.
            code: entries[o.key] ?? o.code ?? '',
            fallback: o.default ?? '',
          })),
        ),
      )
      .catch((err) => setError(String(err)))
    // Chỉ nạp một lần khi mở tab; những lần đổi sau đã nằm trong `rows`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (error) {
    return <p className="font-sans text-xs text-danger">Không tải được bảng kho: {error}</p>
  }
  if (!rows) {
    return <p className="font-sans text-xs text-muted">Đang tải…</p>
  }

  function setCode(key: string, code: string) {
    const next = rows!.map((r) => (r.key === key ? { ...r, code } : r))
    setRows(next)
    const result: Record<string, string> = {}
    for (const r of next) {
      // Ô để trống nghĩa là "dùng lại mã mặc định", nên không lưu gì cả —
      // Go sẽ gieo lại mã xuất xưởng ở lần khởi động sau.
      if (r.code.trim()) result[r.key] = r.code.trim()
    }
    onChange(result)
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="px-1 font-sans text-[11px] leading-relaxed text-muted">
        Mã kho ghi vào cột V của sổ đặt hàng, theo từng nhánh của từng hệ thống. Để trống một ô
        là quay về mã mặc định (hiện mờ trong ô đó).
      </p>
      <div className="grid grid-cols-[1fr_180px] gap-2 px-1 font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
        <span>Nhánh</span>
        <span>Mã kho</span>
      </div>
      {rows.map((r) => (
        <div key={r.key} className="grid grid-cols-[1fr_180px] items-center gap-2">
          <span className="font-mono text-xs text-ink">{r.label}</span>
          <input
            type="text"
            value={r.code}
            placeholder={r.fallback}
            onChange={(e) => setCode(r.key, e.target.value)}
            aria-label={`Mã kho cho ${r.label}`}
            className="rounded-lg border border-border bg-panel px-2 py-1 font-mono text-xs text-ink outline-none focus:border-accent"
          />
        </div>
      ))}
    </div>
  )
}
