import type { OrderRow, PriceMismatchDetail } from '../types'

export interface ScopedMismatch {
  rowIndex: number
  detail: PriceMismatchDetail
}

// BigC represents one PO as multiple result rows (one per store page), and
// its cover page is intentionally not a result row. Scope by PO identity,
// never by page number or by whichever row the user expanded.
export function mismatchesForPO(rows: OrderRow[], po: string): ScopedMismatch[] {
  const result: ScopedMismatch[] = []
  rows.forEach((row, rowIndex) => {
    if (row.po !== po) return
    for (const detail of row.priceMismatchDetails ?? []) {
      result.push({ rowIndex, detail })
    }
  })
  return result
}
