import type { PriceMismatchDetail } from '../types'

export function belowSystemPriceDetails(details: PriceMismatchDetail[]): PriceMismatchDetail[] {
  return details.filter((detail) => detail.invoicePrice < detail.systemPrice)
}
