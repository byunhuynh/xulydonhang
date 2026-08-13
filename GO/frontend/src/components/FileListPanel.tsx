import { useState } from 'react'
import { FaArrowsRotate, FaFolderOpen } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { SelectFiles, ScanOrderFolder } from '../../wailsjs/go/main/App'

export function FileListPanel() {
  const files = useAppStore((s) => s.files)
  const setFiles = useAppStore((s) => s.setFiles)
  const addFiles = useAppStore((s) => s.addFiles)
  const removeFiles = useAppStore((s) => s.removeFiles)
  const appendLog = useAppStore((s) => s.appendLog)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const [selected, setSelected] = useState<Set<string>>(new Set())

  async function reload() {
    try {
      const found = await ScanOrderFolder()
      setFiles(found)
      setSelected(new Set())
      appendLog(`Đã load ${found.length} file từ thư mục đơn hàng.`)
    } catch (err) {
      appendLog(`❌ Lỗi tải thư mục: ${String(err)}`)
    }
  }

  async function pickFiles() {
    try {
      const picked = await SelectFiles()
      if (picked.length === 0) return
      addFiles(picked)
      appendLog(`Đã thêm ${picked.length} file.`)
    } catch (err) {
      appendLog(`❌ Lỗi chọn file: ${String(err)}`)
    }
  }

  function toggleSelect(f: string, e: React.MouseEvent) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (!e.ctrlKey && !e.metaKey) next.clear()
      if (next.has(f)) next.delete(f)
      else next.add(f)
      return next
    })
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Delete' && selected.size > 0) {
      removeFiles([...selected])
      appendLog(`Đã xóa ${selected.size} file khỏi danh sách.`)
      setSelected(new Set())
    }
  }

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3">
      <h2 className="mb-2 text-sm font-semibold text-muted">1. Danh sách file đầu vào</h2>
      <ul
        tabIndex={0}
        onKeyDown={handleKeyDown}
        className="selectable flex-1 overflow-auto rounded-lg border border-border bg-bg font-mono text-xs"
      >
        {files.length === 0 && (
          <li className="p-3 text-muted">
            Chưa có file nào. Kéo-thả file vào cửa sổ hoặc bấm "Chọn file...".
          </li>
        )}
        {files.map((f) => (
          <li
            key={f}
            onClick={(e) => toggleSelect(f, e)}
            className={`cursor-pointer truncate border-b border-border px-3 py-1.5 ${
              selected.has(f) ? 'bg-accent/20 text-accent' : 'text-ink'
            }`}
          >
            {f}
          </li>
        ))}
      </ul>
      <div className="mt-2 flex gap-2">
        <button
          onClick={reload}
          disabled={isProcessing}
          className="inline-flex flex-1 items-center justify-center gap-2 rounded-lg bg-accent/10 px-3 py-2 text-sm font-medium text-accent hover:bg-accent/20 disabled:opacity-40"
        >
          <FaArrowsRotate /> Tải lại đơn hàng
        </button>
        <button
          onClick={pickFiles}
          disabled={isProcessing}
          className="inline-flex flex-1 items-center justify-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium text-ink hover:border-accent disabled:opacity-40"
        >
          <FaFolderOpen /> Chọn file...
        </button>
      </div>
    </section>
  )
}
