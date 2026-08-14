package coop

import "testing"

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
