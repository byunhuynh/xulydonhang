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

// knownDivergences_Lotte lists (fixture row index, column) cells where
// this Go port intentionally computes a different, verified-more-correct
// value than the frozen Python fixture — per this plan's testing policy
// (see the plan's Global Constraints / the spec's "Chiến lược kiểm
// chứng"). Empty until a real, hand-verified case is found; add entries
// here only with a comment citing the specific PDF evidence that proves
// Python is wrong on that cell — never to silence an unexplained diff.
// Key format: "<source PDF filename>:<fixture row index>:<column>", e.g.
// "260727-01013-00057.pdf:0:D". The source PDF filename is required so an
// entry added as evidence for one specific PDF's cell doesn't silently
// suppress the same row/column check on every other fixture too.
var knownDivergences_Lotte = map[string]bool{}

func loadFrozenLottePricingSource(t *testing.T) *fixturePricingSource {
	t.Helper()
	data, err := os.ReadFile("lotte/testdata/fixtures/_frozen_pricing.json")
	if err != nil {
		t.Skipf("no frozen Lotte pricing fixture found (run Task 8's generate_fixtures.py first): %v", err)
	}
	var frozen frozenPricingFixture
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatalf("failed parsing frozen Lotte pricing fixture: %v", err)
	}
	return &fixturePricingSource{index: pricing.ParseIndex(frozen.RawRows)}
}

// Coverage note: 57/60 of Lotte's original golden fixtures' source PDFs
// were locatable (live folder or the đơn hàng/mẫu đơn hàng/ archive) and
// are committed under lotte/testdata/realpdfs/. The remaining 3
// (260814-01004-00222, 260814-01005-00018, 260814-01012-00035) were not
// found anywhere under đơn hàng/ and their fixture JSONs were moved (via
// git mv, not deleted) to lotte/testdata/fixtures_missing_pdf/ so they no
// longer report as permanent failures here, while staying in git history
// ready to be restored if their PDFs ever resurface.
func TestRealProcessor_MatchesGoldenFixtures_Lotte(t *testing.T) {
	fixturePaths, err := filepath.Glob("lotte/testdata/fixtures/*.json")
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
		t.Skip("no golden fixtures found (run Task 8's generate_fixtures.py first)")
	}

	store, err := productdata.Load("../../../data.xlsx")
	if err != nil {
		t.Fatalf("failed loading production data.xlsx: %v", err)
	}
	pricingSource := loadFrozenLottePricingSource(t)

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

		pdfPath := filepath.Join("lotte", "testdata", "realpdfs", fixture.SourcePDF)
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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, knownDivergences_Lotte)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
}
