import { useEffect } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../store/appStore'
import type { OrderRow } from '../types'

export function useWailsEvents() {
  const appendLog = useAppStore((s) => s.appendLog)
  const appendRow = useAppStore((s) => s.appendRow)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const addFiles = useAppStore((s) => s.addFiles)

  useEffect(() => {
    const offLog = EventsOn('process:log', (line: string) => appendLog(line))
    const offRow = EventsOn('process:row', (row: OrderRow) => appendRow(row))
    const offDone = EventsOn('process:done', () => setProcessing(false))
    const offDrop = EventsOn('files:dropped', (paths: string[]) => addFiles(paths))

    return () => {
      offLog()
      offRow()
      offDone()
      offDrop()
    }
  }, [appendLog, appendRow, setProcessing, addFiles])
}
