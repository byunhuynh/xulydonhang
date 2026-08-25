import { useMemo, useRef, useState } from 'react'
import { useModalEntrance } from '../lib/useModalEntrance'
import {
  MAX_RANGE_DAYS,
  addDays,
  formatRangeLabel,
  isSelectableDay,
  maxSelectableDate,
  normalizeRange,
  parseISODate,
  presetRange,
  toISODate,
  validateRange,
  type TMDTDateRange,
} from '../lib/tmdtDateRange'

interface Props {
  fileNames: string[]
  onConfirm: (range: TMDTDateRange) => void
  onCancel: () => void
}

const WEEKDAYS = ['T2', 'T3', 'T4', 'T5', 'T6', 'T7', 'CN']

// Lịch tự vẽ chứ không dùng <input type="date"> của WebView2: input gốc
// không cho vô hiệu hoá từng ngày theo ràng buộc 7 ngày, và giao diện của
// nó lạc hẳn khỏi tông màu app.
export function TMDTDateRangeModal({ fileNames, onConfirm, onCancel }: Props) {
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef)

  // "Hôm nay" chốt một lần lúc mở modal: nếu đọc lại mỗi lần render, một
  // modal mở qua nửa đêm sẽ tự đổi ràng buộc dưới tay người dùng.
  const today = useMemo(() => parseISODate(toISODate(new Date())), [])
  const [month, setMonth] = useState(() => {
    const max = maxSelectableDate(today)
    return new Date(Date.UTC(max.getUTCFullYear(), max.getUTCMonth(), 1))
  })
  // anchor khác null CHỈ khi đã chọn ngày đầu nhưng chưa chọn ngày thứ hai
  // — tức đang "giữa" một lượt chọn khoảng. Đây là lúc duy nhất giới hạn
  // 7 ngày phải hiện trên lịch (isSelectableDay nhận anchor để vô hiệu hoá
  // những ngày cách anchor quá xa). Không dùng chung 1 cờ với range: nếu
  // gộp lại, sau khi range đã có giá trị sẽ không còn cách nào phân biệt
  // "đang chọn ngày thứ hai" với "đã chọn xong" — bug cũ chính là vậy.
  const [anchor, setAnchor] = useState<string | null>(null)
  const [range, setRange] = useState<TMDTDateRange | null>(null)

  const error = range ? validateRange(range, today) : 'Chưa chọn khoảng thời gian.'

  // Ô đầu tiên của lưới là thứ Hai của tuần chứa ngày 1.
  const gridStart = useMemo(() => {
    const weekdayMon0 = (month.getUTCDay() + 6) % 7
    return addDays(month, -weekdayMon0)
  }, [month])

  const days = useMemo(
    () => Array.from({ length: 42 }, (_, i) => addDays(gridStart, i)),
    [gridStart],
  )

  function pick(day: Date) {
    const iso = toISODate(day)
    if (!anchor) {
      // Ngày đầu của một lượt chọn mới: giữ làm anchor, hiện tạm như một
      // khoảng 1 ngày trong lúc chờ người dùng bấm ngày thứ hai.
      setAnchor(iso)
      setRange({ from: iso, to: iso })
      return
    }
    // Ngày thứ hai: đóng khoảng lại và xoá anchor — lượt chọn tiếp theo
    // (nếu có) sẽ lại bắt đầu từ nhánh "ngày đầu" ở trên.
    setRange(normalizeRange(anchor, iso))
    setAnchor(null)
  }

  function applyPreset(preset: 'yesterday' | '3days' | '7days') {
    const r = presetRange(preset, today)
    // Preset cho ra khoảng đã hoàn tất ngay, không phải đang giữa lượt
    // chọn — anchor phải về null để lịch không bị khoá theo giới hạn 7
    // ngày tính từ r.from.
    setAnchor(null)
    setRange(r)
    setMonth(new Date(Date.UTC(parseISODate(r.to).getUTCFullYear(), parseISODate(r.to).getUTCMonth(), 1)))
  }

  function inRange(day: Date): boolean {
    if (!range) return false
    const iso = toISODate(day)
    return iso >= range.from && iso <= range.to
  }

  return (
    <div
      ref={backdropRef}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-modal="true"
      aria-label="Chọn khoảng thời gian lấy đơn TMĐT"
    >
      <div ref={cardRef} className="w-full max-w-md rounded-lg border border-border bg-panel p-5 shadow-xl">
        <h2 className="font-sans text-base font-semibold text-ink">Lấy đơn TMĐT theo khoảng ngày</h2>
        <p className="mt-1 font-sans text-xs text-muted">
          {fileNames.join(', ')} — chỉ lấy đến hết ngày hôm qua, tối đa {MAX_RANGE_DAYS} ngày.
        </p>

        <div className="mt-3 flex gap-1.5">
          {([
            ['yesterday', 'Hôm qua'],
            ['3days', '3 ngày'],
            ['7days', '7 ngày'],
          ] as const).map(([key, label]) => (
            <button
              key={key}
              type="button"
              onClick={() => applyPreset(key)}
              className="rounded-md border border-border px-2.5 py-1 font-sans text-xs font-medium text-muted hover:bg-white/[0.04] hover:text-ink"
            >
              {label}
            </button>
          ))}
        </div>

        <div className="mt-4 flex items-center justify-between">
          <button
            type="button"
            aria-label="Tháng trước"
            onClick={() => setMonth(new Date(Date.UTC(month.getUTCFullYear(), month.getUTCMonth() - 1, 1)))}
            className="rounded-md border border-border px-2 py-1 font-sans text-xs text-muted hover:text-ink"
          >
            ‹
          </button>
          <span className="font-sans text-sm font-semibold text-ink">
            Tháng {month.getUTCMonth() + 1}/{month.getUTCFullYear()}
          </span>
          <button
            type="button"
            aria-label="Tháng sau"
            onClick={() => setMonth(new Date(Date.UTC(month.getUTCFullYear(), month.getUTCMonth() + 1, 1)))}
            className="rounded-md border border-border px-2 py-1 font-sans text-xs text-muted hover:text-ink"
          >
            ›
          </button>
        </div>

        <div className="mt-2 grid grid-cols-7 gap-1">
          {WEEKDAYS.map((w) => (
            <div key={w} className="py-1 text-center font-sans text-[11px] font-medium text-muted">
              {w}
            </div>
          ))}
          {days.map((day) => {
            const iso = toISODate(day)
            const otherMonth = day.getUTCMonth() !== month.getUTCMonth()
            const selectable = isSelectableDay(day, today, anchor)
            const selected = inRange(day)
            return (
              <button
                key={iso}
                type="button"
                disabled={!selectable}
                aria-pressed={selected}
                onClick={() => pick(day)}
                className={`rounded-md py-1.5 font-mono text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-30 ${
                  selected
                    ? 'bg-accent/[0.18] font-semibold text-accent'
                    : otherMonth
                      ? 'text-muted/60 hover:bg-white/[0.04]'
                      : 'text-ink hover:bg-white/[0.06]'
                }`}
              >
                {day.getUTCDate()}
              </button>
            )
          })}
        </div>

        <div className="mt-4 min-h-[1.25rem] font-sans text-xs">
          {error ? (
            <span className="text-danger">{error}</span>
          ) : (
            <span className="text-muted">Đã chọn: {formatRangeLabel(range as TMDTDateRange)}</span>
          )}
        </div>

        <div className="mt-3 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-md border border-border px-3 py-1.5 font-sans text-xs font-medium text-muted hover:text-ink"
          >
            Huỷ
          </button>
          <button
            type="button"
            disabled={error !== null}
            onClick={() => range && onConfirm(range)}
            className="rounded-md bg-accent px-3 py-1.5 font-sans text-xs font-semibold text-black disabled:cursor-not-allowed disabled:opacity-40"
          >
            Bắt đầu xử lý
          </button>
        </div>
      </div>
    </div>
  )
}
