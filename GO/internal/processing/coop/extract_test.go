package coop

import (
	"strings"
	"testing"
)

func TestExtractProducts_SingleProductBlock(t *testing.T) {
	text := "3564270-4  Chai tay toilet CHUNGBLUE180g   EA   C24   809424.00   809424.00   1.00   24.00   .00   809,424.00\nSub Total"
	products, err := ExtractProducts(text)
	if err != nil {
		t.Fatalf("ExtractProducts returned error: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("ExtractProducts = %d products, want 1: %+v", len(products), products)
	}
	if products[0].Barcode != "3564270-4" {
		t.Fatalf("Barcode = %q, want %q", products[0].Barcode, "3564270-4")
	}
	if products[0].Qty != 24 {
		t.Fatalf("Qty = %v, want 24", products[0].Qty)
	}
	if products[0].Cost != 809424 {
		t.Fatalf("Cost = %v, want 809424", products[0].Cost)
	}
}

// Real PO 104743481-00: Coop prints the CTKM gift as its own line with
// every money column ".00". The bare ".00" form has no leading digit, so
// the line used to yield a single number and refuse the whole page.
func TestExtractProducts_ZeroCostGiftLine(t *testing.T) {
	const gift = "  3613120-9  KM2T1-TuiNLS1L(NGX Blue CL)                     EA   EA               .00             .00         .00      100.00         .00              .00 \n"
	for _, tc := range []struct {
		name string
		text string
	}{
		{"gift last", "  3558665-1  NGX BLUE h.thao moc tui3.6L                     EA   C04        162881.25       162881.25      120.00      120.00         .00    19,545,750.00 \n" +
			"             MUA 1 TANG 1 QUAN KEM \n" + gift +
			"            Sub Total -       120.00      220.00         .00    19,545,750.00 \n"},
		{"gift first", gift +
			"  3558665-1  NGX BLUE h.thao moc tui3.6L                     EA   C04        162881.25       162881.25      120.00      120.00         .00    19,545,750.00 \n" +
			"            Sub Total -       120.00      220.00         .00    19,545,750.00 \n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			products, err := ExtractProducts(tc.text)
			if err != nil {
				t.Fatalf("ExtractProducts returned error: %v", err)
			}
			var giftLine *Product
			for i := range products {
				if products[i].Barcode == "3613120-9" {
					giftLine = &products[i]
				}
			}
			if len(products) != 2 || giftLine == nil {
				t.Fatalf("ExtractProducts = %+v, want the paid line plus gift 3613120-9", products)
			}
			if giftLine.Qty != 100 || giftLine.Cost != 0 {
				t.Errorf("gift line = %+v, want Qty 100, Cost 0", *giftLine)
			}
		})
	}
}

func TestExtractProducts_TwoProductBlocks(t *testing.T) {
	text := "3564270-4  Chai tay toilet   1.00   24.00   809,424.00\n" +
		"3564271-9  Chai tay khac    1.00   12.00   400,000.00\nSub Total"
	products, err := ExtractProducts(text)
	if err != nil {
		t.Fatalf("ExtractProducts returned error: %v", err)
	}
	if len(products) != 2 {
		t.Fatalf("ExtractProducts = %d products, want 2: %+v", len(products), products)
	}
	if products[0].Barcode != "3564270-4" || products[1].Barcode != "3564271-9" {
		t.Fatalf("barcodes = %q, %q", products[0].Barcode, products[1].Barcode)
	}
}

func TestExtractProducts_NoSkuAnchorsReturnsEmpty(t *testing.T) {
	products, err := ExtractProducts("no product lines here\nSub Total")
	if err != nil {
		t.Fatalf("ExtractProducts returned error: %v", err)
	}
	if len(products) != 0 {
		t.Fatalf("ExtractProducts = %v, want empty", products)
	}
}

// TestExtractProducts_RefusesAFractionalQuantity feeds the real glued text
// 103909234-00.pdf produced before its rotated-page word gaps were sized
// correctly: the four numeric columns 341640.00 | 341640.00 | 7.00 | 28.00
// | .00 arrived as one token, and the quantity came back as 7.0028 instead
// of 28.
//
// Nothing already in place catches this. The Sub Total matches perfectly —
// only the quantity is wrong, the money is right — and the implied unit
// price 2,391,480 / 7.0028 = 341,502 sits inside the same sane-price band
// the true 2,391,480 / 28 = 85,410 does. The page reported a clean
// "Hoàn Thành" and wrote a wrong quantity (and, from it, a wrong case
// count and weight) straight into the workbook.
func TestExtractProducts_RefusesAFractionalQuantity(t *testing.T) {
	text := "Currency- VND Viet Nam Dong\n" +
		"SKUNumberDescription VendorPartNo.U/MU/M Cost Cost CSPcsPcs Cost\n" +
		"3547984-5NGBLUEhuongthaomoc3kg EAC04 341640.00341640.007.0028.00.00 2,391,480.00\n" +
		"SubTotal-7.0028.00.00 2,391,480.00\n"

	products, err := ExtractProducts(text)
	if err == nil {
		t.Fatalf("ExtractProducts accepted a fractional quantity: %+v", products)
	}
	for _, want := range []string{"3547984-5", "7.0028"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q — it has to say which line and what was read", err, want)
		}
	}
}

// TestExtractProducts_RefusesAZeroQuantity covers the other shape the same
// glue produces: the heuristic lands on the ".00" Qty Rec/Pcs column
// instead of the real one. The smallest quantity across 153 real archived
// Coop PDFs (378 product lines) is 3, so zero is not a quantity this
// vendor orders — it only ever means the read went wrong.
func TestExtractProducts_RefusesAZeroQuantity(t *testing.T) {
	// The same glue, landing one column over: the run-together ".00.00"
	// tail parses as the number "00.00", and with no comma-formatted
	// figure in the block to anchor on, the heuristic falls back to
	// "second-to-last number" and takes it. Confirmed to produce
	// Qty:0 against the real selector.
	text := pageWith([]string{
		"  3547984-5  NG BLUE huong thao moc 3kg      EA   C04   341640.00   1.00.00.00   683280.00",
	}, "683,280.00")

	products, err := ExtractProducts(text)
	if err == nil {
		t.Fatalf("ExtractProducts accepted a zero quantity: %+v", products)
	}
	if !strings.Contains(err.Error(), "3547984-5") {
		t.Errorf("error %q does not name the offending line", err)
	}
}

// TestExtractProducts_AcceptsWholeNumberQuantities guards the check against
// firing on good data. Every one of the 378 product lines across the 153
// real archived Coop PDFs has an integer quantity, ranging 3 to 300.
func TestExtractProducts_AcceptsWholeNumberQuantities(t *testing.T) {
	products, err := ExtractProducts(pageWith(sixRealRows, "3,825,877.56"))
	if err != nil {
		t.Fatalf("ExtractProducts rejected a page whose quantities are all whole numbers: %v", err)
	}
	for _, p := range products {
		if p.Qty != 8 {
			t.Errorf("Qty = %v for %s, want 8", p.Qty, p.Barcode)
		}
	}
}
