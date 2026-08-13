// Vô hiệu hoá các hành vi "trang web" của webview (zoom bằng
// Ctrl+cuộn/Ctrl +/-) để ứng dụng cảm giác như desktop app. Menu chuột phải
// đã được Wails xử lý mặc định (hiện khi có text được chọn hoặc trên
// input/textarea/contentEditable, ẩn ở nơi khác) nên không cần can thiệp —
// việc này giữ cho "click phải → Copy" hoạt động để copy số PO / log lỗi.
export function installDesktopFeel(): void {
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
