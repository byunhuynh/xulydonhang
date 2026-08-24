import { useEffect, useId, useRef, useState } from 'react'
import { FaCheck, FaChevronDown } from 'react-icons/fa6'
import {
  JIT_PERIOD_OPTIONS,
  isActiveJITPeriod,
  menuOpenAfterEscape,
  menuOpenAfterOutsideMouseDown,
  type JITPeriod,
} from '../lib/jitPeriodMenu'

interface JITPeriodMenuProps {
  value: string
  disabled: boolean
  onChange: (period: JITPeriod) => void
  ariaLabel: string
}

export function JITPeriodMenu({ value, disabled, onChange, ariaLabel }: JITPeriodMenuProps) {
  const [isOpen, setIsOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const menuId = useId()
  const selectedOption = JIT_PERIOD_OPTIONS.find((option) => option.value === value)

  useEffect(() => {
    if (disabled) setIsOpen(false)
  }, [disabled])

  useEffect(() => {
    if (!isOpen) return

    function handleMouseDown(event: MouseEvent) {
      const isInsideMenu = menuRef.current?.contains(event.target as Node) ?? false
      setIsOpen((open) => menuOpenAfterOutsideMouseDown(open, isInsideMenu))
    }

    function handleKeyDown(event: KeyboardEvent) {
      setIsOpen((open) => menuOpenAfterEscape(open, event.key))
    }

    document.addEventListener('mousedown', handleMouseDown)
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('mousedown', handleMouseDown)
      document.removeEventListener('keydown', handleKeyDown)
    }
  }, [isOpen])

  function selectPeriod(period: JITPeriod) {
    setIsOpen(false)
    if (!isActiveJITPeriod(value, period)) onChange(period)
  }

  return (
    <div ref={menuRef} className="relative shrink-0">
      <button
        type="button"
        disabled={disabled}
        onClick={() => setIsOpen((open) => !open)}
        className="inline-flex items-center gap-1.5 rounded-md border border-border bg-panel px-2.5 py-1 font-sans text-xs font-semibold text-accent transition-colors hover:border-accent/60 hover:bg-accent/[0.08] focus:outline-none focus-visible:ring-2 focus-visible:ring-accent/60 disabled:cursor-not-allowed disabled:opacity-50"
        aria-label={ariaLabel}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        aria-controls={menuId}
      >
        {selectedOption?.label ?? value}
        <FaChevronDown size={9} aria-hidden="true" className={isOpen ? 'rotate-180 transition-transform' : 'transition-transform'} />
      </button>
      {isOpen && (
        <div id={menuId} role="menu" className="absolute right-0 z-20 mt-1 min-w-[7.5rem] rounded-md border border-border bg-panel p-1 shadow-lg">
          {JIT_PERIOD_OPTIONS.map((option) => {
            const isActive = isActiveJITPeriod(value, option.value)
            return (
              <button
                key={option.value}
                type="button"
                role="menuitemradio"
                aria-checked={isActive}
                onClick={() => selectPeriod(option.value)}
                className={`flex w-full items-center justify-between rounded px-2 py-1.5 text-left font-sans text-xs transition-colors hover:bg-accent/[0.10] hover:text-accent focus:outline-none focus-visible:bg-accent/[0.10] focus-visible:text-accent ${isActive ? 'text-accent' : 'text-ink'}`}
              >
                {option.label}
                {isActive && <FaCheck size={10} aria-hidden="true" />}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
