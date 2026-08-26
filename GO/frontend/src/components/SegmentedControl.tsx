interface SegmentedControlProps {
  options: readonly { value: string; label: string }[]
  value: string
  disabled?: boolean
  onChange: (value: string) => void
  ariaLabel: string
}

// Vài lựa chọn loại trừ nhau, tất cả đều ngắn, và người dùng đổi ngay sau
// khi nhìn kết quả - đó là mô tả của một segmented control, không phải
// của một menu bật/tắt. Các nút liền khối cho thấy cả những lựa chọn còn
// lại lẫn cái đang chọn cùng lúc, và bỏ được một cú bấm khỏi thao tác
// thường gặp nhất.
//
// role="group" chứ không phải "radiogroup": ở bộ chọn buổi JIT, các nút
// này KHÔNG chỉ đổi trạng thái cục bộ mà gửi thẳng một lệnh ghi Excel cho
// cả file PDF, nên chúng là nút hành động, và aria-pressed nói đúng điều
// đó. Một radiogroup sẽ hứa với trình đọc màn hình rằng mũi tên
// trái/phải di chuyển lựa chọn mà không ghi gì - lời hứa mà thành phần
// này không giữ.
//
// value không khớp lựa chọn nào (chuỗi rỗng) là trạng thái HỢP LỆ: bảng
// định tuyến MISA dùng nó cho khoá chưa map, và "không nút nào sáng" là
// đúng thứ cần hiện ra để người dùng thấy ngay chỗ phải bấm.
export function SegmentedControl({ options, value, disabled, onChange, ariaLabel }: SegmentedControlProps) {
  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className={`inline-flex shrink-0 overflow-hidden rounded-md border border-border bg-bg ${
        disabled ? 'opacity-50' : ''
      }`}
    >
      {options.map((option, index) => {
        const isActive = value === option.value
        return (
          <button
            key={option.value}
            type="button"
            disabled={disabled}
            aria-pressed={isActive}
            onClick={() => {
              if (!isActive) onChange(option.value)
            }}
            className={`px-2.5 py-1 font-sans text-xs transition-colors disabled:cursor-not-allowed ${
              index > 0 ? 'border-l border-border' : ''
            } ${
              isActive
                ? 'bg-accent/[0.16] font-semibold text-accent'
                : 'font-medium text-muted hover:bg-white/[0.04] hover:text-ink'
            }`}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
