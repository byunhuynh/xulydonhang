package processing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

// knownDivergences_Fujimart lists (fixture, row index, column) cells
// where this Go port intentionally computes a different, verified-more-
// correct value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>". Empty until a real, hand-verified
// case is confirmed; add entries here only with a comment citing the
// specific PDF/Python-line evidence — never to silence an unexplained
// diff.
//
// Coverage note: this test validates against all 15 real FujiMart PDFs
// available when this plan was executed (committed into
// fujimart/testdata/realpdfs/, not read from the live đơn hàng/ tree —
// see Task 5). More real FujiMart PDFs may exist beyond these 15;
// adding them later is a matter of copying into realpdfs/ and re-running
// generate_fixtures.py — this test globs its inputs, so no code change
// is needed here when that happens.
var knownDivergences_Fujimart = map[string]bool{
	// Store 11021 ("FujiMart Hoàng Cầu")'s real Python fixture has a
	// DOUBLE space between the store code and the branch name —
	// "11021  FujiMart Hoàng Cầu" — on both of its real fixtures
	// (102001302607001667.pdf, 7 rows; 102001302608000155.pdf, 7 rows).
	// Confirmed NOT reproducible from this port's text-layer-only
	// extraction: fujimart.ParseOrderInfo's own doc comment (extract.go)
	// already documents storeInfo as coming from OCR in real Python
	// (xulydonhang.py:8895-8899, tesseract over a rendered image of the
	// "Nơi nhận:" region) — a deliberate, already-scoped design decision
	// ("Không cần OCR") to instead derive it from the plain text layer as
	// storeCode + " " + branchLine (always exactly one space). Directly
	// verified against this port's own extracted text for both PDFs
	// (fujimart/testdata/realpdfs/102001302607001667.pdf and
	// .../102001302608000155.pdf): the store-code line ("11021") and the
	// branch-name line ("FujiMart Hoàng Cầu" post-decode) each
	// come from SEPARATE, independently-trimmed text lines with no
	// embedded whitespace signal anywhere that could produce a second
	// space — the double space is an OCR rendering-width artifact
	// (tesseract segmenting a wider-than-usual pixel gap for this one
	// store's field layout) that has no analog in the PDF's text layer,
	// so it cannot be reconstructed without OCR. Every other real
	// fixture's store name matches exactly with a single space,
	// confirming this is a genuine, isolated OCR quirk for this one
	// store rather than a systematic bug in the single-space join.
	"102001302607001667.pdf:0:E": true,
	"102001302607001667.pdf:1:E": true,
	"102001302607001667.pdf:2:E": true,
	"102001302607001667.pdf:3:E": true,
	"102001302607001667.pdf:4:E": true,
	"102001302607001667.pdf:5:E": true,
	"102001302607001667.pdf:6:E": true,
	"102001302608000155.pdf:0:E": true,
	"102001302608000155.pdf:1:E": true,
	"102001302608000155.pdf:2:E": true,
	"102001302608000155.pdf:3:E": true,
	"102001302608000155.pdf:4:E": true,
	"102001302608000155.pdf:5:E": true,
	"102001302608000155.pdf:6:E": true,
}

func loadFrozenFujimartPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("fujimart/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen FujiMart pricing fixture found (run Task 5's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen FujiMart pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_Fujimart(t *testing.T) {
	fixturePaths, err := filepath.Glob("fujimart/testdata/fixtures/*.json")
	if err != nil {
		t.Fatalf("failed globbing fixtures: %v", err)
	}
	var realFixtures []string
	for _, p := range fixturePaths {
		if filepath.Base(p) != "_frozen_pricing.json" {
			realFixtures = append(realFixtures, p)
		}
	}
	if len(realFixtures) == 0 {
		t.Skip("no golden fixtures found (run Task 5's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenFujimartPricingSource(t)

	var mismatches []string
	for _, fixturePath := range realFixtures {
		raw, err := os.ReadFile(fixturePath)
		if err != nil {
			t.Fatalf("failed reading %s: %v", fixturePath, err)
		}
		var fixture fixtureData
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("failed parsing %s: %v", fixturePath, err)
		}

		pdfPath := filepath.Join("fujimart", "testdata", "realpdfs", fixture.SourcePDF)
		excelPath := filepath.Join(t.TempDir(), "dondathang.xlsx")
		copyFile(t, "excelwriter/testdata/dondathang.xlsx", excelPath)

		rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
		rows, err := rp.Process(context.Background(), pdfPath)
		if err != nil {
			mismatches = append(mismatches, fixture.SourcePDF+": Process returned error: "+err.Error())
			continue
		}
		if len(rows) == 0 || rows[0].StatusKind == StatusKindFailed {
			mismatches = append(mismatches, fixture.SourcePDF+": Process produced a Failed row")
			continue
		}

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Fujimart)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
