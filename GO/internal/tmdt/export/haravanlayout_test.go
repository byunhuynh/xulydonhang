package export

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/tmdt/haravan"
)

// Đơn 2 dòng hàng, có giảm giá — dựng theo đúng response thật của Haravan.
const rawOrderTwoItems = `{
  "id": 1835002920,
  "name": "2608235MS2QR44",
  "created_at": "2026-08-23T14:21:55Z",
  "source_name": "shopee",
  "location_name": "Kho Hà Nội",
  "subtotal_price": 139000.0,
  "total_discounts": 10000.0,
  "total_price": 129000.0,
  "note_attributes": [
    {"name": "X-Haravan-SalesChannel-BranchId", "value": "461880029"},
    {"name": "X-Haravan-SalesChannel-BranchName", "value": "Blue Việt Nam"}
  ],
  "line_items": [
    {"title": "Nước giặt xả Blue Đậm đặc", "sku": "TP30244_02", "variant_title": "Default Title", "quantity": 1, "price": 139000.0},
    {"title": "Bột Tẩy Lồng Máy Giặt Blue 150g", "sku": "TP32743", "variant_title": "1 Gói Lẻ 150g", "quantity": 1, "price": 0.0}
  ]
}`

func TestHaravanLayoutMatchesExportFormat(t *testing.T) {
	var o haravan.Order
	if err := json.Unmarshal([]byte(rawOrderTwoItems), &o); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.xlsx")
	w, err := NewHaravanWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.AddOrder("Shopee", &o); err != nil {
		t.Fatal(err)
	}
	if w.Count() != 1 {
		t.Errorf("Count = %d, want 1 (đếm theo đơn, không phải dòng hàng)", w.Count())
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if got := f.GetSheetList(); len(got) != 1 || got[0] != sheetHaravan {
		t.Fatalf("sheet = %v, want [%s]", got, sheetHaravan)
	}

	cell := func(col string, row int) string {
		v, err := f.GetCellValue(sheetHaravan, col+itoa(row))
		if err != nil {
			t.Fatalf("đọc %s%d: %v", col, row, err)
		}
		return v
	}

	// Tiêu đề phải trùng chữ cái cột với file Haravan xuất ra, vì công thức
	// BX/BY/CG trong file của người dùng tham chiếu Q, T, V, BB theo vị trí.
	for _, tc := range []struct{ col, want string }{
		{"A", "Mã đơn hàng"}, {"J", "Tổng tiền"}, {"M", "Tổng cộng"},
		{"Q", "Ngày đặt hàng"}, {"S", "Số lượng sản phẩm"}, {"T", "Tên sản phẩm"},
		{"V", "Giá trị thuộc tính 1"}, {"AA", "Giá sản phẩm"}, {"AC", "Mã sản phẩm"},
		{"BB", "Thuộc tính"}, {"BR", "Kho bán"}, {"BS", "Kênh bán hàng"},
	} {
		if got := cell(tc.col, 1); got != tc.want {
			t.Errorf("tiêu đề cột %s = %q, want %q", tc.col, got, tc.want)
		}
	}

	// Dòng hàng thứ nhất.
	for _, tc := range []struct{ col, want string }{
		{"A", "2608235MS2QR44"},
		{"J", "139000"},
		{"M", "129000"},
		{"Q", "2026-08-23T21:21:55+07:00"}, // 14:21:55Z -> giờ VN
		{"S", "1"},
		{"T", "Nước giặt xả Blue Đậm đặc"},
		{"V", "Default Title"}, // giữ nguyên, Haravan cũng xuất y vậy
		{"AA", "139000"},
		{"AC", "TP30244_02"},
		{"BR", "Kho Hà Nội"},
		{"BS", "shopee"},
	} {
		if got := cell(tc.col, 2); got != tc.want {
			t.Errorf("dòng 2 cột %s = %q, want %q", tc.col, got, tc.want)
		}
	}

	wantAttr := "X-Haravan-SalesChannel-BranchId : 461880029\nX-Haravan-SalesChannel-BranchName : Blue Việt Nam"
	if got := cell("BB", 2); got != wantAttr {
		t.Errorf("BB2 = %q,\nwant %q", got, wantAttr)
	}

	// Dòng hàng thứ hai: trường cấp đơn lặp lại, trường dòng hàng đổi.
	for _, tc := range []struct{ col, want string }{
		{"A", "2608235MS2QR44"},
		{"J", "139000"},
		{"M", "129000"},
		{"AA", "0"},
		{"AC", "TP32743"},
		{"V", "1 Gói Lẻ 150g"},
	} {
		if got := cell(tc.col, 3); got != tc.want {
			t.Errorf("dòng 3 cột %s = %q, want %q", tc.col, got, tc.want)
		}
	}

	// Các cột không được yêu cầu phải để trống.
	for _, col := range []string{"B", "C", "K", "L", "U", "AB", "AG", "BG", "BH"} {
		if got := cell(col, 2); got != "" {
			t.Errorf("cột %s phải trống, got %q", col, got)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
