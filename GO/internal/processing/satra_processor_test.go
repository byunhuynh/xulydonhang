package processing

import (
	"context"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

// TestNormalizeSatraText validates each of the three transformations that
// normalizeSatraText applies to reconcile this repo's Go PDF library's output
// with the PyMuPDF-shaped input every function in the satra package
// (Tasks 3-4) was built and validated against: U+FFFD stripping, blank-line
// stripping, and 3-line product-block re-joining.
func TestNormalizeSatraText(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		// Test 1: U+FFFD stripping alone
		{
			name: "U+FFFD stripped from mid-line",
			text: "178 500,00�",
			want: "178 500,00",
		},
		{
			name: "U+FFFD stripped with surrounding content preserved",
			text: "Price: 99,000,00�\nNext line",
			want: "Price: 99,000,00\nNext line",
		},
		{
			name: "multiple U+FFFD stripped",
			text: "Line1�\nLine2�\nLine3",
			want: "Line1\nLine2\nLine3",
		},

		// Test 2: Blank-line stripping alone
		{
			name: "blank lines removed",
			text: "Line1\n\nLine2\n\n\nLine3",
			want: "Line1\nLine2\nLine3",
		},
		{
			name: "content preserved with blank lines removed",
			text: "Header\n\nProduct Name\n\nPrice",
			want: "Header\nProduct Name\nPrice",
		},

		// Test 3: 3-line product-block join with single-digit STT
		{
			name: "single-digit STT joined with code, barcode on next line",
			text: "1\n300867\n8936156730985",
			want: "1 300867\n8936156730985",
		},
		{
			name: "single-digit STT with surrounding lines unchanged",
			text: "Header\n1\n300867\n8936156730985\nFooter",
			want: "Header\n1 300867\n8936156730985\nFooter",
		},

		// Test 4: 3-line product-block join with multi-digit STT
		{
			name: "multi-digit STT joined with code, barcode on next line",
			text: "12\n445210\n8936156730992",
			want: "12 445210\n8936156730992",
		},
		{
			name: "multi-digit STT (99) joined correctly",
			text: "99\n123456\n1234567890123",
			want: "99 123456\n1234567890123",
		},

		// Test 5: Negative cases - pattern should NOT misfire
		{
			name: "non-13-digit barcode not joined (12 digits)",
			text: "1\n300867\n893615673098",
			want: "1\n300867\n893615673098",
		},
		{
			name: "non-13-digit barcode not joined (14 digits)",
			text: "1\n300867\n89361567309851",
			want: "1\n300867\n89361567309851",
		},
		{
			name: "only two digit-lines not joined (no barcode line)",
			text: "1\n300867\nNot a barcode",
			want: "1\n300867\nNot a barcode",
		},
		{
			name: "mixed: non-numeric first line not joined",
			text: "STT\n300867\n8936156730985",
			want: "STT\n300867\n8936156730985",
		},
		{
			name: "mixed: non-numeric second line not joined",
			text: "1\nCode\n8936156730985",
			want: "1\nCode\n8936156730985",
		},

		// Test 6: All three transformations combined
		{
			name: "U+FFFD + blank lines + product-block join combined",
			text: "Header\n\nPrice: 99,000,00�\n\n1\n300867\n8936156730985\n\nFooter",
			want: "Header\nPrice: 99,000,00\n1 300867\n8936156730985\nFooter",
		},
		{
			name: "realistic snippet with multiple blocks and replacements",
			text: "Product List\n\n1\n100001\n1000011111111\n\n2\n200002\n2000022222222\n\nEnd",
			want: "Product List\n1 100001\n1000011111111\n2 200002\n2000022222222\nEnd",
		},

		// Test 7: Edge cases
		{
			name: "empty string",
			text: "",
			want: "",
		},
		{
			name: "single line unchanged",
			text: "Single line of text",
			want: "Single line of text",
		},
		{
			name: "valid block at end of text (no trailing newline)",
			text: "1\n300867\n8936156730985",
			want: "1 300867\n8936156730985",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeSatraText(tc.text)
			if got != tc.want {
				t.Fatalf("normalizeSatraText(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

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
	rows, err := rp.Process(context.Background(), "testdata/sample_satra_order.pdf")
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
