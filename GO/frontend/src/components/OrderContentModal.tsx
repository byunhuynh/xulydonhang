import { useState } from 'react'
import { FaXmark, FaCopy, FaCheck, FaPaperPlane } from 'react-icons/fa6'
import type { OrderRow } from '../types'
import { buildZaloMessageForPO, type PriceBasis } from '../lib/zaloMessage'

// A PO group is every OrderRow sharing one PO number - always length 1
// for every vendor except BigC, where one PDF can produce several rows
// (one per store page) that the real app still notifies as a single
// message (see buildZaloMessageForPO's own doc comment for why).
export interface POContentGroup {
  po: string
  rows: OrderRow[]
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
  const messages = groups.map((g) => ({ po: g.po, text: buildZaloMessageForPO(g.rows, processedAt, priceBasisBySku) }))
  const isSingle = groups.length === 1

  function handleCopy(po: string, text: string) {
    navigator.clipboard.writeText(text).catch(() => {})
    setCopiedPO(po)
    setTimeout(() => setCopiedPO((cur) => (cur === po ? null : cur)), 1200)
  }

  function handleCopyAll() {
    navigator.clipboard.writeText(messages.map((m) => m.text).join('\n\n---\n\n')).catch(() => {})
    setCopiedPO('__all__')
    setTimeout(() => setCopiedPO((cur) => (cur === '__all__' ? null : cur)), 1200)
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-6" onClick={onClose}>
      <div
        className="flex max-h-[80vh] w-full max-w-lg flex-col rounded-xl border border-border bg-panel shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-4 py-3">
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
          className="flex-1 overflow-auto p-4"
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
                  className="selectable max-w-[88%] whitespace-pre-wrap break-words rounded-2xl rounded-br-sm px-3.5 py-3 text-[13px] leading-relaxed text-white shadow-md"
                  style={{ backgroundColor: '#0068FF' }}
                >
                  {m.text}
                </div>
              </div>
              <div className="mt-1 flex items-center justify-end gap-1.5 pr-1 text-[10px] text-muted">
                <FaPaperPlane size={9} />
                {processedAt || 'vừa xong'}
              </div>
              {!isSingle && (
                <div className="mt-1 flex justify-end">
                  <button
                    onClick={() => handleCopy(m.po, m.text)}
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

        <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-3">
          <button
            onClick={isSingle ? () => handleCopy(messages[0].po, messages[0].text) : handleCopyAll}
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
    </div>
  )
}
