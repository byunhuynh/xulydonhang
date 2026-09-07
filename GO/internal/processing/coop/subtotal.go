package coop

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	subTotalMarkerPattern = regexp.MustCompile(`(?i)` + spacedPattern("SubTotal"))
	// afterSubTotalPattern marks where the Sub Total figures stop. This
	// generator prints "Notes -", "FOB -", "Ship Via -" and the PO-wide
	// "Total -" after them, and on a multi-page PO that grand Total is a
	// DIFFERENT number from this page's Sub Total — reading past it would
	// compare a page against the whole order and fail every long PO.
	afterSubTotalPattern = regexp.MustCompile(`(?i)` + spacedPattern("Notes") + `|` + spacedPattern("FOB") + `|` + spacedPattern("ShipVia") + `|` + spacedPattern("Total") + `\s*-`)
)

// SubTotal reads the extended-cost total this page prints on its own
// "Sub Total" line — the sum the page's product rows are supposed to add
// up to.
//
// The numbers do not always sit on the marker's own line: this generator
// puts them there on most pages but spreads them over the following lines
// on others (real: 103269228-00), so the search runs from the marker to
// the next section heading rather than to the end of the line. The LAST
// figure in that span is the extended cost; the ones before it are the
// case and piece quantities.
//
// ok=false means the page does not state a figure that can be read — ten
// of the 152 archived Coop pages print "Sub Total -" with no numbers
// surviving into the extracted text at all. Callers must treat that as
// "nothing to check", never as zero.
func SubTotal(text string) (float64, bool) {
	loc := subTotalMarkerPattern.FindStringIndex(text)
	if loc == nil {
		return 0, false
	}
	tail := text[loc[1]:]
	if end := afterSubTotalPattern.FindStringIndex(tail); end != nil {
		tail = tail[:end[0]]
	}
	nums := findDecimalNumbers(tail)
	if len(nums) == 0 {
		return 0, false
	}
	return parseAmount(nums[len(nums)-1])
}

// parseAmount turns one of this generator's figures ("3,825,877.56")
// into a number.
func parseAmount(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// subTotalToleranceDong bounds how far the extracted rows may sum from
// the page's own printed Sub Total. Both sides are extended costs the
// page states to the đồng, so any real difference is a whole missing or
// duplicated row, not rounding; 1 đồng only absorbs float addition. Same
// value and same reasoning as closeEnough's own maxDiffDong.
const subTotalToleranceDong = 1.0

// checkAgainstSubTotal compares what was extracted against what the page
// says its rows add up to.
//
// This exists because the failure it catches is SILENT. Real PO
// 103823984-00 has six products; a line-break defect in PDF extraction
// made all six arrive as one line of text, and since products are
// anchored on lines carrying a SKU, exactly one product came out —
// carrying the FIRST row's code and the LAST row's cost. Nothing
// downstream could tell that from a genuine one-line order, so a wrong
// order was written to the workbook with no warning at all. The page
// states the answer itself, so refuse the page instead of guessing.
func checkAgainstSubTotal(products []Product, text string) error {
	want, ok := SubTotal(text)
	if !ok {
		return nil
	}
	got := 0.0
	for _, p := range products {
		got += p.Cost
	}
	if math.Abs(got-want) <= subTotalToleranceDong {
		return nil
	}
	return fmt.Errorf("tổng tiền %d dòng đọc được (%s) không khớp Sub Total trên PO (%s) — nhiều khả năng đọc thiếu hoặc thừa dòng sản phẩm",
		len(products), formatAmount(got), formatAmount(want))
}

// formatAmount renders a figure the way the PO itself prints it, so the
// error message can be compared against the paper by eye.
func formatAmount(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	dot := strings.IndexByte(s, '.')
	intPart, frac := s[:dot], s[dot:]
	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")

	var b strings.Builder
	for i, d := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	if neg {
		return "-" + b.String() + frac
	}
	return b.String() + frac
}
