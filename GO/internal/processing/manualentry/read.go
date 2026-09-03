// Package manualentry đọc "đơn hàng tay.xlsx" - đơn hàng người dùng tự
// gõ tay khi file gốc không đọc được tự động (vd PDF thực chất là ảnh
// chụp/scan, không có lớp văn bản - xem coop_processor.go's "trang không
// có văn bản" check). Nhận diện qua TÊN SHEET (SheetName), không phải
// tên file - người dùng đặt tên file gì cũng được, giống cách
// internal/tmdt/detect.go nhận diện workbook TMĐT qua tên sheet.
package manualentry

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// SheetName là tên sheet DUY NHẤT app tìm để nhận diện 1 workbook là đơn
// hàng nhập tay - khớp đúng file mẫu đã tạo cho người dùng
// ("đơn hàng tay.xlsx", sheet "Đơn hàng tay").
const SheetName = "Đơn hàng tay"

// exampleRowPrefix đánh dấu dòng ví dụ minh hoạ trong file mẫu (cột PO
// bắt đầu bằng "VD-") - Load bỏ qua dòng này, không coi là 1 đơn thật.
const exampleRowPrefix = "VD-"

// Line là 1 dòng sản phẩm trong sheet - đúng 9 cột đã chốt thiết kế,
// đọc thô (chưa quy đổi mã hàng qua Store.ResolveSku, chưa gộp theo PO -
// đó là việc của caller, xem processing.processManualEntryDocument).
type Line struct {
	PO           string
	EntryDate    string
	CancelDate   string
	System       string
	CustomerCode string
	ShipTo       string
	RawSKU       string
	Qty          float64
	InvoicePrice float64
}

// IsWorkbook báo file có phải workbook đơn hàng nhập tay hay không - kiểm
// tra ĐUÔI FILE trước (đỡ tốn công mở file .pdf/.txt không liên quan gì),
// rồi mới mở thật để soi tên sheet. Không mở được (không phải xlsx hợp
// lệ, hoặc đang bị khoá) trả về false, KHÔNG lỗi - để caller tự nhiên rơi
// về đường xử lý PDF thường và tự báo lỗi phù hợp ở đó.
func IsWorkbook(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return false
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		return false
	}
	defer f.Close()
	for _, name := range f.GetSheetList() {
		if name == SheetName {
			return true
		}
	}
	return false
}

// Load đọc toàn bộ dòng sản phẩm hợp lệ trong sheet - bỏ qua dòng tiêu
// đề (dòng 1), dòng ví dụ minh hoạ, và dòng có cột PO rỗng (dòng trống
// người dùng chưa xoá). KHÔNG validate gì thêm ở đây (SKU rỗng, Qty<=0)
// - caller tự quyết định cách xử lý dòng thiếu dữ liệu theo từng đơn.
func Load(path string) ([]Line, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("manualentry: mở %s: %w", path, err)
	}
	defer f.Close()

	// RawCellValue: true - giống lý do productdata.Store.Load đã ghi chú:
	// không có nó, GetRows trả về chuỗi ĐÃ LÀM TRÒN theo định dạng hiển
	// thị của ô (vd số lượng/giá có thể bị làm tròn sai) thay vì giá trị
	// số thật.
	rows, err := f.GetRows(SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("manualentry: đọc sheet %q: %w", SheetName, err)
	}

	// Rất hiếm, nhưng workbook có thể dùng hệ ngày 1904 (mặc định cũ của
	// Excel bản Mac) - lệch 4 năm nếu quy đổi số sê-ri bằng hệ 1900.
	date1904 := false
	if props, propsErr := f.GetWorkbookProps(); propsErr == nil && props.Date1904 != nil {
		date1904 = *props.Date1904
	}

	cell := func(row []string, idx int) string {
		if idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}
	parseNumber := func(s string) float64 {
		n, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
		return n
	}

	var lines []Line
	for i, row := range rows {
		if i == 0 {
			continue // dòng tiêu đề
		}
		po := cell(row, 0)
		if po == "" || strings.HasPrefix(po, exampleRowPrefix) {
			continue
		}
		lines = append(lines, Line{
			PO:           po,
			EntryDate:    dateCell(cell(row, 1), date1904),
			CancelDate:   dateCell(cell(row, 2), date1904),
			System:       cell(row, 3),
			CustomerCode: cell(row, 4),
			ShipTo:       cell(row, 5),
			RawSKU:       cell(row, 6),
			Qty:          parseNumber(cell(row, 7)),
			InvoicePrice: parseNumber(cell(row, 8)),
		})
	}
	return lines, nil
}

// maxExcelSerial là 31/12/9999 trong hệ ngày 1900 - số sê-ri lớn nhất
// chính Excel còn chấp nhận là một ngày.
const maxExcelSerial = 2958465

// dateCell đổi ô ngày về đúng chuỗi "DD/MM/YYYY" mà cả pipeline dùng.
//
// Load đọc với RawCellValue (bắt buộc, xem ghi chú ở trên: nếu không Số
// lượng/Đơn giá sẽ bị làm tròn theo định dạng hiển thị) - và chính vì
// thế, ô nào người dùng để Excel tự nhận là NGÀY sẽ trả về số sê-ri thô
// ("46235") chứ không phải "01/08/2026". Một giá trị đó làm hỏng hai
// chỗ cùng lúc: nó được ghi thẳng vào cột Ngày đặt/Hạn giao của
// dondathang.xlsx, và nó chính là timeToCheck mà
// pricing.isWithinDateRange đem so với tên cột CTKM - số sê-ri không
// khớp cột "D/M-D/M" nào cả, nên đơn IM LẶNG không được áp CTKM nào.
//
// Ô người dùng gõ tay dạng chữ ("01/08/2026") vốn đã đúng nên trả
// nguyên, ô rỗng cũng vậy: chỉ số nằm trong khoảng sê-ri hợp lệ mới quy
// đổi, nên một số gõ nhầm kiểu "1082026" không bị biến thành ngày.
func dateCell(raw string, date1904 bool) string {
	serial, err := strconv.ParseFloat(raw, 64)
	if err != nil || serial < 1 || serial > maxExcelSerial {
		return raw
	}
	t, err := excelize.ExcelDateToTime(serial, date1904)
	if err != nil {
		return raw
	}
	return t.Format("02/01/2006")
}
