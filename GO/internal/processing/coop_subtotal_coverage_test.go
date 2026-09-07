package processing

import (
	"path/filepath"
	"testing"

	"order-processor/internal/processing/coop"
)

// minSubTotalCoverage is the number of archived Coop pages whose own
// "Sub Total" figure must stay readable. Measured at 142 of 152 when the
// check was added; the other ten print "Sub Total -" with no numbers
// surviving into the extracted text at all, and are skipped by design.
const minSubTotalCoverage = 140

// TestCoopSubTotalCheckStaysArmed guards the cross-check itself rather
// than any one page. coop.SubTotal returning "not readable" is a silent
// no-op — the order still processes — so a regression in how that figure
// is found would quietly disarm the whole safety net across every Coop
// order at once, and nothing else in the suite would notice.
//
// It does not assert the sums agree: coop.ExtractProducts now refuses a
// page whose rows do not add up, so a disagreement anywhere in the
// archive already fails the golden-fixture suites loudly.
func TestCoopSubTotalCheckStaysArmed(t *testing.T) {
	files, err := filepath.Glob("coop/testdata/realpdfs/*.pdf")
	if err != nil || len(files) == 0 {
		t.Skipf("no archived Coop PDFs found: %v", err)
	}

	pagesWithProducts, readable := 0, 0
	for _, path := range files {
		texts, _, err := extractPageTexts(path)
		if err != nil {
			continue
		}
		for _, text := range texts {
			products, err := coop.ExtractProducts(text)
			if err != nil || len(products) == 0 {
				continue
			}
			pagesWithProducts++
			if _, ok := coop.SubTotal(text); ok {
				readable++
			}
		}
	}

	t.Logf("%d of %d archived Coop pages state a readable Sub Total", readable, pagesWithProducts)
	if readable < minSubTotalCoverage {
		t.Errorf("only %d of %d pages have a readable Sub Total, want at least %d — the cross-check has been disarmed for the rest",
			readable, pagesWithProducts, minSubTotalCoverage)
	}
}
