import { FaLock, FaRotate } from 'react-icons/fa6'
import type { LockStatus } from '../store/appStore'

interface LockOverlayProps {
  status: Exclude<LockStatus, 'unlocked'>
}

// Blocks all interaction with the app - no close button, no
// click-outside-to-dismiss (unlike SettingsModal). Two distinct
// messages: a confirmed expiry ("locked") vs a genuinely undetermined
// status ("checking" - first check still pending, or a later check
// failed and is being retried). Deliberately not the same message,
// since telling a user "hết hạn" when the real cause is a network
// hiccup would be misleading.
export function LockOverlay({ status }: LockOverlayProps) {
  const isLocked = status === 'locked'

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-bg/95">
      <div className="mx-4 flex max-w-md flex-col items-center gap-4 rounded-xl border border-border bg-panel p-8 text-center">
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
      </div>
    </div>
  )
}
