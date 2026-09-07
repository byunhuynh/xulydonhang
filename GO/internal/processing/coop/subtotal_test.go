package coop

import (
	"strings"
	"testing"
)

// pageWith builds a minimal page in the shape Coop's own report generator
// produces: a header the SKU anchor pattern ignores, one line per product,
// then the Sub Total line carrying the page's own extended-cost total.
func pageWith(productLines []string, subTotal string) string {
	var b strings.Builder
	b.WriteString("Currency- VND Viet Nam Dong\n")
	b.WriteString(" SKU Number  Description                    Vendor Part No.  U/M  U/M\n")
	for _, line := range productLines {
		b.WriteString(line + "\n")
	}
	b.WriteString("                    Sub Total -        12.00       48.00         .00        " + subTotal + "\n")
	b.WriteString("             Notes - Xin vui long kem DDH khi giao hang\n")
	return b.String()
}

var sixRealRows = []string{
	"  3547984-5  NG BLUE huong thao moc 3kg      EA   C04   341640.00   341640.00   2.00   8.00   .00   683,280.00",
	"  3547985-0  NG BLUE huong nuoc hoa 3kg      EA   C04   341640.00   341640.00   2.00   8.00   .00   683,280.00",
	"  3585298-1  NGX Clean&Clean hg Lily T3.2L   EA   C04   324915.00   324915.00   2.00   8.00   .00   649,830.00",
	"  3585299-6  NGX Clean&Clean hg p.hongT3.2L  EA   C04   324915.00   324915.00   2.00   8.00   .00   649,830.00",
	"  3590926-3  NRC Blue khong mui Tui 3.2L     EA   C04   289914.39   289914.39   2.00   8.00   .00   579,828.78",
	"  3590927-8  NRC Blue chanh Tui 3.2L         EA   C04   289914.39   289914.39   2.00   8.00   .00   579,828.78",
}

func TestExtractProducts_AcceptsAPageThatAddsUp(t *testing.T) {
	products, err := ExtractProducts(pageWith(sixRealRows, "3,825,877.56"))
	if err != nil {
		t.Fatalf("ExtractProducts returned error on a page that adds up: %v", err)
	}
	if len(products) != 6 {
		t.Fatalf("got %d products, want 6", len(products))
	}
}

// TestExtractProducts_RejectsAPageMissingRows is the whole point of the
// check: extracting fewer lines than the PO actually has used to be
// SILENT. Real PO 103823984-00 came out as a single product carrying the
// last row's cost, and nothing downstream could tell that from a genuine
// one-line order.
func TestExtractProducts_RejectsAPageMissingRows(t *testing.T) {
	// The exact shape the real defect produced: the first row's SKU
	// carrying the last row's extended cost, alone on the page.
	collapsed := []string{
		"  3547984-5  NG BLUE huong thao moc 3kg      EA   C04   289914.39   289914.39   2.00   8.00   .00   579,828.78",
	}
	_, err := ExtractProducts(pageWith(collapsed, "3,825,877.56"))
	if err == nil {
		t.Fatal("ExtractProducts accepted a page whose lines do not add up to its own Sub Total")
	}
	// The message has to carry both numbers: the person reading it needs
	// to see how far off the page is, not just that something is wrong.
	for _, want := range []string{"579,828.78", "3,825,877.56"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestExtractProducts_SkipsTheCheckWhenSubTotalIsUnreadable(t *testing.T) {
	// Ten of the 152 archived Coop pages have a "Sub Total -" line whose
	// numbers never made it into the extracted text. Those must keep
	// working exactly as before rather than failing the order.
	page := strings.Replace(pageWith(sixRealRows, "3,825,877.56"),
		"Sub Total -        12.00       48.00         .00        3,825,877.56", "Sub Total -", 1)
	if _, err := ExtractProducts(page); err != nil {
		t.Fatalf("ExtractProducts returned error when the Sub Total is unreadable: %v", err)
	}
}

func TestSubTotal(t *testing.T) {
	if got, ok := SubTotal(pageWith(sixRealRows, "3,825,877.56")); !ok || got != 3825877.56 {
		t.Errorf("SubTotal = (%v, %v), want (3825877.56, true)", got, ok)
	}
	if _, ok := SubTotal("khong co dong nao ten Sub Total"); ok {
		t.Error("SubTotal reported ok on a page with no Sub Total line")
	}
}
