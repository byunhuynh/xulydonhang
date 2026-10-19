import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { FaXmark, FaCloudArrowUp, FaSpinner } from 'react-icons/fa6'
import { GetAppSettings, MisaResolveRoutes, PushMisa, SaveAppSettings } from '../../wailsjs/go/main/App'
import { useAppStore } from '../store/appStore'
import {
  MISA_BRANCH_OPTIONS,
  MISA_ORDER_META,
  branchTotals,
  buildMisaGroups,
  canPush,
  doneOrderKeys,
  pendingGroups,
  rememberRouting,
  shouldPreselect,
  summarizeOrderStatuses,
  type MisaGroup,
  type MisaOrderStatus,
} from '../lib/misaBranch'
import { groupKeyFor } from '../lib/zaloGrouping'
import { SegmentedControl } from './SegmentedControl'
import { useModalEntrance } from '../lib/useModalEntrance'

const TONE_CLASSES: Record<(typeof MISA_ORDER_META)[keyof typeof MISA_ORDER_META]['tone'], string> = {
  accent: 'bg-accent/15 text-accent',
  success: 'bg-success/15 text-success',
  warning: 'bg-warning/15 text-warning',
  danger: 'bg-danger/15 text-danger',
  muted: 'bg-white/5 text-muted',
}

function OrderStatusBadge({ status }: { status: MisaOrderStatus | undefined }) {
  if (!status) return <span className="font-sans text-[11px] text-muted">Chưa đẩy</span>
  const meta = MISA_ORDER_META[status.status] ?? { label: status.status, tone: 'muted' as const }
  return (
    <span
      title={status.message}
      className={`inline-flex items-center gap-1 justify-self-start whitespace-nowrap rounded-full px-2 py-0.5 font-sans text-[11px] font-semibold ${TONE_CLASSES[meta.tone]}`}
    >
      {status.status === 'pushing' && <FaSpinner size={9} className="animate-spin" />}
      {meta.label}
    </span>
  )
}

interface MisaPushModalProps {
  onClose: () => void
}

