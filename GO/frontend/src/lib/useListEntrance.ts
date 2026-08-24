// useListEntrance: hiệu ứng GSAP cho các danh sách chỉ CHÈN THÊM ở cuối
// (ResultTable's rows, LogPanel's logLines) - mỗi lần `count` tăng, chỉ
// những phần tử MỚI (từ index đã thấy lần trước tới count-1) được tween
// mờ dần + trượt nhẹ vào, những phần tử cũ không bị động tới (không
// revert lại animation đã hoàn tất của chúng).
import { useRef, type RefObject } from 'react'
import { useGSAP } from '@gsap/react'
import gsap from 'gsap'

export function useListEntrance(containerRef: RefObject<HTMLElement | null>, itemSelector: string, count: number) {
  const seenRef = useRef(0)

  useGSAP(
    () => {
      const container = containerRef.current
      if (!container) return

      // Danh sách bị RESET về ít phần tử hơn lần trước (vd một batch xử
      // lý mới gọi resetRows()/clearLog()) - chỉ đồng bộ lại mốc đã thấy,
      // không tween "co lại" hay tween lại từ đầu, tránh giật hình khi
      // component re-mount với dữ liệu cũ đã render sẵn.
      if (count <= seenRef.current) {
        seenRef.current = count
        return
      }

      const items = container.querySelectorAll<HTMLElement>(itemSelector)
      const newItems = Array.from(items).slice(seenRef.current)
      seenRef.current = count
      if (newItems.length === 0) return

      gsap.fromTo(
        newItems,
        { opacity: 0, y: 6 },
        { opacity: 1, y: 0, duration: 0.28, ease: 'power2.out', stagger: 0.035, clearProps: 'opacity,transform' },
      )
    },
    { scope: containerRef, dependencies: [count] },
  )
}
