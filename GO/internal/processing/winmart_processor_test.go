package processing

import (
	"context"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleWinmartFile(t *testing.T) {
	store, err := productdata.Load("productdata/testdata/data.xlsx")
	if err != nil {
		t.Fatalf("Load productdata failed: %v", err)
	}
	excelPath := copyTestWorkbookForProcessor(t)

	// Empty price index on purpose: this file's real barcodes aren't in
	// the small test fixture, so products are expected to come back as
	// price mismatches (Warning), not Done.
	pricingSource := &fixturePricingSource{index: pricing.ParseIndex(nil)}

	rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
	rows, err := rp.Process(context.Background(), "testdata/sample_winmart_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.System != "Winmart" {
		t.Fatalf("row.System = %q, want %q", row.System, "Winmart")
	}
	if row.PO != "4194002858" {
		t.Fatalf("row.PO = %q, want %q", row.PO, "4194002858")
	}
	// The test fixture's data.xlsx WINMART row (added in Step 1) uses a
	// different address ("789 Trần Hưng Đạo, Phường Cầu Kho, Quận 1,
	// Tp.HCM, VNM") than this real file's fuzzy-match input ("Khu trung
	// tâm thương mại Vincom Lê Thánh Tông..."). Observed (run directly):
	// unlike every prior vendor's equivalent test in this series, this
	// does NOT cross the >95 PartialRatio threshold, so
	// GetCustomerCodeByFuzzyAddress reports no match and
	// processWinmartSegment falls back to "Không xác định" — asserting
	// the actually-observed value, not a guess.
	if row.MaKhachHang != "Không xác định" {
		t.Fatalf("row.MaKhachHang = %q, want %q", row.MaKhachHang, "Không xác định")
	}
}
