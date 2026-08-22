package productdata

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
)

// TestParseFloat_HandlesVietnameseCommaDecimal is the core regression
// test for switching this Store's data source to a live Google Sheet.
// data.xlsx's raw excelize read has always used a dot decimal separator
// ("3.475" — see loadProducts' own comment, and
// TestParseFloat_HandlesDataXlsxDotDecimal below); a Google Sheets CSV
// export renders the SAME kind of value in Vietnamese locale, comma
// decimal, confirmed empirically against the real production sheet
// (14nGamUI8fhPiVPNm-Nf_VyHK-8Cx8rvG3KQFFT0spe0, SanPham tab, SKU
// TP31630: raw JSON "v" value 3.48, CSV-rendered "3,48"). Before this
// fix, parseFloat's original comma-strips-as-thousands-separator logic
// would have silently turned "3,48" into 348 — a 100x magnitude error,
// not just a rounding one.
func TestParseFloat_HandlesVietnameseCommaDecimal(t *testing.T) {
	if got := parseFloat("3,48"); got != 3.48 {
		t.Errorf("parseFloat(%q) = %v, want 3.48", "3,48", got)
	}
	if got := parseFloat("0,75"); got != 0.75 {
		t.Errorf("parseFloat(%q) = %v, want 0.75", "0,75", got)
	}
}

// TestParseFloat_HandlesDataXlsxDotDecimal is an inertness check: the
// pre-existing data.xlsx code path (dot decimal, occasional
// comma-thousands-separator) must parse exactly as it always has.
func TestParseFloat_HandlesDataXlsxDotDecimal(t *testing.T) {
	if got := parseFloat("3.475"); got != 3.475 {
		t.Errorf("parseFloat(%q) = %v, want 3.475", "3.475", got)
	}
	if got := parseFloat("1,234.5"); got != 1234.5 {
		t.Errorf("parseFloat(%q) = %v, want 1234.5 (comma-thousands, dot-decimal)", "1,234.5", got)
	}
}

// readCSVFixture parses a small real captured slice of the live
// production Google Sheet's own CSV export (see sheets_source.go's own
// doc comment for the spreadsheet id) — header row simplified to plain
// column labels (the real sheet's own row 1 is corrupted/unrelated
// leftover data, harmlessly skipped by loadCustomerRows/loadProducts
// regardless of its content, so isn't worth reproducing byte-for-byte),
// every DATA row preserved exactly as fetched, including the Vietnamese
// comma-decimal weight formatting this test exists to cover.
func readCSVFixture(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed opening %s: %v", path, err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("failed parsing %s as CSV: %v", path, err)
	}
	return rows
}

func TestLoadProducts_ParsesRealSheetsCSVSample(t *testing.T) {
	rows := readCSVFixture(t, filepath.Join("testdata", "sanpham_sheets_sample.csv"))
	products, skuMapping := loadProducts(rows)

	info, ok := products["TP31630"]
	if !ok {
		t.Fatal("products[TP31630] not found")
	}
	if info.Name != "Nước rửa chén Blue hương Chanh túi 3.2L (QC*4)" {
		t.Errorf("products[TP31630].Name = %q, want the real captured name", info.Name)
	}
	if info.WeightKg != 3.48 {
		t.Errorf("products[TP31630].WeightKg = %v, want 3.48 (Vietnamese comma-decimal \"3,48\" parsed correctly, not 348)", info.WeightKg)
	}
	if info.PackSize != 4 {
		t.Errorf("products[TP31630].PackSize = %v, want 4", info.PackSize)
	}

	// The barcode column ("Mã hàng BigC") must still resolve via
	// skuMapping, unaffected by the decimal-separator fix — this is a
	// pure-string column, never touched by parseFloat.
	if got := skuMapping["8936156731630"]; got != "TP31630" {
		t.Errorf("skuMapping[8936156731630] = %q, want %q", got, "TP31630")
	}
}

func TestLoadCustomerRows_ParsesRealSheetsCSVSample(t *testing.T) {
	rows := readCSVFixture(t, filepath.Join("testdata", "makh_sheets_sample.csv"))
	customers := loadCustomerRows(rows)

	if len(customers) != 3 {
		t.Fatalf("len(customers) = %d, want 3 (header row skipped)", len(customers))
	}
	if customers[0][0] != "SATRA" || customers[0][2] != "MN_MT_STFPD" {
		t.Errorf("customers[0] = %v, want system=SATRA, customer code=MN_MT_STFPD", customers[0])
	}
}

// TestLoadFromSheets_MissingGidReturnsClearError covers the one part of
// LoadFromSheets that's reachable without a live network call: a
// settings.ini missing the MAKH/SANPHAM keys this feature needs must
// fail with a clear, specific error identifying which key is missing —
// not a generic HTTP failure once the (never-reached) network call
// eventually times out.
func TestLoadFromSheets_MissingGidReturnsClearError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.ini")
	if err := os.WriteFile(path, []byte("[GoogleSheets]\n<gid>\nCOOP = 123\n</gid>\n"), 0o644); err != nil {
		t.Fatalf("failed writing settings file: %v", err)
	}

	_, err := LoadFromSheets(path, NewHTTPClient())
	if err == nil {
		t.Fatal("LoadFromSheets returned nil error for a settings.ini with no MAKH/SANPHAM gid")
	}
	if !containsSubstring(err.Error(), "MAKH") {
		t.Errorf("LoadFromSheets error = %q, want it to mention the missing MAKH key", err.Error())
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
