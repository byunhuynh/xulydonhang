export interface PriceMismatchDetail {
  sku: string
  productName: string
  invoicePrice: number
  systemPrice: number
  excelRow: number
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
  priceMismatchCount: number
  priceMismatchDetails: PriceMismatchDetail[]
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
