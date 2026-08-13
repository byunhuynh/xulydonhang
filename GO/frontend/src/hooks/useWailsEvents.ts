import { useEffect } from 'react'
import { EventsOn, OnFileDrop, OnFileDropOff } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../store/appStore'
import type { OrderRow } from '../types'

export function useWailsEvents() {
  const appendLog = useAppStore((s) => s.appendLog)
  const appendRow = useAppStore((s) => s.appendRow)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const setStt = useAppStore((s) => s.setStt)
  const addFiles = useAppStore((s) => s.addFiles)

  useEffect(() => {
    const offLog = EventsOn('process:log', (line: string) => appendLog(line))
    const offRow = EventsOn('process:row', (row: OrderRow) => appendRow(row))
    const offDone = EventsOn('process:done', (finalStt: number) => {
      setProcessing(false)
      setStt(finalStt)
    })
    const offDrop = EventsOn('files:dropped', (paths: string[]) => addFiles(paths))
    // Attaches the actual dragover/dragleave/drop DOM listeners on window;
    // without this call the Go-side runtime.OnFileDrop callback never fires
    // and WebView2 falls back to navigating the window to the dropped file.
    // The real file-list update flows through the files:dropped event above.
    OnFileDrop(() => {}, false)

    return () => {
      offLog()
      offRow()
      offDone()
      offDrop()
      OnFileDropOff()
    }
  }, [appendLog, appendRow, setProcessing, setStt, addFiles])
}
