import { useEffect, useRef } from 'react'
import { FaTrashCan, FaUpRightAndDownLeftFromCenter, FaDownLeftAndUpRightToCenter } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { SectionHeader } from './SectionHeader'
import { useListEntrance } from '../lib/useListEntrance'

type Accent = 'danger' | 'success' | 'warning' | 'accent'

const accentClasses: Record<Accent, { border: string; badge: string }> = {
  danger: { border: 'border-danger/50', badge: 'bg-danger/15 text-danger' },
  success: { border: 'border-success/50', badge: 'bg-success/15 text-success' },
  warning: { border: 'border-warning/50', badge: 'bg-warning/15 text-warning' },
  accent: { border: 'border-accent/50', badge: 'bg-accent/15 text-accent' },
}

type Segment =
  | { kind: 'status'; start: number; end: number; accent: Accent }
  | { kind: 'sku' | 'promo' | 'date'; start: number; end: number }

// buildSegments finds the short, already-known-format pieces worth
// highlighting inside a log line and returns their [start, end) spans, in
// order. Only a FEW things get highlighted - the surrounding text
// (product name, numbers, file names, raw error detail after a colon)
// stays in the normal ink color, so the line doesn't turn into a wall of
// pills. Order matters: "SAI GIÁ" carries the same ⚠️ marker as other,
// lower-stakes warnings (e.g. "đã có 1 batch đang xử lý") - it must be
// checked before the generic ⚠️/❌ leading-emoji fallbacks so it reads as
// danger, not warning.
function buildSegments(text: string): Segment[] {
  const segments: Segment[] = []

  const saiGia = '⚠️ SAI GIÁ!'
  const saiGiaIdx = text.indexOf(saiGia)
  const dungGia = 'Đúng giá'
  const dungGiaIdx = saiGiaIdx === -1 ? text.indexOf(dungGia) : -1

  // Both cases are formatSkuLogLine's output (processor_shared.go):
  // "<sku>[ <productName>] — Đúng giá[, KM: <promo> (áp dụng <range>)]" or
  // "<sku>[ <productName>] — ⚠️ SAI GIÁ! ...[, đã thử KM: <promo> (áp dụng <range>)]"
  // - only these two shapes carry a mã hàng / CTKM / thời gian áp dụng to tag.
  let isPriceCheckLine = false
  if (saiGiaIdx !== -1) {
    segments.push({ kind: 'status', start: saiGiaIdx, end: saiGiaIdx + saiGia.length, accent: 'danger' })
    isPriceCheckLine = true
  } else if (dungGiaIdx !== -1) {
    segments.push({ kind: 'status', start: dungGiaIdx, end: dungGiaIdx + dungGia.length, accent: 'success' })
    isPriceCheckLine = true
  } else {
    // Every other real log line puts its marker emoji at the very start,
    // followed by a short human phrase and, often, ": <raw technical
    // detail>" - only the short phrase (up to the colon, or the whole
    // line when there's no colon) is worth highlighting.
    const leadingAccent: [string, Accent][] = [
      ['❌', 'danger'],
      ['⚠️', 'warning'],
      ['🚀', 'accent'],
      ['✅', 'success'],
    ]
    for (const [emoji, accent] of leadingAccent) {
      if (text.startsWith(emoji)) {
        const colonIdx = text.indexOf(':')
        const end = colonIdx === -1 ? text.length : colonIdx
        segments.push({ kind: 'status', start: 0, end, accent })
        break
      }
    }
  }

  if (isPriceCheckLine) {
    // mã hàng: the SKU is always the line's first token - it never
    // contains a space, unlike the product name that may follow it.
    const spaceIdx = text.indexOf(' ')
    if (spaceIdx > 0) {
      segments.push({ kind: 'sku', start: 0, end: spaceIdx })
    }

    // CTKM (promo name) + thời gian áp dụng (its date range), when
    // present: ", KM: <promo> (áp dụng <range>)" or ", đã thử KM: <promo>
    // (áp dụng <range>)" - the "(áp dụng ...)" suffix is optional on its
    // own (a promo can match with no printed date range).
    const kmIdx = text.indexOf('KM: ')
    if (kmIdx !== -1) {
      const promoStart = kmIdx + 'KM: '.length
      const apDungMarker = ' (áp dụng '
      const apDungIdx = text.indexOf(apDungMarker, promoStart)
      const promoEnd = apDungIdx === -1 ? text.length : apDungIdx
      if (promoEnd > promoStart) {
        segments.push({ kind: 'promo', start: promoStart, end: promoEnd })
      }
      if (apDungIdx !== -1) {
        const dateStart = apDungIdx + apDungMarker.length
        const closeIdx = text.indexOf(')', dateStart)
        if (closeIdx !== -1) {
          segments.push({ kind: 'date', start: dateStart, end: closeIdx })
        }
      }
    }
  }

  return segments
}

