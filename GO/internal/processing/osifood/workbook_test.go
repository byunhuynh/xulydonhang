package osifood

import (
	"path/filepath"
	"testing"
)

// realWorkbook is the one real campaign sheet available: four products,
// seventeen store blocks, five of which answered with a quantity.
var realWorkbook = filepath.Join("testdata", "CTKM-16-31.10-HA-THANH-LONG-AN-1.xlsx")

func TestIsWorkbook_RecognizesTheRealCampaignSheet(t *testing.T) {
	if !IsWorkbook(realWorkbook) {
		t.Error("IsWorkbook = false for the real campaign sheet")
	}
	if IsWorkbook(filepath.Join("testdata", "realpdfs", "DH-HA-THANH-LA1-Binh-Loi-12.10.pdf")) {
		t.Error("IsWorkbook = true for a PDF")
	}
	if IsWorkbook(filepath.Join("..", "productdata", "testdata", "data.xlsx")) {
		t.Error("IsWorkbook = true for an unrelated workbook")
	}
}

func TestLoadWorkbook_ReadsOneOrderPerStoreThatAnswered(t *testing.T) {
	wb, err := LoadWorkbook(realWorkbook)
	if err != nil {
		t.Fatalf("LoadWorkbook: %v", err)
	}
	// Saved 2026-10-10T01:26:38Z, i.e. 08:26 on the 10th in Việt Nam.
	if wb.EntryDate != "10/10/2026" {
		t.Errorf("EntryDate = %q, want %q", wb.EntryDate, "10/10/2026")
	}

	type summary struct {
		code   string
		lines  int
		qty    float64
		amount float64
	}
	var got []summary
	for _, o := range wb.Orders {
		if !o.HasPrintedTotals {
			t.Errorf("store %s: no totals read from the supplier row", o.StoreCode)
		}
		got = append(got, summary{o.StoreCode, len(o.Lines), o.PrintedQty, o.PrintedAmount})
	}
	// Twelve other stores have a block but ordered nothing — including
	// 68014, whose order came separately as a PDF — and the two trailing
	// template blocks have no store code at all.
	want := []summary{
		{"79001", 4, 16, 2192326.4},
		{"79003", 4, 24, 3076330.4},
		{"79004", 4, 24, 3076330.4},
		{"79005", 4, 24, 3076330.4},
		{"79009", 4, 24, 3076330.4},
	}
	if len(got) != len(want) {
		t.Fatalf("orders = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].code != want[i].code || got[i].lines != want[i].lines || got[i].qty != want[i].qty ||
			got[i].amount < want[i].amount-0.01 || got[i].amount > want[i].amount+0.01 {
			t.Errorf("order %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	first := wb.Orders[0]
	if first.StoreName != "OsiFood Bình Hòa" {
		t.Errorf("StoreName = %q, want %q", first.StoreName, "OsiFood Bình Hòa")
	}
	if first.DeliveryAddress != "288 Phan Văn Trị, Phường Bình Lợi Trung, Quận Bình Thạnh - Quyên 0899868181" {
		t.Errorf("DeliveryAddress = %q", first.DeliveryAddress)
	}
	wantLine := WorkbookLine{
		Barcode: "8936156730244", Name: "Nước giặt xả Blue đậm đặc H. Thảo mộc 2.1 l",
		Price: 110500.5, PromoText: "Mua 1 tặng 1 (Quấn kèm)", PromoStart: "16/10/2026",
		Qty: 4, Total: 442002,
	}
	if first.Lines[0] != wantLine {
		t.Errorf("first line =\n  %+v\nwant\n  %+v", first.Lines[0], wantLine)
	}
}

func TestPromoStart(t *testing.T) {
	for cell, want := range map[string]string{
		"16/10 - 31/10/2026": "16/10/2026",
		"1/9-15/9/2026":      "01/09/2026",
		"":                   "",
		"đang cập nhật":      "",
	} {
		if got := promoStart(cell); got != want {
			t.Errorf("promoStart(%q) = %q, want %q", cell, got, want)
		}
	}
}

func TestPONumber(t *testing.T) {
	if got := PONumber("79001", "10/10/2026"); got != "79001-101026" {
		t.Errorf("PONumber = %q, want %q", got, "79001-101026")
	}
}
