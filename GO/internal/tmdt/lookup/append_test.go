package lookup

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// newLookupWorkbook dựng workbook có 2 bảng tra cứu tối thiểu: 1 dòng
// tiêu đề + 2 dòng dữ liệu ở "data shop", 1 shop ở "Mã misa".
func newLookupWorkbook(t *testing.T) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for _, s := range []string{SheetDataShop, SheetMisa} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet(%q): %v", s, err)
		}
	}
	f.DeleteSheet("Sheet1")

	dataShop := [][]interface{}{
		{"Tên sản phẩm", "Phân loại", "Mã combo", "MÃ TP 1", "SLTP1", "MÃ TP 2", "SLTP2", "MÃ TP 3", "SLTP3", "MÃ TP 4", "SLTP4"},
		{"Sản phẩm A", "Loại 1", "SP001", "TP001", "1", "", "", "", "", "", ""},
		{"Sản phẩm B", "Loại 2", "SP002", "TP002", "2", "", "", "", "", "", ""},
	}
	for i, row := range dataShop {
		r := row
		if err := f.SetSheetRow(SheetDataShop, sprintfAxis(i+1), &r); err != nil {
			t.Fatalf("SetSheetRow: %v", err)
		}
	}
	misa := [][]interface{}{
		{"", "Tên Kênh", "KÊNH BÁN", "Mã MISA"},
		{"", "", "", ""},
		{"", "Shop X", "SHOPEE", "MN_TMDT_00001"},
	}
	for i, row := range misa {
		r := row
		if err := f.SetSheetRow(SheetMisa, sprintfAxis(i+1), &r); err != nil {
			t.Fatalf("SetSheetRow: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "lookup.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	return path
}

func sprintfAxis(row int) string {
	cell, _ := excelize.CoordinatesToCellName(1, row)
	return cell
}

func TestAppendComboRows(t *testing.T) {
	path := newLookupWorkbook(t)

	firstRow, err := AppendComboRows(path, []ComboRow{{
		Product: "Sản phẩm mới", Variant: "Combo 3 Túi", Combo: "SP777",
		TP:      [4]string{"TP777", "TP888", "", ""},
		SL:      [4]string{"3", "1", "", ""},
	}})
	if err != nil {
		t.Fatalf("AppendComboRows: %v", err)
	}
	if firstRow != 4 {
		t.Fatalf("firstRow = %d, muốn 4 (ngay dưới 3 dòng đã có)", firstRow)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở lại: %v", err)
	}
	defer f.Close()

	want := map[string]string{
		"A4": "Sản phẩm mới", "B4": "Combo 3 Túi", "C4": "SP777",
		"D4": "TP777", "E4": "3", "F4": "TP888", "G4": "1",
		"H4": "", "I4": "", "J4": "", "K4": "",
	}
	for cell, expect := range want {
		got, _ := f.GetCellValue(SheetDataShop, cell)
		if got != expect {
			t.Errorf("%s = %q, muốn %q", cell, got, expect)
		}
	}
	// Dòng cũ không được đụng tới.
	if got, _ := f.GetCellValue(SheetDataShop, "A2"); got != "Sản phẩm A" {
		t.Errorf("A2 = %q — dòng có sẵn bị ghi đè", got)
	}
	// Ghi vào "data shop" không được đụng tới sheet "Mã misa" — đó là bảng
	// tra cứu KHÁC, người dùng gõ tay riêng, mất một dòng ở đây là mất dữ
	// liệu thật không lấy lại được.
	if v, _ := f.GetCellValue(SheetMisa, "B1"); v != "Tên Kênh" {
		t.Errorf("Mã misa!B1 = %q — tiêu đề bảng Mã misa bị hỏng", v)
	}
	if v, _ := f.GetCellValue(SheetMisa, "B3"); v != "Shop X" {
		t.Errorf("Mã misa!B3 = %q — dòng shop có sẵn trong Mã misa bị hỏng", v)
	}
	f.Close()

	// Nạp lại: mã mới phải tra được ngay, cả hai nhánh tra.
	tb, err := Load(path)
	if err != nil {
		t.Fatalf("Load sau khi bổ sung: %v", err)
	}
	row, ok := tb.ByCombo("SP777")
	if !ok {
		t.Fatalf("ByCombo(SP777) không tìm thấy")
	}
	if row.TP[0] != "TP777" || row.SL[1] != "1" {
		t.Errorf("dòng vừa bổ sung sai: %+v", row)
	}
	// KHÔNG kiểm ByProductVariant ở đây: dòng vừa thêm có Mã combo
	// ("SP777") không rỗng, và byProductVariant trong FromRows chỉ index
	// theo khoá Product+Variant+Combo — nên chỉ khớp được qua
	// ByProductVariant(product, variant) khi Combo của dòng đó RỖNG (xem
	// TestLoadAndLookup: "chỉ khớp dòng bỏ trống Mã combo"). Đây là hành vi
	// gốc, không phải lỗi của AppendComboRows. Trường hợp Combo rỗng được
	// kiểm riêng ở TestAppendComboRowsFindableByProductVariantWhenComboBlank.

	// "Mã misa" vẫn tra được đúng shop đã có từ trước — bổ sung "data shop"
	// không làm hỏng bảng tra cứu shop.
	if c, ok := tb.MisaCode("Shop X"); !ok || c != "MN_TMDT_00001" {
		t.Errorf("MisaCode(Shop X) = %q, ok=%v — Mã misa bị ảnh hưởng sau khi bổ sung data shop", c, ok)
	}
}

func TestAppendComboRowsFindableByProductVariantWhenComboBlank(t *testing.T) {
	// Đây chính là hình dạng dòng mà modal "mã còn thiếu" (Task 11) sinh ra
	// cho một dòng hàng KHÔNG có Mã sản phẩm: Combo để trống. Theo khoá mà
	// FromRows dựng (Product+Variant+Combo), đây là DUY NHẤT hình dạng mà một
	// dòng no-SKU có thể được ByProductVariant tra thấy — combo không rỗng
	// thì không bao giờ khớp (xem TestAppendComboRows). Nếu đường này hỏng,
	// app sẽ hỏi lại đúng sản phẩm đã khai ở MỌI lần chạy sau: lời hứa
	// "không hỏi lại lần sau" của cả tính năng AppendComboRows vỡ ngay.
	path := newLookupWorkbook(t)

	if _, err := AppendComboRows(path, []ComboRow{{
		Product: "Sản phẩm không SKU", Variant: "Loại Duy Nhất", Combo: "",
		TP: [4]string{"TP999", "", "", ""},
		SL: [4]string{"5", "", "", ""},
	}}); err != nil {
		t.Fatalf("AppendComboRows: %v", err)
	}

	tb, err := Load(path)
	if err != nil {
		t.Fatalf("Load sau khi bổ sung: %v", err)
	}
	row, ok := tb.ByProductVariant("Sản phẩm không SKU", "Loại Duy Nhất")
	if !ok {
		t.Fatalf("ByProductVariant không tìm thấy dòng Combo rỗng vừa bổ sung — app sẽ hỏi lại sản phẩm này mỗi lần chạy")
	}
	if row.TP[0] != "TP999" || row.SL[0] != "5" {
		t.Errorf("dòng vừa bổ sung sai: %+v", row)
	}
}

func TestAppendComboRowsSkipsTrailingBlankRows(t *testing.T) {
	// Bảng gõ tay hay có dòng trống ở cuối; dòng mới phải chèn ngay dưới
	// dòng CÓ DỮ LIỆU cuối cùng, không nhảy xuống sau vùng trống.
	path := newLookupWorkbook(t)
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở: %v", err)
	}
	if err := f.SetCellValue(SheetDataShop, "A9", ""); err != nil {
		t.Fatalf("SetCellValue: %v", err)
	}
	if err := f.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.Close()

	firstRow, err := AppendComboRows(path, []ComboRow{{Product: "X", Combo: "SPX", TP: [4]string{"TPX"}, SL: [4]string{"1"}}})
	if err != nil {
		t.Fatalf("AppendComboRows: %v", err)
	}
	if firstRow != 4 {
		t.Errorf("firstRow = %d, muốn 4 — dòng trống cuối bảng không được tính", firstRow)
	}
}

func TestAppendComboRowsEmptyIsNoop(t *testing.T) {
	path := newLookupWorkbook(t)
	if _, err := AppendComboRows(path, nil); err != nil {
		t.Fatalf("AppendComboRows(nil): %v", err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("workbook bị hỏng sau lần gọi rỗng: %v", err)
	}
}
