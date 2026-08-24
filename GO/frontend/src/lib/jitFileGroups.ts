import type { OrderRow } from '../types'

export interface JITFileGroup {
  sourceId: string
  fileName: string
  orderDate: string
  warehouse: string
  period: string
  excelRows: number[]
  orderCount: number
}

export function skipsPriceReconciliation(row: OrderRow): boolean {
  return row.system === 'JIT-CHOICE'
}

export function groupJITFiles(rows: OrderRow[]): JITFileGroup[] {
  const groups = new Map<string, JITFileGroup>()
  for (const row of rows) {
    if (row.system !== 'JIT-CHOICE' || !row.sourceId || !row.fileName || (row.excelRows?.length ?? 0) === 0) continue
    let group = groups.get(row.sourceId)
    if (!group) {
      group = {
        sourceId: row.sourceId,
        fileName: row.fileName,
        orderDate: row.entryDate,
        warehouse: row.shipTo,
        period: row.jitPeriod,
        excelRows: [],
        orderCount: 0,
      }
      groups.set(row.sourceId, group)
    }
    group.orderCount++
    for (const excelRow of row.excelRows) {
      if (!group.excelRows.includes(excelRow)) group.excelRows.push(excelRow)
    }
  }
  return [...groups.values()]
}
