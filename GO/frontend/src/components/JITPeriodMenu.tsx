import { JIT_PERIOD_OPTIONS, type JITPeriod } from '../lib/jitPeriodMenu'
import { SegmentedControl } from './SegmentedControl'

interface JITPeriodMenuProps {
  value: string
  disabled: boolean
  onChange: (period: JITPeriod) => void
  ariaLabel: string
}

// Ba buổi giao loại trừ nhau - dùng chung SegmentedControl với bảng định
// tuyến MISA. Dùng chung ĐÚNG MỘT component chứ không chép lại style là
// cách duy nhất giữ hai chỗ giống nhau về lâu dài.
export function JITPeriodMenu({ value, disabled, onChange, ariaLabel }: JITPeriodMenuProps) {
  return (
    <SegmentedControl
      options={JIT_PERIOD_OPTIONS}
      value={value}
      disabled={disabled}
      onChange={(next) => onChange(next as JITPeriod)}
      ariaLabel={ariaLabel}
    />
  )
}
