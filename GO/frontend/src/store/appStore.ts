import { create } from 'zustand'
import type { LogEntry, OrderRow } from '../types'
import type { PriceBasis } from '../lib/zaloMessage'

export type LockStatus = 'checking' | 'unlocked' | 'locked'

interface AppState {
  files: string[]
  stt: number
  isProcessing: boolean
  logLines: LogEntry[]
  rows: OrderRow[]
  lockStatus: LockStatus
  selectedPOs: Set<string>
  resolvedChoice: Record<string, PriceBasis>
  togglePOSelection: (po: string) => void
  toggleAllPOs: (allPOs: string[], checked: boolean) => void
  clearSelection: () => void
  setResolvedChoice: (key: string, choice: PriceBasis) => void
  clearResolvedChoice: () => void
  setFiles: (files: string[]) => void
  addFiles: (files: string[]) => void
  removeFiles: (files: string[]) => void
  setStt: (stt: number) => void
  setProcessing: (processing: boolean) => void
  appendLog: (line: string) => void
  clearLog: () => void
  appendRow: (row: OrderRow) => void
  resetRows: () => void
  adjustRowDonGia: (rowIndex: number, delta: number) => void
  setLockStatus: (status: LockStatus) => void
}

export const useAppStore = create<AppState>((set) => ({
  files: [],
  stt: 1,
  isProcessing: false,
  logLines: [],
  rows: [],
  // Blocks the UI by default until the first "applock:status" event
  // arrives from the backend (see useWailsEvents.ts) - fail-safe: never
  // briefly render as unlocked before the license has actually been
  // verified.
  lockStatus: 'checking',
  selectedPOs: new Set(),
  resolvedChoice: {},
  setFiles: (files) => set({ files }),
  addFiles: (newFiles) =>
    set((state) => ({
      files: Array.from(new Set([...state.files, ...newFiles])),
    })),
  removeFiles: (toRemove) =>
    set((state) => ({
      files: state.files.filter((f) => !toRemove.includes(f)),
    })),
  setStt: (stt) => set({ stt }),
  setProcessing: (isProcessing) => set({ isProcessing }),
  appendLog: (line) =>
    set((state) => ({
      logLines: [
        ...state.logLines,
        { time: new Date().toLocaleTimeString('vi-VN', { hour12: false }), text: line },
      ],
    })),
  clearLog: () => set({ logLines: [] }),
  appendRow: (row) => set((state) => ({ rows: [...state.rows, row] })),
  resetRows: () => set({ rows: [] }),
  // Applied after ConfirmPrice succeeds for one mismatched SKU: donGia is
  // the order's total (sum of unitPrice * qty across every product line,
  // computed once on the Go side - see PriceMismatchDetail's own doc
  // comment), and SystemPrice is already the price counted in that total
  // by default, so only the delta between the newly confirmed price and
  // whichever price was previously in effect needs to be added - not a
  // full recomputation from scratch, which the frontend has no way to do
  // (it never receives the order's other, non-mismatched line items).
  adjustRowDonGia: (rowIndex, delta) =>
    set((state) => {
      const row = state.rows[rowIndex]
      if (!row) return state
      const current = Number(row.donGia)
      if (Number.isNaN(current)) return state
      const rows = [...state.rows]
      rows[rowIndex] = { ...row, donGia: String(Math.round(current + delta)) }
      return { rows }
    }),
  setLockStatus: (lockStatus) => set({ lockStatus }),
  togglePOSelection: (po) =>
    set((state) => {
      const next = new Set(state.selectedPOs)
      if (next.has(po)) next.delete(po)
      else next.add(po)
      return { selectedPOs: next }
    }),
  toggleAllPOs: (allPOs, checked) => set({ selectedPOs: checked ? new Set(allPOs) : new Set() }),
  clearSelection: () => set({ selectedPOs: new Set() }),
  setResolvedChoice: (key, choice) =>
    set((state) => ({ resolvedChoice: { ...state.resolvedChoice, [key]: choice } })),
  clearResolvedChoice: () => set({ resolvedChoice: {} }),
}))
