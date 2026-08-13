import { useEffect } from 'react'
import { FaPaperPlane, FaCloudArrowUp, FaRocket } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { GetSTT, SetSTT, ProcessFiles } from '../../wailsjs/go/main/App'

export function ControlPanel() {
  const stt = useAppStore((s) => s.stt)
  const setStt = useAppStore((s) => s.setStt)
  const files = useAppStore((s) => s.files)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const appendLog = useAppStore((s) => s.appendLog)
  const resetRows = useAppStore((s) => s.resetRows)

  useEffect(() => {
    GetSTT()
      .then(setStt)
      .catch((err) => appendLog(`❌ Lỗi đọc STT: ${String(err)}`))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function handleSttBlur() {
    try {
      await SetSTT(stt)
    } catch (err) {
      appendLog(`❌ Lỗi ghi STT: ${String(err)}`)
    }
  }

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
    <section className="flex h-full flex-col justify-between rounded-xl border border-border bg-panel p-3">
      <div>
        <h2 className="mb-2 text-sm font-semibold text-muted">2. Cấu hình &amp; Thực thi</h2>
        <label className="text-xs text-muted">Số thứ tự đơn hàng bắt đầu</label>
        <input
          type="number"
          value={stt}
          disabled={isProcessing}
          onChange={(e) => setStt(Number(e.target.value))}
          onBlur={handleSttBlur}
          className="selectable mt-1 w-full rounded-lg border border-border bg-bg px-3 py-2 text-center font-mono text-lg text-ink focus:border-accent focus:outline-none disabled:opacity-40"
        />
      </div>
      <div className="mt-4 flex flex-col gap-2">
        <button
          disabled
          title="Sẽ có ở giai đoạn sau"
          className="inline-flex items-center justify-center gap-2 rounded-lg bg-[#0068ff]/20 px-3 py-2 text-sm font-medium text-[#0068ff] opacity-40"
        >
          <FaPaperPlane /> Gửi thông báo Zalo
        </button>
        <button
          disabled
          title="Sẽ có ở giai đoạn sau"
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium text-muted opacity-40"
        >
          <FaCloudArrowUp /> Push MISA
        </button>
        <button
          onClick={handleProcess}
          disabled={isProcessing}
          className="inline-flex items-center justify-center gap-2 rounded-lg bg-success px-3 py-3 text-sm font-bold text-bg hover:brightness-110 disabled:opacity-40"
        >
          <FaRocket /> XỬ LÝ ĐƠN HÀNG
        </button>
      </div>
    </section>
  )
}
