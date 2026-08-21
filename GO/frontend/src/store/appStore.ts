import { create } from 'zustand'
import type { LogEntry, OrderRow } from '../types'

interface AppState {
  files: string[]
  stt: number
  isProcessing: boolean
  logLines: LogEntry[]
  rows: OrderRow[]
  setFiles: (files: string[]) => void
  addFiles: (files: string[]) => void
  removeFiles: (files: string[]) => void
  setStt: (stt: number) => void
  setProcessing: (processing: boolean) => void
  appendLog: (line: string) => void
  clearLog: () => void
  appendRow: (row: OrderRow) => void
  resetRows: () => void
}

export const useAppStore = create<AppState>((set) => ({
  files: [],
  stt: 1,
  isProcessing: false,
  logLines: [],
  rows: [],
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
}))
