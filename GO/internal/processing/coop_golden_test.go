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

// loadFrozenPricingSource builds the golden test's offline PricingSource
// from _frozen_pricing.json. Reuses the fixturePricingSource type already
// declared in coop_processor_test.go (same package) rather than
// redeclaring it — it wraps a single frozen *pricing.Index and satisfies
// the same PricingSource interface as the production HTTPSource.
func loadFrozenPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("coop/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen pricing fixture found (run Task 12's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

// knownDivergences_Coop lists (fixture, row index, column) cells where
// this Go port intentionally computes a different, verified-more-correct
// value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>", matching every other vendor's own
// knownDivergences_<Vendor> map (see e.g. knownDivergences_BigC).
//
// Coop (Phase 2a) is the one vendor in this project whose stated testing
// policy is strict bug-for-bug parity with Python, unlike every vendor
// shipped since (Phase 2b's explicit policy is "correct main flow, don't
// need to replicate old Python bugs" — see the Phase 2b roadmap memory).
// This map is a DELIBERATE, one-time exception to Coop's own stricter
// policy, made with the project owner's explicit sign-off (2026-08-22):
// after fixing a real Go PDF-extraction bug where this generator's own
// font metrics defeated a fixed word-gap threshold (see
// rotatedPageGapThreshold's own doc comment in pdfextract.go), Go's
// output for these 4 fixtures' column-E store-name field became clean
// ("Co.opMart Cai Lay") — but the FROZEN fixture's own "want" value
// still carries the SAME character-spacing artifact Go used to produce
// before that fix ("Co. opMar t Cai Lay"), confirming Python/PyMuPDF's
// real output has this exact bug too, on this exact font. Verified by
// checking a DIFFERENT fixture (103269932-00, also fixed by the same
// pdfextract.go change) whose frozen ground truth was already clean
// before the fix — proving Python does NOT always have this bug, so
// these 4 entries are a confirmed Python defect on these specific
// files, not a generically-expected PyMuPDF quirk to special-case
// broadly. The user's own words when presented the choice: "cách nào
// tốt nhất, tôi muốn nếu có thể khắc phục nên là 'Co.opMart Cai Lay'
// thay vì 'Co. opMar t Cai Lay'" — prefer Go's correct output, do not
// intentionally reproduce Python's bug to force parity.
var knownDivergences_Coop = map[string]bool{
	"103297732-00.pdf:0:E": true,
	"103297732-00.pdf:1:E": true,
	"103297732-00.pdf:2:E": true,
	"103297732-00.pdf:3:E": true,
	"103311304-00.pdf:0:E": true,
	"103311304-00.pdf:1:E": true,
	"103311304-00.pdf:2:E": true,
	"103362540-00.pdf:0:E": true,
	"103362540-00.pdf:1:E": true,
	"103362540-00.pdf:2:E": true,
	"103400368-00.pdf:0:E": true,
	"103400368-00.pdf:1:E": true,
	"103400368-00.pdf:2:E": true,
	"103400368-00.pdf:3:E": true,
	"103400368-00.pdf:4:E": true,
	"103400368-00.pdf:5:E": true,
	"103400368-00.pdf:6:E": true,
}

// Coverage note: this test validates against the 151 real Coop PDFs that
// could still be located (out of the 155 fixtures originally generated),
// committed into coop/testdata/realpdfs/ instead of being read from the
// live đơn hàng/ tree — that tree is continuously reorganized by a real,
// concurrently-running production instance of this application (files get
// moved into a dated archive under "đơn hàng/mẫu đơn hàng/<date>/" and
// renamed), which made every fixture here fail before this migration. See
// the sibling Emart/FujiMart/Kingfood golden tests for the same pattern.
// The 4 fixtures whose source PDFs could not be located in either the live
// folder or the archive (103108366-00, 103125805-00, 103125992-00,
// 103133307-00) were moved to coop/testdata/fixtures_missing_pdf/ (not
// deleted — they remain valid ground truth if their PDFs ever resurface)
// so this test's glob no longer picks them up and reports them as
// mismatches on every run.
func TestRealProcessor_MatchesGoldenFixtures(t *testing.T) {
	fixturePaths, err := filepath.Glob("coop/testdata/fixtures/*.json")
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
		t.Skip("no golden fixtures found (run Task 12's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenPricingSource(t)

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

		pdfPath := filepath.Join("coop", "testdata", "realpdfs", fixture.SourcePDF)
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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Coop)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
