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
    // Không logo, không tên app, không cả đường kẻ đáy: tên app đã nằm ở
    // khối thương hiệu bên dưới (để trên này nữa là đọc hai lần cùng một
    // dòng chữ), còn logo và hàng tab đều vắt qua ranh giới nên kẻ vạch
    // ngang là gạch bỏ chúng. Ở đây chỉ còn vùng kéo cửa sổ và 3 nút.
    <div className="wails-drag flex h-8 shrink-0 items-center bg-panel">
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
