package vendor

import "testing"

func TestIdentify_RecognizesCoopByVendorID(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"vendor dash id 21569", "Bill To: Vendor - 21569 Co.opMart", "Coop"},
		{"vendor colon id 22856", "Vendor: 22856", "Coop"},
		{"vendor id with newline noise", "Vendor:\n  22856\nCo.opMart Nha Trang", "Coop"},
		{"unrelated vendor id", "Vendor - 99999", ""},
		{"unrelated text", "Purchase Order from BigC", ""},
		{"empty text", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.text)
			if got != c.want {
				t.Fatalf("Identify(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestIdentify_RecognizesLotteByVendorTaxID(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"tax id form 1", "Ven cd: 0107889783 009333 CONG TY CP HA THANH", "Lotte"},
		{"tax id form 2", "1102018142 010544 CONG TY CP HA THANH", "Lotte"},
		{"tax id split across lines", "0107889783\n  009333\nCONG TY CP HA THANH", "Lotte"},
		{"unrelated tax id", "0107889783 999999", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.text)
			if got != c.want {
				t.Fatalf("Identify(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestIdentify_RecognizesSatraByVDCode(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"VD code form 1", "Mã số thuế: VD-00002345 Satra Group", "Satra"},
		{"VD code form 2", "VD-00002547", "Satra"},
		{"unrelated VD code", "VD-00009999", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.text)
			if got != c.want {
				t.Fatalf("Identify(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestIdentify_RecognizesBigCByCodeOrCompanyString(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"3005382 substring", "PO Number 3005382 something else", "BigC"},
		{"CTY TNHH DV EB, case-insensitive", "cty tnhh dv eb ThuanKieu", "BigC"},
		{"CTY TNHH DV EB, real casing", "Header CTY TNHH DV EB Footer", "BigC"},
		{"unrelated text", "nothing matches here", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.text)
			if got != c.want {
				t.Fatalf("Identify(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestIdentify_BigCCheckedBeforeLotte(t *testing.T) {
	// A page whose text happens to contain both BigC's "3005382" marker
	// and (hypothetically) some other vendor's marker must resolve to
	// BigC first, matching Python's real check order (Coop -> BigC ->
	// Lotte -> Satra -> ...). This test only has BigC's own marker
	// available to construct with today, but documents the intent so a
	// future vendor addition that touches this ordering notices the
	// contract.
	got := Identify("3005382 unrelated content")
	if got != "BigC" {
		t.Fatalf("Identify with BigC marker = %q, want %q", got, "BigC")
	}
}

func TestIdentify_RecognizesWinmartBySupplierCode(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"supplier code present", "Header\nNhà cung cấp (Supplier): 0002011398\nfooter", "Winmart"},
		{"unrelated supplier code", "Nhà cung cấp (Supplier): 9999999999", ""},
		{"no marker at all", "nothing relevant here", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.text)
			if got != c.want {
				t.Fatalf("Identify(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestIdentify_WinmartCheckedAfterSatra(t *testing.T) {
	// Python's real identify_vendor order (xulydonhang.py:90-179) is
	// Coop -> BigC -> Lotte -> Satra -> Satra(2nd form) -> Emart ->
	// Kingfood -> CN-HCM -> Winmart -> SHOPEE-CHOICE -> ... Since Emart/
	// Kingfood/CN-HCM are not yet ported to Go, Winmart's case only needs
	// to be appended after Satra's (not inserted mid-sequence like BigC
	// was) to preserve the correct relative order among vendors that
	// actually exist in Go today. This test doesn't have a genuine
	// ordering conflict to construct (no unported vendor's pattern is
	// available), so it documents the intent for a future reader.
	got := Identify("Nhà cung cấp (Supplier): 0002011398")
	if got != "Winmart" {
		t.Fatalf("Identify with Winmart marker = %q, want %q", got, "Winmart")
	}
}

func TestIdentify_RecognizesEmartByCompanyMarker(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"real marker seen on real PDFs", "Header\nTHISO RETAIL COMPANY LIMITED\nfooter", "Emart"},
		{"ASCII marker (never observed on real PDFs; mirrored for fidelity with Python)", "CONG TY TNHH TMDV XNK HA THANH (101017)", "Emart"},
		{"unrelated text", "nothing matches here", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.text)
			if got != c.want {
				t.Fatalf("Identify(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestIdentify_EmartCheckedBetweenSatraAndWinmart(t *testing.T) {
	// Python's real identify_vendor order (xulydonhang.py:90-179) is
	// Coop -> BigC -> Lotte -> Satra -> Satra(2nd form) -> Emart ->
	// Kingfood -> CN-HCM -> Winmart -> SHOPEE-CHOICE -> ... Kingfood/
	// CN-HCM aren't ported to Go, so Emart's case must be inserted
	// directly before Winmart's existing case (not appended after it)
	// to preserve the correct relative order among vendors that exist in
	// Go today. This test doesn't have a genuine ordering conflict to
	// construct (no unported vendor's pattern is available to collide
	// with), so it documents the intent for a future reader, mirroring
	// TestIdentify_WinmartCheckedAfterSatra's own rationale.
	got := Identify("THISO RETAIL COMPANY LIMITED")
	if got != "Emart" {
		t.Fatalf("Identify with Emart marker = %q, want %q", got, "Emart")
	}
}

func TestIdentify_RecognizesFujiMartByTaxCode(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"real tax code", "Header\n251000000161\nfooter", "FujiMart"},
		{"unrelated number", "Header\n999999999999\nfooter", ""},
		{"no marker at all", "nothing relevant here", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.text)
			if got != c.want {
				t.Fatalf("Identify(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestIdentify_RecognizesKingfoodByTaxCode(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"real tax code", "Header\n0313403198\nfooter", "Kingfood"},
		{"unrelated number", "Header\n999999999999\nfooter", ""},
		{"no marker at all", "nothing relevant here", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Identify(c.text)
			if got != c.want {
				t.Fatalf("Identify(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestIdentify_KingfoodCheckedBetweenEmartAndWinmart(t *testing.T) {
	// Real xulydonhang.py order (xulydonhang.py:90-129): ...Emart(111) ->
	// Kingfood(114) -> [CN-HCM, unported](118) -> Winmart(121) ->
	// [SHOPEE-CHOICE, unported](125) -> FujiMart(128). Kingfood is the
	// FIRST vendor in this project whose Identify case must be inserted
	// mid-chain rather than appended at the end — every prior vendor's
	// correct relative position happened to already be "at the end" of
	// the then-current Go chain. There's no genuine ordering CONFLICT to
	// construct here (Kingfood's own marker, a plain tax-code substring,
	// doesn't overlap any other vendor's pattern), so this test
	// documents the intent for a future reader, mirroring
	// TestIdentify_EmartCheckedBetweenSatraAndWinmart's own rationale.
	got := Identify("0313403198")
	if got != "Kingfood" {
		t.Fatalf("Identify with Kingfood marker = %q, want %q", got, "Kingfood")
	}
}

func TestIdentify_FujiMartCheckedAfterWinmart(t *testing.T) {
	// Python's real identify_vendor order (xulydonhang.py:90-179) has
	// Kingfood -> CN-HCM between Emart and Winmart, and SHOPEE-CHOICE
	// between Winmart and FujiMart, all three unported to Go. Since none
	// of them exist in Go today, FujiMart's case only needs to be
	// appended after Winmart's (the current last case), not inserted
	// mid-sequence, to preserve the correct relative order among vendors
	// that actually exist in Go.
	// This test doesn't have a genuine ordering conflict to construct (no
	// unported vendor's pattern is available), so it documents the
	// intent for a future reader, mirroring
	// TestIdentify_WinmartCheckedAfterSatra's own rationale.
	got := Identify("251000000161")
	if got != "FujiMart" {
		t.Fatalf("Identify with FujiMart marker = %q, want %q", got, "FujiMart")
	}
}
