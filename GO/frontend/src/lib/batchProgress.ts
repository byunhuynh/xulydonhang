// Tiến trình của một lô xử lý, do backend phát qua sự kiện
// "process:progress" sau khi mỗi file chạy xong. Backend vốn đã biết con
// số này (nó đang lặp qua danh sách file) - trước đây chỉ là không nói ra,
// nên frontend chỉ hiện được "Đang xử lý..." mà không cho biết còn bao lâu.
export interface BatchProgress {
  done: number
  total: number
}

export function createBatchProgress(): BatchProgress {
  return { done: 0, total: 0 }
}

// progressPercent kẹp kết quả trong 0..100 thay vì tin tuyệt đối vào sự
// kiện: một lô bị huỷ giữa chừng rồi chạy lô mới có thể để lại một sự
// kiện cũ đến muộn với done lớn hơn total của lô hiện tại.
export function progressPercent({ done, total }: BatchProgress): number {
  if (total <= 0) return 0
  // Làm tròn XUỐNG chứ không làm tròn gần nhất: 5/8 là 62%, không phải
  // 63%. Thanh tiến trình không bao giờ được báo nhiều hơn phần đã thực
  // sự chạy xong, và 100% chỉ xuất hiện khi file cuối cùng đã đóng.
  const percent = Math.floor((done / total) * 100)
  if (percent < 0) return 0
  if (percent > 100) return 100
  return percent
}

// Trả về chuỗi rỗng khi chưa có lô nào công bố kích thước, để chỗ hiển
// thị tự biết là không có gì để nói - thanh trạng thái khi đó giữ nguyên
// chữ "Đang xử lý" như cũ thay vì hiện "0/0 file · 0%" vô nghĩa.
export function formatBatchProgress(progress: BatchProgress): string {
  if (progress.total <= 0) return ''
  const done = Math.min(Math.max(progress.done, 0), progress.total)
  return `${done}/${progress.total} file · ${progressPercent(progress)}%`
}
