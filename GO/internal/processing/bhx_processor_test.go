package processing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/misapush"
	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

const bhxRealOrder = "BHX_HCM_Q12-07300PO2611606019.pdf"

// bhxPricingRows mirrors the live BHX price tab: list prices with the
// thousands dot ("71.296"), keyed by our own product code.
func bhxPricingRows() [][]string {
	return [][]string{
		{"STT", "Mã hàng", "TÊN SẢN PHẨM", "ĐƠN GIÁ"},
		{"3", "TP30886_02", "Nước giặt xả Blue Đậm đặc Hương Nước hoa túi 3,6L", "71.296"},
		{"4", "TP30879_02", "Nước giặt xả Blue Đậm đặc Hương Thảo Mộc túi 3.6L", "71.296"},
	}
}

// bhxTestProcessor builds a processor over the shared data.xlsx fixture
// plus branch 36560's MaKH row (as the live sheet has it) and the two
// products, on a temporary copy.
func bhxTestProcessor(t *testing.T, pricingRows [][]string, withCustomer bool) (*RealProcessor, string) {
	t.Helper()
	data, err := os.ReadFile("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("failed reading productdata fixture: %v", err)
	}
	dataPath := filepath.Join(t.TempDir(), "data.xlsx")
	if err := os.WriteFile(dataPath, data, 0o644); err != nil {
		t.Fatalf("failed writing temp productdata: %v", err)
	}
	f, err := excelize.OpenFile(dataPath)
	if err != nil {
		t.Fatalf("failed opening temp productdata: %v", err)
	}
	appendRow := func(sheet string, values ...interface{}) {
		rows, err := f.GetRows(sheet)
		if err != nil {
			t.Fatalf("GetRows(%s): %v", sheet, err)
		}
		cell, _ := excelize.CoordinatesToCellName(1, len(rows)+1)
		if err := f.SetSheetRow(sheet, cell, &values); err != nil {
			t.Fatalf("SetSheetRow(%s): %v", sheet, err)
		}
	}
	// Neither the receiving store (7300) nor a code that merely ends in
	// 36560 may be picked.
	appendRow("MaKH", "BHX", "BHX7300", "MN_MT_BHX7300", "")
	appendRow("MaKH", "BHX", "BHX136560", "MN_MT_BHX136560", "")
	if withCustomer {
		appendRow("MaKH", "BHX", "MN_MT_BHX36560", "MN_MT_BHX36560", "")
	}
	appendRow("SanPham", "TP30879_02", "Nước giặt xả Blue Đậm đặc Hương Thảo Mộc túi 3.6L", 3.8, 4, nil, nil, nil, nil, nil, nil, "8936156730879")
	appendRow("SanPham", "TP30886_02", "Nước giặt xả Blue Đậm đặc Hương Nước Hoa túi 3.6L", 3.8, 4, nil, nil, nil, nil, nil, nil, "8936156730886")
	if err := f.Save(); err != nil {
		t.Fatalf("failed saving temp productdata: %v", err)
	}
	f.Close()

	store, err := productdata.Load(dataPath)
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)
	return &RealProcessor{
		Store:     store,
		Pricing:   &fixturePricingSource{index: pricing.ParseIndex(pricingRows)},
		ExcelPath: excelPath,
	}, excelPath
}

func processBhxRealOrder(t *testing.T, rp *RealProcessor) OrderRow {
	t.Helper()
	rows, err := rp.Process(context.Background(), filepath.Join("bhx", "testdata", "realpdfs", bhxRealOrder))
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1: %+v", len(rows), rows)
	}
	return rows[0]
}

