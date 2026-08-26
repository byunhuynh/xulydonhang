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

// knownDivergences_Satra lists (fixture, row index, column) cells where
// this Go port intentionally computes a different, verified-more-correct
// value than the frozen Python fixture. Key format:
// "<source_pdf>:<row index>:<column>", e.g.
// "P-005508192.pdf:3:AU" — see this plan's Global Constraints for the
// specific, already-anticipated AU/invoice-bonus-row case this may be
// needed for. Empty until a real, hand-verified case is confirmed; add
// entries here only with a comment citing the specific PDF/Python-line
// evidence — never to silence an unexplained diff.
var knownDivergences_Satra = map[string]bool{}

func loadFrozenSatraPricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("satra/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen Satra pricing fixture found (run Task 6's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen Satra pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

// TestRealProcessor_MatchesGoldenFixtures_Satra runs against stable,
// git-tracked real PDFs under satra/testdata/realpdfs/ instead of the live
// đơn hàng/08-2026/ folder, which is continuously reorganized by a
// concurrently-running production instance of this application (files get
// moved into a dated archive and renamed) — matching the pattern
// established by Emart/FujiMart/Kingfood/Coop/Lotte. 33 of the 36 original
// fixtures' source PDFs were recoverable from the đơn hàng/mẫu đơn hàng/
// archive tree; the remaining 3 (P-005523317, P-005523651, P-005523835 —
// all with entry dates of 15-17/08/2026, the most recent batch at the time
// of this search) were not yet archived and have been moved to
// satra/testdata/fixtures_missing_pdf/ pending their PDFs resurfacing.
func TestRealProcessor_MatchesGoldenFixtures_Satra(t *testing.T) {
	fixturePaths, err := filepath.Glob("satra/testdata/fixtures/*.json")
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
		t.Skip("no golden fixtures found (run Task 6's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenSatraPricingSource(t)

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

		pdfPath := filepath.Join("satra", "testdata", "realpdfs", fixture.SourcePDF)
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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Satra)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
