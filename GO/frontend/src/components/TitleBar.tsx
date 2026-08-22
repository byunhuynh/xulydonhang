import { useEffect, useState } from 'react'
import { FaXmark } from 'react-icons/fa6'
import { Quit, WindowIsMaximised, WindowMinimise, WindowToggleMaximise } from '../../wailsjs/runtime/runtime'

export function TitleBar() {
  const [isMaximized, setIsMaximized] = useState(false)

  useEffect(() => {
    WindowIsMaximised().then(setIsMaximized)
  }, [])

  function toggleMaximize() {
    WindowToggleMaximise()
    setIsMaximized((v) => !v)
  }

  return (
    <div className="wails-drag flex h-8 shrink-0 items-center border-b border-border bg-panel pl-3">
      <img src="/logo.svg" alt="" className="no-drag h-4 w-auto" draggable={false} />
      <span className="ml-2 font-sans text-[11px] font-semibold tracking-wide text-muted">
        Blue Hà Thành - Order System v3.0
      </span>
      <div className="wails-no-drag ml-auto flex h-full items-stretch">
        <button
          type="button"
          onClick={WindowMinimise}
          title="Thu nhỏ"
          className="flex w-11 items-center justify-center text-muted transition-colors hover:bg-border hover:text-ink"
        >
          <div className="h-px w-2.5 bg-current" />
        </button>
        <button
          type="button"
          onClick={toggleMaximize}
          title={isMaximized ? 'Khôi phục' : 'Phóng to'}
          className="flex w-11 items-center justify-center text-muted transition-colors hover:bg-border hover:text-ink"
        >
          {isMaximized ? (
            <div className="relative h-2.5 w-2.5">
              <div className="absolute right-0 top-0 h-2 w-2 border border-current" />
              <div className="absolute bottom-0 left-0 h-2 w-2 border border-current bg-panel" />
            </div>
          ) : (
            <div className="h-2.5 w-2.5 border border-current" />
          )}
        </button>
        <button
          type="button"
          onClick={Quit}
          title="Đóng"
          className="flex w-11 items-center justify-center text-muted transition-colors hover:bg-danger hover:text-ink"
        >
          <FaXmark size={13} />
        </button>
      </div>
    </div>
  )
}
