import { useEffect, useMemo, useState } from 'react'
import { FaArrowsRotate, FaFolderOpen, FaMagnifyingGlass, FaXmark, FaUpRightAndDownLeftFromCenter, FaDownLeftAndUpRightToCenter } from 'react-icons/fa6'
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

type FileStatus = 'proc' | 'done' | 'err'

// deriveFileStatus reads real backend log lines (runBatch in app.go emits
// "Đang xử lý <file>..." right before each file and "❌ Lỗi xử lý <file>: "
// on failure - both via filepath.Base, matching fileName() here) instead
// of guessing: a file is "proc" from the moment its own line appears
// until either the next file's line appears (implying this one finished)
// or the whole batch's process:done event flips any still-"proc" entry to
// "done" - there is no explicit "this one file finished" event, so both
// signals are needed to know a status is final.
function deriveFileStatus(logLines: { text: string }[], isProcessing: boolean): Record<string, FileStatus> {
  const map: Record<string, FileStatus> = {}
  for (const { text } of logLines) {
    const procMatch = text.match(/^Đang xử lý (.+)\.\.\.$/)
    if (procMatch) {
      for (const k of Object.keys(map)) {
        if (map[k] === 'proc') map[k] = 'done'
      }
      map[procMatch[1]] = 'proc'
      continue
    }
    const errMatch = text.match(/^❌ Lỗi xử lý (.+?): /)
    if (errMatch) map[errMatch[1]] = 'err'
  }
  if (!isProcessing) {
    for (const k of Object.keys(map)) {
      if (map[k] === 'proc') map[k] = 'done'
    }
  }
  return map
}

const statusDotClass: Record<FileStatus, string> = {
  done: 'bg-success',
  err: 'bg-danger',
  proc: 'bg-accent shadow-[0_0_6px_theme(colors.accent)] animate-pulse',
}

export function FileListPanel({
  size,
  onToggleExpand,
}: {
  size: 'balanced' | 'expanded' | 'compact'
  onToggleExpand: () => void
}) {
  const expanded = size === 'expanded'
  const files = useAppStore((s) => s.files)
  const setFiles = useAppStore((s) => s.setFiles)
  const addFiles = useAppStore((s) => s.addFiles)
  const removeFiles = useAppStore((s) => s.removeFiles)
  const appendLog = useAppStore((s) => s.appendLog)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const logLines = useAppStore((s) => s.logLines)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [filter, setFilter] = useState('')

  const statusMap = useMemo(() => deriveFileStatus(logLines, isProcessing), [logLines, isProcessing])
  const visibleFiles = useMemo(
    () => (filter.trim() === '' ? files : files.filter((f) => fileName(f).toLowerCase().includes(filter.trim().toLowerCase()))),
    [files, filter],
  )

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
    <section
      className={`flex min-h-0 flex-col rounded-xl border border-border bg-panel p-3.5 transition-all ${
        size === 'compact' ? 'flex-none' : size === 'expanded' ? 'flex-1' : 'flex-[2]'
      }`}
    >
      <SectionHeader
        index="01"
        title="File đầu vào"
        action={
          <div className="flex items-center gap-1">
            <span className="font-mono text-[10px] text-muted">{files.length}</span>
            <button
              onClick={reload}
              disabled={isProcessing}
              title="Tải lại"
              className="rounded p-1 text-muted transition-colors hover:text-accent disabled:opacity-40"
            >
              <FaArrowsRotate size={11} />
            </button>
            <button
              onClick={onToggleExpand}
              title={expanded ? 'Thu gọn' : 'Mở rộng'}
              className="rounded p-1 text-muted transition-colors hover:text-accent"
            >
              {expanded ? <FaDownLeftAndUpRightToCenter size={11} /> : <FaUpRightAndDownLeftFromCenter size={11} />}
            </button>
          </div>
        }
      />
      {files.length > 5 && (
        <div className="mb-2 flex items-center gap-2 rounded-lg border border-border bg-bg px-2.5 py-1.5">
          <FaMagnifyingGlass size={10} className="flex-shrink-0 text-muted" />
          <input
            type="text"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Lọc theo tên file..."
            className="w-full bg-transparent font-mono text-xs text-ink outline-none placeholder:text-muted"
          />
        </div>
      )}
      <ul
        tabIndex={0}
        onKeyDown={handleKeyDown}
        className={`selectable flex-1 overflow-auto rounded-lg border border-border bg-bg p-2 ${
          size === 'compact' ? 'max-h-[210px]' : ''
        }`}
      >
        {files.length === 0 && (
          <li className="flex h-full items-center justify-center rounded-lg border border-dashed border-border p-6 text-center font-mono text-xs text-muted">
            Chưa có file nào. Kéo-thả file vào cửa sổ hoặc bấm "Chọn file...".
          </li>
        )}
        {files.length > 0 && visibleFiles.length === 0 && (
          <li className="flex h-full items-center justify-center text-center font-mono text-xs text-muted">
            Không có file nào khớp "{filter}".
          </li>
        )}
        {visibleFiles.map((f) => {
          const status = statusMap[fileName(f)]
          return (
            <li
              key={f}
              onClick={(e) => toggleSelect(f, e)}
              className={`group mb-1 flex cursor-pointer items-center gap-2 rounded-lg border px-2.5 py-1.5 last:mb-0 ${
                selected.has(f)
                  ? 'border-accent/50 bg-accent/10'
                  : 'border-border bg-panel/50 hover:border-border/80 hover:bg-panel'
              }`}
            >
              <span className="flex h-5 w-8 flex-shrink-0 items-center justify-center rounded bg-accent/15 font-mono text-[8.5px] font-bold text-accent">
                {fileKind(f)}
              </span>
              <span
                className={`flex-1 truncate font-mono text-xs ${selected.has(f) ? 'text-accent' : 'text-ink'}`}
                title={f}
              >
                {fileName(f)}
              </span>
              {status && <span className={`h-1.5 w-1.5 flex-shrink-0 rounded-full ${statusDotClass[status]}`} title={
                status === 'done' ? 'Đã xong' : status === 'err' ? 'Lỗi' : 'Đang xử lý'
              } />}
              <button
                onClick={(e) => removeOne(f, e)}
                title="Xóa khỏi danh sách"
                className="flex-shrink-0 rounded p-1 text-muted opacity-0 transition-all hover:text-danger group-hover:opacity-100"
              >
                <FaXmark size={12} />
              </button>
            </li>
          )
        })}
      </ul>
      <div className="mt-2">
        <button
          onClick={pickFiles}
          disabled={isProcessing}
          className="inline-flex w-full items-center justify-center gap-2 rounded-lg border border-border px-3 py-1.5 text-xs font-medium text-ink transition-colors hover:border-accent hover:text-accent disabled:opacity-40"
        >
          <FaFolderOpen size={11} /> Chọn thêm file...
        </button>
      </div>
    </section>
  )
}
