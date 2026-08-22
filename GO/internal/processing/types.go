package processing

// PriceMismatchDetail is ONE product flagged as a price mismatch —
// surfaced so the user can review it after processing and choose, per
// SKU, whether to keep the PO's own invoice price or the system's
// computed price (see excelwriter.ConfirmPrice). ExcelRow is the real
// row number in the "Don dat hang" sheet this product was written to —
// required to edit the right cell later, since nothing else in the
// returned OrderRow identifies individual product rows.
type PriceMismatchDetail struct {
	SKU          string  `json:"sku"`
	ProductName  string  `json:"productName"`
	InvoicePrice float64 `json:"invoicePrice"`
	SystemPrice  float64 `json:"systemPrice"`
	ExcelRow     int     `json:"excelRow"`
}

// OrderRow là một dòng trong bảng kết quả, ánh xạ đúng các cột của bảng
// gốc: Tên file, Trang, Hệ thống, Mã khách hàng, PO, Đơn giá, Trạng thái.
type OrderRow struct {
	FileName    string `json:"fileName"`
	Page        string `json:"page"`
	System      string `json:"system"`
	MaKhachHang string `json:"maKhachHang"`
	PO          string `json:"po"`
	DonGia      string `json:"donGia"`
	Status      string `json:"status"`
	StatusKind  string `json:"statusKind"`

	// DriveURL is the constructed "view" link from driveupload.Upload -
	// populated the moment a row is built (fire-and-forget: the real
	// upload may still be in progress or even fail in the background,
	// this URL is a best-effort placeholder from the start). Empty
	// string if the row's file was never uploaded (e.g. a Failed row
	// with no successfully-written Excel data to link to).
	DriveURL string `json:"driveUrl"`

	// PriceMismatchCount is the same "saigia" count already embedded in
	// Status's text ("... - Có N mã sai giá") — surfaced as its own typed
	// field so the frontend can render a dedicated price-reconciliation
	// column without parsing the number back out of a display string. 0
	// for a StatusKindFailed row (pricing was never evaluated), not a
	// meaningful "all correct" signal in that case — the frontend must
	// check StatusKind first.
	PriceMismatchCount int `json:"priceMismatchCount"`

	// SkuLog holds one formatted diagnostic line per product this row's
	// underlying file/page produced (see formatSkuLogLine in
	// processor_shared.go): price-match status and any promotion
	// detected. Populated by RealProcessor's vendor-specific segment
	// handlers as they compute matched/khuyenmai for each product — not
	// a new computation, just surfacing values already derived for the
	// Excel write. Not part of the Excel output or any golden fixture;
	// app.go emits these via "process:log" before this row's own
	// "process:row" event, so they're not serialized to JSON either.
	SkuLog []string `json:"-"`

	// PriceMismatchDetails holds one entry per product flagged as a
	// price mismatch in this row's file/page — unlike SkuLog, this IS
	// serialized to JSON (sent to the frontend as part of "process:row"),
	// since the frontend needs it to let the user review/resolve each
	// mismatch after processing.
	PriceMismatchDetails []PriceMismatchDetail `json:"priceMismatchDetails"`
}

// Các giá trị Status giữ nguyên ký hiệu (emoji) của bản gốc để hiển thị
// đúng ngữ nghĩa cũ; StatusKind là discriminator kiểu (typed) dùng để
// frontend phân loại màu/icon mà không phụ thuộc vào khớp chuỗi con emoji.
const (
	StatusDone    = "✅ Hoàn Thành"
	StatusWarning = "⚠️ Hoàn Thành"
	StatusFailed  = "❌ Thất bại"

	StatusKindDone    = "done"
	StatusKindWarning = "warning"
	StatusKindFailed  = "failed"
)
