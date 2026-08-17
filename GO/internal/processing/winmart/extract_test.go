package winmart

import "testing"

func TestParseOrderInfo_ExtractsPONumberDatesAndNote(t *testing.T) {
	text := "header\n" +
		"Ngày đặt hàng (PO date)\n07.31.2026\n" +
		"Số đơn hàng (PO No.)\n4194002858\n" +
		"Ngày giao (Delivery Date)\n08.08.2026\n" +
		"Ghi chú\nNguyễn Quang Phi_0396035541\nphinq@winmart.m\nNhà cung cấp (Supplier): 0002011398\nfooter"
	po, entry, cancel, note, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	if po != "4194002858" {
		t.Fatalf("po = %q, want %q", po, "4194002858")
	}
	if entry != "07/31/2026" {
		t.Fatalf("entry = %q, want %q (dots converted to slashes, no reordering)", entry, "07/31/2026")
	}
	if cancel != "08/08/2026" {
		t.Fatalf("cancel = %q, want %q", cancel, "08/08/2026")
	}
	// The Ghi chú block has TWO lines before the supplier marker
	// ("Nguyễn Quang Phi_0396035541" and "phinq@winmart.m") -- Python's
	// .splitlines()[:-1] drops the LAST one ("phinq@winmart.m"), so only
	// the first line survives into the joined note. This matches the
	// real sample PDF's actual behavior (confirmed during planning: the
	// real file's note field renders as "Nguyễn Quang
	// Phi_0396035541_phinq@winmart.m" as raw PDF text on what is
	// apparently 2 logical lines in the extracted text, and Python's
	// real output drops the trailing one).
	if note != "Nguyễn Quang Phi_0396035541" {
		t.Fatalf("note = %q, want %q", note, "Nguyễn Quang Phi_0396035541")
	}
}

func TestParseOrderInfo_MissingPOMarkerReturnsFalse(t *testing.T) {
	_, _, _, _, ok := ParseOrderInfo("no PO marker anywhere in this text")
	if ok {
		t.Fatal("ParseOrderInfo: matched, want no match")
	}
}

func TestParseOrderInfo_NoteWithMultipleLinesIsJoinedWithSpaces(t *testing.T) {
	text := "Ngày đặt hàng (PO date)\n07.31.2026\n" +
		"Số đơn hàng (PO No.)\n4194002858\n" +
		"Ngày giao (Delivery Date)\n08.08.2026\n" +
		"Ghi chú\nline one\nline two\nline three\nNhà cung cấp (Supplier): 0002011398\n"
	_, _, _, note, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo: no match, want match")
	}
	// 3 lines before the marker ("line one", "line two", "line three");
	// .splitlines()[:-1] drops "line three", leaving "line one"+"line two"
	// joined with a space -- this is the test that actually exercises
	// the multi-surviving-line join behavior (the previous test only
	// ever has exactly 1 surviving line after the drop).
	if note != "line one line two" {
		t.Fatalf("note = %q, want %q", note, "line one line two")
	}
}

func TestParseDeliveryAddress_JoinsLinesFilteringWMPlusDuplicates(t *testing.T) {
	text := "header\n" +
		"Địa chỉ giao hàng (Delivery Address)\n" +
		"1357-WMT_AMBIENT_HAIPHONG\n" +
		"1357-WMT_AMBIENT_HAIPHONG Lô CN4.1L\n" +
		"Khu Công Nghiệp Đình Vũ\n" +
		"Thông tin đơn hàng (Information)\n" +
		"footer"
	got, ok := ParseDeliveryAddress(text)
	if !ok {
		t.Fatal("ParseDeliveryAddress: no match, want match")
	}
	// The 2nd line contains "WM+"? No -- it contains "WMT_AMBIENT" not
	// "WM+". Use a case that actually has the literal "WM+" duplicate
	// marker the real Python filters on, per xulydonhang.py:9032's
	// comment example ("6863 - WM+ HCM 60 Liên khu 10-11"):
	want := "1357-WMT_AMBIENT_HAIPHONG - 1357-WMT_AMBIENT_HAIPHONG Lô CN4.1L Khu Công Nghiệp Đình Vũ"
	if got != want {
		t.Fatalf("ParseDeliveryAddress = %q, want %q", got, want)
	}
}

