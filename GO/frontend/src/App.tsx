import { useEffect, useRef, useState } from 'react'
import { FaGears, FaCircleInfo, FaGear } from 'react-icons/fa6'
import { ProcessTab } from './components/ProcessTab'
import { InfoTab } from './components/InfoTab'
import { SettingsModal } from './components/SettingsModal'
import { TitleBar } from './components/TitleBar'
import { LockOverlay } from './components/LockOverlay'
import { ZaloQRModal } from './components/ZaloQRModal'
import { TMDTMissingModal } from './components/TMDTMissingModal'
import AnimatedBlueLogo from './components/AnimatedBlueLogo'
import StartupLogoLoader from './components/StartupLogoLoader'
import { useWailsEvents } from './hooks/useWailsEvents'
import { useAppStore } from './store/appStore'
import { formatBatchProgress } from './lib/batchProgress'
import { InitializeApp } from '../wailsjs/go/main/App'

type TabKey = 'process' | 'info'

function App() {
  useWailsEvents()
  const [tab, setTab] = useState<TabKey>('process')
  const isProcessing = useAppStore((s) => s.isProcessing)
  const batchProgress = useAppStore((s) => s.batchProgress)
  const lockStatus = useAppStore((s) => s.lockStatus)
  const [isSettingsOpen, setIsSettingsOpen] = useState(false)
  // Chuỗi rỗng khi lô chưa công bố kích thước - thanh trạng thái khi đó
  // vẫn chỉ nói "Đang xử lý" như trước chứ không hiện "0/0 file".
  const progressLabel = formatBatchProgress(batchProgress)
  const [startupState, setStartupState] = useState<'loading' | 'ready' | 'error'>('loading')
  const startupStarted = useRef(false)

  const initialize = () => {
    setStartupState('loading')
    InitializeApp()
      .then(() => setStartupState('ready'))
      .catch(() => setStartupState('error'))
  }

  useEffect(() => {
    if (startupStarted.current) return
    startupStarted.current = true
    initialize()
  }, [])

  return (
    <div className="relative flex h-screen flex-col">
      <TitleBar />
      {/* Vắt qua ranh giới title bar / header. wails-drag để kéo cửa sổ
          bằng vùng logo vẫn được như phần còn lại của title bar. */}
      <AnimatedBlueLogo active={isProcessing} className="brand-logo wails-drag" />
      <header className="border-b border-border bg-panel/60 px-4">
        {/* Cả hàng được NÂNG lên nằm vắt qua ranh giới title bar giống
            logo, để logo, tên app và tab cùng canh giữa trên một đường
            thay vì logo ở trên còn tab tụt xuống dưới. brand-row cũng
            chừa sẵn lề phải cho 3 nút cửa sổ ở góc title bar. */}
        <div className="brand-row relative z-10 flex items-center gap-1 pb-4">
        <div className="mr-4 flex items-center gap-2.5">
          <div className="brand-logo-slot" aria-hidden />
          <div className="flex flex-col leading-[1.15]">
            <span className="text-sm font-extrabold text-ink">Blue Hà Thành</span>
            <span className="font-mono text-[9.5px] tracking-wider text-muted">ORDER SYSTEM · V3.0</span>
          </div>
        </div>
        <TabButton active={tab === 'process'} onClick={() => setTab('process')}>
          <FaGears /> Xử lý Đơn hàng
        </TabButton>
        <TabButton active={tab === 'info'} onClick={() => setTab('info')}>
          <FaCircleInfo /> Thông tin
        </TabButton>
        <button
          type="button"
          onClick={() => setIsSettingsOpen(true)}
          className="ml-2 rounded-lg border border-border p-2 text-muted transition-colors hover:border-accent hover:text-accent"
          title="Cấu hình"
        >
          <FaGear size={14} />
        </button>
        <div className="ml-auto flex items-center gap-2 rounded-full border border-border px-3 py-1.5 font-mono text-[11px] text-muted">
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              isProcessing ? 'animate-pulse bg-accent shadow-[0_0_8px_theme(colors.accent)]' : 'bg-success shadow-[0_0_8px_theme(colors.success)]'
            }`}
          />
          {isProcessing ? (
            <>
              Đang xử lý
              {progressLabel && <span className="tabular-nums text-accent">{progressLabel}</span>}
            </>
          ) : (
            'Sẵn sàng'
          )}
        </div>
        </div>
      </header>
      <main className="flex-1 overflow-hidden p-4">
        {tab === 'process' ? <ProcessTab /> : <InfoTab />}
      </main>
      <footer className="border-t border-border px-4 py-2 text-center text-xs text-muted">
        © 2026 HUỲNH ĐẠT THÀNH. All rights reserved
      </footer>
      {isSettingsOpen && <SettingsModal onClose={() => setIsSettingsOpen(false)} />}
      {lockStatus !== 'unlocked' && <LockOverlay status={lockStatus} />}
      <ZaloQRModal />
      {/* Cạnh ZaloQRModal chứ không nằm trong ControlPanel: modal này do sự
          kiện backend điều khiển nên phải sống độc lập với tab đang mở. */}
      <TMDTMissingModal />
      {startupState !== 'ready' && (
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-bg/95 backdrop-blur-sm">
          <div className="w-[min(420px,calc(100vw-32px))] rounded-2xl border border-border bg-panel p-8 text-center shadow-2xl">
            {startupState === 'loading' ? (
              <StartupLogoLoader />
            ) : (
              <>
                <p className="text-base font-semibold text-danger">Không tải được dữ liệu</p>
                <p className="mt-2 text-sm text-muted">Vui lòng kiểm tra kết nối mạng rồi thử lại.</p>
                <button
                  type="button"
                  onClick={initialize}
                  className="mt-6 rounded-lg bg-accent px-5 py-2.5 text-sm font-semibold text-bg transition-opacity hover:opacity-90"
                >
                  Thử lại
                </button>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      className={`inline-flex items-center gap-2 rounded-t-lg border-b-2 px-4 py-2.5 text-sm font-semibold transition-colors ${
        active
          ? 'border-accent bg-bg text-accent'
          : 'border-transparent text-muted hover:text-ink'
      }`}
    >
      {children}
    </button>
  )
}

export default App
