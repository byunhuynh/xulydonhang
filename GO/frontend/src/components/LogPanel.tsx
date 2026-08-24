import { useEffect, useRef } from 'react'
import { FaTrashCan, FaUpRightAndDownLeftFromCenter, FaDownLeftAndUpRightToCenter } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { SectionHeader } from './SectionHeader'

type Accent = 'danger' | 'success' | 'warning' | 'accent'

const accentClasses: Record<Accent, { border: string; badge: string }> = {
  danger: { border: 'border-danger/50', badge: 'bg-danger/15 text-danger' },
  success: { border: 'border-success/50', badge: 'bg-success/15 text-success' },
  warning: { border: 'border-warning/50', badge: 'bg-warning/15 text-warning' },
  accent: { border: 'border-accent/50', badge: 'bg-accent/15 text-accent' },
}

// markerSpan finds the short, already-known-format marker phrase inside a
// log line and returns its [start, end) span, or null if none matches.
// Only the FEW important words get highlighted (badge + color) - the
// surrounding text (SKU, product name, numbers, file names, raw error
// detail after a colon) stays in the normal ink color. Order matters:
// "SAI GIÁ" carries the same ⚠️ marker as other, lower-stakes warnings
// (e.g. "đã có 1 batch đang xử lý") - it must be checked before the
// generic ⚠️/❌ leading-emoji fallbacks so it reads as danger, not warning.
function markerSpan(text: string): { start: number; end: number; accent: Accent } | null {
  const saiGia = '⚠️ SAI GIÁ!'
  const saiGiaIdx = text.indexOf(saiGia)
  if (saiGiaIdx !== -1) {
    return { start: saiGiaIdx, end: saiGiaIdx + saiGia.length, accent: 'danger' }
  }

  const dungGia = 'Đúng giá'
  const dungGiaIdx = text.indexOf(dungGia)
  if (dungGiaIdx !== -1) {
    return { start: dungGiaIdx, end: dungGiaIdx + dungGia.length, accent: 'success' }
  }

  // Every other real log line puts its marker emoji at the very start,
  // followed by a short human phrase and, often, ": <raw technical
  // detail>" - only the short phrase (up to the colon, or the whole line
  // when there's no colon) is worth highlighting.
  const leadingAccent: [string, Accent][] = [
    ['❌', 'danger'],
    ['⚠️', 'warning'],
    ['🚀', 'accent'],
  ]
  for (const [emoji, accent] of leadingAccent) {
    if (text.startsWith(emoji)) {
      const colonIdx = text.indexOf(':')
      const end = colonIdx === -1 ? text.length : colonIdx
      return { start: 0, end, accent }
    }
  }

  return null
}

function LogLine({ time, text }: { time: string; text: string }) {
  const marker = markerSpan(text)
  const border = marker ? accentClasses[marker.accent].border : 'border-transparent'

  return (
    <div className={`flex gap-2.5 whitespace-pre-wrap border-l-2 py-0.5 pl-2.5 text-ink ${border}`}>
      <span className="flex-shrink-0 text-muted opacity-70">{time}</span>
      <span>
        {marker ? (
          <>
            {text.slice(0, marker.start)}
            <b className={`rounded px-1.5 font-sans font-bold ${accentClasses[marker.accent].badge}`}>
              {text.slice(marker.start, marker.end)}
            </b>
            {text.slice(marker.end)}
          </>
        ) : (
          text
        )}
      </span>
    </div>
  )
}

export function LogPanel({
  size,
  onToggleExpand,
}: {
  size: 'balanced' | 'expanded' | 'compact'
  onToggleExpand: () => void
}) {
  const logLines = useAppStore((s) => s.logLines)
  const clearLog = useAppStore((s) => s.clearLog)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [logLines])

  return (
    <section
      className={`flex min-h-0 flex-col rounded-xl border border-border bg-panel p-3.5 transition-all ${
        size === 'compact' ? 'flex-none' : 'flex-1'
      }`}
    >
      <SectionHeader
        index="03"
        title="Nhật ký hệ thống"
        action={
          <div className="flex items-center gap-1">
            <button
              onClick={clearLog}
              title="Xóa nhật ký"
              className="rounded p-1 text-muted transition-colors hover:text-danger"
            >
              <FaTrashCan size={11} />
            </button>
            <button
              onClick={onToggleExpand}
              title={size === 'expanded' ? 'Thu gọn' : 'Mở rộng'}
              className="rounded p-1 text-muted transition-colors hover:text-accent"
            >
              {size === 'expanded' ? (
                <FaDownLeftAndUpRightToCenter size={11} />
              ) : (
                <FaUpRightAndDownLeftFromCenter size={11} />
              )}
            </button>
          </div>
        }
      />
      <div
        className={`selectable flex-1 overflow-auto rounded-lg border border-border bg-bg p-2.5 font-mono text-xs ${
          size === 'compact' ? 'max-h-[150px]' : ''
        }`}
      >
        {logLines.length === 0 && (
          <div className="flex h-full items-center justify-center text-muted">Chưa có hoạt động nào.</div>
        )}
        {logLines.map((entry, i) => (
          <LogLine key={i} time={entry.time} text={entry.text} />
        ))}
        <div ref={bottomRef} />
      </div>
    </section>
  )
}