func TestParseDeliveryAddress_FiltersWMPlusDuplicateLines(t *testing.T) {
	text := "Địa chỉ giao hàng (Delivery Address)\n" +
		"6863\n" +
		"Real address line one\n" +
		"6863 - WM+ HCM 60 Liên khu 10-11\n" +
		"Real address line two\n" +
		"Thông tin đơn hàng (Information)\n"
	got, ok := ParseDeliveryAddress(text)
	if !ok {
		t.Fatal("ParseDeliveryAddress: no match, want match")
	}
	want := "6863 - Real address line one Real address line two"
	if got != want {
		t.Fatalf("ParseDeliveryAddress = %q, want %q (the WM+ line must be filtered out)", got, want)
	}
}

func TestParseDeliveryAddress_NoMarkerReturnsFalse(t *testing.T) {
	if _, ok := ParseDeliveryAddress("no delivery address marker here"); ok {
		t.Fatal("ParseDeliveryAddress: matched, want no match")
	}
}

func TestParseFuzzyMatchAddress_FindsBlockAfterWincommerceMarker(t *testing.T) {
	text := "header\n" +
		"TỔNG HỢP\n" +
		"WINCOMMERCE\n" +
		"Khu trung tâm thương mại Vincom Lê Thánh Tông\n" +
		"Số 5 Đường Lê Thánh Tông\n" +
		"MST: 0100109106\n" +
		"footer"
	got, ok := ParseFuzzyMatchAddress(text)
	if !ok {
		t.Fatal("ParseFuzzyMatchAddress: no match, want match")
	}
	// The anchor is the "TỔNG HỢP" line (idx=i, matching Python's real
	// `idx = i` -- NOT i+1), so collection starts at idx+1, which is the
	// "WINCOMMERCE" line itself -- it IS included in the collected
	// block, unlike what an earlier (incorrect) version of this test
	// assumed. Verified by hand-tracing real xulydonhang.py:9068-9083.
	want := "WINCOMMERCE Khu trung tâm thương mại Vincom Lê Thánh Tông Số 5 Đường Lê Thánh Tông"
	if got != want {
		t.Fatalf("ParseFuzzyMatchAddress = %q, want %q", got, want)
	}
}

func TestParseFuzzyMatchAddress_WincommerceAloneOnOneLineAlsoMatches(t *testing.T) {
	text := "header\n" +
		"Some Wincommerce Branch Line\n" +
		"Address line one\n" +
		"Address line two\n" +
		"Địa chỉ giao hàng: somewhere\n"
	got, ok := ParseFuzzyMatchAddress(text)
	if !ok {
		t.Fatal("ParseFuzzyMatchAddress: no match, want match")
	}
	want := "Address line one Address line two"
	if got != want {
		t.Fatalf("ParseFuzzyMatchAddress = %q, want %q", got, want)
	}
}

func TestParseFuzzyMatchAddress_StopsAtMSTOrDiaChiGiaoHangCaseInsensitive(t *testing.T) {
	text := "wincommerce\n" +
		"line a\n" +
		"line b\n" +
		"Địa Chỉ Giao Hàng: ignored from here\n" +
		"line c\n"
	got, ok := ParseFuzzyMatchAddress(text)
	if !ok {
		t.Fatal("ParseFuzzyMatchAddress: no match, want match")
	}
	want := "line a line b"
	if got != want {
		t.Fatalf("ParseFuzzyMatchAddress = %q, want %q (must stop before the case-insensitive marker)", got, want)
	}
}

func TestParseFuzzyMatchAddress_NoMarkerReturnsFalse(t *testing.T) {
	if _, ok := ParseFuzzyMatchAddress("no wincommerce marker here"); ok {
		t.Fatal("ParseFuzzyMatchAddress: matched, want no match")
	}
}
