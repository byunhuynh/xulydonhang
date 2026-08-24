import { useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { FaXmark, FaCopy, FaCheck, FaPaperPlane } from 'react-icons/fa6'
import type { OrderRow } from '../types'
import { buildZaloMessageForPO, buildZaloMessageForJITFile, type PriceBasis } from '../lib/zaloMessage'
import { markupToHtml } from '../lib/richtext'
import { useModalEntrance } from '../lib/useModalEntrance'

// A PO group is every OrderRow sharing one PO number - always length 1
// for every vendor except BigC, where one PDF can produce several rows
// (one per store page) that the real app still notifies as a single
// message (see buildZaloMessageForPO's own doc comment for why). JIT
// groups instead share one sourceId (one PDF, many DIFFERENT po per
// page) - `po` then holds the group's display label (the PDF's
// fileName, not a real po) and `period` carries the delivery period the
// user currently has selected for that file (see
// buildZaloMessageForJITFile's own doc comment for why this can't just
// read row.jitPeriod).
export interface POContentGroup {
  po: string
  rows: OrderRow[]
  period?: string
}

export function OrderContentModal({
  groups,
  processedAt,
  priceBasisBySku,
  onClose,
}: {
  groups: POContentGroup[]
  processedAt: string
  priceBasisBySku: Record<number, PriceBasis>
  onClose: () => void
}) {
  const [copiedPO, setCopiedPO] = useState<string | null>(null)
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef)
  const messages = groups.map((g) => ({
    po: g.po,
    text:
      g.rows[0]?.system === 'JIT-CHOICE'
        ? buildZaloMessageForJITFile(g.rows, g.period ?? g.rows[0]?.jitPeriod ?? '', processedAt)
        : buildZaloMessageForPO(g.rows, processedAt, priceBasisBySku),
  }))
  const isSingle = groups.length === 1
  // markupToHtml (lib/richtext.ts) dịch cùng cú pháp **/{color:}/list mà
  // ChromedpSender.SendMessage sẽ THẬT SỰ dán khi gửi (xem doc comment
  // đầu zaloMessage.ts) - preview vì vậy hiện đúng đậm/màu/list như tin
  // nhắn thật sẽ trông thế nào, không phải các ký tự ** trần trụi.
  const renderedMessages = useMemo(() => messages.map((m) => ({ po: m.po, html: markupToHtml(m.text) })), [messages])

  // Copy dạng HTML THẬT (không phải chuỗi markup thô "**...**") - dán
  // trực tiếp vào ô soạn tin Zalo (hay bất kỳ nơi nào hiểu clipboard
  // text/html, vd Word/Gmail) sẽ giữ đúng đậm/màu/list, đúng những gì
  // ChromedpSender tự paste khi gửi tự động. writeText thô trước đây
  // dán vào Zalo ra nguyên ký tự ** vì Zalo không hiểu cú pháp markup
  // riêng của app này khi gõ/dán dạng text thường. text/plain vẫn kèm
  // theo làm phương án dự phòng cho nơi không nhận text/html.
  async function copyRichHtml(html: string, plainText: string) {
    try {
      await navigator.clipboard.write([
        new ClipboardItem({
          'text/html': new Blob([html], { type: 'text/html' }),
          'text/plain': new Blob([plainText], { type: 'text/plain' }),
        }),
      ])
    } catch {
      // ClipboardItem/write có thể không sẵn có ở vài môi trường - vẫn
      // còn hơn không copy được gì, dù mất định dạng.
      await navigator.clipboard.writeText(plainText).catch(() => {})
    }
  }

  function handleCopy(po: string, html: string, text: string) {
    copyRichHtml(html, text)
    setCopiedPO(po)
    setTimeout(() => setCopiedPO((cur) => (cur === po ? null : cur)), 1200)
  }

  function handleCopyAll() {
    const html = renderedMessages.map((m) => m.html).join('<hr/>')
    const text = messages.map((m) => m.text).join('\n\n---\n\n')
    copyRichHtml(html, text)
    setCopiedPO('__all__')
    setTimeout(() => setCopiedPO((cur) => (cur === '__all__' ? null : cur)), 1200)
  }

  // Portal thẳng ra document.body: ProcessTab.tsx bọc nội dung trong các
  // div "animate-rise" (dùng CSS transform cho hiệu ứng trượt lên lúc
  // vào trang) — MỘT tổ tiên có transform sẽ trở thành containing block
  // MỚI cho mọi phần tử con "position: fixed", khiến popup này bị giới
  // hạn trong khung của div đó (chỉ rộng bằng vùng bảng kết quả) thay vì
  // phủ kín cả cửa sổ app như style fixed inset-0 vốn định làm - đúng
  // nguyên nhân phần đầu/cuối popup bị khuất. Portal ra ngoài toàn bộ
  // cây DOM đó là cách chuẩn để tránh lỗi này (LockOverlay/SettingsModal
  // không dính lỗi này vì chúng được mount ở App.tsx, ngoài ProcessTab).
  return createPortal(
    <div ref={backdropRef} className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-6" onClick={onClose}>
      <div
        ref={cardRef}
        className="flex max-h-[80vh] w-full max-w-lg flex-col rounded-xl border border-border bg-panel shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex shrink-0 items-center justify-between border-b border-border px-4 py-3">
          <div>
            <h3 className="text-sm font-bold text-ink">
              {isSingle ? `Nội dung tin nhắn — ${groups[0].po || groups[0].rows[0]?.fileName}` : `Nội dung tin nhắn — ${groups.length} đơn đã chọn`}
            </h3>
            <p className="text-[11px] text-muted">Xem trước tin nhắn Zalo sẽ gửi cho khách</p>
          </div>
          <button
            onClick={onClose}
            className="rounded p-1.5 text-muted transition-colors hover:bg-white/5 hover:text-ink"
          >
            <FaXmark size={16} />
          </button>
        </div>

        {/* Chat-style preview - a light dotted backdrop with each message
            rendered as an outgoing bubble in Zalo's own brand blue, so
            this reads as "what will actually appear in the chat" rather
            than a plain text dump. A PO that groups several rows (BigC's
            multi-store case) still renders as exactly one bubble - see
            buildZaloMessageForPO. */}
        <div
          className="min-h-0 flex-1 overflow-auto p-4"
          style={{
            backgroundColor: '#12141c',
            backgroundImage: 'radial-gradient(rgba(255,255,255,0.05) 1px, transparent 1px)',
            backgroundSize: '14px 14px',
          }}
        >
          {messages.map((m, idx) => (
            <div key={m.po || idx} className={idx > 0 ? 'mt-5' : ''}>
              {!isSingle && (
                <div className="mb-1.5 flex items-center gap-2">
                  <span className="rounded-full bg-white/10 px-2.5 py-0.5 font-mono text-[11px] font-semibold text-ink">
                    {m.po}
                  </span>
                  {groups[idx].rows.length > 1 && (
                    <span className="text-[10px] text-muted">gộp {groups[idx].rows.length} dòng → 1 tin nhắn</span>
                  )}
                </div>
              )}
              <div className="flex justify-end">
                <div
                  className="selectable max-w-[88%] break-words rounded-2xl rounded-br-sm px-3.5 py-3 text-[13px] leading-relaxed text-white shadow-md [&_ol]:list-decimal [&_ol]:pl-5 [&_ul]:list-disc [&_ul]:pl-5 [&_li]:mb-1"
                  style={{ backgroundColor: '#0068FF' }}
                  dangerouslySetInnerHTML={{ __html: renderedMessages[idx].html }}
                />
              </div>
              <div className="mt-1 flex items-center justify-end gap-1.5 pr-1 text-[10px] text-muted">
                <FaPaperPlane size={9} />
                {processedAt || 'vừa xong'}
              </div>
              {!isSingle && (
                <div className="mt-1 flex justify-end">
                  <button
                    onClick={() => handleCopy(m.po, renderedMessages[idx].html, m.text)}
                    className={`inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-[11px] font-semibold transition-colors ${
                      copiedPO === m.po
                        ? 'border-success/50 bg-success/10 text-success'
                        : 'border-border text-muted hover:border-accent hover:text-accent'
                    }`}
                  >
                    {copiedPO === m.po ? <FaCheck size={9} /> : <FaCopy size={9} />}
                    {copiedPO === m.po ? 'Đã copy' : 'Copy'}
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>

        <div className="flex shrink-0 items-center justify-end gap-2 border-t border-border px-4 py-3">
          <button
            onClick={
              isSingle
                ? () => handleCopy(messages[0].po, renderedMessages[0]?.html ?? '', messages[0].text)
                : handleCopyAll
            }
            className={`inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs font-semibold transition-colors ${
              copiedPO === (isSingle ? messages[0]?.po : '__all__')
                ? 'border-success/50 bg-success/10 text-success'
                : 'border-border text-ink hover:border-accent hover:text-accent'
            }`}
          >
            {copiedPO === (isSingle ? messages[0]?.po : '__all__') ? <FaCheck size={11} /> : <FaCopy size={11} />}
            {copiedPO === (isSingle ? messages[0]?.po : '__all__') ? 'Đã copy' : isSingle ? 'Copy nội dung' : 'Copy tất cả'}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
