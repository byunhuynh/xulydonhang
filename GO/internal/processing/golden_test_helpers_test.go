package processing

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/processing/pricing"
)

// whitespaceRunPattern collapses any run of whitespace to a single space —
// used only by the column-E whitespace-tolerant comparison below.
var whitespaceRunPattern = regexp.MustCompile(`\s+`)

// collapseWhitespace normalizes runs of whitespace to a single space, for
// comparisons that need to tolerate a documented whitespace-only
// divergence (see the column-E handling in compareRowsAgainstFixture)
// without also masking a genuine content bug.
func collapseWhitespace(s string) string {
	return whitespaceRunPattern.ReplaceAllString(s, " ")
}

// stripAllWhitespace removes every whitespace character entirely (not
// just collapsing runs — see collapseWhitespace), for a documented
// divergence where whitespace appears at DIFFERENT character positions
// on each side, not merely a different RUN WIDTH at the same position.
// Confirmed real case (see knownDivergences_Coop's own doc comment):
// Python/PyMuPDF's real output for 4 archived Coop PDFs has a genuine
// mid-word-splitting bug on this exact font ("Co. opMar t Cai Lay"),
// where collapseWhitespace alone ("Co. opMar t Cai Lay", unchanged —
// every run is already width 1) can never equal Go's correct output
// ("Co.opMart Cai Lay") no matter how it's collapsed; only removing
// whitespace ENTIRELY makes both sides identical ("Co.opMartCaiLay").
func stripAllWhitespace(s string) string {
	return whitespaceRunPattern.ReplaceAllString(s, "")
}

// fixturePricingSource is a PricingSource that always returns the same
// frozen *pricing.Index — used by both the "real sample file" processor
// tests (with a small inline index) and the golden-fixture tests (with
// an index parsed from a captured _frozen_pricing.json).
type fixturePricingSource struct {
	index *pricing.Index
}

func (f *fixturePricingSource) FetchIndex(sheetKey string) (*pricing.Index, error) {
	return f.index, nil
}

func copyTestWorkbookForProcessor(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/dondathang.xlsx")
	if err != nil {
		t.Fatalf("failed reading test workbook fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "dondathang.xlsx")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed writing temp workbook: %v", err)
	}
	return path
}

// frozenPricingFixture mirrors _frozen_pricing.json's shape: the single
// raw CSV snapshot both the price and promotion views are derived from
// (see Task 3's design note and Task 12's harness comment).
type frozenPricingFixture struct {
	RawRows [][]string `json:"raw_rows"`
}

type fixtureData struct {
	SourcePDF string           `json:"source_pdf"`
	Rows      []map[string]any `json:"rows"`
}

