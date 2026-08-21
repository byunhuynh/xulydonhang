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
}

export interface LogEntry {
  time: string
  text: string
}
