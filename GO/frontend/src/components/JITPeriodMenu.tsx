import { JIT_PERIOD_OPTIONS, isActiveJITPeriod, type JITPeriod } from '../lib/jitPeriodMenu'

interface JITPeriodMenuProps {
  value: string
  disabled: boolean
  onChange: (period: JITPeriod) => void
  ariaLabel: string
}

// Ba lựa chọn loại trừ nhau, cả ba đều ngắn, và người dùng đổi buổi ngay
// sau khi nhìn kết quả - đó là mô tả của một segmented control, không
// phải của một menu bật/tắt. Bản menu cũ giấu hai lựa chọn còn lại sau
// một cú bấm và giấu luôn cả buổi đang chọn cho tới khi mở ra; ba nút
// liền khối cho thấy cả ba lựa chọn lẫn cái đang chọn cùng lúc và bỏ
// được một cú bấm khỏi thao tác thường gặp nhất với đơn Top Value.
//
// role="group" chứ không phải "radiogroup": các nút này KHÔNG chỉ đổi
// trạng thái cục bộ mà gửi thẳng một lệnh ghi Excel cho cả file PDF, nên
// chúng là nút hành động, và aria-pressed nói đúng điều đó. Một
// radiogroup sẽ hứa với trình đọc màn hình rằng mũi tên trái/phải di
// chuyển lựa chọn mà không ghi gì - lời hứa mà thành phần này không giữ.
export function JITPeriodMenu({ value, disabled, onChange, ariaLabel }: JITPeriodMenuProps) {
  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className={`inline-flex shrink-0 overflow-hidden rounded-md border border-border bg-bg ${
        disabled ? 'opacity-50' : ''
      }`}
    >
      {JIT_PERIOD_OPTIONS.map((option, index) => {
        const isActive = isActiveJITPeriod(value, option.value)
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
