// Package tmdt là nhánh xử lý đơn thương mại điện tử (Shopee/TikTok Shop
// đồng bộ qua Haravan) — song song với các nhánh vendor siêu thị dạng PDF
// trong internal/processing.
package tmdt

import (
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/tmdt/lookup"
)

// SheetHaravan là sheet app ghi dữ liệu đơn vào, nằm ngay trong workbook
// tra cứu của người dùng. Người dùng đã tạo sẵn sheet trống tên đúng như
// vậy; nếu không có, WriteHaravanSheet tự tạo.
const SheetHaravan = "Haravan"

// IsWorkbook nhận diện workbook TMĐT bằng SỰ CÓ MẶT CỦA HAI BẢNG TRA CỨU,
// không bằng tên file. Tên file thay đổi theo tháng ("XUẤT HÀNG 25-08
// HN-LA MỚI.xlsx", "XUẤT HÀNG HN-LA MỚI.xlsx"...) nên bắt theo tên vừa
// mong manh vừa dễ nhận nhầm; hai sheet "data shop" + "Mã misa" thì chỉ
// workbook này mới có, và cũng chính là thứ nhánh TMĐT thực sự cần đọc.
//
// Sheet "Haravan" KHÔNG nằm trong điều kiện: đó là sheet đầu ra, app tự
// tạo nếu thiếu.
//
// Mọi lỗi (không phải xlsx, file hỏng, không tồn tại) đều trả về false
// chứ không phải error: hàm này chạy trên từng file người dùng thả vào,
// và "không phải file TMĐT" là câu trả lời đúng cho mọi trường hợp đó.
func IsWorkbook(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return false
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		return false
	}
	defer f.Close()

	has := map[string]bool{}
	for _, name := range f.GetSheetList() {
		has[strings.ToLower(strings.TrimSpace(name))] = true
	}
	return has[strings.ToLower(lookup.SheetDataShop)] && has[strings.ToLower(lookup.SheetMisa)]
}
