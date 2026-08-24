export const JIT_PERIOD_OPTIONS = [
  { value: 'sáng', label: 'Sáng' },
  { value: 'chiều', label: 'Chiều' },
  { value: 'tối', label: 'Tối' },
] as const

export type JITPeriod = (typeof JIT_PERIOD_OPTIONS)[number]['value']

export function isActiveJITPeriod(value: string, period: JITPeriod): boolean {
  return value === period
}
