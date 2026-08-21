package processing

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
