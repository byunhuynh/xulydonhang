package processing

import (
	"context"
	"strings"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

// processCoopSampleWithTestStore chạy một PDF Coop qua RealProcessor với
// MaKH của productdata/testdata/data.xlsx — sheet này không có cửa hàng
// Coop thật nào, nên mọi mẫu dùng ở đây đều là cửa hàng chưa có mã.
func processCoopSampleWithTestStore(t *testing.T, pdfPath string) OrderRow {
	t.Helper()
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	priceCsv := [][]string{
		{"STT", "Mã hàng", "Tên", "Giá"},
		{"1", "1234567", "Nước giặt", "141.272"},
	}
	rp := &RealProcessor{
		Store: store, Pricing: &fixturePricingSource{index: pricing.ParseIndex(priceCsv)},
		ExcelPath: copyTestWorkbookForProcessor(t),
	}
	rows, err := rp.Process(context.Background(), pdfPath)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	return rows[0]
}

// Ca thật: cửa hàng Coopfood 2231 mới khai trương, chưa có dòng trong MaKH.
// Trước đây đơn rơi về COOPMART (bị so CTKM của Coopmart) và vẫn báo
// "Hoàn thành" dù không có mã khách hàng.
func TestRealProcessor_CoopfoodStoreMissingFromMaKHStaysCoopfood(t *testing.T) {
	row := processCoopSampleWithTestStore(t, "testdata/sample_coopfood_unmapped_store.pdf")

	if row.System != "COOPFOOD" {
		t.Errorf("System = %q, want COOPFOOD (Ship To %q carries the -CF store marker)", row.System, row.ShipTo)
	}
	if row.StatusKind != StatusKindWarning {
		t.Errorf("StatusKind = %q, want %q for a store missing from MaKH", row.StatusKind, StatusKindWarning)
	}
	if !strings.Contains(row.Status, "2231") || !strings.Contains(row.Status, "chưa có trong MaKH") {
		t.Errorf("Status = %q, want it to name store 2231 as missing from MaKH", row.Status)
	}
}

// Không có dấu -CF thì giữ nguyên mặc định COOPMART, nhưng vẫn phải cảnh
// báo thiếu MaKH.
func TestRealProcessor_CoopmartStoreMissingFromMaKHWarns(t *testing.T) {
	row := processCoopSampleWithTestStore(t, "testdata/sample_coop_order.pdf")

	if row.System != "COOPMART" {
		t.Errorf("System = %q, want COOPMART", row.System)
	}
	if row.StatusKind != StatusKindWarning {
		t.Errorf("StatusKind = %q, want %q for a store missing from MaKH", row.StatusKind, StatusKindWarning)
	}
	if !strings.Contains(row.Status, "140") || !strings.Contains(row.Status, "chưa có trong MaKH") {
		t.Errorf("Status = %q, want it to name store 140 as missing from MaKH", row.Status)
	}
}
