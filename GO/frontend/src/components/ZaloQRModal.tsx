import { useMemo, useRef, useState } from 'react'
import { FaArrowsRotate, FaQrcode, FaXmark } from 'react-icons/fa6'
import DOMPurify from 'dompurify'
import { useAppStore } from '../store/appStore'
import { CancelZaloLogin, RefreshZaloQR } from '../../wailsjs/go/main/App'
import { useModalEntrance } from '../lib/useModalEntrance'

// zaloQR đến từ outerHTML thật của trang login.zalo.me (site đầu tiên,
// đã tin cậy đủ để cả app tự động hoá thao tác trên đó) chứ không phải
// input người dùng gõ - nhưng vẫn đi qua dangerouslySetInnerHTML nên phải
// khử trùng trước khi chèn vào DOM, phòng trường hợp trang phía Zalo có
// nội dung bất thường (hoặc bị can thiệp giữa đường). Dùng DOMPurify với
// hồ sơ SVG (parser thật, không phải regex tự chế trước đây - regex dễ bị
// lách qua vd javascript: URI trong href/xlink:href, <foreignObject>,
// <image href="...">...) thay vì chỉ cắt <script>/on*= bằng tay.
function sanitizeSvg(markup: string): string {
  return DOMPurify.sanitize(markup, { USE_PROFILES: { svg: true, svgFilters: true } })
}

// ZaloQRModal hiện mã QR đăng nhập Zalo NGAY trong app - thay cho việc mở
// 1 cửa sổ Chrome rời (ChromedpSender giờ chạy ẩn hoàn toàn, xem
// GO/internal/zalosend/chromedp_sender.go's EnsureLoggedIn). zaloQR trong
// store là markup SVG lấy TRỰC TIẾP từ trang (không phải ảnh chụp màn
// hình) qua sự kiện zalo:qr - render thẳng qua dangerouslySetInnerHTML nên
// nét ở mọi kích thước, không phải ảnh raster.
export function ZaloQRModal() {
  const zaloQR = useAppStore((s) => s.zaloQR)
  const setZaloQR = useAppStore((s) => s.setZaloQR)
  const appendLog = useAppStore((s) => s.appendLog)
  const [refreshing, setRefreshing] = useState(false)
  const safeSvg = useMemo(() => (zaloQR ? sanitizeSvg(zaloQR) : ''), [zaloQR])
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  // deps: [!!zaloQR] - component ở lại mounted xuyên suốt (App.tsx render
  // nó vô điều kiện), chỉ tự return null lúc chưa có QR - phải theo dõi
  // đúng thời điểm zaloQR chuyển null -> có giá trị để hiệu ứng mở chạy
  // lại mỗi lần popup thật sự xuất hiện, không chỉ chạy 1 lần lúc mount.
  useModalEntrance(backdropRef, cardRef, [!!zaloQR])

  if (!zaloQR) return null

  async function handleRefresh() {
    setRefreshing(true)
    try {
      await RefreshZaloQR()
    } catch (err) {
      appendLog(`❌ Lỗi làm mới mã QR: ${String(err)}`)
    } finally {
      setRefreshing(false)
    }
  }

  // Ẩn popup ngay (không đợi Go phản hồi) rồi báo Go dừng hẳn lượt chờ
  // đăng nhập đang chạy nền - không chỉ ẩn giao diện, nếu không lượt gửi
  // vẫn treo tới 120s phía sau và nút gửi sẽ báo "đang có lượt gửi khác
  // chạy" nếu người dùng thử gửi lại ngay.
  function handleClose() {
    setZaloQR(null)
    CancelZaloLogin().catch((err) => appendLog(`❌ Lỗi huỷ đăng nhập: ${String(err)}`))
  }

  return (
    <div ref={backdropRef} className="fixed inset-0 z-[60] flex items-center justify-center bg-black/70 p-6">
      <div
        ref={cardRef}
        className="relative flex w-full max-w-xs flex-col items-center gap-4 rounded-xl border border-border bg-panel px-6 py-6 shadow-2xl"
      >
        <button
          onClick={handleClose}
          title="Đóng, không đăng nhập nữa"
          className="absolute right-3 top-3 rounded p-1 text-muted transition-colors hover:bg-white/5 hover:text-ink"
        >
          <FaXmark size={14} />
        </button>
        <div className="flex items-center gap-2 text-sm font-bold text-ink">
          <FaQrcode style={{ color: '#0068FF' }} />
          Quét mã để đăng nhập Zalo
        </div>
        <div
          className="flex h-56 w-56 items-center justify-center rounded-lg bg-white p-2 [&_svg]:h-full [&_svg]:w-full"
          dangerouslySetInnerHTML={{ __html: safeSvg }}
        />
        <p className="text-center text-[11px] leading-relaxed text-muted">
          Mở Zalo trên điện thoại → Quét QR → chọn "Đăng nhập trên trình duyệt"
        </p>
        <button
          onClick={handleRefresh}
          disabled={refreshing}
          className="inline-flex items-center gap-2 rounded-lg border border-border px-3 py-1.5 text-xs font-semibold text-muted transition-colors hover:border-accent hover:text-accent disabled:opacity-60"
        >
          <FaArrowsRotate size={11} className={refreshing ? 'animate-spin' : ''} />
          Làm mới mã QR
        </button>
      </div>
    </div>
  )
}
