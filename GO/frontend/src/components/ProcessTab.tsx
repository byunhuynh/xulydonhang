import { ControlPanel } from './ControlPanel'
import { FileListPanel } from './FileListPanel'
import { LogPanel } from './LogPanel'
import { ResultTable } from './ResultTable'

export function ProcessTab() {
  return (
    <div className="grid h-full grid-rows-[minmax(0,2fr)_minmax(0,3fr)] gap-4">
      <div className="grid grid-cols-[3fr_1fr] gap-4 overflow-hidden">
        <FileListPanel />
        <ControlPanel />
      </div>
      <div className="grid grid-rows-[1fr_1fr] gap-4 overflow-hidden">
        <LogPanel />
        <ResultTable />
      </div>
    </div>
  )
}
