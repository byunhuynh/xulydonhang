export interface PriceMismatchDetail {
  sku: string
  productName: string
  invoicePrice: number
  systemPrice: number
  qty: number
  excelRow: number
  promoText: string
  promoDateRange: string
}

export interface PromoItem {
  sku: string
  productName: string
  qty: number
}

export interface OrderRow {
  fileName: string
  page: string
  system: string
  maKhachHang: string
  po: string
  donGia: string
  status: string
  statusKind: string
  driveUrl: string
  priceMismatchCount: number
  priceMismatchDetails: PriceMismatchDetail[]
  shipTo: string
  entryDate: string
  cancelDate: string
  totalWeightKg: string
  totalPackages: number
  promoItems: PromoItem[]
}

export interface LogEntry {
  time: string
  text: string
}

export interface AppSettings {
  gid: Record<string, string>
  zalo: Record<string, string>
  reminder: Record<string, string>
}
