/**
 * Logo "Blue" dùng chung cho cả app.
 *
 * Khi `active` bật, CHÍNH dải sóng cyan của logo bò một vòng quanh khối
 * tím rồi về đúng chỗ cũ và bò tiếp - dùng làm chỉ báo "app đang bận"
 * (header truyền isProcessing vào, màn khởi động truyền active cứng).
 *
 * Mỗi khung hình, snakePath dựng lại thuộc tính `d` của path cyan sao
 * cho từng điểm giữ nguyên khoảng cách vuông góc tới đường chạy như
 * trong logo gốc, chỉ trượt đi `shift` đơn vị dọc đường đó. Nhờ vậy dải
 * sóng uốn theo viền kiểu con rắn chứ không trượt cứng, và ở shift = 0
 * (hoặc = trọn một chu vi) nó trùng khít hình gốc nên vòng lặp không
 * thấy mối nối.
 *
 * Đường chạy do buildRibbon dựng: KHÔNG phải chính đường bao khối tím
 * mà là đường đi giữa dải sóng (đường bao dời ra ngoài nửa bề rộng dải).
 * Chạy thẳng trên đường bao thì hai mũi nhọn của khối tím làm dải sóng
 * bị kéo toạc; xem chú thích đầu snakePath.ts.
 *
 * Chiều chạy do LOGO_TRACK_D quyết định: điểm đầu là mũi dưới-trái nên
 * shift tăng dần = đi từ dưới trái -> sang phải theo đáy -> ngược lên
 * cạnh phải -> vòng qua đỉnh -> đổ xuống cạnh trái.
 *
 * Vòng lặp CHỈ chạy khi active - lúc app rảnh logo đứng yên, không tốn
 * CPU dựng lại path 60 lần mỗi giây cho một hình không ai nhìn.
 */
import { memo, useRef } from 'react'
import { useGSAP } from '@gsap/react'
import gsap from 'gsap'
import { buildRibbon, parsePathPoints, renderSnakePath, type Ribbon } from '../lib/snakePath'
import { LOGO_BODY_D, LOGO_TEXT_D, LOGO_TRACK_D, LOGO_VIEWBOX, LOGO_WAVE_D } from './blueLogo'
import './AnimatedBlueLogo.css'

gsap.registerPlugin(useGSAP)

/** Thời gian bò trọn một vòng viền, tính bằng giây. */
export const LAP_DURATION = 2
/**
 * Nghỉ tại chỗ cũ bao lâu trước khi bò vòng tiếp. Để 0 vì đây là chỉ
 * báo bận: app còn chạy thì vòng chạy không được đứng khựng. Vẫn giữ
 * hằng số này vì trạng thái cuối vòng trùng khít trạng thái đầu, nên
 * chỉ cần đổi số là có quãng nghỉ mà không sai tư thế.
 */
export const HOLD_SECONDS = 0

/**
 * Đường chạy + toạ độ (s, v) của từng điểm dải sóng. Nặng nhất là hai
 * lượt chiếu (302 điểm x ~1700 mẫu), nên tính một lần rồi dùng lại -
 * logo header và màn khởi động dùng chung kết quả này.
 */
let ribbon: Ribbon | null = null
const getRibbon = () => {
  ribbon ??= buildRibbon(parsePathPoints(LOGO_WAVE_D), parsePathPoints(LOGO_TRACK_D))
  return ribbon
}

type AnimatedBlueLogoProps = {
  className?: string
  /** Vòng lặp chỉ chạy khi true; false đưa dải sóng về đúng hình gốc. */
  active: boolean
}

const AnimatedBlueLogo = memo(({ className = '', active }: AnimatedBlueLogoProps) => {
  const rootRef = useRef<HTMLDivElement>(null)

  useGSAP(
    () => {
      const wave = rootRef.current?.querySelector<SVGPathElement>('.animated-blue-logo__wave')
      if (!wave) return

      // GSAP không tự hoàn nguyên được `d` vì tween chạy trên một object
      // trung gian rồi ghi thẳng vào attribute, nên phải tự đặt lại.
      const restore = () => wave.setAttribute('d', LOGO_WAVE_D)

      if (!active) {
        restore()
        return
      }

      const mm = gsap.matchMedia()
      mm.add(
        {
          animate: '(prefers-reduced-motion: no-preference)',
          reduced: '(prefers-reduced-motion: reduce)',
        },
        (context) => {
          if (context.conditions?.reduced) {
            restore()
            return
          }

          const { track, anchors } = getRibbon()
          const crawl = { shift: 0 }
          gsap.to(crawl, {
            shift: track.perimeter,
            duration: LAP_DURATION,
            ease: 'none',
            repeat: -1,
            repeatDelay: HOLD_SECONDS,
            onUpdate: () => wave.setAttribute('d', renderSnakePath(anchors, track, crawl.shift)),
          })
          return restore
        },
      )
      return () => mm.revert()
    },
    // revertOnUpdate: mỗi lần active đổi giá trị, tween của lần chạy
    // TRƯỚC phải bị huỷ sạch trước khi nhánh mới ở trên chạy - thiếu cờ
    // này, tween lặp vô hạn cũ vẫn sống và ghi đè `d` ngay sau khi
    // nhánh active=false vừa đặt lại hình gốc.
    { scope: rootRef, dependencies: [active], revertOnUpdate: true },
  )

  return (
    <div ref={rootRef} className={`animated-blue-logo ${className}`} aria-label="Blue">
      <svg viewBox={LOGO_VIEWBOX} xmlns="http://www.w3.org/2000/svg">
        {/* Vẽ trước khối tím, đúng thứ tự trong file SVG gốc. */}
        <path
          className="animated-blue-logo__wave"
          fill="#28C5F2"
          fillRule="evenodd"
          d={LOGO_WAVE_D}
        />
        <path fill="#5453A1" fillRule="evenodd" d={LOGO_BODY_D} />
        <path fill="#FFFFFF" fillRule="evenodd" d={LOGO_TEXT_D} />
      </svg>
    </div>
  )
})

export default AnimatedBlueLogo
