package processing

import (
	"context"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

func TestRealProcessor_ProcessesRealSampleSatraFile(t *testing.T) {
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
	rows, err := rp.Process(context.Background(), "testdata/sample_satra_order.pdf", 1)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Process returned %d rows, want 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.System != "Satra" {
		t.Fatalf("row.System = %q, want %q", row.System, "Satra")
	}
	if row.PO != "P-005508192" {
		t.Fatalf("row.PO = %q, want %q", row.PO, "P-005508192")
	}
	// The test fixture's data.xlsx SATRA row (added in Task 2) uses a
	// different address than this real file's — run the test first to see
	// whether it fuzzy-matches above the >95 threshold anyway (normalized
	// short Vietnamese addresses can sometimes score higher than
	// expected). Observed by actually running this test against the real
	// sample PDF: the fixture's synthetic SATRA row's address does NOT
	// fuzzy-match "44 Đường Số 1, Phường Tân Mỹ,HCM,VNM" above the >95
	// threshold, so GetCustomerCodeByFuzzyAddress reports no match and
	// processSatraSegment falls back to "Không xác định" — pinned to that
	// actually-observed value, not assumed.
	if row.MaKhachHang != "Không xác định" {
		t.Fatalf("row.MaKhachHang = %q, want %q (test fixture's data.xlsx SATRA row's address doesn't fuzzy-match this real file's)", row.MaKhachHang, "Không xác định")
	}
	// This file's real barcodes aren't in the small test fixture's price
	// index (see pricingSource above), so all 3 real products are
	// expected to come back as price mismatches, same as Coop/Lotte's
	// analogous real-sample tests.
	if row.StatusKind != StatusKindWarning {
		t.Fatalf("row.StatusKind = %q, want %q (all 3 real products should price-mismatch against the empty index)", row.StatusKind, StatusKindWarning)
	}
}
