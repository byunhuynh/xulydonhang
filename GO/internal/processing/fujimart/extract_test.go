package fujimart

import "testing"

func TestDecodeMojibake_AppliesKnownMappings(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"Tay Son", "FujiMart T©y S¬n", "FujiMart Tây Sơn"},
		{"Hoang Cau", "FujiMart Hoµng CÇu", "FujiMart Hoàng Cầu"},
		{"10 Tran Phu - Ha Dong", "FujiMart 10 TrÇn Phó-Hµ §«ng", "FujiMart 10 Trần Phú-Hà Đông"},
		{"Thuy Khue", "FujiMart Thôy Khuª", "FujiMart Thụy Khuê"},
		{"Le Duan", "FujiMart Lª DuÈn", "FujiMart Lê Duẩn"},
		{"Linh Dam", "FujiMart Linh §µm", "FujiMart Linh Đàm"},
		{"89 Lac Long Quan", "FujiMart 89 L¹c Long Qu©n", "FujiMart 89 Lạc Long Quân"},
		{"Ngoc Khanh", "FujiMart Ngäc Kh¸nh", "FujiMart Ngọc Khánh"},
		{"Huynh Thuc Khang", "FujiMart Huúnh Thóc Kh¸ng", "FujiMart Huỳnh Thúc Kháng"},
		{"Tan Mai", "FujiMart T©n Mai", "FujiMart Tân Mai"},
		{"Nguyen Co Thach", "FujiMart NguyÔn C¬ Th¹ch", "FujiMart Nguyễn Cơ Thạch"},
		{"no mojibake at all", "FujiMart Times City", "FujiMart Times City"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decodeFujimartMojibake(c.input)
			if got != c.want {
				t.Errorf("decodeFujimartMojibake(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

func TestDecodeMojibake_PassesThroughUnmappedRunes(t *testing.T) {
	// A future PDF from an unseen store branch could contain a
	// character not in the verified 15-entry table — must NOT error or
	// guess, just leave it unchanged so the gap is visible rather than
	// silently wrong.
	input := "FujiMart 東京" // arbitrary unmapped runes
	got := decodeFujimartMojibake(input)
	if got != input {
		t.Errorf("decodeFujimartMojibake(%q) = %q, want unchanged %q", input, got, input)
	}
}

func TestParseOrderInfo_ExtractsRealSampleFields(t *testing.T) {
	// Text shape mirrors this repo's OWN extractPageTexts output against
	// the real sample đơn hàng/08-2026/103001302608001342.pdf (confirmed
	// during planning by running the actual Go PDF pipeline directly —
	// NOT just PyMuPDF's shape), including the empty leading line Go's
	// extraction produces (PyMuPDF's doesn't) and the "Ngµy giao:"/value
	// split across two lines that PyMuPDF keeps on one line.
	text := "\n" +
		"Thµnh tiÒn\n" +
		"FujiMart T©y S¬n\n" +
		"Ghi chó:\n" +
		"STT\n" +
		"§iÖn tho¹i:\n" +
		"0862138966\n" +
		"C«ng ty CP Hµ Thµnh Long An 1\n" +
		"251000000161\n" +
		"NCC:\n" +
		"N¬i nhËn:\n" +
		"11031\n" +
		"18/08/2026\n" +
		"103001302608001342\n" +
		"14:43\n" +
		"Sè §¬n:\n" +
		"Ngµy ®Æt:\n" +
		"Page 1 of 1\n" +
		"Fax:\n" +
		"Ngµy giao:\n" +
		"20/08/2026\n" +
		"§Þa chØ:\n" +
		"1\n12.0\n1,695,264\nTUI\n141,272\nBLUE -N­íc giÆt\n8936156730879\n2006324377\n" +
		"VAT\n"

	poNumber, entryDate, cancelDate, storeInfo, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if poNumber != "103001302608001342" {
		t.Errorf("poNumber = %q, want %q", poNumber, "103001302608001342")
	}
	if entryDate != "18/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "18/08/2026")
	}
	if cancelDate != "20/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "20/08/2026")
	}
	if storeInfo != "11031 FujiMart Tây Sơn" {
		t.Errorf("storeInfo = %q, want %q", storeInfo, "11031 FujiMart Tây Sơn")
	}
}

func TestParseOrderInfo_MissingPONumberMarkerFailsCleanly(t *testing.T) {
	// No "Sè §¬n:" marker anywhere -> entry_date never resolves -> ok=false.
	// Mirrors Python's real crash risk here (entry_date would be an
	// undefined variable, NameError on first use) with a clean failure
	// instead, per this codebase's established policy.
	_, _, _, _, ok := ParseOrderInfo("nothing relevant here\nno markers at all\n")
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for text with no markers, want false")
	}
}

