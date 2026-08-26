import { useEffect } from 'react'
import { EventsOn, OnFileDrop, OnFileDropOff } from '../../wailsjs/runtime/runtime'
import { useAppStore, type LockStatus } from '../store/appStore'
import type { OrderRow } from '../types'
import type { BatchProgress } from '../lib/batchProgress'
import type { TMDTMissingCombo } from '../lib/tmdtMissing'

export function useWailsEvents() {
  const appendLog = useAppStore((s) => s.appendLog)
  const upsertRow = useAppStore((s) => s.upsertRow)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const setBatchProgress = useAppStore((s) => s.setBatchProgress)
  const setStt = useAppStore((s) => s.setStt)
  const addFiles = useAppStore((s) => s.addFiles)
  const setLockStatus = useAppStore((s) => s.setLockStatus)
  const deselectPO = useAppStore((s) => s.deselectPO)
  const setZaloQR = useAppStore((s) => s.setZaloQR)
  const setTMDTMissing = useAppStore((s) => s.setTMDTMissing)

  useEffect(() => {
    const offLog = EventsOn('process:log', (line: string) => appendLog(line))
    const offRow = EventsOn('process:row', (row: OrderRow) => upsertRow(row))
    const offProgress = EventsOn('process:progress', (progress: BatchProgress) => setBatchProgress(progress))
    const offDone = EventsOn('process:done', (finalStt: number) => {
      setProcessing(false)
      setStt(finalStt)
    })
    const offDrop = EventsOn('files:dropped', (paths: string[]) => addFiles(paths))
    const offLock = EventsOn('applock:status', (status: LockStatus) => setLockStatus(status))
    const offZaloLog = EventsOn('zalo:log', (line: string) => appendLog(line))
    // Chuỗi rỗng nghĩa là "ẩn popup QR" (đã đăng nhập, hoặc hết giờ chờ) -
    // setZaloQR tự đổi chuỗi rỗng thành null.
    const offZaloQR = EventsOn('zalo:qr', (svgMarkup: string) => setZaloQR(svgMarkup))
    // Bỏ chọn ĐÚNG những PO đã gửi thành công, từng cái một khi backend
    // báo về. Trước đây zalo:done xoá TOÀN BỘ lựa chọn bất kể kết quả:
    // nếu login hết giờ (không gửi được gì) hoặc chỉ vài PO thành công,
    // người dùng phải đọc log rồi tự chọn lại - và chọn lại đúng nhóm cũ
    // sẽ GỬI TRÙNG cho những PO đã gửi được (đây là nhóm Zalo thật của
    // khách/nhà cung cấp). Giữ nguyên lựa chọn ở các PO thất bại để thấy
    // ngay còn gì cần gửi lại.
    const offZaloSent = EventsOn('zalo:sent', (data: { po: string; ok: boolean }) => {
      if (data?.ok === true) deselectPO(data.po)
    })
    // zalo:done là tín hiệu "batch đã kết thúc" - dọn nốt popup QR nếu vì
    // lý do gì đó vẫn còn hiện (EnsureLoggedIn đã tự gửi onQR("") trước
    // khi trả về, nhưng dọn lại ở đây cho chắc, không dựa vào đúng 1 chỗ).
    // Nhánh TMĐT dừng giữa batch để hỏi mã còn thiếu. Sự kiện này chỉ
    // phát khi thực sự có mã thiếu, và backend đang chờ trên channel cho
    // tới khi modal gọi Resolve/Cancel (hoặc hết hạn 10 phút).
    const offTMDTMissing = EventsOn('tmdt:missing', (list: TMDTMissingCombo[]) => setTMDTMissing(list))
    const offZaloDone = EventsOn('zalo:done', () => {
      appendLog('🏁 Đã kết thúc lượt gửi Zalo.')
      setZaloQR(null)
    })
    // Attaches the actual dragover/dragleave/drop DOM listeners on window;
    // without this call the Go-side runtime.OnFileDrop callback never fires
    // and WebView2 falls back to navigating the window to the dropped file.
    // The real file-list update flows through the files:dropped event above.
    OnFileDrop(() => {}, false)

    return () => {
      offLog()
      offRow()
      offProgress()
      offDone()
      offDrop()
      offLock()
      offZaloLog()
      offZaloQR()
      offZaloSent()
      offZaloDone()
      offTMDTMissing()
      OnFileDropOff()
    }
  }, [appendLog, upsertRow, setBatchProgress, setProcessing, setStt, addFiles, setLockStatus, deselectPO, setZaloQR, setTMDTMissing])
}
