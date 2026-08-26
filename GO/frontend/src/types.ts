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
  sourceId: string
  page: string
  system: string
  maKhachHang: string
  po: string
  resultKey: string
  maVanDon: string
  donGia: string
  status: string
  statusKind: string
  excelRows: number[]
  jitPeriod: string
  driveUrl: string
  priceMismatchCount: number
  priceMismatchDetails: PriceMismatchDetail[]
  shipTo: string
  entryDate: string
  cancelDate: string
  totalWeightKg: string
  totalPackages: number
  // Tổng SỐ LƯỢNG sản phẩm (Qty cộng dồn qua mọi dòng) - chỉ JIT gán giá
  // trị này (xem OrderRow.TotalQty, types.go), 0 ở mọi vendor khác.
  totalQty: number
  // Mọi mã SKU dòng này đã ghi, CHƯA loại trùng (1 dòng/PO có thể lặp
  // lại 1 SKU ở dòng khuyến mãi riêng - đó là dữ liệu thật). Loại trùng
  // qua NHIỀU dòng (vd 1 SKU xuất hiện ở nhiều PO khác nhau trong CÙNG 1
  // file JIT) là việc của bên gọi sau khi đã gộp nhóm - excelRows.length
  // KHÔNG phải số mã khác nhau (mỗi lần ghi luôn ra 1 số dòng Excel mới,
  // cộng dồn qua nhiều PO chỉ ra tổng số DÒNG, không phải số SKU khác
  // nhau - đã xác nhận sai qua thực tế: 170 PO gần như 1 SKU/đơn báo
  // "173 mã hàng" trong khi số SKU khác nhau thật ít hơn nhiều).
  skus: string[]
  // Số ĐƠN duy nhất một dòng tóm tắt đại diện — chỉ nhánh TMĐT gán (1
  // dòng = 1 nhóm shop+ngày, không phải 1 đơn), 0 ở mọi vendor khác vì
  // ở đó số đơn chính là số dòng.
  totalOrders: number
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
  haravan: Record<string, string>
  misa: Record<string, string>
  misa_routing: Record<string, string>
}
