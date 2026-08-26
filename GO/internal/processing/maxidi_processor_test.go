package processing

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/misapush"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

// maxidiPricingSource returns the two-column price sheet shape Maxidi's
// real tab uses: no promotion date-range columns at all, and prices in
// Vietnamese notation ("8.455" = 8455 đồng), which pricing.ParseIndex
// already normalizes.
func maxidiPricingSource() *fixturePricingSource {
	return &fixturePricingSource{index: pricing.ParseIndex([][]string{
		{"STT", "Mã hàng", "TÊN SẢN PHẨM", " ĐƠN GIÁ"},
		{"1", "GC02344", "Nước tẩy Javel Cleanwise 550G", "8.455"},
	})}
}

func maxidiTestProcessor(t *testing.T) (*RealProcessor, string) {
	t.Helper()
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)
	return &RealProcessor{Store: store, Pricing: maxidiPricingSource(), ExcelPath: excelPath}, excelPath
}

func TestRealProcessor_ProcessesRealMaxidiBinhDuongDeliveryNote(t *testing.T) {
	rp, excelPath := maxidiTestProcessor(t)

	rows, err := rp.Process(context.Background(), filepath.Join("maxidi", "testdata", "realpdfs", "00000000054823.pdf"))
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.StatusKind != StatusKindDone {
		t.Fatalf("StatusKind = %q (%s), want %q — a Maxidi PO carries no price of its own, so it can never be a price mismatch", row.StatusKind, row.Status, StatusKindDone)
	}
	if row.System != "Maxidi" {
		t.Errorf("System = %q, want %q", row.System, "Maxidi")
	}
	if row.MaKhachHang != "LA_GC_00002" {
		t.Errorf("MaKhachHang = %q, want the code from the MaKH sheet's Maxidi row", row.MaKhachHang)
	}
	if row.PO != "HO-PO00085936" {
		t.Errorf("PO = %q, want %q", row.PO, "HO-PO00085936")
	}
	if row.EntryDate != "26/08/2026" {
		t.Errorf("EntryDate = %q, want %q", row.EntryDate, "26/08/2026")
	}
	// CancelDate is column D, "Ngày giao hàng" — for Maxidi that is a
	// real, separately printed delivery date, not a copy of the order
	// date the way Kingfood's and JMart's is.
	if row.CancelDate != "24/09/2026" {
		t.Errorf("CancelDate = %q, want the printed delivery date %q", row.CancelDate, "24/09/2026")
	}
	if !strings.HasPrefix(row.ShipTo, "Khu A, Kho Liên Anh") {
		t.Errorf("ShipTo = %q, want the delivery address printed on the note", row.ShipTo)
	}
	// 10,800 units × 8,455 đ. The order carries 900 CARTONS of 12; the
	// unit count is what goes into the workbook.
	if row.DonGia != "91314000" {
		t.Errorf("DonGia = %q, want %q (10800 × 8455)", row.DonGia, "91314000")
	}
	if row.TotalPackages != 900 {
		t.Errorf("TotalPackages = %d, want 900 (10800 units ÷ 12 per carton)", row.TotalPackages)
	}
	if len(row.PriceMismatchDetails) != 0 || row.PriceMismatchCount != 0 {
		t.Errorf("PriceMismatchCount = %d, details = %+v, want none", row.PriceMismatchCount, row.PriceMismatchDetails)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	// Row 9 is the order's note/header row, row 10 its single product.
	for _, c := range []struct{ cell, want string }{
		{"A9", "26/08/2026"},
		{"B9", "ĐĐHMAXIDI-HO-PO00085936"},
		{"C9", "Chưa thực hiện"},
		{"D9", "24/09/2026"},
		{"G9", "LA_GC_00002"},
		{"H9", "CHI NHÁNH BÌNH DƯƠNG - CÔNG TY TNHH MAXIDI VIỆT NAM"},
		{"I9", "Khu A, Kho Liên Anh, số 189/8 Lê Hồng Phong, KP Tân Phước, P.Tân Đông Hiệp, Hồ Chí Minh, Bình Dương"},
		{"J9", "0317899481-002"},
		{"T9", "Có"},
		{"V9", "GC_TP"},
		{"AE9", "8"},
		{"AJ9", "OEM"},
		{"AM9", "LA"},
		{"AV9", "60"},
		{"Q10", "GC02344"},
		{"S10", "Nước tẩy Javel Cleanwise 550G"},
		{"X10", "10800"},
		{"Y10", "8455"},
		{"AU10", "900"},
		{"J10", "0317899481-002"},
	} {
		got, _ := f.GetCellValue("Don dat hang", c.cell)
		if got != c.want {
			t.Errorf("%s = %q, want %q", c.cell, got, c.want)
		}
	}

	// The delivery-time note printed on the PO goes into the "Diễn giải"
	// column so whoever picks the order sees it.
	desc, _ := f.GetCellValue("Don dat hang", "L9")
	if !strings.Contains(desc, "THỜI GIAN GIAO HÀNG BUỔI SÁNG + CHIỀU") {
		t.Errorf("L9 = %q, want it to carry the PO's own remarks", desc)
	}
	if !strings.Contains(desc, "HO-PO00085936") {
		t.Errorf("L9 = %q, want it to carry the PO number", desc)
	}
}

func TestRealProcessor_BillsMaxidiDongNaiOrdersToTheDongNaiBranch(t *testing.T) {
	// Same customer code as Bình Dương, different legal entity: the tax
	// code printed on the note is the only thing that separates them, and
	// the invoicing address it selects is deliberately NOT the delivery
	// address on the same page.
	rp, excelPath := maxidiTestProcessor(t)

	rows, err := rp.Process(context.Background(), filepath.Join("maxidi", "testdata", "realpdfs", "00000000054824.pdf"))
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].StatusKind == StatusKindFailed {
		t.Fatalf("unexpected result rows: %+v", rows)
	}
	if rows[0].MaKhachHang != "LA_GC_00002" {
		t.Errorf("MaKhachHang = %q, want the same shared code as the Bình Dương branch", rows[0].MaKhachHang)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()

	name, _ := f.GetCellValue("Don dat hang", "H9")
	if name != "CHI NHÁNH ĐỒNG NAI - CÔNG TY TNHH MAXIDI VIỆT NAM" {
		t.Errorf("H9 = %q, want the Đồng Nai branch name", name)
	}
	tax, _ := f.GetCellValue("Don dat hang", "J9")
	if tax != "0317899481-001" {
		t.Errorf("J9 = %q, want %q", tax, "0317899481-001")
	}
	invoiceAddr, _ := f.GetCellValue("Don dat hang", "I9")
	if invoiceAddr != "Kho số 12, khuôn viên ICD Tân Cảng Long Bình, Phường Long Bình, Tỉnh Đồng Nai, Việt Nam." {
		t.Errorf("I9 (invoicing address) = %q, want the Đồng Nai branch's registered address", invoiceAddr)
	}
	shipTo, _ := f.GetCellValue("Don dat hang", "E9")
	if shipTo == invoiceAddr {
		t.Errorf("E9 (delivery address) = %q, must be the address printed on the note, not the invoicing address", shipTo)
	}
}

func TestRealProcessor_FailsMaxidiOrderWhenTheSkuHasNoPrice(t *testing.T) {
	// 00000000043538 is a real note for GC00722, which this test's price
	// sheet does not list. With no price on the PO either, there is
	// nothing left to fall back to — the order must fail loudly rather
	// than be written at 0 đ.
	rp, excelPath := maxidiTestProcessor(t)

	rows, err := rp.Process(context.Background(), filepath.Join("maxidi", "testdata", "realpdfs", "00000000043538.pdf"))
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].StatusKind != StatusKindFailed {
		t.Fatalf("StatusKind = %q (%s), want %q", rows[0].StatusKind, rows[0].Status, StatusKindFailed)
	}
	if !strings.Contains(rows[0].Status, "8935355300722") {
		t.Errorf("Status = %q, want it to name the barcode that has no price", rows[0].Status)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()
	if got, _ := f.GetCellValue("Don dat hang", "A9"); got != "" {
		t.Errorf("A9 = %q, want nothing written for a failed order", got)
	}
}

func TestRealProcessor_MaxidiOrderIsPushableToMISA(t *testing.T) {
	// Two things gate an order reaching AMIS Kế toán, and a vendor that
	// misses either one is silently skipped by the push modal rather than
	// reported — the exact failure that kept ten vendors unpushable
	// before. Both are checked here rather than left to the golden-
	// fixture sweep, which Maxidi has no Python-generated fixtures for.
	rp, _ := maxidiTestProcessor(t)

	rows, err := rp.Process(context.Background(), filepath.Join("maxidi", "testdata", "realpdfs", "00000000054823.pdf"))
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	row := rows[0]
	if row.StatusKind == StatusKindFailed {
		t.Fatalf("Process produced a Failed row: %+v", row)
	}
	// One note row plus one product row, at rows 9 and 10 of the fresh
	// 8-header-row template.
	if len(row.ExcelRows) != 2 || row.ExcelRows[0] != 9 || row.ExcelRows[1] != 10 {
		t.Errorf("ExcelRows = %v, want [9 10] — MISA push splits the workbook by these row numbers", row.ExcelRows)
	}
	key := misapush.RouteKey(row.System, row.MaKhachHang, row.ShipTo)
	if branch := misapush.Lookup(misapush.SeedRouting(), key); branch != misapush.BranchHTLA {
		t.Errorf("routing key %q resolves to branch %q, want %q", key, branch, misapush.BranchHTLA)
	}
}
