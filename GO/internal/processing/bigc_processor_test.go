package processing

import (
	"context"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleBigcFile(t *testing.T) {
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
	rows, err := rp.Process(context.Background(), "testdata/sample_bigc_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	// 19 store pages -> 19 OrderRows (one per store page, matching
	// Python's per-page-call saigia/tongtien scoping — see this plan's
	// Task 6 design notes on why this is "1 row per store", not "1 row
	// per file", despite all stores sharing one PO and one combined
	// Excel write).
	if len(rows) != 19 {
		t.Fatalf("Process returned %d rows, want 19: %+v", len(rows), rows)
	}
	for i, row := range rows {
		if row.System != "BigC" {
			t.Fatalf("rows[%d].System = %q, want %q", i, row.System, "BigC")
		}
		if row.PO != "2631057733376" {
			t.Fatalf("rows[%d].PO = %q, want %q", i, row.PO, "2631057733376")
		}
		if row.StatusKind != StatusKindWarning {
			t.Fatalf("rows[%d].StatusKind = %v, want %v (empty price index -> every product mismatches)", i, row.StatusKind, StatusKindWarning)
		}
	}
}
