import { useState } from 'react'
import { FaGears, FaCircleInfo } from 'react-icons/fa6'
import { ProcessTab } from './components/ProcessTab'
import { InfoTab } from './components/InfoTab'
import { useWailsEvents } from './hooks/useWailsEvents'
import { useAppStore } from './store/appStore'

type TabKey = 'process' | 'info'

function App() {
  useWailsEvents()
  const [tab, setTab] = useState<TabKey>('process')
  const isProcessing = useAppStore((s) => s.isProcessing)

  return (
    <div className="flex h-screen flex-col">
      <header className="flex items-center gap-1 border-b border-border bg-panel/60 px-4 pt-3">
        <div className="mr-4 flex items-center gap-2.5 pb-3.5">
          <img
            src="/logo.svg"
            alt="Blue Hà Thành"
            className="no-drag h-[26px] w-auto drop-shadow-[0_0_12px_rgba(40,197,242,0.4)]"
            draggable={false}
          />
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
        <div className="ml-auto mb-3.5 flex items-center gap-2 rounded-full border border-border px-3 py-1.5 font-mono text-[11px] text-muted">
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              isProcessing ? 'animate-pulse bg-accent shadow-[0_0_8px_theme(colors.accent)]' : 'bg-success shadow-[0_0_8px_theme(colors.success)]'
            }`}
          />
          {isProcessing ? 'Đang xử lý...' : 'Sẵn sàng'}
        </div>
      </header>
      <main className="flex-1 overflow-hidden p-4">
        {tab === 'process' ? <ProcessTab /> : <InfoTab />}
      </main>
      <footer className="border-t border-border px-4 py-2 text-center text-xs text-muted">
        © 2026 HUỲNH ĐẠT THÀNH. All rights reserved
      </footer>
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
