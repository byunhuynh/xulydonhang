import { useState } from 'react'
import { FaPaperPlane, FaSpinner } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { SendZaloMessages } from '../../wailsjs/go/main/App'
import {
  buildZaloMessageForPO,
  buildZaloMessageForJITFile,
  buildZaloMessageForTMDTShop,
  buildPriceBasisForPO,
  tmdtShopFromGroupKey,
} from '../lib/zaloMessage'
import { groupKeyFor } from '../lib/zaloGrouping'

// Nút gửi Zalo nằm RIÊNG trong thanh "Đã chọn N đơn" của bảng kết quả.
// Trước đây nó thế chỗ nút XỬ LÝ ĐƠN HÀNG ngay khi có đơn được tick:
// cùng vị trí, cùng cỡ, nên bấm theo thói quen để xử lý lô mới là gửi
// tin thật vào nhóm Zalo của khách.
export function ZaloSendButton() {
  const rows = useAppStore((s) => s.rows)
  const selectedPOs = useAppStore((s) => s.selectedPOs)
  const resolvedChoice = useAppStore((s) => s.resolvedChoice)
  const receivedAt = useAppStore((s) => s.receivedAt)
  const jitPeriodState = useAppStore((s) => s.jitPeriodState)
  const appendLog = useAppStore((s) => s.appendLog)
  // Chặn bấm lần hai khi lượt trước chưa trả về: mỗi lần bấm là một lượt
  // gửi thật, bấm đúp là khách nhận hai tin.
  const [sending, setSending] = useState(false)

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
      // Nhóm TMĐT gom theo shop: key thô là "tmdt|{shop}".
      const tmdtShop = tmdtShopFromGroupKey(key)
      // Đúng mốc giờ đã được đóng dấu lúc dòng đầu tiên của nhóm này xuất
      // hiện trên bảng - CÙNG giá trị OrderContentModal dùng cho bản xem
      // trước (nó cũng lấy theo dòng đầu của nhóm). Không được tính
      // new Date() mới ở đây: tin khách nhận sẽ lệch giờ so với tin người
      // dùng vừa duyệt, và dòng "Xử lý lúc" sẽ thành giờ GỬI chứ không
      // phải giờ xử lý.
      const processedAt = receivedAt[indices[0]] ?? ''
      const message = tmdtShop
        ? buildZaloMessageForTMDTShop(groupRows, processedAt)
        : isJIT
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
        displayLabel: tmdtShop || (isJIT ? (groupRows[0]?.fileName ?? '') : ''),
      }
    })
    appendLog(`📨 Bắt đầu gửi ${jobs.length} tin Zalo...`)
    setSending(true)
    try {
      await SendZaloMessages(jobs)
    } catch (err) {
      appendLog(`❌ Lỗi gửi Zalo: ${String(err)}`)
    } finally {
      setSending(false)
    }
  }

  return (
    <button
      type="button"
      onClick={handleSendZalo}
      disabled={sending || selectedPOs.size === 0}
      className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 font-sans text-xs font-bold text-white transition-[filter] hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:brightness-100"
      style={{ backgroundColor: '#0068FF' }}
    >
      {sending ? <FaSpinner size={11} className="animate-spin" /> : <FaPaperPlane size={11} />}
      {sending ? 'Đang gửi Zalo...' : `Gửi ${selectedPOs.size} tin Zalo`}
    </button>
  )
}
