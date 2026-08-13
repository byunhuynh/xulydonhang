// Vô hiệu hoá các hành vi "trang web" của webview (menu chuột phải mặc
// định, zoom bằng Ctrl+cuộn/Ctrl +/-) để ứng dụng cảm giác như desktop app.
export function installDesktopFeel(): void {
  window.addEventListener('contextmenu', (e) => {
    e.preventDefault()
  })

  window.addEventListener(
    'wheel',
    (e) => {
      if (e.ctrlKey) {
        e.preventDefault()
      }
    },
    { passive: false }
  )

  window.addEventListener('keydown', (e) => {
    const isZoomKey = e.key === '+' || e.key === '-' || e.key === '=' || e.key === '0'
    if ((e.ctrlKey || e.metaKey) && isZoomKey) {
      e.preventDefault()
    }
  })
}
