// GO/frontend/src/components/KeyValueEditor.tsx
import { useState } from 'react'
import { FaTrash, FaPlus } from 'react-icons/fa6'

interface KeyValueEditorProps {
  entries: Record<string, string>
  onChange: (entries: Record<string, string>) => void
  keyLabel: string
  valueLabel: string
  valueType: 'text' | 'number' | 'toggle'
}

interface Row {
  id: number
  key: string
  value: string
}

let nextRowId = 0

function toRows(entries: Record<string, string>): Row[] {
  return Object.entries(entries).map(([key, value]) => ({ id: nextRowId++, key, value }))
}

// KeyValueEditor là bảng key-value dùng chung cho cả 3 tab của
// SettingsModal (GID/Zalo/Nhắc nhở) — mỗi tab chỉ khác nhau ở nhãn cột
// và valueType. Dòng có khóa hoặc giá trị rỗng bị BỎ QUA khi gọi
// onChange (không tính vào entries, không báo lỗi) — cho phép người
// dùng gõ dở dang mà không bị validate ngay lập tức.
export function KeyValueEditor({ entries, onChange, keyLabel, valueLabel, valueType }: KeyValueEditorProps) {
  const [rows, setRows] = useState<Row[]>(() => toRows(entries))

  function commit(next: Row[]) {
    setRows(next)
    const result: Record<string, string> = {}
    for (const row of next) {
      if (row.key.trim() === '' || row.value.trim() === '') continue
      result[row.key] = row.value
    }
    onChange(result)
  }

  function updateKey(id: number, key: string) {
    commit(rows.map((r) => (r.id === id ? { ...r, key } : r)))
  }

  function updateValue(id: number, value: string) {
    commit(rows.map((r) => (r.id === id ? { ...r, value } : r)))
  }

  function removeRow(id: number) {
    commit(rows.filter((r) => r.id !== id))
  }

  function addRow() {
    setRows([...rows, { id: nextRowId++, key: '', value: '' }])
  }

  const keyCounts = new Map<string, number>()
  for (const row of rows) {
    if (row.key.trim() === '') continue
    keyCounts.set(row.key, (keyCounts.get(row.key) ?? 0) + 1)
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="grid grid-cols-[1fr_1fr_auto] gap-2 px-1 font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
        <span>{keyLabel}</span>
        <span>{valueLabel}</span>
        <span></span>
      </div>
      {rows.map((row) => {
        const isDuplicate = row.key.trim() !== '' && (keyCounts.get(row.key) ?? 0) > 1
        return (
          <div key={row.id} className="grid grid-cols-[1fr_1fr_auto] items-center gap-2">
            <input
              value={row.key}
              onChange={(e) => updateKey(row.id, e.target.value)}
              className={`rounded border bg-bg px-2 py-1.5 font-mono text-xs text-ink outline-none ${
                isDuplicate ? 'border-danger' : 'border-border focus:border-accent'
              }`}
            />
            {valueType === 'toggle' ? (
              <input
                type="checkbox"
                checked={row.value === '1'}
                onChange={(e) => updateValue(row.id, e.target.checked ? '1' : '')}
                className="h-4 w-4 accent-accent"
              />
            ) : (
              <input
                value={row.value}
                onChange={(e) => {
                  if (valueType === 'number' && e.target.value !== '' && !/^\d*$/.test(e.target.value)) return
                  updateValue(row.id, e.target.value)
                }}
                className="rounded border border-border bg-bg px-2 py-1.5 font-mono text-xs text-ink outline-none focus:border-accent"
              />
            )}
            <button
              type="button"
              onClick={() => removeRow(row.id)}
              className="rounded p-1.5 text-muted transition-colors hover:text-danger"
            >
              <FaTrash size={11} />
            </button>
          </div>
        )
      })}
      <button
        type="button"
        onClick={addRow}
        className="mt-1 inline-flex items-center gap-1.5 self-start rounded border border-border px-2.5 py-1 font-sans text-[11px] font-semibold text-muted transition-colors hover:border-accent hover:text-accent"
      >
        <FaPlus size={9} /> Thêm dòng
      </button>
    </div>
  )
}
