import { ControlPanel } from './ControlPanel'
import { FileListPanel } from './FileListPanel'
import { LogPanel } from './LogPanel'
import { ResultTable } from './ResultTable'

export function ProcessTab() {
  return (
    <div className="grid h-full grid-rows-[minmax(0,2fr)_minmax(0,3fr)] gap-4">
      <div className="grid grid-cols-[3fr_1fr] gap-4 overflow-hidden">
        <div className="animate-rise h-full min-h-0 [animation-delay:0ms]">
          <FileListPanel />
        </div>
        <div className="animate-rise h-full min-h-0 [animation-delay:60ms]">
          <ControlPanel />
        </div>
      </div>
      <div className="grid grid-rows-[1fr_1fr] gap-4 overflow-hidden">
        <div className="animate-rise h-full min-h-0 [animation-delay:120ms]">
          <LogPanel />
        </div>
        <div className="animate-rise h-full min-h-0 [animation-delay:180ms]">
          <ResultTable />
        </div>
      </div>
    </div>
  )
}
