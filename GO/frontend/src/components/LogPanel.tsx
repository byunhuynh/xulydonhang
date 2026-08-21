import { useEffect, useRef } from 'react'
import { FaTrashCan } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { SectionHeader } from './SectionHeader'

function lineAccent(line: string): string {
  // Order matters: "SAI GIÁ" carries the same ⚠️ marker as other,
  // lower-stakes warnings (e.g. "đã có 1 batch đang xử lý") — it needs
  // to read as danger (red), not just warning (amber), so it's checked
  // before the generic ⚠️/❌ fallbacks.
  if (line.includes('SAI GIÁ')) return 'border-danger/60 text-danger font-semibold'
  if (line.includes('❌')) return 'border-danger/60 text-danger'
  if (line.includes('Đúng giá')) return 'border-success/60 text-success'
  if (line.includes('⚠️')) return 'border-warning/60 text-warning'
  if (line.includes('🚀')) return 'border-accent/60 text-accent'
  return 'border-transparent text-ink'
}

export function LogPanel() {
  const logLines = useAppStore((s) => s.logLines)
  const clearLog = useAppStore((s) => s.clearLog)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [logLines])

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3.5">
      <SectionHeader
        index="03"
        title="Nhật ký hệ thống"
        action={
          <button
            onClick={clearLog}
            className="inline-flex items-center gap-1.5 text-xs font-medium text-muted transition-colors hover:text-danger"
          >
            <FaTrashCan /> Xóa nhật ký
          </button>
        }
      />
      <div className="selectable flex-1 overflow-auto rounded-lg border border-border bg-bg p-2.5 font-mono text-xs">
        {logLines.length === 0 && (
          <div className="flex h-full items-center justify-center text-muted">Chưa có hoạt động nào.</div>
        )}
        {logLines.map((entry, i) => (
          <div
            key={i}
            className={`flex gap-2.5 whitespace-pre-wrap border-l-2 py-0.5 pl-2.5 ${lineAccent(entry.text)}`}
          >
            <span className="flex-shrink-0 text-muted opacity-70">{entry.time}</span>
            <span>{entry.text}</span>
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </section>
  )
}
