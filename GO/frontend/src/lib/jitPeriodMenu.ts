export const JIT_PERIOD_OPTIONS = [
  { value: 'sáng', label: 'Sáng' },
  { value: 'chiều', label: 'Chiều' },
  { value: 'tối', label: 'Tối' },
] as const

export type JITPeriod = (typeof JIT_PERIOD_OPTIONS)[number]['value']

export function isActiveJITPeriod(value: string, period: JITPeriod): boolean {
  return value === period
}

export function menuOpenAfterEscape(isOpen: boolean, key: string): boolean {
  return key === 'Escape' ? false : isOpen
}

export function menuOpenAfterOutsideMouseDown(isOpen: boolean, isInsideMenu: boolean): boolean {
  return isInsideMenu ? isOpen : false
}
