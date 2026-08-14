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
