import { useEffect, useState } from 'react'
import { FaPaperPlane, FaCloudArrowUp, FaRocket, FaSpinner } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { GetSTT, InspectTMDTFiles, ProcessFiles, SendZaloMessages } from '../../wailsjs/go/main/App'
import { TMDTDateRangeModal } from './TMDTDateRangeModal'
import type { TMDTDateRange } from '../lib/tmdtDateRange'
import { buildZaloMessageForPO, buildZaloMessageForJITFile, buildPriceBasisForPO } from '../lib/zaloMessage'
import { groupKeyFor } from '../lib/zaloGrouping'

export function ControlPanel() {
  const stt = useAppStore((s) => s.stt)
  const setStt = useAppStore((s) => s.setStt)
  const files = useAppStore((s) => s.files)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const appendLog = useAppStore((s) => s.appendLog)
  const resetRows = useAppStore((s) => s.resetRows)
  const rows = useAppStore((s) => s.rows)
  const selectedPOs = useAppStore((s) => s.selectedPOs)
  const resolvedChoice = useAppStore((s) => s.resolvedChoice)
  const receivedAt = useAppStore((s) => s.receivedAt)
  const jitPeriodState = useAppStore((s) => s.jitPeriodState)

  // Danh sách file TMĐT đang chờ người dùng chọn khoảng ngày. Modal chỉ
  // bật khi người dùng bấm "Xử lý" — thả file vào không hỏi gì.
  const [pendingTMDT, setPendingTMDT] = useState<string[] | null>(null)

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
      await ProcessFiles(files, stt, ranges)
    } catch (err) {
      appendLog(`❌ Lỗi xử lý: ${String(err)}`)
      setProcessing(false)
    }
  }

  // rowsForGroupKey cục bộ - CHỈ cần đọc, không cần state riêng - lặp
  // toàn bộ rows tìm đúng những dòng thuộc nhóm này (khớp cách
  // ResultTable.tsx's rowsForGroupKey đã dùng, không tái sử dụng trực
  // tiếp vì hàm đó vẫn là closure riêng của ResultTable.tsx, xem Task 6).
  // groupKeyFor (lib/zaloGrouping.ts) quyết định po hay sourceId là khoá
  // nhóm tuỳ vendor - PHẢI dùng chung định nghĩa với ResultTable.tsx để
  // dòng người dùng tick chọn và dòng thực sự đưa vào job gửi khớp nhau.
  function rowsForGroupKey(key: string): number[] {
    return rows.reduce<number[]>((acc, row, idx) => {
      if (groupKeyFor(row) === key) acc.push(idx)
      return acc
    }, [])
  }

  async function handleSendZalo() {
    const jobs = [...selectedPOs].map((key) => {
      const indices = rowsForGroupKey(key)
      const groupRows = indices.map((idx) => rows[idx])
      const isJIT = groupRows[0]?.system === 'JIT-CHOICE'
      // Đúng mốc giờ đã được đóng dấu lúc dòng đầu tiên của nhóm này xuất
      // hiện trên bảng - CÙNG giá trị OrderContentModal dùng cho bản xem
      // trước (nó cũng lấy theo dòng đầu của nhóm). Không được tính
      // new Date() mới ở đây: tin khách nhận sẽ lệch giờ so với tin người
      // dùng vừa duyệt, và dòng "Xử lý lúc" sẽ thành giờ GỬI chứ không
      // phải giờ xử lý.
      const processedAt = receivedAt[indices[0]] ?? ''
      const message = isJIT
        ? buildZaloMessageForJITFile(
            groupRows,
            // Buổi giao THEO GIÁ TRỊ NGƯỜI DÙNG ĐANG CHỌN, không phải giá
            // trị lúc xử lý PDF (xem buildZaloMessageForJITFile's doc).
            jitPeriodState.periodBySource[key] ?? groupRows[0]?.jitPeriod ?? '',
            processedAt,
          )
        : buildZaloMessageForPO(groupRows, processedAt, buildPriceBasisForPO(rows, indices, resolvedChoice))
      return {
        po: key,
        system: groupRows[0]?.system ?? '',
        // 2 ký tự đầu của mã khách hàng là miền (MN/MB) - cần để Go ghép
        // đúng key Cài đặt > Zalo (vd "MNBIGC"), vì system một mình
        // không phân biệt miền (xem zalosend.ResolveContact).
        customerCode: groupRows[0]?.maKhachHang ?? '',
        message,
        // po ở trên giờ là sourceId (hash) cho JIT, không đọc được - gửi
        // kèm tên file PDF để log Go hiện thứ có ý nghĩa thay vì hash
        // (xem ZaloJob.DisplayLabel, app.go).
        displayLabel: isJIT ? (groupRows[0]?.fileName ?? '') : '',
      }
    })
    appendLog(`📨 Bắt đầu gửi ${jobs.length} tin Zalo...`)
    try {
      await SendZaloMessages(jobs)
    } catch (err) {
      appendLog(`❌ Lỗi gửi Zalo: ${String(err)}`)
    }
  }

  const hasSelection = selectedPOs.size > 0

  return (
    <>
      <section className="flex flex-shrink-0 items-center gap-3 rounded-xl border border-border bg-panel px-4 py-3">
        <div
          title="Sẽ có ở giai đoạn sau"
          className="flex items-center gap-2 rounded-lg border border-dashed border-border px-3 py-2 text-xs font-medium text-muted opacity-60"
        >
          <FaCloudArrowUp /> Push MISA
          <span className="rounded-full bg-white/5 px-1.5 py-0.5 font-mono text-[8px] font-bold tracking-wide">
            SẮP RA MẮT
          </span>
        </div>
        {hasSelection ? (
          <button
            onClick={handleSendZalo}
            className="ml-auto inline-flex items-center justify-center gap-2 rounded-lg px-5 py-2.5 text-sm font-extrabold tracking-wide text-white transition-transform hover:brightness-110 active:scale-[0.98]"
            style={{ backgroundColor: '#0068FF' }}
          >
            <FaPaperPlane /> GỬI {selectedPOs.size} TIN ZALO
          </button>
        ) : (
          <button
            onClick={handleProcess}
            disabled={isProcessing}
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
        )}
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
    </>
  )
}
