import { useEffect } from 'react'
import { EventsOn, OnFileDrop, OnFileDropOff } from '../../wailsjs/runtime/runtime'
import { useAppStore, type LockStatus } from '../store/appStore'
import type { OrderRow } from '../types'

export function useWailsEvents() {
  const appendLog = useAppStore((s) => s.appendLog)
  const upsertRow = useAppStore((s) => s.upsertRow)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const setStt = useAppStore((s) => s.setStt)
  const addFiles = useAppStore((s) => s.addFiles)
  const setLockStatus = useAppStore((s) => s.setLockStatus)
  const deselectPO = useAppStore((s) => s.deselectPO)

  useEffect(() => {
    const offLog = EventsOn('process:log', (line: string) => appendLog(line))
    const offRow = EventsOn('process:row', (row: OrderRow) => upsertRow(row))
    const offDone = EventsOn('process:done', (finalStt: number) => {
      setProcessing(false)
      setStt(finalStt)
    })
    const offDrop = EventsOn('files:dropped', (paths: string[]) => addFiles(paths))
    const offLock = EventsOn('applock:status', (status: LockStatus) => setLockStatus(status))
    const offZaloLog = EventsOn('zalo:log', (line: string) => appendLog(line))
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
    // zalo:done chỉ còn là tín hiệu "batch đã kết thúc". Chưa có phần
    // nào khác trong app cần biết điều đó ngoài dòng log tổng kết này,
    // nhưng vẫn lắng nghe để không mất tín hiệu kết thúc batch.
    const offZaloDone = EventsOn('zalo:done', () => appendLog('🏁 Đã kết thúc lượt gửi Zalo.'))
    // Attaches the actual dragover/dragleave/drop DOM listeners on window;
    // without this call the Go-side runtime.OnFileDrop callback never fires
    // and WebView2 falls back to navigating the window to the dropped file.
    // The real file-list update flows through the files:dropped event above.
    OnFileDrop(() => {}, false)

    return () => {
      offLog()
      offRow()
      offDone()
      offDrop()
      offLock()
      offZaloLog()
      offZaloSent()
      offZaloDone()
      OnFileDropOff()
    }
  }, [appendLog, upsertRow, setProcessing, setStt, addFiles, setLockStatus, deselectPO])
}
