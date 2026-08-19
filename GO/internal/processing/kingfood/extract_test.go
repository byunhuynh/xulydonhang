package kingfood

import "testing"

func TestParseOrderInfo_ExtractsRealSampleFields(t *testing.T) {
	// Text shape mirrors this repo's OWN extractPageTexts output against
	// a real sample Kingfood PDF (confirmed during planning by running
	// the actual Go PDF pipeline directly, then cross-checked against
	// PyMuPDF's output on the SAME file) — including the tab characters
	// Go's extraction inserts between words in multi-word labels, where
	// PyMuPDF inserts plain spaces. \t below is a literal tab character.
	text := "\n" +
		"Page\t1\t/\t2\n" +
		"PO\tNumber:\n" +
		"PO1002601888\n" +
		"Nơi\tgiao:\n" +
		"KHO\tSEEDLOG\n" +
		"Ngày\tGiao\tHàng\tDự\tKiến:\n" +
		"05-08-2026\n" +
		"Ngày\tGiao\tHàng\tNCC\tXác\n" +
		"Nhận:\n" +
		"05-08-2026\n" +
		"Ngày\tĐặt\tHàng:\n" +
		"03-08-2026\n" +
		"Quá\tcảnh:\n"

	poNumber, entryDate, cancelDate, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "PO1002601888" {
		t.Errorf("poNumber = %q, want %q", poNumber, "PO1002601888")
	}
	if entryDate != "03/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "03/08/2026")
	}
	if cancelDate != "05/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "05/08/2026")
	}
}

func TestParseOrderInfo_MissingPONumberMarkerFailsCleanly(t *testing.T) {
	// No "PO Number:" marker anywhere -> poNumber resolves empty ->
	// ok=false. Mirrors Python's real crash risk here (a downstream
	// datetime.strptime on an unresolved/garbage date string would raise
	// ValueError, uncaught) with a clean failure instead, per this
	// codebase's established policy — Kingfood has NO cross-validate/
	// fallback logic to backfill a missing date (unlike FujiMart/
	// Winmart/Emart), so a single missing marker is unrecoverable.
	_, _, _, ok := ParseOrderInfo("nothing relevant here\nno markers at all\n")
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no markers, want false")
	}
}

func TestParseOrderInfo_MalformedDateFailsCleanly(t *testing.T) {
	// A date that doesn't match dd-mm-yyyy should fail cleanly rather
	// than reproducing Python's real datetime.strptime crash.
	text := "PO\tNumber:\n" +
		"PO1002601888\n" +
		"Ngày\tGiao\tHàng\tNCC\tXác\n" +
		"Nhận:\n" +
		"not-a-date\n" +
		"Ngày\tĐặt\tHàng:\n" +
		"03-08-2026\n"
	_, _, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for a malformed cancelDate, want false")
	}
}

func TestNormalizeTabs_ReplacesTabsWithSpaces(t *testing.T) {
	got := normalizeTabs("PO\tNumber:\nKHO\tSEEDLOG")
	want := "PO Number:\nKHO SEEDLOG"
	if got != want {
		t.Errorf("normalizeTabs(...) = %q, want %q", got, want)
	}
}

func TestExtractProducts_ParsesRealSampleSingleProduct(t *testing.T) {
	// Exact shape of this repo's OWN extractPageTexts output for the
	// product-table region of a real single-product Kingfood PDF,
	// confirmed during planning by running the actual Go PDF pipeline —
	// including tab characters within the multi-word "Khu vực"/
	// "TỔNG CỘNG" markers and the product name line.
	text := "%\tHSD\n" +
		"Khu\tvực\n" +
		"1\n" +
		"8936156732620\n" +
		"BLUE\t-\tVIÊN\tGIẶT\tXẢ\tPHẤN\tHỒNG\tTÚI\n" +
		"30\tVIÊN\n" +
		"TÚI\n" +
		"300\n" +
		"12\n" +
		"25\tThùng\n" +
		"102.143\n" +
		"27%\n" +
		"0%\n" +
		"30%\n" +
		"52.195,073\n" +
		"8%\n" +
		"1.252.682\n" +
		"15.658.522\n" +
		"16.911.204\n" +
		"80%\n" +
		"Nhiệt\tđộ\n" +
		"phòng\n" +
		"TỔNG\tCỘNG\n" +
		"300\n"

	products := ExtractProducts(text)
	if len(products) != 1 {
		t.Fatalf("len(products) = %d, want 1", len(products))
	}
	want := Product{Barcode: "8936156732620", OUQty: "300", TotalPrice: "52.195,073"}
	if products[0] != want {
		t.Errorf("products[0] = %+v, want %+v", products[0], want)
	}
}

func TestExtractProducts_ParsesRealSampleTwoProducts(t *testing.T) {
	// Confirmed during planning: a real Kingfood PDF (PO1002586301) has
	// 2 distinct products in one order — this must loop correctly, not
	// just handle the single-product case.
	text := "Khu\tvực\n" +
		"1\n" +
		"8936156730992\n" +
		"BLUE\t-\tNƯỚC\tGIẶT\tXẢ\tĐẬM\tĐẶC\tTÚI\n" +
		"3.6\tL\n" +
		"TÚI\n" +
		"120\n" +
		"10\n" +
		"12\tThùng\n" +
		"85.000\n" +
		"20%\n" +
		"0%\n" +
		"10%\n" +
		"61.200,000\n" +
		"8%\n" +
		"500.000\n" +
		"6.500.000\n" +
		"7.000.000\n" +
		"90%\n" +
		"Nhiệt\tđộ\n" +
		"phòng\n" +
		"2\n" +
		"8936156732620\n" +
		"BLUE\t-\tVIÊN\tGIẶT\tXẢ\tPHẤN\tHỒNG\tTÚI\n" +
		"30\tVIÊN\n" +
		"TÚI\n" +
		"300\n" +
		"12\n" +
		"25\tThùng\n" +
		"102.143\n" +
		"27%\n" +
		"0%\n" +
		"30%\n" +
		"52.195,073\n" +
		"8%\n" +
		"1.252.682\n" +
		"15.658.522\n" +
		"16.911.204\n" +
		"80%\n" +
		"Nhiệt\tđộ\n" +
		"phòng\n" +
		"TỔNG\tCỘNG\n" +
		"420\n"

	products := ExtractProducts(text)
	if len(products) != 2 {
		t.Fatalf("len(products) = %d, want 2", len(products))
	}
	if products[0].Barcode != "8936156730992" {
		t.Errorf("products[0].Barcode = %q, want %q", products[0].Barcode, "8936156730992")
	}
	if products[1].Barcode != "8936156732620" {
		t.Errorf("products[1].Barcode = %q, want %q", products[1].Barcode, "8936156732620")
	}
}

func TestExtractProducts_NoTableMarkerReturnsEmpty(t *testing.T) {
	products := ExtractProducts("no khu vuc marker or tong cong anywhere in this text")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}

func TestExtractProducts_NoEndMarkerReturnsEmpty(t *testing.T) {
	products := ExtractProducts("Khu\tvực\n1\n8936156732620\nsome text with no end marker\n")
	if products != nil {
		t.Errorf("ExtractProducts = %v, want nil", products)
	}
}
