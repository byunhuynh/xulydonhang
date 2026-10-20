/**
 * Màn hình chờ lúc mới mở app: logo "Blue" với dải sóng cyan bò vòng
 * quanh khối tím, kèm dòng chữ trạng thái.
 *
 * Phần bò vòng nằm trong AnimatedBlueLogo (dùng chung với logo trên
 * header lúc app đang xử lý đơn); ở đây chỉ thêm phần mở màn, nhịp thở
 * và ba dấu chấm.
 */
import { memo, useRef } from 'react'
import { useGSAP } from '@gsap/react'
import gsap from 'gsap'
import AnimatedBlueLogo from './AnimatedBlueLogo'
import './StartupLogoLoader.css'

gsap.registerPlugin(useGSAP)

type StartupLogoLoaderProps = {
  label?: string
}

const StartupLogoLoader = memo(({ label = 'Đang tải dữ liệu' }: StartupLogoLoaderProps) => {
  const rootRef = useRef<HTMLDivElement>(null)

  useGSAP(
    () => {
      const mm = gsap.matchMedia()
      mm.add(
        {
          animate: '(prefers-reduced-motion: no-preference)',
          reduced: '(prefers-reduced-motion: reduce)',
        },
        (context) => {
          if (context.conditions?.reduced) {
            gsap.set('.startup-logo__mark', { autoAlpha: 1 })
            return
          }

          gsap
            .timeline()
            .from('.startup-logo__mark', {
              autoAlpha: 0,
              scale: 0.9,
              transformOrigin: '50% 50%',
              duration: 0.7,
              ease: 'power3.out',
            })
            // Nhịp thở nhẹ, bắt đầu sau khi logo vào xong.
            .to(
              '.startup-logo__mark',
              {
                scale: 1.02,
                transformOrigin: '50% 50%',
                duration: 2.2,
                ease: 'sine.inOut',
                yoyo: true,
                repeat: -1,
              },
              0.7,
            )

          gsap.to('.startup-logo__dot', {
            y: -3,
            autoAlpha: 1,
            duration: 0.42,
            ease: 'sine.inOut',
            yoyo: true,
            repeat: -1,
            stagger: { each: 0.14 },
          })
        },
        rootRef,
      )
      return () => mm.revert()
    },
    { scope: rootRef },
  )

  return (
    <div ref={rootRef} className="startup-logo">
      <AnimatedBlueLogo active className="startup-logo__mark" />
      <p className="startup-logo__label">
        {label}
        <span className="startup-logo__dots" aria-hidden="true">
          <span className="startup-logo__dot">.</span>
          <span className="startup-logo__dot">.</span>
          <span className="startup-logo__dot">.</span>
        </span>
      </p>
    </div>
  )
})

export default StartupLogoLoader