func TestParseOrderInfo_BackfillsEntryDateFromCancelDateAndUsesItForPONumber(t *testing.T) {
	// No "Sè §¬n:" marker anywhere, so the raw position-based entryDate
	// can ONLY come from the backfill path (cancelDate - 2 days) — never
	// from the i-3 line. cancelDate resolves normally via "Ngµy giao:".
	// This is the key regression test for the ordering bug: poNumber
	// must be looked up using the BACKFILLED entryDate ("18/08/2026"),
	// not "Không tìm thấy" and not whatever the raw (nonexistent) i-3
	// line would have been. Mirrors xulydonhang.py:8862-8887 where
	// po_number's re.search always runs against the FINAL entry_date.
	text := "18/08/2026\n" +
		"999888777\n" +
		"Ngµy giao: 20/08/2026\n"

	poNumber, entryDate, cancelDate, _, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if entryDate != "18/08/2026" {
		t.Errorf("entryDate = %q, want backfilled %q", entryDate, "18/08/2026")
	}
	if cancelDate != "20/08/2026" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "20/08/2026")
	}
	if poNumber != "999888777" {
		t.Errorf("poNumber = %q, want %q (must use the BACKFILLED entryDate for the lookup)", poNumber, "999888777")
	}
}

func TestParseOrderInfo_BackfillsCancelDateFromEntryDate(t *testing.T) {
	// No "Ngµy giao:" marker anywhere, so cancelDate can only come from
	// the backfill path (entryDate + 2 days). entryDate resolves
	// normally via the "Sè §¬n:" position offset.
	text := "18/08/2026\n" +
		"999888777\n" +
		"x\n" +
		"Sè §¬n:\n"

	poNumber, entryDate, cancelDate, _, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if entryDate != "18/08/2026" {
		t.Errorf("entryDate = %q, want %q", entryDate, "18/08/2026")
	}
	if cancelDate != "20/08/2026" {
		t.Errorf("cancelDate = %q, want backfilled %q", cancelDate, "20/08/2026")
	}
	if poNumber != "999888777" {
		t.Errorf("poNumber = %q, want %q", poNumber, "999888777")
	}
}

func TestParseOrderInfo_BothDatesUnresolvableFailsCleanly(t *testing.T) {
	// Distinct from TestParseOrderInfo_MissingPONumberMarkerFailsCleanly:
	// here the "Sè §¬n:" marker IS present (so this isn't just "no PO
	// marker at all"), but fewer than 3 lines precede it, so entryDate
	// still never resolves via the i-3 offset. No "Ngµy giao:" marker
	// exists either, so cancelDate can't backfill entryDate, and
	// entryDate can't backfill cancelDate. Both fallbacks are exhausted
	// -> ok=false.
	text := "x\n" +
		"y\n" +
		"Sè §¬n:\n"

	_, entryDate, cancelDate, _, ok := ParseOrderInfo(text)
	if ok {
		t.Fatal("ParseOrderInfo returned ok=true for unresolvable dates, want false")
	}
	if entryDate != "Không tìm thấy" {
		t.Errorf("entryDate = %q, want %q", entryDate, "Không tìm thấy")
	}
	if cancelDate != "Không tìm thấy" {
		t.Errorf("cancelDate = %q, want %q", cancelDate, "Không tìm thấy")
	}
}

func TestParseOrderInfo_LastSoDonMarkerWins(t *testing.T) {
	// The entryDate marker loop must NOT break on the first "Sè §¬n:"
	// match — Python's loop has no early exit either, so the LAST
	// matching line's i-3 offset wins. Two markers here: the first
	// resolves (wrongly, if a break existed) to "irrelevant0" (3 lines
	// before the FIRST marker); the second (real, last) marker resolves
	// to "18/08/2026", 3 lines before ITSELF.
	text := "irrelevant0\n" +
		"irrelevant1\n" +
		"irrelevant2\n" +
		"Sè §¬n:\n" +
		"filler\n" +
		"18/08/2026\n" +
		"999888777\n" +
		"filler2\n" +
		"Sè §¬n:\n" +
		"Ngµy giao: 20/08/2026\n"

	poNumber, entryDate, _, _, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false, want true")
	}
	if entryDate != "18/08/2026" {
		t.Errorf("entryDate = %q, want %q (last marker's i-3 line)", entryDate, "18/08/2026")
	}
	if poNumber != "999888777" {
		t.Errorf("poNumber = %q, want %q", poNumber, "999888777")
	}
}

func TestParseOrderInfo_MissingStoreInfoStillSucceeds(t *testing.T) {
	// Store info is best-effort (matches Python's tenstore defaulting to
	// "" when its OCR regex doesn't match) — must NOT gate ok.
	text := "\n" +
		"Thµnh tiÒn\n" +
		"no FujiMart line at all here\n" +
		"11031\n" +
		"18/08/2026\n" +
		"103001302608001342\n" +
		"14:43\n" +
		"Sè §¬n:\n" +
		"Ngµy giao: 20/08/2026\n"

	poNumber, _, cancelDate, storeInfo, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false for missing store info, want true")
	}
	if poNumber != "103001302608001342" {
		t.Errorf("poNumber = %q, want %q", poNumber, "103001302608001342")
	}
	if cancelDate != "20/08/2026" {
		t.Errorf("cancelDate = %q, want %q (same-line layout must also work)", cancelDate, "20/08/2026")
	}
	if storeInfo != "" {
		t.Errorf("storeInfo = %q, want empty", storeInfo)
	}
}
