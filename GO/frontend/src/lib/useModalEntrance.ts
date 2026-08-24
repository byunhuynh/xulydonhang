// useModalEntrance: hiệu ứng mở dùng chung cho mọi modal/overlay trong
// app (nền mờ dần + thẻ nội dung phóng to nhẹ, trượt lên từ dưới) - để
// SettingsModal/LockOverlay/ZaloQRModal/OrderContentModal cùng chung một
// "ngôn ngữ chuyển động" thay vì mỗi nơi tự bịa một kiểu khác nhau.
import { type RefObject } from 'react'
import { useGSAP } from '@gsap/react'
import gsap from 'gsap'

export function useModalEntrance(
  backdropRef: RefObject<HTMLElement | null>,
  cardRef: RefObject<HTMLElement | null>,
  // deps: dùng khi modal có 1 giai đoạn "đang tải" render ra một backdrop/
  // card KHÁC (node DOM khác) trước khi nội dung thật xuất hiện (vd
  // SettingsModal chờ GetAppSettings) - truyền [dữ_liệu_đã_sẵn_sàng] để
  // hiệu ứng chạy lại đúng lúc trên card THẬT, không chỉ trên placeholder.
  deps: unknown[] = [],
) {
  useGSAP(
    () => {
      if (!backdropRef.current || !cardRef.current) return
      const tl = gsap.timeline({ defaults: { ease: 'power3.out' } })
      tl.fromTo(backdropRef.current, { opacity: 0 }, { opacity: 1, duration: 0.18 }).fromTo(
        cardRef.current,
        { opacity: 0, y: 14, scale: 0.96 },
        { opacity: 1, y: 0, scale: 1, duration: 0.28 },
        '-=0.08',
      )
    },
    { scope: backdropRef, dependencies: deps },
  )
}