export function MisaPushModal({ onClose }: MisaPushModalProps) {
  const rows = useAppStore((s) => s.rows)
  const isPushing = useAppStore((s) => s.isPushing)
  const setPushing = useAppStore((s) => s.setPushing)
  const misaResults = useAppStore((s) => s.misaResults)
  const clearMisaResults = useAppStore((s) => s.clearMisaResults)
  const misaOrders = useAppStore((s) => s.misaOrders)
  const appendLog = useAppStore((s) => s.appendLog)

  const [groups, setGroups] = useState<MisaGroup[] | null>(null)
  // Đơn bị bỏ vì không có dòng nào trong sổ đặt hàng (xem buildMisaGroups)
  // — modal PHẢI cảnh báo rõ, không được để chúng âm thầm biến mất.
  const [skipped, setSkipped] = useState<{ key: string; po: string; system: string }[]>([])
  const [remember, setRemember] = useState(true)
  const [error, setError] = useState('')
  // Đơn đã vào sổ (hoặc đang đẩy) — khoá lại, không cho đẩy trùng. Suy từ
  // misaOrders trong store, KHÔNG từ misaResults: handlePush xoá
  // misaResults ở đầu mỗi lượt để màn hình không trộn lượt cũ/mới, còn
  // misaOrders sống suốt cả lô, kể cả khi đóng rồi mở lại modal. Trước
  // đây "trí nhớ" này nằm trong state của modal nên đóng-mở lại là mất,
  // và đơn đã vào sổ lại được tick để đẩy trùng.
  const doneKeys = doneOrderKeys(misaOrders)
  // Trạng thái đã thấy lần trước của từng đơn — để chỉ bỏ tick đơn "đã
  // tồn tại" ĐÚNG lúc kết quả đó mới về, không đè lên tick tay sau đó.
  const seenStatus = useRef<Record<string, string>>({})
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef, [!!groups])

  useEffect(() => {
    // groupKeyFor truyền vào đây là chỗ DUY NHẤT nối modal với định
    // nghĩa khoá nhóm dùng chung của bảng kết quả và nút gửi Zalo.
    const { groups: seeds, skipped: skippedSeeds } = buildMisaGroups(rows, groupKeyFor)
    setSkipped(skippedSeeds)
    const statuses = useAppStore.getState().misaOrders
    for (const [key, s] of Object.entries(statuses)) seenStatus.current[key] = s.status
    MisaResolveRoutes(
      seeds.map((s) => ({ system: s.system, customerCode: s.customerCode, shipTo: s.shipTo })),
    )
      .then((infos) =>
        setGroups(
          seeds.map((s, i) => ({
            ...s,
            routeKey: infos[i]?.key ?? s.system,
            routeLabel: infos[i]?.label ?? s.system,
            // Đơn đã đẩy rồi thì hiện đúng nhánh đã đẩy, không phải nhánh
            // mặc định — mở lại modal giữa chừng không được đổi nhánh ngầm.
            branch: statuses[s.key]?.branch || (infos[i]?.branch ?? ''),
            selected: shouldPreselect(statuses[s.key]),
          })),
        ),
      )
      .catch((err) => setError(String(err)))
    // Chụp một lần lúc mở modal: bảng kết quả không đổi trong lúc modal
    // đang mở (nút Push đã bị khoá khi đang xử lý lô).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    // Đơn vừa được MISA báo "đã tồn tại" thì bỏ tick sẵn: đẩy lại nó chỉ
    // khiến cả nhánh bị chặn lần nữa. Người dùng vẫn tick lại được.
    const untick = new Set<string>()
    for (const [key, s] of Object.entries(misaOrders)) {
      if (seenStatus.current[key] !== s.status && !shouldPreselect(s)) untick.add(key)
      seenStatus.current[key] = s.status
    }
    if (untick.size === 0) return
    setGroups((cur) => cur && cur.map((g) => (untick.has(g.key) ? { ...g, selected: false } : g)))
  }, [misaOrders])

  function setGroupBranch(key: string, branch: string) {
    setGroups((cur) => cur && cur.map((g) => (g.key === key ? { ...g, branch } : g)))
  }

  function toggleGroup(key: string) {
    setGroups((cur) => cur && cur.map((g) => (g.key === key ? { ...g, selected: !g.selected } : g)))
  }

  // onlyBranch giới hạn lượt đẩy vào ĐÚNG một nhánh. Nút "ghi phần hợp lệ"
  // nêu con số của riêng một nhánh, nên nó phải chỉ đẩy nhánh đó — hai
  // nhánh cùng lỗi là hai nút mang hai con số khác nhau, mà bấm cái nào
  // cũng ép cả hai thì con số trên nút thành nói dối.
  async function handlePush(force = false, onlyBranch?: string) {
    if (!groups) return
    const pending = pendingGroups(groups, doneKeys).filter(
      (g) => !onlyBranch || g.branch === onlyBranch,
    )
    const jobs = pending.map((g) => ({
      key: g.key,
      po: g.po,
      routeKey: g.routeKey,
      branch: g.branch,
      excelRows: g.excelRows,
    }))
    if (jobs.length === 0) return

    if (remember) {
      try {
        const settings = await GetAppSettings()
        await SaveAppSettings({
          ...settings,
          misa_routing: { ...settings.misa_routing, ...rememberRouting(groups) },
        })
      } catch (err) {
        // Không chặn việc đẩy: ghi nhớ chỉ là tiện lợi cho lần sau.
        appendLog(`⚠️ Không lưu được nhánh đã chọn: ${String(err)}`)
      }
    }

    clearMisaResults()
    setPushing(true)
    try {
      await PushMisa(jobs, force)
    } catch (err) {
      appendLog(`❌ Lỗi đẩy MISA: ${String(err)}`)
      setPushing(false)
    }
  }

  const totals = groups ? branchTotals(groups) : null
  const ready = groups ? canPush(groups, doneKeys, skipped.length) : false
  const orderStatuses = groups
    ? groups.map((g) => misaOrders[g.key]).filter((s): s is MisaOrderStatus => !!s)
    : []
  const statusSummary = summarizeOrderStatuses(orderStatuses)

  // Portal thẳng ra document.body, giống TMDTDateRangeModal và
  // OrderContentModal. ProcessTab.tsx bọc ControlPanel trong div
  // "animate-rise", mà animation đó dùng CSS transform với fill-mode
  // "both" nên transform KHÔNG BAO GIỜ trở lại none. Một tổ tiên có
  // transform là containing block mới cho con "position: fixed", nên modal
  // này bị nhốt trong dải toolbar cao ~60px thay vì phủ kín cửa sổ —
  // đúng triệu chứng "modal nằm trong app, khuất không chọn được".
  return createPortal(
    <div ref={backdropRef} className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={onClose}>
      <div
        ref={cardRef}
        className="flex max-h-[85vh] w-[880px] max-w-full flex-col rounded-xl border border-border bg-panel p-4"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-sans text-sm font-bold text-ink">
            Push MISA{groups ? ` — ${groups.length} đơn` : ''}
          </h2>
          <button type="button" onClick={onClose} className="text-muted hover:text-ink">
            <FaXmark size={16} />
          </button>
        </div>

        {error && <p className="font-sans text-xs text-danger">Không phân giải được nhánh: {error}</p>}
        {!groups && !error && <p className="font-sans text-xs text-muted">Đang tải…</p>}

        {groups && skipped.length > 0 && (
          <div className="mb-3 flex-shrink-0 rounded-lg border border-danger bg-panel px-3 py-2 font-sans text-xs text-danger">
            <p className="font-bold">
              {skipped.length} đơn không đẩy được: chưa có dòng nào trong sổ đặt hàng.
            </p>
            <p className="mt-1 text-muted">
              {skipped
                .slice(0, 5)
                .map((s) => `${s.po} (${s.system})`)
                .join(', ')}
              {skipped.length > 5 ? ` … và ${skipped.length - 5} đơn nữa` : ''}
            </p>
          </div>
        )}

        {groups && (
          <>
            <div className="flex-1 overflow-y-auto">
              <div className="grid grid-cols-[auto_1fr_1fr_auto_auto_112px] items-center gap-2 px-1 pb-1 font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
                <span />
                <span>Số đơn hàng</span>
                <span>Hệ thống</span>
                <span>Dòng</span>
                <span>Nhánh</span>
                <span>Trạng thái MISA</span>
              </div>
              {groups.map((g) => {
                const status = misaOrders[g.key]
                const locked = doneKeys.includes(g.key)
                // Lý do chỉ hiện cho đơn CẦN người xem: vào sổ rồi hay đang
                // đẩy thì nhãn đã đủ nói.
                const showMessage = !!status && status.status !== 'ok' && status.status !== 'pushing'
                return (
                  <div
                    key={g.key}
                    className="grid grid-cols-[auto_1fr_1fr_auto_auto_112px] items-center gap-x-2 gap-y-0.5 border-b border-border/50 px-1 py-1.5 last:border-0"
                  >
                    <input
                      type="checkbox"
                      checked={g.selected && !locked}
                      disabled={locked || isPushing}
                      onChange={() => toggleGroup(g.key)}
                      className="h-4 w-4 accent-accent"
                    />
                    <span className="truncate font-mono text-xs text-ink" title={g.po}>{g.po}</span>
                    <span className="truncate font-sans text-xs text-muted" title={g.routeLabel}>{g.routeLabel}</span>
                    <span className="tabular-nums font-mono text-xs text-muted">{g.excelRows.length}</span>
                    <SegmentedControl
                      options={MISA_BRANCH_OPTIONS}
                      value={g.branch}
                      disabled={locked || isPushing}
                      onChange={(branch) => setGroupBranch(g.key, branch)}
                      ariaLabel={`Nhánh kế toán cho đơn ${g.po}`}
                    />
                    <OrderStatusBadge status={status} />
                    {showMessage && (
                      <p
                        className={`col-span-5 col-start-2 font-sans text-[11px] leading-snug ${
                          status.status === 'held'
                            ? 'text-muted'
                            : status.status === 'exists' || status.status === 'unknown'
                              ? 'text-warning'
                              : 'text-danger'
                        }`}
                      >
                        {status.message}
                      </p>
                    )}
                  </div>
                )
              })}
            </div>

            {statusSummary.length > 0 && (
              <div className="mt-3 flex flex-wrap items-center gap-1.5 border-t border-border pt-3">
                {statusSummary.map(({ status, count }) => (
                  <span
                    key={status}
                    className={`rounded-full px-2 py-0.5 font-sans text-[11px] font-semibold ${TONE_CLASSES[MISA_ORDER_META[status].tone]}`}
                  >
                    {count} {MISA_ORDER_META[status].label.toLowerCase()}
                  </span>
                ))}
                {isPushing && (
                  <span className="ml-auto font-sans text-[11px] text-muted">
                    Có thể đóng cửa sổ này — lượt đẩy vẫn chạy tiếp, mở lại để xem kết quả.
                  </span>
                )}
              </div>
            )}

            {misaResults.length > 0 && (
              <div className="mt-2 flex flex-col gap-1">
                {misaResults.map((r) => (
                  <div key={r.branch} className="flex flex-col gap-1.5">
                    <p className={`font-sans text-xs ${r.ok ? 'text-success' : 'text-danger'}`}>
                      {MISA_BRANCH_OPTIONS.find((o) => o.value === r.branch)?.label ?? r.branch}:{' '}
                      {r.ok ? `đã ghi ${r.valid} chứng từ vào sổ` : r.message}
                    </p>
                    {/* Chỉ hiện khi backend nói canForce: MISA đã đọc được
                        file, chỉ ra dòng hỏng, và còn dòng hợp lệ. Nhãn nêu
                        THẲNG con số bỏ lại, vì đây là đường ghi thiếu đơn
                        vào sổ kế toán mà MISA vẫn báo thành công — người
                        bấm phải thấy mình đang bỏ lại bao nhiêu. */}
                    {!r.ok && r.canForce && !isPushing && (
                      <button
                        type="button"
                        onClick={() => handlePush(true, r.branch)}
                        className="self-start rounded-lg border border-warning/60 px-3 py-1.5 font-sans text-[11px] font-semibold text-warning transition-colors hover:bg-warning/10"
                      >
                        Chỉ ghi {r.valid} chứng từ hợp lệ, bỏ qua {r.invalid} dòng lỗi
                      </button>
                    )}
                  </div>
                ))}
              </div>
            )}

            <div className="mt-3 flex items-center justify-between gap-3 border-t border-border pt-3">
              <label className="flex items-center gap-2 font-sans text-xs text-muted">
                <input
                  type="checkbox"
                  checked={remember}
                  onChange={(e) => setRemember(e.target.checked)}
                  className="h-3.5 w-3.5 accent-accent"
                />
                Ghi nhớ nhánh đã chọn
              </label>
              <span className="ml-auto font-sans text-xs text-muted">
                {MISA_BRANCH_OPTIONS.map((o) => {
                  const t = totals?.[o.value] ?? { orders: 0, rows: 0 }
                  return `${o.label}: ${t.orders} đơn / ${t.rows} dòng`
                }).join(' · ')}
              </span>
              <button
                type="button"
                // PHẢI bọc trong arrow: onClick={handlePush} sẽ truyền
                // MouseEvent vào tham số force, mà object là truthy — bấm
                // nút thường cũng thành ghi ép, bỏ qua dòng lỗi im lặng.
                onClick={() => handlePush(false)}
                disabled={!ready || isPushing}
                className="inline-flex items-center gap-2 rounded-lg bg-accent px-4 py-2 font-sans text-xs font-bold text-[#0a1620] transition-opacity disabled:cursor-not-allowed disabled:opacity-40"
              >
                {isPushing ? <FaSpinner className="animate-spin" /> : <FaCloudArrowUp />}
                {isPushing ? 'ĐANG ĐẨY…' : 'ĐẨY LÊN MISA'}
              </button>
            </div>
          </>
        )}
      </div>
    </div>,
    document.body,
  )
}
