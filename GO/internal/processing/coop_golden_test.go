package processing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"order-processor/internal/processing/pricing"
	"order-processor/internal/processing/productdata"
)

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

		pdfPath := filepath.Join("..", "..", "..", "đơn hàng", "08-2026", fixture.SourcePDF)
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

		compareRowsAgainstFixture(t, excelPath, fixture, &mismatches, nil)
	}

	if len(mismatches) > 0 {
		t.Fatalf("%d/%d fixtures mismatched:\n%s", len(mismatches), len(realFixtures), joinLines(mismatches))
	}
	t.Logf("all %d fixtures matched", len(realFixtures))
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

	textColumns := []string{"A", "B", "C", "D", "E", "G", "L", "Q", "S", "T", "U", "V", "AJ", "AM", "AO", "AP", "AQ"}
	floatColumns := []string{"X", "Y", "AT"}
	intColumns := []string{"AE", "AU", "AV"}

	isAllowed := func(rowIdx int, col string) bool {
		if allowedDivergences == nil {
			return false
		}
		return allowedDivergences[fmt.Sprintf("%d:%s", rowIdx, col)]
	}

	for i, expectedRow := range fixture.Rows {
		rowNum := startRow + i
		cell := func(col string) string {
			v, _ := f.GetCellValue("Don dat hang", fmt.Sprintf("%s%d", col, rowNum))
			return v
		}

		for _, col := range textColumns {
			if isAllowed(i, col) {
				continue
			}
			expected := stringify(expectedRow[col])
			got := cell(col)
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