func TestRealProcessor_ProcessesRealBhxOrder(t *testing.T) {
	rp, excelPath := bhxTestProcessor(t, bhxPricingRows(), true)
	row := processBhxRealOrder(t, rp)

	if row.StatusKind != StatusKindDone {
		t.Fatalf("StatusKind = %q (%s), want %q\nlog: %s", row.StatusKind, row.Status, StatusKindDone, strings.Join(row.SkuLog, "\n"))
	}
	if row.System != "BHX" || row.MaKhachHang != "MN_MT_BHX36560" || row.PO != "07300PO2611606019" {
		t.Errorf("System/MaKhachHang/PO = %q/%q/%q", row.System, row.MaKhachHang, row.PO)
	}
	if row.EntryDate != "10/11/2026" || row.CancelDate != "13/11/2026" {
		t.Errorf("EntryDate/CancelDate = %q/%q, want 10/11/2026 and the end of the delivery window 13/11/2026", row.EntryDate, row.CancelDate)
	}
	if row.ShipTo != "7300 - BHX_HCM_Q12 - 31 Vườn Lài" {
		t.Errorf("ShipTo = %q", row.ShipTo)
	}
	// The PO's own pre-VAT total, 35,933,333.34.
	if row.DonGia != "35933333" {
		t.Errorf("DonGia = %q, want %q", row.DonGia, "35933333")
	}
	if row.TotalPackages != 126 {
		t.Errorf("TotalPackages = %d, want 2 × 252/4", row.TotalPackages)
	}

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		t.Fatalf("failed reopening workbook: %v", err)
	}
	defer f.Close()
	for _, c := range []struct{ cell, want string }{
		{"A9", "10/11/2026"},
		{"B9", "ĐĐHBHX-07300PO2611606019"},
		{"D9", "13/11/2026"},
		{"G9", "MN_MT_BHX36560"},
		{"V9", "LA_KHO2026"},
		{"AJ9", "MT_MN"},
		{"AM9", "LA"},
		{"Q10", "TP30886_02"},
		{"X10", "252"},
		{"Q11", "TP30879_02"},
		{"X11", "252"},
	} {
		got, _ := f.GetCellValue("Don dat hang", c.cell)
		if got != c.want {
			t.Errorf("%s = %q, want %q", c.cell, got, c.want)
		}
	}
	// The PO's own unrounded unit price, total ÷ quantity.
	if got, _ := f.GetCellValue("Don dat hang", "Y10"); !strings.HasPrefix(got, "71296.29") {
		t.Errorf("Y10 = %q, want 17,966,666.67 / 252 ≈ 71296.296", got)
	}

	key := misapush.RouteKey(row.System, row.MaKhachHang, row.ShipTo)
	if branch := misapush.Lookup(misapush.SeedRouting(), key); branch != misapush.BranchHTLA {
		t.Errorf("routing key %q resolves to branch %q, want %q", key, branch, misapush.BranchHTLA)
	}
}

func TestRealProcessor_FlagsBhxPriceThatDisagreesWithThePriceTab(t *testing.T) {
	pricingRows := bhxPricingRows()
	pricingRows[2][3] = "75.000"
	rp, _ := bhxTestProcessor(t, pricingRows, true)
	row := processBhxRealOrder(t, rp)

	if row.StatusKind != StatusKindWarning || row.PriceMismatchCount != 1 {
		t.Fatalf("StatusKind = %q (%s), PriceMismatchCount = %d, want one price mismatch", row.StatusKind, row.Status, row.PriceMismatchCount)
	}
	if d := row.PriceMismatchDetails[0]; d.SKU != "TP30879_02" || d.SystemPrice != 75000 {
		t.Errorf("mismatch = %+v, want TP30879_02 vs system 75000", d)
	}
}

func TestRealProcessor_FailsBhxOrderForAStoreWithNoCustomerCode(t *testing.T) {
	rp, excelPath := bhxTestProcessor(t, bhxPricingRows(), false)
	row := processBhxRealOrder(t, rp)

	if row.StatusKind != StatusKindFailed || !strings.Contains(row.Status, "BHX36560") {
		t.Fatalf("StatusKind = %q, Status = %q, want a failure naming BHX36560", row.StatusKind, row.Status)
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
