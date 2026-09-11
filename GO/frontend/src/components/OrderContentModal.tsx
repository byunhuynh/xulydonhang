import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { FaXmark, FaCopy, FaCheck, FaPaperPlane, FaUsers, FaTriangleExclamation, FaGear } from 'react-icons/fa6'
import type { OrderRow } from '../types'
import { PreviewZaloTargets } from '../../wailsjs/go/main/App'
import { zaloTargetQueriesFor, targetByPO, type ZaloTargetPreview } from '../lib/zaloTargets'
import { useAppStore } from '../store/appStore'
import {
  buildZaloMessageForPO,
  buildZaloMessageForJITFile,
  buildZaloMessageForTMDTShop,
  tmdtShopFromGroupKey,
  type PriceBasis,
} from '../lib/zaloMessage'
import { markupToHtml } from '../lib/richtext'
import { useModalEntrance } from '../lib/useModalEntrance'
import { poCompleteness, type POCompleteness } from '../lib/poCompleteness'

// Dải cảnh báo "đơn chưa đủ", ngay trên bong bóng tin. Tin nhắn cộng tiền
// từ những dòng ĐÃ GHI, nên một dòng PO không ghi được (mã chưa có trong
// SanPham) làm tổng tiền trong tin thấp hơn PO mà nhìn tin không thể biết -
// dải này là chỗ duy nhất trước lúc gửi nói ra điều đó.
function IncompletePOBanner({ info }: { info: POCompleteness }) {
  const { totals, missing } = info
  return (
    <div className="mb-2 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-[11px] leading-relaxed">
      <div className="flex items-center gap-1.5 font-semibold text-warning">
        <FaTriangleExclamation size={10} />
        Đơn chưa đủ — số liệu trong tin chưa gồm phần thiếu
      </div>
      {totals && (
        <div className="mt-0.5 text-muted">
          PO in{' '}
          <span className="font-mono font-semibold text-ink">
            {formatQty(totals.printedQty)} SL · {formatDong(totals.printedAmount)}đ
          </span>
          , mới ghi{' '}
          <span className="font-mono font-semibold text-ink">
            {formatQty(totals.writtenQty)} SL · {formatDong(totals.writtenAmount)}đ
          </span>
        </div>
      )}
      {missing.length > 0 && (
        <ul className="mt-1 space-y-0.5 text-muted">
          {missing.map((item) => (
            <li key={item.barcode}>
              <span className="font-mono text-ink">{item.barcode}</span> {item.description} — {formatQty(item.qty)} SL:
              chưa có trong SanPham
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function formatQty(n: number): string {
  return n.toLocaleString('vi-VN')
}

function formatDong(n: number): string {
  return Math.round(n).toLocaleString('vi-VN')
}

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

// Dải "nhóm nào sẽ nhận tin này", ngay trên bong bóng tin. Hai trạng
// thái, cố ý trông rất khác nhau:
//
//   - Đã gán: tên nhóm + key mờ bên cạnh. Key vẫn hiện dù đã có tên vì
//     nó là thứ người dùng dùng để tìm đúng dòng trong Cài đặt khi muốn
//     đổi nhóm.
//   - Chưa gán: cảnh báo đỏ + key cần thêm + nút mở thẳng Cài đặt > Zalo.
//     Job không có nhóm sẽ bị SKIP lúc gửi (xem zalosend.ErrNoContact),
//     nên đây là cảnh báo THẬT chứ không phải trang trí: không sửa thì
//     tin này không đi đâu cả.
function ZaloTargetBanner({ target, onOpenSettings }: { target: ZaloTargetPreview; onOpenSettings: () => void }) {
  if (target.configured) {
    return (
      <div className="mb-1.5 flex flex-wrap items-center justify-end gap-x-2 gap-y-1 text-[11px]">
        <span className="text-muted">Sẽ gửi tới</span>
        <span className="inline-flex items-center gap-1.5 rounded-full border border-accent/40 bg-accent/10 px-2.5 py-0.5 font-semibold text-accent">
          <FaUsers size={9} />
          {target.groupName}
        </span>
        <span className="font-mono text-[10px] text-muted">key: {target.key}</span>
      </div>
    )
  }
  return (
    <div className="mb-1.5 flex flex-wrap items-center justify-end gap-x-2 gap-y-1 text-[11px]">
      <span className="inline-flex items-center gap-1.5 rounded-full border border-danger/40 bg-danger/10 px-2.5 py-0.5 font-semibold text-danger">
        <FaTriangleExclamation size={9} />
        Chưa gán nhóm — tin này sẽ không gửi được
      </span>
      {target.suggestedKey && <span className="font-mono text-[10px] text-muted">key: {target.suggestedKey}</span>}
      <button
        onClick={onOpenSettings}
        className="inline-flex items-center gap-1.5 rounded-lg border border-border px-2 py-0.5 font-semibold text-muted transition-colors hover:border-accent hover:text-accent"
      >
        <FaGear size={9} />
        Mở Cài đặt &gt; Zalo
      </button>
    </div>
  )
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
  // Nhóm Zalo dự kiến nhận từng tin, tra ở backend (xem lib/zaloTargets.ts
  // để biết vì sao KHÔNG tự tra ở đây). null = chưa có câu trả lời: dải
  // nhóm khi đó không hiện gì cả, thay vì nhấp nháy "chưa gán nhóm" rồi
  // đổi ý - báo động giả ở đây tệ hơn là chờ thêm một nhịp.
  const [targets, setTargets] = useState<Record<string, ZaloTargetPreview | undefined> | null>(null)
  const openSettings = useAppStore((s) => s.openSettings)
  const settingsTab = useAppStore((s) => s.settingsTab)
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef)

  // Hỏi nhóm nhận MỘT LẦN cho cả danh sách lúc mở modal. `groups` là
  // mảng dựng mới mỗi lần render nên không thể làm dependency trực tiếp -
  // dùng chuỗi khoá gộp nhóm, thứ thật sự quyết định câu hỏi. Cờ
  // `cancelled` chặn setState sau khi modal đã đóng giữa chừng.
  //
  // settingsTab cũng là dependency: đóng popup Cài đặt xong phải hỏi lại,
  // vì người dùng vừa vào đó để GÁN NHÓM còn thiếu - không hỏi lại thì
  // dải nhóm vẫn đỏ dù đã sửa xong, và người dùng không có cách nào biết
  // là đã sửa đúng ngoài việc đóng mở lại cả modal này.
  const groupSignature = groups.map((g) => `${g.po}\u0000${g.rows[0]?.system ?? ''}\u0000${g.rows[0]?.maKhachHang ?? ''}`).join('\u0001')
  useEffect(() => {
    let cancelled = false
    PreviewZaloTargets(zaloTargetQueriesFor(groups))
      .then((previews) => {
        if (!cancelled) setTargets(targetByPO(previews ?? []))
      })
      // Không đọc được cấu hình thì im lặng bỏ qua dải nhóm: nội dung tin
      // - thứ người dùng mở modal này để xem - vẫn phải hiện bình thường.
      .catch(() => {
        if (!cancelled) setTargets({})
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [groupSignature, settingsTab])

  const messages = groups.map((g) => ({
    po: g.po,
    // Ba nhánh phải khớp ĐÚNG ba nhánh của ZaloSendButton.handleSendZalo:
    // bản xem trước và bản gửi đi lệch nhau là lỗi tệ nhất ở chỗ này.
    text: tmdtShopFromGroupKey(g.rows[0]?.sourceId ?? '')
      ? buildZaloMessageForTMDTShop(g.rows, processedAt)
      : g.rows[0]?.system === 'JIT-CHOICE'
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
              {/* Hiện ở CẢ chế độ 1 đơn lẫn nhiều đơn: câu hỏi "ai nhận
                  tin này" quan trọng như nhau ở cả hai. */}
              {targets?.[m.po] && (
                <ZaloTargetBanner target={targets[m.po]!} onOpenSettings={() => openSettings('zalo')} />
              )}
              {(() => {
                const completeness = poCompleteness(groups[idx].rows)
                return completeness && <IncompletePOBanner info={completeness} />
              })()}
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
