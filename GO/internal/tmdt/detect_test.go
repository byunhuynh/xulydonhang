package tmdt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// makeWorkbook dựng 1 file xlsx tạm với đúng danh sách sheet cho trước.
func makeWorkbook(t *testing.T, name string, sheets ...string) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for _, s := range sheets {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("NewSheet(%q): %v", s, err)
		}
	}
	f.DeleteSheet("Sheet1")
	path := filepath.Join(t.TempDir(), name)
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
	return path
}

func TestIsWorkbook(t *testing.T) {
	full := makeWorkbook(t, "full.xlsx", "Mã misa", "data shop", "Haravan")
	if !IsWorkbook(full) {
		t.Errorf("workbook đủ 3 sheet phải được nhận là workbook TMĐT")
	}

	// Thiếu sheet Haravan: app sẽ tự tạo sheet đó, nên vẫn phải nhận.
	noHaravan := makeWorkbook(t, "no-haravan.xlsx", "Mã misa", "data shop")
	if !IsWorkbook(noHaravan) {
		t.Errorf("thiếu mỗi sheet Haravan vẫn phải nhận là workbook TMĐT")
	}

	// Thiếu bảng tra cứu: không đủ dữ liệu để quy đổi, không phải file TMĐT.
	noLookup := makeWorkbook(t, "no-lookup.xlsx", "Haravan", "Sheet2")
	if IsWorkbook(noLookup) {
		t.Errorf("thiếu bảng tra cứu thì không phải workbook TMĐT")
	}

	// dondathang.xlsx tuyệt đối không được nhận nhầm.
	dondathang := makeWorkbook(t, "dondathang.xlsx", "Don dat hang")
	if IsWorkbook(dondathang) {
		t.Errorf("dondathang.xlsx không phải workbook TMĐT")
	}

	// PDF và file không mở được: false, không panic.
	pdf := filepath.Join(t.TempDir(), "order.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatalf("ghi file pdf giả: %v", err)
	}
	if IsWorkbook(pdf) {
		t.Errorf("file PDF không phải workbook TMĐT")
	}
	if IsWorkbook(filepath.Join(t.TempDir(), "khong-ton-tai.xlsx")) {
		t.Errorf("file không tồn tại phải trả về false")
	}
}
