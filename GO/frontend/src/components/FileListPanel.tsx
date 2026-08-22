import { useEffect, useState } from 'react'
import { FaArrowsRotate, FaFolderOpen, FaXmark } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { SelectFiles, ScanOrderFolder } from '../../wailsjs/go/main/App'
import { SectionHeader } from './SectionHeader'

function fileKind(path: string): string {
  const ext = path.split('.').pop()?.toUpperCase() ?? ''
  return ext.length <= 4 ? ext : ext.slice(0, 4)
}

function fileName(path: string): string {
  return path.split(/[\\/]/).pop() ?? path
}

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

  // Auto-load the "đơn hàng" folder's contents once when the app opens,
  // matching the old Python app's own startup behavior
  // (load_files_from_folder(), called from MyApp.__init__) - without
  // this, a freshly opened app shows an empty file list even when real
  // order files are already sitting in the folder, requiring a manual
  // "Tải lại" click every time before doing anything else.
  useEffect(() => {
    reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

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

  function removeOne(f: string, e: React.MouseEvent) {
    e.stopPropagation()
    removeFiles([f])
    setSelected((prev) => {
      const next = new Set(prev)
      next.delete(f)
      return next
    })
    appendLog(`Đã xóa 1 file khỏi danh sách.`)
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Delete' && selected.size > 0) {
      removeFiles([...selected])
      appendLog(`Đã xóa ${selected.size} file khỏi danh sách.`)
      setSelected(new Set())
    }
  }

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3.5">
      <SectionHeader
        index="01"
        title="Danh sách file đầu vào"
        action={
          <button
            onClick={reload}
            disabled={isProcessing}
            className="inline-flex items-center gap-1.5 text-xs font-medium text-muted transition-colors hover:text-accent disabled:opacity-40"
          >
            <FaArrowsRotate /> Tải lại
          </button>
        }
      />
      <ul
        tabIndex={0}
        onKeyDown={handleKeyDown}
        className="selectable flex-1 overflow-auto rounded-lg border border-border bg-bg p-2"
      >
        {files.length === 0 && (
          <li className="flex h-full items-center justify-center rounded-lg border border-dashed border-border p-6 text-center font-mono text-xs text-muted">
            Chưa có file nào. Kéo-thả file vào cửa sổ hoặc bấm "Chọn file...".
          </li>
        )}
        {files.map((f) => (
          <li
            key={f}
            onClick={(e) => toggleSelect(f, e)}
            className={`group mb-1.5 flex cursor-pointer items-center gap-2.5 rounded-lg border px-2.5 py-2 last:mb-0 ${
              selected.has(f)
                ? 'border-accent/50 bg-accent/10'
                : 'border-border bg-panel/50 hover:border-border/80 hover:bg-panel'
            }`}
          >
            <span className="flex h-6 w-9 flex-shrink-0 items-center justify-center rounded bg-accent/15 font-mono text-[9px] font-bold text-accent">
              {fileKind(f)}
            </span>
            <span
              className={`flex-1 truncate font-mono text-xs ${selected.has(f) ? 'text-accent' : 'text-ink'}`}
              title={f}
            >
              {fileName(f)}
            </span>
            <button
              onClick={(e) => removeOne(f, e)}
              title="Xóa khỏi danh sách"
              className="flex-shrink-0 rounded p-1 text-muted opacity-0 transition-all hover:text-danger group-hover:opacity-100"
            >
              <FaXmark size={13} />
            </button>
          </li>
        ))}
      </ul>
      <div className="mt-2.5">
        <button
          onClick={pickFiles}
          disabled={isProcessing}
          className="inline-flex w-full items-center justify-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium text-ink transition-colors hover:border-accent hover:text-accent disabled:opacity-40"
        >
          <FaFolderOpen /> Chọn file...
        </button>
      </div>
    </section>
  )
}
