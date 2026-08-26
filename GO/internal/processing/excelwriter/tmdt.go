package excelwriter

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// Hằng số nghiệp vụ của đơn TMĐT — mọi dòng đều giống nhau, xác nhận trên
// cả 1.430 dòng của "đơn hàng/mẫu chuẩn.xlsx", không có ngoại lệ nào.
const (
	tmdtStatus   = "Chưa thực hiện" // C
	tmdtNoteRow  = "Không"          // T — đơn TMĐT không có dòng ghi chú
	tmdtVAT      = 8                // AE
	tmdtDebtDays = 15               // AV
)

// TMDTRow là một dòng đặt hàng TMĐT. Mỗi dòng hàng trên sàn sinh ra MỘT
// TMDTRow cho MỖI mã thành phẩm nó quy đổi ra (combo 2 thành phẩm → 2
// TMDTRow), không gộp trùng.
//
// Vì sao không dùng lại Row của nhánh vendor: writeRow ghi AT/AU cho mọi
// dòng không phải dòng ghi chú, còn mẫu chuẩn TMĐT để trống hai ô đó.
// Thêm cờ phủ định nữa vào Row — vốn đã mang sẵn 6 biệt lệ riêng của
// từng vendor (NoCaseCount, StoreName, SiteCode, UseZFormula, IsNoteRow,
// PriceMismatch) — sẽ làm struct đó khó đọc hơn phần tiết kiệm được.
//
// Cột Z (Thành tiền) thì GIỐNG nhánh vendor: cùng công thức "Y{n}*X{n}".
type TMDTRow struct {
	EntryDate    string  // A và D (ngày đơn hàng = ngày giao hàng)
	OrderNumber  string  // B — "ĐĐHTMĐT-{kênh}-{mã đơn}"
	ShipTo       string  // E — "HN" | "LA"
	CustomerCode string  // G — mã MISA
	Description  string  // L
	SKU          string  // Q — mã thành phẩm
	ProductName  string  // S
	IsPromoItem  bool    // U — true khi đơn giá = 0 (hàng tặng)
	Warehouse    string  // V — "TP_HN_12" | "LA_KHOTMDT"
	Qty          float64 // X
	UnitPrice    float64 // Y
	RegionCode   string  // AJ — "TMĐT_MB" | "TMĐT_MN"
	StatCode     string  // AM — "HN" | "LA"
	Note         string  // AO — mã đơn hàng gốc trên sàn
}

// WriteTMDTRows ghi nối tiếp rows vào sheet "Don dat hang", trả về số
// dòng đầu tiên đã ghi. Không tự gọi ClearOrderRows: batch đã dọn file
// một lần ở đầu (xem runReservedBatch), nên hàm này chỉ append — nhờ đó
// một batch trộn file PDF vendor với file TMĐT vẫn ra đủ cả hai.
func WriteTMDTRows(path string, rows []TMDTRow) (startRow int, err error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return 0, fmt.Errorf("excelwriter: mở %s: %w", path, err)
	}
	defer f.Close()

	existing, err := f.GetRows(sheetName)
	if err != nil {
		return 0, fmt.Errorf("excelwriter: đọc %s: %w", sheetName, err)
	}
	// Dữ liệu bắt đầu từ dòng 9 (dòng 1–8 là khối tiêu đề khuôn AMIS).
	currentRow := len(existing) + 1
	if currentRow < 9 {
		currentRow = 9
	}
	firstRow := currentRow

	if len(rows) == 0 {
		return firstRow, nil
	}

	yesNo := func(b bool) string {
		if b {
			return "Có"
		}
		return "Không"
	}

	for _, row := range rows {
		writes := []struct {
			col   string
			value interface{}
		}{
			{"A", row.EntryDate}, {"B", row.OrderNumber}, {"C", tmdtStatus},
			{"D", row.EntryDate}, {"E", row.ShipTo}, {"G", row.CustomerCode},
			{"L", row.Description}, {"Q", row.SKU}, {"S", row.ProductName},
			{"T", tmdtNoteRow}, {"U", yesNo(row.IsPromoItem)}, {"V", row.Warehouse},
			{"X", row.Qty}, {"Y", row.UnitPrice}, {"AE", tmdtVAT},
			{"AJ", row.RegionCode}, {"AM", row.StatCode}, {"AO", row.Note},
			{"AV", tmdtDebtDays},
		}
		for _, w := range writes {
			cell := fmt.Sprintf("%s%d", w.col, currentRow)
			if err := f.SetCellValue(sheetName, cell, w.value); err != nil {
				return 0, fmt.Errorf("excelwriter: ghi %s: %w", cell, err)
			}
		}
		// Z (Thành tiền) là CÔNG THỨC, không phải số đã tính sẵn — giống
		// writeRow của nhánh vendor và giống mẫu chuẩn TMĐT. Ghi công thức
		// để người dùng sửa tay X hoặc Y trong Excel thì thành tiền đi theo.
		if err := f.SetCellFormula(sheetName, fmt.Sprintf("Z%d", currentRow),
			fmt.Sprintf("Y%d*X%d", currentRow, currentRow)); err != nil {
			return 0, fmt.Errorf("excelwriter: ghi công thức Z%d: %w", currentRow, err)
		}
		currentRow++
	}

	if err := f.Save(); err != nil {
		return 0, fmt.Errorf("excelwriter: lưu %s: %w", path, err)
	}
	return firstRow, nil
}
