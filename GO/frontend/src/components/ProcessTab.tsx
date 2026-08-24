import { useState } from 'react'
import { ControlPanel } from './ControlPanel'
import { FileListPanel } from './FileListPanel'
import { LogPanel } from './LogPanel'
import { ResultTable } from './ResultTable'

type SidePanel = 'balanced' | 'file' | 'log'

export function ProcessTab() {
  const [sidePanel, setSidePanel] = useState<SidePanel>('balanced')

  function toggle(which: Exclude<SidePanel, 'balanced'>) {
    setSidePanel((cur) => (cur === which ? 'balanced' : which))
  }

  return (
    <div className="flex h-full flex-col gap-4">
      <div className="animate-rise [animation-delay:0ms]">
        <ControlPanel />
      </div>
      <div
        className={`grid flex-1 gap-4 overflow-hidden transition-all ${
          sidePanel === 'log' ? 'grid-cols-[1fr_440px]' : 'grid-cols-[1fr_300px]'
        }`}
      >
        <div className="animate-rise h-full min-h-0 min-w-0 [animation-delay:60ms]">
          <ResultTable />
        </div>
        <div className="animate-rise flex h-full min-h-0 flex-col gap-4 [animation-delay:120ms]">
          <FileListPanel
            size={sidePanel === 'file' ? 'expanded' : sidePanel === 'log' ? 'compact' : 'balanced'}
            onToggleExpand={() => toggle('file')}
          />
          <LogPanel
            size={sidePanel === 'log' ? 'expanded' : sidePanel === 'file' ? 'compact' : 'balanced'}
            onToggleExpand={() => toggle('log')}
          />
        </div>
      </div>
    </div>
  )
}
