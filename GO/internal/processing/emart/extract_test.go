package emart

import "testing"

func TestParseOrderInfo_ExtractsPONumberDatesAndStore(t *testing.T) {
	text := "Some Header\n" +
		"PO No.\n" +
		": 4501866956\n" +
		"Order By / Date\n" +
		": 03.08.2026 09:15 NGUYEN HOANG NHAT NAM\n" +
		"Delivery Date\n" +
		": 05.08.2026 00:00\n" +
		"Delivery to : EMART GO VAP   366 Phan Văn Trị, P.5, Q. Gò Vấp, TP.HCM\n" +
		"more text"

	poNumber, entryDate, cancelDate, storeName, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "4501866956" {
		t.Errorf("poNumber = %q, want %q", poNumber, "4501866956")
	}
	if entryDate != "03.08.2026" {
		// [:10] truncation happens BEFORE the "." -> "/" replace in
		// Python (entry_date[:10].replace(".", "/")) — 10 characters of
		// "03.08.2026 09:15 ..." is "03.08.2026" (dots not yet
		// replaced at truncation time, this assertion checks the raw
		// truncated form before replace to make the ordering explicit;
		// the real returned value has "/" not ".", see below).
	}
	if entryDate != "03/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "03/08/2026")
	}
	if cancelDate != "05/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "05/08/2026")
	}
	if storeName != "EMART GO VAP" {
		t.Errorf("storeName = %q, want %q (split on 3 spaces, address discarded)", storeName, "EMART GO VAP")
	}
}

func TestParseOrderInfo_NoColonPrefix(t *testing.T) {
	// Python's pattern's ":? ?" makes the colon-and-space fully optional
	// — some real PDFs may render the value directly after the marker
	// line with no ":" at all.
	text := "PO No.\n4501866958\nOrder By / Date\n01.08.2026\nDelivery Date\n03.08.2026\nDelivery to : EMART SALA   1 Đường ABC"
	poNumber, entryDate, cancelDate, storeName, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "4501866958" {
		t.Errorf("poNumber = %q, want %q", poNumber, "4501866958")
	}
	if entryDate != "01/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "01/08/2026")
	}
	if cancelDate != "03/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "03/08/2026")
	}
	if storeName != "EMART SALA" {
		t.Errorf("storeName = %q, want %q", storeName, "EMART SALA")
	}
}

func TestParseOrderInfo_MissingPONumberFailsCleanly(t *testing.T) {
	// Python would carry a None po_number into several downstream string
	// operations (e.g. STT_donhang_str = f"-{po_number}" -> "-None"),
	// silently corrupting the order number instead of failing. This port
	// fails cleanly instead, per this codebase's established
	// no-bug-for-bug-crash-parity policy.
	_, _, _, _, ok := ParseOrderInfo("nothing relevant here")
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no PO No. marker, want false")
	}
}

func TestParseOrderInfo_MissingDateFailsCleanly(t *testing.T) {
	text := "PO No.\n4501866956\nno dates here at all"
	_, _, _, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no date markers, want false")
	}
}

func TestParseOrderInfo_MissingStoreNameStillSucceeds(t *testing.T) {
	// Python tolerates a missing "Delivery to :" line (prints a message,
	// leaves tenstore as None, still calls write_to_dondathang_emart) —
	// the order still gets written, just with a blank store, and the
	// in-app status table shows a warning. storeName="" mirrors that
	// resilience; ok stays true.
	text := "PO No.\n4501866956\nOrder By / Date\n01.08.2026\nDelivery Date\n03.08.2026\nno delivery-to line here"
	poNumber, _, _, storeName, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false for a missing store name, want true (store is best-effort)")
	}
	if poNumber != "4501866956" {
		t.Errorf("poNumber = %q, want %q", poNumber, "4501866956")
	}
	if storeName != "" {
		t.Errorf("storeName = %q, want empty", storeName)
	}
}

func TestExtractProducts_ParsesTableRowsAndDropsZeroPrice(t *testing.T) {
	text := "Article Code Unit Barcode Description PO Unit Qty. in Box PO Qty. Pur. Price(-VAT) Amount(-VAT) Free PO\n" +
		"1234567\n" +
		"893615673120\n" +
		"Nước giặt Blue kháng khuẩn\n" +
		"EA\n" +
		"4\n" +
		"48\n" +
		"26.950\n" +
		"1.293.600\n" +
		"0\n" +
		"7654321\n" +
		"893615673999\n" +
		"Free sample item\n" +
		"EA\n" +
		"1\n" +
		"2\n" +
		"0\n" +
		"0\n" +
		"1\n" +
		"Total Amount(without VAT) :\n" +
		"trailing text not part of the table"

	products := ExtractProducts(text)
	if len(products) != 1 {
		t.Fatalf("len(products) = %d, want 1 (zero-price second item must be dropped)", len(products))
	}
	got := products[0]
	if got.Barcode != "893615673120" {
		t.Errorf("Barcode = %q, want %q", got.Barcode, "893615673120")
	}
	if got.OUQty != 48 {
		t.Errorf("OUQty = %d, want %d (PO Qty. column, not Qty. in Box)", got.OUQty, 48)
	}
	if got.UnitPrice != "26950" {
		t.Errorf("UnitPrice = %q, want %q (dot-stripped per-unit price, NOT the Amount column)", got.UnitPrice, "26950")
	}
}

func TestExtractProducts_NoTableMarkerReturnsEmpty(t *testing.T) {
	// Python's real code would crash here (calling .group(1) on the
	// re.search's None result) — this port returns an empty slice
	// instead, per this codebase's established clean-failure policy; the
	// caller (processEmartSegment) treats an empty product list as an
	// order-level error.
	products := ExtractProducts("no Article Code marker anywhere in this text")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}

func TestExtractProducts_NoMatchingRowsReturnsEmpty(t *testing.T) {
	products := ExtractProducts("Article Code\nnothing shaped like a product row\nTotal Amount(without VAT) :")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}
