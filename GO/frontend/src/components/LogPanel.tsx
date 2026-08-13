import { useEffect, useRef } from 'react'
import { FaTrashCan } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'

export function LogPanel() {
  const logLines = useAppStore((s) => s.logLines)
  const clearLog = useAppStore((s) => s.clearLog)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [logLines])

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-muted">3. Nhật ký hệ thống</h2>
        <button
          onClick={clearLog}
          className="inline-flex items-center gap-1 text-xs text-muted hover:text-accent"
        >
          <FaTrashCan /> Xóa nhật ký
        </button>
      </div>
      <div className="selectable flex-1 overflow-auto rounded-lg border border-border bg-bg p-2 font-mono text-xs text-ink">
        {logLines.map((line, i) => (
          <div key={i} className="whitespace-pre-wrap py-0.5">
            {line}
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </section>
  )
}
