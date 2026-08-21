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

// knownDivergences_JMart lists (fixture, row index, column) cells where
// this Go port intentionally computes a different, verified-more-
// correct value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>". Empty until a real, hand-verified
// case is confirmed; add entries here only with a comment citing the
// specific PDF and xulydonhang.py line evidence — never to silence an
// unexplained diff.
//
// ⚠️ Coverage note, more significant than usual: this test validates
// against exactly ONE real JMart PDF — the only one available anywhere
// in this project when this plan was executed. There is no second
// sample to cross-check the product-table backward-scan algorithm's
// robustness against (e.g. a product name that wraps across more lines,
// a QC/conversion-factor value other than "1.000", a different unit
// word). A clean pass here is real evidence for exactly this one order
// and its 3 products — not broad evidence this vendor's extraction
// logic generalizes correctly to JMart's full real order variety. If
// more real JMart PDFs surface later, copying them into realpdfs/ and
// re-running generate_fixtures.py requires no code change here (this
// test globs its inputs) — but the underlying extraction logic in the
// jmart package should be re-scrutinized against any new sample before
// trusting it, not just assumed correct because this test still passes.
var knownDivergences_JMart = map[string]bool{}

func loadFrozenJMartPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("jmart/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen JMart pricing fixture found (run Task 5's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen JMart pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

func TestRealProcessor_MatchesGoldenFixtures_JMart(t *testing.T) {
	fixturePaths, err := filepath.Glob("jmart/testdata/fixtures/*.json")
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
	pricingSource := loadFrozenJMartPricingSource(t)

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

		pdfPath := filepath.Join("jmart", "testdata", "realpdfs", fixture.SourcePDF)
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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_JMart)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixture(s) matched (single-sample coverage — see knownDivergences_JMart's own doc comment)", len(realFixtures))
}
