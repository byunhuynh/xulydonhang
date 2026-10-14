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

// Ca thật 104743481-00: Coop in quà CTKM (3613120-9, 100 túi NLS) thành
// một dòng riêng, mọi cột tiền ".00". Trước đây cả đơn bị từ chối. Dòng
// này không được ghi vào sổ đặt hàng: quà đã do CTKM "2+1 tặng NLS 1L"
// của chính hai mã bán sinh ra, ghi thêm là xuất quà hai lần.
func TestRealProcessor_CoopZeroCostGiftLineIsNotWritten(t *testing.T) {
	row := processCoopSampleWithTestStore(t, "testdata/sample_coop_zero_cost_gift_line.pdf")

	if row.StatusKind == StatusKindFailed {
		t.Fatalf("Status = %q, want the order processed", row.Status)
	}
	// Dòng ghi chú + 2 mã bán; bảng giá test không có CTKM nên không có dòng quà.
	if len(row.ExcelRows) != 3 {
		t.Errorf("ExcelRows = %v, want 3 rows (note + 2 billed lines, no gift line)", row.ExcelRows)
	}
	logged := strings.Join(row.SkuLog, "\n")
	if !strings.Contains(logged, "3613120") {
		t.Errorf("SkuLog = %q, want a line saying gift 3613120 was skipped", logged)
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
