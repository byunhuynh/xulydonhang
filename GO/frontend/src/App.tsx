import { useState } from 'react'
import { FaGears, FaCircleInfo } from 'react-icons/fa6'
import { ProcessTab } from './components/ProcessTab'
import { InfoTab } from './components/InfoTab'
import { useWailsEvents } from './hooks/useWailsEvents'

type TabKey = 'process' | 'info'

function App() {
  useWailsEvents()
  const [tab, setTab] = useState<TabKey>('process')

  return (
    <div className="flex h-screen flex-col">
      <header className="flex items-center gap-1 border-b border-border bg-panel px-4 pt-3">
        <TabButton active={tab === 'process'} onClick={() => setTab('process')}>
          <FaGears /> Xử lý Đơn hàng
        </TabButton>
        <TabButton active={tab === 'info'} onClick={() => setTab('info')}>
          <FaCircleInfo /> Thông tin
        </TabButton>
      </header>
      <main className="flex-1 overflow-hidden p-4">
        {tab === 'process' ? <ProcessTab /> : <InfoTab />}
      </main>
      <footer className="border-t border-border px-4 py-2 text-center text-xs text-muted">
        © 2026 Blue Hà Thành. All rights reserved.
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
      className={`inline-flex items-center gap-2 rounded-t-lg px-4 py-2 text-sm font-medium transition-colors ${
        active ? 'bg-bg text-accent' : 'text-muted hover:text-ink'
      }`}
    >
      {children}
    </button>
  )
}

export default App
