import { FileListPanel } from './FileListPanel'

function Placeholder({ label }: { label: string }) {
  return (
    <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-border text-sm text-muted">
      {label}
    </div>
  )
}

export function ProcessTab() {
  return (
    <div className="grid h-full grid-rows-[minmax(0,2fr)_minmax(0,3fr)] gap-4">
      <div className="grid grid-cols-[3fr_1fr] gap-4 overflow-hidden">
        <FileListPanel />
        <Placeholder label="ControlPanel (Task 11)" />
      </div>
      <div className="grid grid-rows-[1fr_1fr] gap-4 overflow-hidden">
        <Placeholder label="LogPanel (Task 9)" />
        <Placeholder label="ResultTable (Task 10)" />
      </div>
    </div>
  )
}
