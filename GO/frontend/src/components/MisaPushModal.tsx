import { useEffect, useRef, useState } from 'react'
import { FaXmark, FaCloudArrowUp, FaSpinner } from 'react-icons/fa6'
import { GetAppSettings, MisaResolveRoutes, PushMisa, SaveAppSettings } from '../../wailsjs/go/main/App'
import { useAppStore } from '../store/appStore'
import {
  MISA_BRANCH_OPTIONS,
  branchTotals,
  buildMisaGroups,
  canPush,
  pendingGroups,
  rememberRouting,
  type MisaGroup,
} from '../lib/misaBranch'
import { groupKeyFor } from '../lib/zaloGrouping'
import { SegmentedControl } from './SegmentedControl'
import { useModalEntrance } from '../lib/useModalEntrance'

interface MisaPushModalProps {
  onClose: () => void
}

export function MisaPushModal({ onClose }: MisaPushModalProps) {
  const rows = useAppStore((s) => s.rows)
  const isPushing = useAppStore((s) => s.isPushing)
  const setPushing = useAppStore((s) => s.setPushing)
  const misaResults = useAppStore((s) => s.misaResults)
  const clearMisaResults = useAppStore((s) => s.clearMisaResults)
  const appendLog = useAppStore((s) => s.appendLog)

  const [groups, setGroups] = useState<MisaGroup[] | null>(null)
  // Đơn bị bỏ vì không có dòng nào trong sổ đặt hàng (xem buildMisaGroups)
  // — modal PHẢI cảnh báo rõ, không được để chúng âm thầm biến mất.
  const [skipped, setSkipped] = useState<{ key: string; po: string; system: string }[]>([])
  const [remember, setRemember] = useState(true)
  const [error, setError] = useState('')
  // Nhánh đã ghi THÀNH CÔNG vào sổ, cộng dồn qua các lượt bấm trong CÙNG
  // một phiên mở modal. PHẢI tách khỏi misaResults: handlePush gọi
  // clearMisaResults() ở đầu mỗi lượt đẩy để màn hình kết quả không trộn
  // lượt cũ/mới, nhưng nếu locked/pendingGroups/canPush suy trực tiếp từ
  // misaResults thì cùng một lần xoá đó cũng xoá luôn "trí nhớ" nhánh đã
  // vào sổ - đẩy lần 2 mà nhánh kia lỗi sẽ khiến nhánh đã ok tự tick lại
  // và mở khoá, đẩy lần 3 ghi TRÙNG toàn bộ chứng từ của nhánh đó vào sổ
  // kế toán. doneBranches chỉ được CỘNG THÊM, không bao giờ bị xoá trong
  // suốt vòng đời modal - đóng modal (component unmount) mới mất, đúng
  // ý nghĩa "phiên mới thì phải hỏi lại từ đầu".
  const [doneBranches, setDoneBranches] = useState<string[]>([])
  const backdropRef = useRef<HTMLDivElement>(null)
  const cardRef = useRef<HTMLDivElement>(null)
  useModalEntrance(backdropRef, cardRef, [!!groups])

  useEffect(() => {
    // groupKeyFor truyền vào đây là chỗ DUY NHẤT nối modal với định
    // nghĩa khoá nhóm dùng chung của bảng kết quả và nút gửi Zalo.
    const { groups: seeds, skipped: skippedSeeds } = buildMisaGroups(rows, groupKeyFor)
    setSkipped(skippedSeeds)
    MisaResolveRoutes(
      seeds.map((s) => ({ system: s.system, customerCode: s.customerCode, shipTo: s.shipTo })),
    )
      .then((infos) =>
        setGroups(
          seeds.map((s, i) => ({
            ...s,
            routeKey: infos[i]?.key ?? s.system,
            routeLabel: infos[i]?.label ?? s.system,
            branch: infos[i]?.branch ?? '',
            selected: true,
          })),
        ),
      )
      .catch((err) => setError(String(err)))
    // Chụp một lần lúc mở modal: bảng kết quả không đổi trong lúc modal
    // đang mở (nút Push đã bị khoá khi đang xử lý lô).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    // Cộng dồn nhánh ok mới xuất hiện trong misaResults vào doneBranches -
    // xem comment khai báo doneBranches ở trên vì sao hai thứ này không
    // được gộp làm một.
    const newlyOk = misaResults.filter((r) => r.ok).map((r) => r.branch)
    if (newlyOk.length === 0) return
    setDoneBranches((cur) => {
      const merged = new Set(cur)
      let changed = false
      for (const b of newlyOk) {
        if (!merged.has(b)) {
          merged.add(b)
          changed = true
        }
      }
      return changed ? [...merged] : cur
    })
  }, [misaResults])

  function setGroupBranch(key: string, branch: string) {
    setGroups((cur) => cur && cur.map((g) => (g.key === key ? { ...g, branch } : g)))
  }

  function toggleGroup(key: string) {
    setGroups((cur) => cur && cur.map((g) => (g.key === key ? { ...g, selected: !g.selected } : g)))
  }

  async function handlePush() {
    if (!groups) return
    const jobs = pendingGroups(groups, doneBranches).map((g) => ({
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
      await PushMisa(jobs)
    } catch (err) {
      appendLog(`❌ Lỗi đẩy MISA: ${String(err)}`)
      setPushing(false)
    }
  }

  const totals = groups ? branchTotals(groups) : null
  const ready = groups ? canPush(groups, doneBranches, skipped.length) : false

  return (
    <div ref={backdropRef} className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div
        ref={cardRef}
        className="flex max-h-[80vh] w-[720px] flex-col rounded-xl border border-border bg-panel p-4"
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
              <div className="grid grid-cols-[auto_1fr_1fr_auto_auto] items-center gap-2 px-1 pb-1 font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
                <span />
                <span>Số đơn hàng</span>
                <span>Hệ thống</span>
                <span>Dòng</span>
                <span>Nhánh</span>
              </div>
              {groups.map((g) => {
                const locked = doneBranches.includes(g.branch)
                return (
                  <div key={g.key} className="grid grid-cols-[auto_1fr_1fr_auto_auto] items-center gap-2 py-1">
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
                  </div>
                )
              })}
            </div>

            {misaResults.length > 0 && (
              <div className="mt-3 flex flex-col gap-1 border-t border-border pt-3">
                {misaResults.map((r) => (
                  <p
                    key={r.branch}
                    className={`font-sans text-xs ${r.ok ? 'text-success' : 'text-danger'}`}
                  >
                    {MISA_BRANCH_OPTIONS.find((o) => o.value === r.branch)?.label ?? r.branch}:{' '}
                    {r.ok ? `đã ghi ${r.valid} chứng từ vào sổ` : r.message}
                  </p>
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
                onClick={handlePush}
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
    </div>
  )
}
