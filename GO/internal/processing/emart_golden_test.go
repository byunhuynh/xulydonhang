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

// knownDivergences_Emart lists (fixture, row index, column) cells where
// this Go port intentionally computes a different, verified-more-correct
// value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>". Empty until a real, hand-verified
// case is confirmed; add entries here only with a comment citing the
// specific PDF/Python-line evidence — never to silence an unexplained
// diff.
//
// Coverage note: this test currently validates against 9 of the
// originally-targeted 17 real Emart PDFs. The other 8 were not available
// when the golden fixtures (emart/testdata/fixtures/) and their source
// PDFs (emart/testdata/realpdfs/) were generated — a live, concurrently-
// running production instance of this same application was still
// processing them at the time. Adding them later is a matter of copying
// the additional real PDFs into emart/testdata/realpdfs/ and re-running
// emart/testdata/generate_fixtures.py; this test and
// TestPdfOpen_ExtractsTextFromRealEmartPDFsWithInlineImages both glob
// their inputs, so no code change is needed here when that happens.
var knownDivergences_Emart = map[string]bool{}

func loadFrozenEmartPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("emart/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen Emart pricing fixture found (run Task 5's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen Emart pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_Emart(t *testing.T) {
	fixturePaths, err := filepath.Glob("emart/testdata/fixtures/*.json")
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
	pricingSource := loadFrozenEmartPricingSource(t)

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

		// Task 5's own revised design: source PDFs live in this repo's
		// stable, git-tracked emart/testdata/realpdfs/ directory, NOT
		// the live đơn hàng/ tree every other vendor's golden test reads
		// from (that live folder was demonstrated mid-session to be an
		// unstable dependency — see Task 5's brief for the full story).
		pdfPath := filepath.Join("emart", "testdata", "realpdfs", fixture.SourcePDF)
		excelPath := filepath.Join(t.TempDir(), "dondathang.xlsx")
		copyFile(t, "excelwriter/testdata/dondathang.xlsx", excelPath)

		rp := &RealProcessor{Store: store, Pricing: pricingSource, ExcelPath: excelPath}
		rows, err := rp.Process(context.Background(), pdfPath, 1)
		if err != nil {
			mismatches = append(mismatches, fixture.SourcePDF+": Process returned error: "+err.Error())
			continue
		}
		if len(rows) == 0 || rows[0].StatusKind == StatusKindFailed {
			mismatches = append(mismatches, fixture.SourcePDF+": Process produced a Failed row")
			continue
		}

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Emart)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
