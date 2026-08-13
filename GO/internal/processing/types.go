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
}

// Các giá trị Status giữ nguyên ký hiệu (emoji) của bản gốc để frontend
// phân loại màu/icon theo đúng ngữ nghĩa cũ.
const (
	StatusDone    = "✅ Hoàn Thành"
	StatusWarning = "⚠️ Hoàn Thành"
	StatusFailed  = "❌ Thất bại"
)
