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
			EntryDate:    cell(row, 1),
			CancelDate:   cell(row, 2),
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
