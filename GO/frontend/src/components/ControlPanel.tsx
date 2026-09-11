import { useState } from 'react'
import { FaCloudArrowUp, FaRocket, FaSpinner } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { InspectTMDTFiles, ProcessFiles } from '../../wailsjs/go/main/App'
import { TMDTDateRangeModal } from './TMDTDateRangeModal'
import type { TMDTDateRange } from '../lib/tmdtDateRange'
import { MisaPushModal } from './MisaPushModal'

export function ControlPanel() {
  const files = useAppStore((s) => s.files)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const appendLog = useAppStore((s) => s.appendLog)
  const resetRows = useAppStore((s) => s.resetRows)
  const rows = useAppStore((s) => s.rows)
  const isPushing = useAppStore((s) => s.isPushing)
  const clearMisaResults = useAppStore((s) => s.clearMisaResults)

  // Danh sách file TMĐT đang chờ người dùng chọn khoảng ngày. Modal chỉ
  // bật khi người dùng bấm "Xử lý" — thả file vào không hỏi gì.
  const [pendingTMDT, setPendingTMDT] = useState<string[] | null>(null)
  // Modal chọn đơn + nhánh kế toán để đẩy lên MISA.
  const [isMisaOpen, setIsMisaOpen] = useState(false)

  async function handleProcess() {
    if (files.length === 0) {
      appendLog('Không có file nào để xử lý!')
      return
    }
    // Hỏi backend file nào là workbook TMĐT — nhận theo NỘI DUNG file chứ
    // không theo đuôi hay tên, đúng cùng một phép thử mà runReservedBatch
    // dùng để rẽ nhánh, nên modal lịch bật đúng bằng lúc nhánh TMĐT chạy.
    let tmdtFiles: string[] = []
    try {
      tmdtFiles = await InspectTMDTFiles(files)
    } catch (err) {
      appendLog(`❌ Lỗi kiểm tra file TMĐT: ${String(err)}`)
      return
    }
    if (tmdtFiles.length > 0) {
      setPendingTMDT(tmdtFiles)
      return
    }
    await startBatch({})
  }

  async function startBatch(ranges: Record<string, TMDTDateRange>) {
    resetRows()
    setProcessing(true)
    appendLog('🚀 Bắt đầu xử lý...')
    try {
      await ProcessFiles(files, ranges)
    } catch (err) {
      appendLog(`❌ Lỗi xử lý: ${String(err)}`)
      setProcessing(false)
    }
  }

  // Nút này LUÔN là "Xử lý đơn hàng", có tick chọn đơn hay không. Nút gửi
  // Zalo nằm riêng ở thanh chọn đơn của bảng kết quả (ZaloSendButton).
  return (
    <>
      <section className="flex flex-shrink-0 items-center gap-3 rounded-xl border border-border bg-panel px-4 py-3">
        <button
          type="button"
          onClick={() => {
            clearMisaResults()
            setIsMisaOpen(true)
          }}
          disabled={rows.length === 0 || isProcessing || isPushing}
          title={rows.length === 0 ? 'Xử lý đơn hàng trước đã' : 'Đẩy đơn vừa xử lý lên AMIS Kế toán'}
          className="inline-flex items-center gap-2 rounded-lg border border-border px-3 py-2 font-sans text-xs font-semibold text-muted transition-colors hover:border-accent hover:text-accent disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:border-border disabled:hover:text-muted"
        >
          <FaCloudArrowUp /> Push MISA
        </button>
        <button
          onClick={handleProcess}
          disabled={isProcessing || isPushing}
          className={`ml-auto inline-flex items-center justify-center gap-2 rounded-lg bg-gradient-to-br from-accent to-[#1a9dc4] px-5 py-2.5 text-sm font-extrabold tracking-wide text-[#0a1620] transition-transform hover:brightness-110 active:scale-[0.98] disabled:opacity-60 ${
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
      </section>
      {pendingTMDT && (
        <TMDTDateRangeModal
          fileNames={pendingTMDT.map((p) => p.split(/[\\/]/).pop() ?? p)}
          onCancel={() => setPendingTMDT(null)}
          onConfirm={(range) => {
            const ranges: Record<string, TMDTDateRange> = {}
            for (const p of pendingTMDT) ranges[p] = range
            setPendingTMDT(null)
            void startBatch(ranges)
          }}
        />
      )}
      {isMisaOpen && <MisaPushModal onClose={() => setIsMisaOpen(false)} />}
    </>
  )
}
