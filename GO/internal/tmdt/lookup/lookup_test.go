package lookup

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// buildWorkbook dựng workbook tra cứu tối giản theo đúng cấu trúc file thật.
func buildWorkbook(t *testing.T) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	if _, err := f.NewSheet(SheetDataShop); err != nil {
		t.Fatal(err)
	}
	dataShop := [][]any{
		{"Tên sản phẩm", "Phân loại", "Mã combo", "MÃ TP 1", "SLTP1", "MÃ TP 2", "SLTP2", "MÃ TP 3", "SLTP3", "MÃ TP 4", "SLTP4"},
		// Dòng có Mã combo: dùng khi đơn có Mã sản phẩm.
		{"Bột tẩy lồng", "Combo 3 Túi", "SP000442", "TP10127", 3},
		// Dòng bỏ trống Mã combo: dùng khi đơn KHÔNG có Mã sản phẩm (tra theo tên + phân loại).
		{"Bột tẩy lồng", "Combo 3 Túi", "", "TP10127", 3},
		{"Combo 2 món", "Bộ đôi", "SP999", "TP001", 1, "TP002", 2},
	}
	for i, r := range dataShop {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow(SheetDataShop, cell, &r); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := f.NewSheet(SheetMisa); err != nil {
		t.Fatal(err)
	}
	misa := [][]any{
		{"", "Tên Kênh", "KÊNH BÁN", "Mã MISA"},
		{},
		{"", "Blue Việt Nam", "TIKTOK", "MN_TMDT_00015"},
		{"", "Tẩy lồng máy giặt Blue", "TIKTOK", "MN_TMDT_00016"},
	}
	for i, r := range misa {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow(SheetMisa, cell, &r); err != nil {
			t.Fatal(err)
		}
	}
	f.DeleteSheet("Sheet1")

	path := filepath.Join(t.TempDir(), "mapping.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAndLookup(t *testing.T) {
	tables, err := Load(buildWorkbook(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tables.Combos != 3 || tables.Misa != 2 {
		t.Errorf("đếm được %d dòng sản phẩm / %d shop, want 3/2", tables.Combos, tables.Misa)
	}

	// Tra theo Mã sản phẩm.
	if r, ok := tables.ByCombo("SP000442"); !ok || r.TP[0] != "TP10127" || r.SL[0] != "3" {
		t.Errorf("ByCombo(SP000442) = %+v, ok=%v", r, ok)
	}
	// VLOOKUP của Excel không phân biệt hoa thường.
	if _, ok := tables.ByCombo("sp000442"); !ok {
		t.Error("ByCombo phải không phân biệt hoa thường")
	}
	// Tra theo tên + phân loại: chỉ khớp dòng có Mã combo để trống.
	if r, ok := tables.ByProductVariant("Bột tẩy lồng", "Combo 3 Túi"); !ok || r.Combo != "" {
		t.Errorf("ByProductVariant = %+v, ok=%v — phải khớp dòng bỏ trống Mã combo", r, ok)
	}
	if _, ok := tables.ByProductVariant("Không có", "Gì cả"); ok {
		t.Error("ByProductVariant phải trả về không tìm thấy")
	}
	// Nhiều thành phần.
	if r, ok := tables.ByCombo("SP999"); !ok || r.TP[1] != "TP002" || r.SL[1] != "2" {
		t.Errorf("ByCombo(SP999) thành phần 2 = %+v", r)
	}

	if c, ok := tables.MisaCode("Tẩy lồng máy giặt Blue"); !ok || c != "MN_TMDT_00016" {
		t.Errorf("MisaCode = %q, ok=%v", c, ok)
	}
	if _, ok := tables.MisaCode("CLEVY VIỆT NAM"); ok {
		t.Error("shop chưa khai báo phải trả về không tìm thấy")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "khong-ton-tai.xlsx")); err == nil {
		t.Fatal("mong đợi lỗi khi workbook không tồn tại")
	}
}
