import { useRef } from 'react'
import { FaLock, FaRotate, FaPowerOff } from 'react-icons/fa6'
import { Quit } from '../../wailsjs/runtime/runtime'
import type { LockStatus } from '../store/appStore'
import { useModalEntrance } from '../lib/useModalEntrance'

interface LockOverlayProps {
  status: Exclude<LockStatus, 'unlocked'>
}

// Blocks all interaction with the app - no click-outside-to-dismiss
// (unlike SettingsModal). Two distinct messages: a confirmed expiry
// ("locked") vs a genuinely undetermined status ("checking" - first
// check still pending, or a later check failed and is being retried).
// Deliberately not the same message, since telling a user "hết hạn"
// when the real cause is a network hiccup would be misleading.
//
// Includes its own "Đóng ứng dụng" button: this overlay's z-[100]
// paints over TitleBar's own close button (TitleBar has no explicit
// z-index, so it always loses to this fixed+z-indexed layer), so
// without a button here there would be no way to quit the app at all
// while locked/checking.
export function LockOverlay({ status }: LockOverlayProps) {
  const isLocked = status === 'locked'
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef)

  return (
    <div ref={backdropRef} className="fixed inset-0 z-[100] flex items-center justify-center bg-bg/95">
      <div ref={cardRef} className="mx-4 flex max-w-md flex-col items-center gap-4 rounded-xl border border-border bg-panel p-8 text-center">
        {isLocked ? (
          <FaLock size={28} className="text-danger" />
        ) : (
          <FaRotate size={28} className="animate-spin text-accent" />
        )}
        <h2 className="font-sans text-base font-bold text-ink">
          {isLocked ? 'App đã hết hạn sử dụng' : 'Đang xác minh tình trạng sử dụng...'}
        </h2>
        <p className="font-sans text-sm text-muted">
          {isLocked
            ? 'Vui lòng liên hệ quản trị viên để gia hạn.'
            : 'Không xác minh được tình trạng sử dụng (có thể do mất kết nối mạng). Đang tự động thử lại...'}
        </p>
        <button
          type="button"
          onClick={Quit}
          className="mt-2 inline-flex items-center gap-2 rounded-lg border border-border px-4 py-2 font-sans text-xs font-semibold text-muted transition-colors hover:border-danger hover:text-danger"
        >
          <FaPowerOff size={12} /> Đóng ứng dụng
        </button>
      </div>
    </div>
  )
}
