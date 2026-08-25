package export

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/tmdt/haravan"
)

const rawOrder = `{
  "id": 1138637099,
  "name": "#100020",
  "created_at": "2026-08-19T01:12:10.062Z",
  "source_name": "shopee",
  "ref_order_number": "2508190XYZABC",
  "financial_status": "paid",
  "total_price": 1200000.0,
  "subtotal_price": 1200000.0,
  "currency": "VND",
  "customer": {"first_name": "Demo", "last_name": "Haravan"},
  "note_attributes": [
    {"name": "X-Haravan-SalesChannel-BranchId", "value": "749474051"},
    {"name": "X-Haravan-SalesChannel-BranchName", "value": "Blue Việt Nam"}
  ],
  "shipping_lines": [{"code": "Nhanh", "title": "Nhanh", "price": 15000.0}],
  "shipping_address": {"first_name": "Demo", "address1": "182 Lê Đại Hành", "province": "Hồ Chí Minh"},
  "line_items": [
    {"product_id": 21, "title": "Đầm babydoll", "quantity": 2, "price": 600000.0,
     "price_original": 800000.0, "sku": "SKU-1", "variant_title": "Combo 2",
     "properties": [{"name": "X-Haravan-SalesChannel-LineId", "value": "887-1"}, {"name": "Màu", "value": "Đỏ"}]}
  ]
}`

func TestWriteExcel(t *testing.T) {
	var o haravan.Order
	if err := json.Unmarshal([]byte(rawOrder), &o); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.xlsx")
	if err := Write(path, []Row{{Order: &o, Channel: "Shopee"}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("mở file: %v", err)
	}
	defer f.Close()

	for _, s := range []string{sheetOrders, sheetItems, sheetStats} {
		if _, err := f.GetSheetIndex(s); err != nil {
			t.Errorf("thiếu sheet %s", s)
		}
	}

	rows, err := f.GetRows(sheetOrders)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("sheet %s có %d dòng, want 2", sheetOrders, len(rows))
	}
	if rows[0][0] != "Sàn" || rows[1][0] != "Shopee" {
		t.Errorf("cột Sàn: header=%q, value=%q", rows[0][0], rows[1][0])
	}
	if rows[0][1] != "Tên shop" || rows[1][1] != "Blue Việt Nam" {
		t.Errorf("cột Tên shop: header=%q, value=%q", rows[0][1], rows[1][1])
	}
	if rows[1][3] != "2508190XYZABC" {
		t.Errorf("mã đơn trên sàn = %q", rows[1][3])
	}
	if rows[1][21] != "15,000" {
		t.Errorf("phí vận chuyển = %q, want 15,000", rows[1][21])
	}
	if rows[1][27] != "Nhanh" {
		t.Errorf("dịch vụ vận chuyển = %q", rows[1][27])
	}

	items, err := f.GetRows(sheetItems)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("sheet %s có %d dòng, want 2", sheetItems, len(items))
	}
	if items[1][6] != "SKU-1" {
		t.Errorf("mã sản phẩm = %q", items[1][6])
	}
	// variant_title + properties, đã lọc khoá nội bộ X-Haravan-*
	if items[1][8] != "Combo 2 | Màu: Đỏ" {
		t.Errorf("thuộc tính = %q", items[1][8])
	}
	if items[1][11] != "800,000" {
		t.Errorf("giá gốc = %q, want 800,000", items[1][11])
	}

	stats, err := f.GetRows(sheetStats)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 3 {
		t.Fatalf("sheet %s có %d dòng (header + Shopee + TỔNG), got %v", sheetStats, len(stats), stats)
	}
	if stats[1][0] != "Shopee" || stats[1][1] != "Blue Việt Nam" || stats[1][2] != "1" || stats[1][3] != "2" {
		t.Errorf("thống kê theo shop: %v", stats[1])
	}
}
