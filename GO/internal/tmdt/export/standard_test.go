package export

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/tmdt/haravan"
	"order-processor/internal/tmdt/lookup"
)

// mappingWorkbook dựng workbook tra cứu tối giản đúng cấu trúc file thật.
func mappingWorkbook(t *testing.T) *lookup.Tables {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	for _, s := range []string{lookup.SheetDataShop, lookup.SheetMisa} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatal(err)
		}
	}
	write := func(sheet string, rows [][]any) {
		for i, r := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if err := f.SetSheetRow(sheet, cell, &r); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(lookup.SheetDataShop, [][]any{
		{"Tên sản phẩm", "Phân loại", "Mã combo", "MÃ TP 1", "SLTP1", "MÃ TP 2", "SLTP2", "MÃ TP 3", "SLTP3", "MÃ TP 4", "SLTP4"},
		{"Bột tẩy lồng", "Combo 3 Túi", "SP000442", "TP10127", 3},
		{"Bột tẩy lồng", "1 Túi", "", "TP10127", 1},
		{"Combo 2 món", "Bộ đôi", "SP999", "TP001", 1, "TP002", 2},
	})
	write(lookup.SheetMisa, [][]any{
		{"", "Tên Kênh", "KÊNH BÁN", "Mã MISA"},
		{},
		{"", "Tẩy lồng máy giặt Blue", "TIKTOK", "MN_TMDT_00016"},
	})
	f.DeleteSheet("Sheet1")

	path := filepath.Join(t.TempDir(), "mapping.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	tables, err := lookup.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

func order(t *testing.T, raw string) *haravan.Order {
	t.Helper()
	var o haravan.Order
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		t.Fatal(err)
	}
	return &o
}

func TestStandardWriterComputesDerivedColumns(t *testing.T) {
	tables := mappingWorkbook(t)

	// Đơn 1: shop có mã MISA, một dòng có SKU (tra theo mã), một dòng không SKU
	// (tra theo tên + phân loại), một dòng combo 2 thành phần.
	o1 := order(t, `{
      "name": "585000000000000001",
      "created_at": "2026-08-23T14:21:55Z",
      "source_name": "tiktokshop",
      "location_name": "Kho Hà Nội",
      "subtotal_price": 139000.0,
      "total_price": 129000.0,
      "note_attributes": [{"name": "X-Haravan-SalesChannel-BranchName", "value": "Tẩy lồng máy giặt Blue"}],
      "line_items": [
        {"title": "Bột tẩy lồng", "variant_title": "Combo 3 Túi", "sku": "SP000442", "quantity": 1, "price": 139000.0},
        {"title": "Bột tẩy lồng", "variant_title": "1 Túi", "sku": "", "quantity": 2, "price": 29000.0},
        {"title": "Combo 2 món", "variant_title": "Bộ đôi", "sku": "SP999", "quantity": 1, "price": 99000.0},
        {"title": "Hàng lạ", "variant_title": "Chưa khai báo", "sku": "", "quantity": 1, "price": 1000.0}
      ]
    }`)

	// Đơn 2: shop CLEVY — bỏ trống toàn bộ MÃ TP/SLTP, và chưa có mã MISA.
	o2 := order(t, `{
      "name": "585000000000000002",
      "created_at": "2026-08-23T15:00:00Z",
      "source_name": "tiktokshop",
      "note_attributes": [{"name": "X-Haravan-SalesChannel-BranchName", "value": "CLEVY VIỆT NAM"}],
      "line_items": [
        {"title": "Bột tẩy lồng", "variant_title": "Combo 3 Túi", "sku": "SP000442", "quantity": 1, "price": 139000.0}
      ]
    }`)

	path := filepath.Join(t.TempDir(), "out.xlsx")
	w, err := NewStandardWriter(path, tables)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []*haravan.Order{o1, o2} {
		if err := w.AddOrder("TikTok Shop", o); err != nil {
			t.Fatal(err)
		}
	}
	if w.Count() != 2 {
		t.Errorf("Count = %d, want 2 đơn", w.Count())
	}
	warns := w.Warnings()
	if len(warns) != 2 {
		t.Errorf("Warnings = %d mục, want 2 (shop CLEVY + sản phẩm chưa khai báo): %v", len(warns), warns)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cell := func(col string, row int) string {
		v, err := f.GetCellValue(sheetHaravan, col+itoa(row))
		if err != nil {
			t.Fatalf("đọc %s%d: %v", col, row, err)
		}
		return v
	}

	for _, tc := range []struct{ col, want string }{
		{"A", "Mã đơn hàng"}, {"D", "Ngày đặt hàng"}, {"F", "Tên sản phẩm"},
		{"G", "Giá trị thuộc tính 1"}, {"I", "Mã sản phẩm"}, {"L", "Kênh bán hàng"},
		{"M", "Thời gian Đặt"}, {"N", "MÃ TP 1"}, {"O", "SLTP1"},
		{"P", "MÃ TP 2"}, {"Q", "SLTP2"}, {"R", "MÃ TP 3"}, {"S", "SLTP3"},
		{"T", "MÃ TP 4"}, {"U", "SLTP4"}, {"V", "Shop"}, {"W", "Mã misa"},
	} {
		if got := cell(tc.col, 1); got != tc.want {
			t.Errorf("tiêu đề %s = %q, want %q", tc.col, got, tc.want)
		}
	}

	type check struct {
		row  int
		col  string
		want string
		why  string
	}
	for _, c := range []check{
		{2, "N", "TP10127", "tra theo Mã sản phẩm SP000442"},
		{2, "O", "3", "SLTP1 của SP000442"},
		{2, "V", "Tẩy lồng máy giặt Blue", "tên shop lấy từ note_attributes"},
		{2, "W", "MN_TMDT_00016", "mã MISA tra theo tên shop"},
		{2, "M", "08-23-26", "ngày đặt theo giờ VN, định dạng như file cũ"},

		{3, "N", "TP10127", "không có SKU → tra theo tên + phân loại"},
		{3, "O", "1", "SLTP1 của dòng '1 Túi'"},

		{4, "N", "TP001", "combo 2 thành phần"},
		{4, "O", "1", ""},
		{4, "P", "TP002", ""},
		{4, "Q", "2", ""},
		{4, "R", "", "thành phần 3 để trống"},

		{5, "N", lookup.NotAvailable, "sản phẩm chưa khai báo trong data shop"},

		{6, "N", "", "shop CLEVY không quy đổi mã thành phẩm"},
		{6, "O", "", ""},
		{6, "V", "CLEVY VIỆT NAM", ""},
		{6, "W", lookup.NotAvailable, "CLEVY chưa có trong bảng Mã misa"},
	} {
		if got := cell(c.col, c.row); got != c.want {
			t.Errorf("dòng %d cột %s = %q, want %q (%s)", c.row, c.col, got, c.want, c.why)
		}
	}

	// 12 cột dữ liệu Haravan vẫn đúng chỗ, và sheet đúng 23 cột — không còn cột rỗng.
	if got := cell("A", 2); got != "585000000000000001" {
		t.Errorf("A2 = %q", got)
	}
	if got := cell("D", 2); got != "2026-08-23T21:21:55+07:00" {
		t.Errorf("D2 (Ngày đặt hàng) = %q", got)
	}
	rows, err := f.GetRows(sheetHaravan)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rows[0]); got != len(standardHeaders) {
		t.Errorf("sheet có %d cột, want %d", got, len(standardHeaders))
	}
	if got := cell("X", 2); got != "" {
		t.Errorf("không được có cột thứ 24, got %q", got)
	}
}
