import type { OrderRow } from '../types'

export interface JITFileGroup {
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
    if (row.system !== 'JIT-CHOICE' || !row.fileName || (row.excelRows?.length ?? 0) === 0) continue
    let group = groups.get(row.fileName)
    if (!group) {
      group = {
        fileName: row.fileName,
        orderDate: row.entryDate,
        warehouse: row.shipTo,
        period: row.jitPeriod,
        excelRows: [],
        orderCount: 0,
      }
      groups.set(row.fileName, group)
    }
    group.orderCount++
    for (const excelRow of row.excelRows) {
      if (!group.excelRows.includes(excelRow)) group.excelRows.push(excelRow)
    }
  }
  return [...groups.values()]
}