// compareRowsAgainstFixture re-reads excelPath's "Don dat hang" sheet
// starting at the row RealProcessor just wrote (existing header rows
// before it are whatever the copied testdata/dondathang.xlsx template
// had — always 8, per Task 9's fixture), and diffs every column Task
// 12's harness captured against what's actually on disk. Text/SKU
// columns must match exactly; Y (price) and AT (line weight) allow a
// small float tolerance, since values round-trip through JSON and
// openpyxl/excelize float formatting in ways that can differ in the
// last decimal digit without being a real bug.
//
// Task 12's first-run spot-check (see task-12-report.md) found real
// fixtures with Y_has_comment: true (23 of the 155), so this also
// checks the price-mismatch fill/comment flags the brief flagged as
// conditional: whether a comment is attached to the Y cell, and
// whether a non-default (red-fill) style was applied to it — using
// f.GetComments and f.GetCellStyle the same way excelwriter's own test
// (dondathang_test.go) already does, rather than comparing exact ARGB
// strings (openpyxl and excelize report fill color metadata in
// different shapes; what actually matters is "was the mismatch flagged
// at all", which both representations agree on).
func compareRowsAgainstFixture(t *testing.T, excelPath string, fixture fixtureData, mismatches *[]string, allowedDivergences map[string]bool) {
	t.Helper()

	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: failed reopening workbook: %v", fixture.SourcePDF, err))
		return
	}
	defer f.Close()

	existingRows, err := f.GetRows("Don dat hang")
	if err != nil {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: failed reading sheet: %v", fixture.SourcePDF, err))
		return
	}
	// RealProcessor appended len(fixture.Rows) rows starting right after
	// whatever was already in the sheet before Process ran; since this
	// test copies a fresh 8-row-header template per fixture (Task 9's
	// testdata/dondathang.xlsx), the written rows start at row 9 —
	// compute it from the actual row count instead of hardcoding 9, so
	// this still works if that template's header size ever changes.
	startRow := len(existingRows) - len(fixture.Rows) + 1
	if startRow < 1 {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: sheet has fewer rows (%d) than the fixture expects (%d)", fixture.SourcePDF, len(existingRows), len(fixture.Rows)))
		return
	}

	comments, err := f.GetComments("Don dat hang")
	if err != nil {
		*mismatches = append(*mismatches, fmt.Sprintf("%s: failed reading comments: %v", fixture.SourcePDF, err))
		return
	}
	commentedCells := make(map[string]bool, len(comments))
	for _, c := range comments {
		commentedCells[c.Cell] = true
	}

	textColumns := []string{"A", "B", "C", "D", "E", "G", "L", "Q", "S", "T", "U", "V", "AJ", "AM", "AN", "AO", "AP", "AQ"}
	floatColumns := []string{"X", "Y", "AT"}
	intColumns := []string{"AE", "AU", "AV"}

	isAllowed := func(rowIdx int, col string) bool {
		if allowedDivergences == nil {
			return false
		}
		return allowedDivergences[fmt.Sprintf("%s:%d:%s", fixture.SourcePDF, rowIdx, col)]
	}

	for i, expectedRow := range fixture.Rows {
		rowNum := startRow + i
		cell := func(col string) string {
			v, _ := f.GetCellValue("Don dat hang", fmt.Sprintf("%s%d", col, rowNum))
			return v
		}

		for _, col := range textColumns {
			// AN (SiteCode) is a newer column: only Emart's/BigC's own
			// generate_fixtures.py capture it (Emart's COLUMNS list has
			// "AN"; older vendors' - and BigC's own, pending a fixture
			// regen - do not). A fixture that never captured AN has no key
			// at all in expectedRow, which must NOT be treated as "want
			// empty" - that would flag every real (non-empty) AN value
			// this port now writes as a mismatch against ground truth that
			// was simply never recorded, not against a real Python "".
			if col == "AN" {
				if _, captured := expectedRow["AN"]; !captured {
					continue
				}
			}
			expected := stringify(expectedRow[col])
			got := cell(col)
			if isAllowed(i, col) {
				// Column E's allowlist entries document a specific,
				// evidenced whitespace-only divergence, not a license to
				// ignore the cell outright: still compare after
				// normalizing whitespace, so a genuine content bug (wrong
				// branch name, wrong store code, an undecoded mojibake
				// character) hiding behind the same allowlist entry still
				// fails this test. Two different whitespace-only
				// divergence SHAPES are known, tried in order: (1)
				// FujiMart's own case is a different whitespace RUN WIDTH
				// at the same position (collapseWhitespace: any run
				// becomes a single space); (2) Coop's own case (see
				// knownDivergences_Coop) is whitespace at genuinely
				// DIFFERENT character positions — a real Python/PyMuPDF
				// mid-word-splitting bug on that font — which
				// collapseWhitespace alone can never equal, since every
				// run there is already width 1; only removing whitespace
				// ENTIRELY (stripAllWhitespace) makes both sides
				// identical. Every other column keeps the original
				// blanket-skip behavior, unchanged for the other vendors'
				// golden tests that also share this helper.
				if col == "E" {
					if collapseWhitespace(expected) != collapseWhitespace(got) && stripAllWhitespace(expected) != stripAllWhitespace(got) {
						*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col %s: got %q, want %q (mismatch even after whitespace normalization)", fixture.SourcePDF, i, col, got, expected))
					}
				}
				continue
			}
			if expected != got {
				*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col %s: got %q, want %q", fixture.SourcePDF, i, col, got, expected))
			}
		}

		for _, col := range floatColumns {
			if isAllowed(i, col) {
				continue
			}
			expected := toFloat(expectedRow[col])
			got := toFloat(cell(col))
			if !floatCloseEnough(expected, got) {
				*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col %s: got %v, want %v", fixture.SourcePDF, i, col, got, expected))
			}
		}

		for _, col := range intColumns {
			if isAllowed(i, col) {
				continue
			}
			expected := stringify(expectedRow[col])
			got := cell(col)
			if expected != got {
				*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col %s: got %q, want %q", fixture.SourcePDF, i, col, got, expected))
			}
		}

		expectedFormula, _ := expectedRow["Z_is_formula"].(bool)
		gotFormula, _ := f.GetCellFormula("Don dat hang", fmt.Sprintf("Z%d", rowNum))
		if expectedFormula != (gotFormula != "") {
			*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col Z: formula present = %v, want %v", fixture.SourcePDF, i, gotFormula != "", expectedFormula))
		}

		yCell := fmt.Sprintf("Y%d", rowNum)
		expectedHasComment, _ := expectedRow["Y_has_comment"].(bool)
		gotHasComment := commentedCells[yCell]
		if expectedHasComment != gotHasComment {
			*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col Y: has comment = %v, want %v", fixture.SourcePDF, i, gotHasComment, expectedHasComment))
		}

		expectedFillStr, _ := expectedRow["Y_fill"].(string)
		expectedHasFill := expectedFillStr != "" && expectedFillStr != "00000000"
		styleID, err := f.GetCellStyle("Don dat hang", yCell)
		if err != nil {
			*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col Y: failed reading style: %v", fixture.SourcePDF, i, err))
			continue
		}
		gotHasFill := styleID != 0
		if expectedHasFill != gotHasFill {
			*mismatches = append(*mismatches, fmt.Sprintf("%s row %d col Y: has red fill = %v, want %v", fixture.SourcePDF, i, gotHasFill, expectedHasFill))
		}
	}
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return toFloatString(t)
	case bool:
		if t {
			return "Có"
		}
		return "Không"
	default:
		return fmt.Sprintf("%v", t)
	}
}

func toFloatString(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%v", f)
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		var f float64
		fmt.Sscanf(t, "%f", &f)
		return f
	default:
		return 0
	}
}

func floatCloseEnough(a, b float64) bool {
	const tolerance = 0.01
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("failed reading %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("failed writing %s: %v", dst, err)
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += "  - " + l + "\n"
	}
	return out
}
