import type { OrderRow } from '../types'

export function upsertOrderRow(rows: OrderRow[], incoming: OrderRow): OrderRow[] {
  const index = rows.findIndex((row) => row.resultKey === incoming.resultKey)
  if (index < 0) return [...rows, incoming]
  const next = [...rows]
  next[index] = incoming
  return next
}
