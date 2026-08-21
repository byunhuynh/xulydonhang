import { useEffect } from 'react'
import { FaPaperPlane, FaCloudArrowUp, FaRocket, FaSpinner } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { GetSTT, ProcessFiles } from '../../wailsjs/go/main/App'
import { SectionHeader } from './SectionHeader'

export function ControlPanel() {
  const stt = useAppStore((s) => s.stt)
  const setStt = useAppStore((s) => s.setStt)
  const files = useAppStore((s) => s.files)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const appendLog = useAppStore((s) => s.appendLog)
  const resetRows = useAppStore((s) => s.resetRows)

  // STT (số thứ tự đơn hàng) không còn ô nhập tay trên UI, nhưng vẫn cần
  // load giá trị hiện tại từ backend khi mở app — ProcessFiles vẫn cần
  // số bắt đầu đúng, và backend tự tăng/tự lưu lại sau mỗi lần xử lý
  // (app.go runBatch → cfg.SetSTT) mà không cần UI can thiệp.
  useEffect(() => {
    GetSTT()
      .then(setStt)
      .catch((err) => appendLog(`❌ Lỗi đọc STT: ${String(err)}`))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function handleProcess() {
    if (files.length === 0) {
      appendLog('Không có file nào để xử lý!')
      return
    }
    resetRows()
    setProcessing(true)
    appendLog('🚀 Bắt đầu xử lý...')
    try {
      await ProcessFiles(files, stt)
    } catch (err) {
      appendLog(`❌ Lỗi xử lý: ${String(err)}`)
      setProcessing(false)
    }
  }

  return (
    <section className="flex h-full flex-col overflow-y-auto rounded-xl border border-border bg-panel p-3.5">
      <SectionHeader index="02" title="Cấu hình & Thực thi" />
      <div className="flex flex-1 flex-col justify-end gap-2">
        <div
          title="Sẽ có ở giai đoạn sau"
          className="flex flex-col gap-1 rounded-lg border border-dashed border-border px-3 py-2 text-sm font-medium text-muted"
        >
          <span className="inline-flex items-center gap-2 whitespace-nowrap opacity-60">
            <FaPaperPlane /> Gửi thông báo Zalo
          </span>
          <span className="self-start rounded-full bg-white/5 px-2 py-0.5 font-mono text-[9px] font-bold tracking-wide text-muted">
            SẮP RA MẮT
          </span>
        </div>
        <div
          title="Sẽ có ở giai đoạn sau"
          className="flex flex-col gap-1 rounded-lg border border-dashed border-border px-3 py-2 text-sm font-medium text-muted"
        >
          <span className="inline-flex items-center gap-2 whitespace-nowrap opacity-60">
            <FaCloudArrowUp /> Push MISA
          </span>
          <span className="self-start rounded-full bg-white/5 px-2 py-0.5 font-mono text-[9px] font-bold tracking-wide text-muted">
            SẮP RA MẮT
          </span>
        </div>
        <button
          onClick={handleProcess}
          disabled={isProcessing}
          className={`mt-1.5 inline-flex items-center justify-center gap-2 rounded-lg bg-gradient-to-br from-accent to-[#1a9dc4] px-3 py-3.5 text-sm font-extrabold tracking-wide text-[#0a1620] transition-transform hover:brightness-110 active:scale-[0.98] disabled:opacity-60 ${
            !isProcessing ? 'animate-pulse-glow' : ''
          }`}
        >
          {isProcessing ? (
            <>
              <FaSpinner className="animate-spin" /> ĐANG XỬ LÝ...
            </>
          ) : (
            <>
              <FaRocket /> XỬ LÝ ĐƠN HÀNG
            </>
          )}
        </button>
      </div>
    </section>
  )
}
