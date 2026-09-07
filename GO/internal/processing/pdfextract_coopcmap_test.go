package processing

import (
	"testing"

	"order-processor/internal/processing/coop"
)

// TestExtractPageText_CoopCorrectedCmapPageKeepsItsLineBreaks covers a
// real archived Coop PO (103823984-00, six SKUs) whose font carries a
// broken CMap. That sends the page down extractPageTextViaCorrectedCmap,
// which decodes the characters correctly but breaks lines only on the
// BT/T* operators GetPlainText uses — and this generator positions each
// visual row with Td/Tm inside ONE BT block, so all six product rows
// arrived as a single line of text.
//
// coop.ExtractProducts anchors on lines carrying a SKU, so one line meant
// one product. Worse, silently wrong rather than empty: the surviving row
// paired the FIRST SKU with the LAST row's extended cost (3547984-5 at
// 579,828.78 instead of its own 683,280.00), and nothing about that looks
// like an error downstream.
func TestExtractPageText_CoopCorrectedCmapPageKeepsItsLineBreaks(t *testing.T) {
	texts, _, err := extractPageTexts("coop/testdata/realpdfs/103823984-00.pdf")
	if err != nil {
		t.Fatalf("extractPageTexts returned error: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("got %d pages, want 1", len(texts))
	}

	products, err := coop.ExtractProducts(texts[0])
	if err != nil {
		t.Fatalf("ExtractProducts returned error: %v", err)
	}

	want := []coop.Product{
		{Barcode: "3547984-5", Qty: 8, Cost: 683280.00},
		{Barcode: "3547985-0", Qty: 8, Cost: 683280.00},
		{Barcode: "3585298-1", Qty: 8, Cost: 649830.00},
		{Barcode: "3585299-6", Qty: 8, Cost: 649830.00},
		{Barcode: "3590926-3", Qty: 8, Cost: 579828.78},
		{Barcode: "3590927-8", Qty: 8, Cost: 579828.78},
	}
	if len(products) != len(want) {
		t.Fatalf("got %d products, want %d: %+v", len(products), len(want), products)
	}
	for i, w := range want {
		if products[i] != w {
			t.Errorf("product %d = %+v, want %+v", i, products[i], w)
		}
	}
}

// TestExtractPageText_CoopCorrectedCmapPageWithExtraBreaksIsLeftAlone
// pins the other side of the gate above, and it is not hypothetical:
// widening the corrected-CMap fall-through to the symmetric "line counts
// diverge either way" test broke this real fixture (103346096-00) in 22
// places — its store name lost its spaces ("Co.opMartBaoLoc"), its
// quantities came back as 235.0035 instead of 35, and two rows gained a
// spurious price-mismatch flag.
//
// The corrected text for this page carries far MORE newlines than the
// page has visual rows, yet parses correctly. Only the opposite shape —
// materially fewer lines than rows, meaning product rows are glued
// together — is worth distrusting.
func TestExtractPageText_CoopCorrectedCmapPageWithExtraBreaksIsLeftAlone(t *testing.T) {
	texts, _, err := extractPageTexts("coop/testdata/realpdfs/103346096-00.pdf")
	if err != nil {
		t.Fatalf("extractPageTexts returned error: %v", err)
	}
	products, err := coop.ExtractProducts(texts[0])
	if err != nil {
		t.Fatalf("ExtractProducts returned error: %v", err)
	}
	want := []coop.Product{
		{Barcode: "3558665-1", Qty: 67, Cost: 10913043.75},
		{Barcode: "3558666-6", Qty: 35, Cost: 5700843.75},
	}
	if len(products) != len(want) {
		t.Fatalf("got %d products, want %d: %+v", len(products), len(want), products)
	}
	for i, w := range want {
		if products[i] != w {
			t.Errorf("product %d = %+v, want %+v", i, products[i], w)
		}
	}
	// The store name is where the reconstruction's lost word spacing
	// showed up first, but this page's raw text carries spurious breaks
	// mid-token that later normalisation cleans up, so it is the golden
	// fixture's own column E — not this text — that pins it.
}