function LogLine({ time, text }: { time: string; text: string }) {
  // sku (start 0) is pushed before status (whose span sits later in the
  // string, after " — "), so segments must be re-sorted by position
  // before the sequential slice-and-render walk below.
  const segments = buildSegments(text).sort((a, b) => a.start - b.start)
  const status = segments.find((s): s is Segment & { kind: 'status' } => s.kind === 'status')
  const border = status ? accentClasses[status.accent].border : 'border-transparent'

  const nodes: React.ReactNode[] = []
  let cursor = 0
  segments.forEach((seg, i) => {
    if (seg.start > cursor) nodes.push(text.slice(cursor, seg.start))
    const chunk = text.slice(seg.start, seg.end)
    switch (seg.kind) {
      case 'status':
        nodes.push(
          <b key={i} className={`rounded px-1.5 font-sans font-bold ${accentClasses[seg.accent].badge}`}>
            {chunk}
          </b>,
        )
        break
      case 'sku':
        // Bolded, not boxed - already stands out by being first on the
        // line, so a full badge here would just add visual weight.
        nodes.push(
          <b key={i} className="font-sans text-ink">
            {chunk}
          </b>,
        )
        break
      case 'promo':
        // CTKM gets the one new pill color (brandPurple) - the actual
        // "what promotion applied" fact the user asked to see at a glance.
        nodes.push(
          <span key={i} className="rounded bg-brandPurple/15 px-1.5 font-sans font-semibold text-brandPurple">
            {chunk}
          </span>,
        )
        break
      case 'date':
        // Supplementary metadata - styled, not boxed, so three colored
        // pills in a row don't turn the line into visual noise.
        nodes.push(
          <span key={i} className="font-sans italic text-muted">
            {chunk}
          </span>,
        )
        break
    }
    cursor = seg.end
  })
  if (cursor < text.length) nodes.push(text.slice(cursor))

  return (
    <div data-log-line className={`flex gap-2.5 whitespace-pre-wrap border-l-2 py-0.5 pl-2.5 text-ink ${border}`}>
      <span className="flex-shrink-0 text-muted opacity-70">{time}</span>
      <span>{nodes}</span>
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
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [logLines])

  useListEntrance(listRef, '[data-log-line]', logLines.length)

  return (
    <section
      // Thu gọn = CHIỀU CAO CỐ ĐỊNH, không phải chiều cao theo nội dung.
      // Bản cũ dùng flex-none + max-h ở danh sách bên trong: ở cửa sổ thấp
      // (1366×768) mức chặn đó còn LỚN HƠN phần panel vốn được chia khi cân
      // bằng, nên bấm "Mở rộng" panel kia gần như không nhả ra chỗ nào —
      // đo được đúng +17px, và panel này còn tự nhỏ đi 23px khi tự bung.
      className={`flex min-h-0 flex-col rounded-xl border border-border bg-panel p-3.5 ${
        size === 'compact' ? 'h-[130px] flex-none' : 'flex-1'
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
        ref={listRef}
        // Không còn max-h riêng cho compact: chiều cao section đã cố định
        // nên danh sách chỉ việc lấp phần còn lại (min-h-0 để co được và
        // cuộn thay vì đẩy phồng section).
        className="selectable min-h-0 flex-1 overflow-auto rounded-lg border border-border bg-bg p-2.5 font-mono text-xs"
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
