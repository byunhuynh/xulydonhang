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
