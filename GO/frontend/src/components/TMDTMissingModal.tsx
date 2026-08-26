import { useMemo, useRef, useState } from 'react'
import { useAppStore } from '../store/appStore'
import { CancelTMDTMissing, ResolveTMDTMissing } from '../../wailsjs/go/main/App'
import { useModalEntrance } from '../lib/useModalEntrance'
import { draftError, emptyDraft, isDraftFilled, type TMDTComboDraft } from '../lib/tmdtMissing'

// Modal bật ĐÚNG MỘT LẦN cho cả lần chạy, sau khi đã quy đổi hết dữ liệu
// và trước khi ghi bất kỳ file nào. Backend đang chờ trên channel: mọi
// đường ra khỏi modal đều phải gọi Resolve hoặc Cancel, nếu không batch
// treo tới khi hết hạn 10 phút.
export function TMDTMissingModal() {
  const missing = useAppStore((s) => s.tmdtMissing)
  const setTMDTMissing = useAppStore((s) => s.setTMDTMissing)
  const appendLog = useAppStore((s) => s.appendLog)
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef, [missing !== null])

  const [drafts, setDrafts] = useState<TMDTComboDraft[]>([])
  const [busy, setBusy] = useState(false)

  // Dựng lại nháp mỗi khi danh sách thiếu đổi (một lần cho mỗi lần chạy).
  const signature = useMemo(() => (missing ?? []).map((m) => m.key).join('|'), [missing])
  const [seenSignature, setSeenSignature] = useState('')
  if (missing && signature !== seenSignature) {
    setSeenSignature(signature)
    setDrafts(missing.map(emptyDraft))
  }

  if (!missing) return null

  const errors = drafts.map(draftError)
  const firstError = errors.find((e) => e !== null) ?? null
  const filledCount = drafts.filter(isDraftFilled).length

  function updateCell(rowIdx: number, field: 'tp' | 'sl', col: number, value: string) {
    setDrafts((prev) =>
      prev.map((d, i) => {
        if (i !== rowIdx) return d
        const next = [...d[field]] as [string, string, string, string]
        next[col] = value
        return { ...d, [field]: next }
      }),
    )
  }

  async function submit() {
    setBusy(true)
    try {
      await ResolveTMDTMissing(drafts.filter(isDraftFilled))
      setTMDTMissing(null)
    } catch (err) {
      appendLog(`❌ Lỗi gửi khai báo mã: ${String(err)}`)
    } finally {
      setBusy(false)
    }
  }

  async function skip() {
    setBusy(true)
    try {
      await CancelTMDTMissing()
      setTMDTMissing(null)
    } catch (err) {
      appendLog(`❌ Lỗi bỏ qua khai báo mã: ${String(err)}`)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      ref={backdropRef}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
      role="dialog"
      aria-modal="true"
      aria-label="Khai mã thành phẩm còn thiếu"
    >
      <div
        ref={cardRef}
        className="flex max-h-[85vh] w-full max-w-3xl flex-col rounded-lg border border-border bg-panel p-5 shadow-xl"
      >
        <h2 className="font-sans text-base font-semibold text-ink">
          {missing.length} mã chưa khai báo trong sheet &quot;data shop&quot;
        </h2>
        <p className="mt-1 font-sans text-xs text-muted">
          Điền mã thành phẩm và số lượng. Nội dung này được ghi luôn vào sheet &quot;data shop&quot; nên lần sau
          không phải khai lại. Bỏ trống một mục nghĩa là giữ #N/A cho mục đó.
        </p>

        <div className="mt-3 flex-1 overflow-y-auto">
          {drafts.map((d, i) => (
            <div key={d.key} className="mb-3 rounded-md border border-border p-3">
              <div className="font-sans text-xs text-ink">
                {d.product}
                {d.variant ? <span className="text-muted"> · {d.variant}</span> : null}
              </div>
              <div className="mt-0.5 font-mono text-[11px] text-muted">
                {d.combo ? `Mã sản phẩm: ${d.combo}` : 'Không có mã sản phẩm — tra theo tên + phân loại'}
                {' · '}
                {missing[i]?.lineCount ?? 0} dòng hàng
              </div>
              <div className="mt-2 grid grid-cols-4 gap-2">
                {[0, 1, 2, 3].map((col) => (
                  <div key={col}>
                    <label className="block font-sans text-[11px] text-muted">MÃ TP {col + 1}</label>
                    <input
                      value={d.tp[col]}
                      onChange={(e) => updateCell(i, 'tp', col, e.target.value)}
                      className="mt-0.5 w-full rounded border border-border bg-black/20 px-1.5 py-1 font-mono text-xs text-ink"
                    />
                    <label className="mt-1 block font-sans text-[11px] text-muted">SLTP {col + 1}</label>
                    <input
                      value={d.sl[col]}
                      onChange={(e) => updateCell(i, 'sl', col, e.target.value)}
                      className="mt-0.5 w-full rounded border border-border bg-black/20 px-1.5 py-1 font-mono text-xs text-ink"
                    />
                  </div>
                ))}
              </div>
              {errors[i] ? <div className="mt-1.5 font-sans text-[11px] text-danger">{errors[i]}</div> : null}
            </div>
          ))}
        </div>

        <div className="mt-3 flex items-center justify-between">
          <span className="font-sans text-xs text-muted">
            Đã khai {filledCount}/{missing.length} mã
          </span>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={skip}
              className="rounded-md border border-border px-3 py-1.5 font-sans text-xs font-medium text-muted hover:text-ink disabled:opacity-40"
            >
              Bỏ qua, để #N/A
            </button>
            <button
              type="button"
              disabled={busy || firstError !== null || filledCount === 0}
              onClick={submit}
              className="rounded-md bg-accent px-3 py-1.5 font-sans text-xs font-semibold text-black disabled:cursor-not-allowed disabled:opacity-40"
            >
              Lưu và tiếp tục
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
