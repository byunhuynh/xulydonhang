package processing

import "testing"

// TestIsCoopGeneratedPage_DetectsSignatureAcrossSpuriousNewline covers the
// exact reason isCoopGeneratedPage strips ALL whitespace (not just
// whitespace runs) before matching: a genuine Coop page's own generator
// bug (see reconstructLinesFromContent's "\n"-glyph skip) can land a
// spurious newline anywhere, including mid-signature. A check that only
// collapses whitespace runs (like vendor.Identify's own normalization)
// would still see two separate words either side of that spurious break
// and could, depending on exactly where the break lands, fail to match a
// pattern expecting the words joined — this test locks in that
// whitespace of any kind, anywhere, still lets the signature match.
func TestIsCoopGeneratedPage_DetectsSignatureAcrossSpuriousNewline(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"clean signature", "POM343 Date: 1/01/26 JDA Software Version 7.7.0 (PDN_DC) Page: 1", true},
		{"signature split by a spurious newline mid-word", "POM343\nJDA Software\nVersion 7.7.0 (PDN_DC)\nPage: 1", true},
		{"signature split character-by-character", "J\nD\nA\n \nS\no\nf\nt\nw\na\nr\ne\n \nV\ne\nr\ns\ni\no\nn", true},
		{"unrelated JMart page", "Đơn vị : HỆ THỐNG SIÊU THỊ JMART\nPHIẾU ĐẶT HÀNG NHÀ CUNG CẤP", false},
		{"empty text", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isCoopGeneratedPage(c.text); got != c.want {
				t.Errorf("isCoopGeneratedPage(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

// TestExtractPageText_CoopSpuriousNewlineGlyph_KeepsPONumberIntact is the
// core regression test for this whole fix: 103226908-00.pdf is a real
// archived Coop PDF whose content stream embeds a literal U+000A
// (newline) as its own zero-width glyph run positioned mid-token, inside
// the P/O number's own digit sequence — confirmed by direct inspection
// of page.Content().Text (X≈75.6/104.4, Y=584.4, S="\n", W=0, sitting
// between the "1"/"0" and "-"/"0" runs of "103226908-00"). Before this
// fix, GetPlainText's raw output (trusted outright once past the old
// bare nl>=5 floor) contained this same embedded break, splitting the PO
// number across lines and making downstream regex-based extraction fail
// silently. Requires golden fixture PDFs to be present.
func TestExtractPageText_CoopSpuriousNewlineGlyph_KeepsPONumberIntact(t *testing.T) {
	file, r, err := pdfOpen("coop/testdata/realpdfs/103226908-00.pdf")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	defer file.Close()
	if r.NumPage() < 1 {
		t.Fatal("expected at least 1 page")
	}
	page := r.Page(1)
	text, err := extractPageText(page)
	if err != nil {
		t.Fatalf("extractPageText returned error: %v", err)
	}
	if !containsSubstring(text, "103226908-00") {
		t.Errorf("extracted text does not contain the intact PO number %q — got: %.300s", "103226908-00", text)
	}
}

// TestExtractPageText_NonCoopVendor_UnaffectedByLineCountCheck locks in
// the fix for a real regression this whole line-count-divergence
// mechanism caused before it was scoped to Coop pages only: JMart's real
// sample order (a legitimately different generator that wraps a single
// field's value across several GetPlainText lines — 86 raw newlines
// against only 32 true Y-distinct rows, a real diff of 54, easily past
// lineCountDivergenceTolerance) got switched to
// reconstructLinesFromContent, whose Y/X reading order does NOT
// reproduce this generator's true reading order, corrupting the
// extracted text into zero recognizable products. isCoopGeneratedPage's
// gate exists specifically to keep this vendor (and every vendor besides
// Coop) on the exact extraction path it had before this whole mechanism
// existed — this test asserts extractPageText's output for JMart's real
// sample is byte-identical to GetPlainText's own raw output.
func TestExtractPageText_NonCoopVendor_UnaffectedByLineCountCheck(t *testing.T) {
	file, r, err := pdfOpen("testdata/sample_jmart_order.pdf")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	defer file.Close()
	if r.NumPage() < 1 {
		t.Fatal("expected at least 1 page")
	}
	page := r.Page(1)
	raw, err := page.GetPlainText(nil)
	if err != nil {
		t.Fatalf("GetPlainText returned error: %v", err)
	}
	got, err := extractPageText(page)
	if err != nil {
		t.Fatalf("extractPageText returned error: %v", err)
	}
	if got != raw {
		t.Errorf("extractPageText changed a non-Coop page's text (JMart's own generator quirk was mistaken for the Coop line-count bug this check exists for)\nraw:  %.300s\ngot:  %.300s", raw, got)
	}
}

// TestExtractPageText_CoopExistingReconstructionCase_StillWorks is an
// inertness check for the "\n"-glyph skip added to
// reconstructLinesFromContent: 103157888-00.pdf is the PDF this whole
// fallback was originally calibrated against (see minPlausibleLines'
// doc comment — GetPlainText collapses it to a single line, W=0 for
// every glyph so word gaps come only from literal " " runs), and it was
// already passing before this change. Confirms skipping "\n"-glyph runs
// doesn't disturb a page that has none.
func TestExtractPageText_CoopExistingReconstructionCase_StillWorks(t *testing.T) {
	file, r, err := pdfOpen("coop/testdata/realpdfs/103157888-00.pdf")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	defer file.Close()
	if r.NumPage() < 1 {
		t.Fatal("expected at least 1 page")
	}
	page := r.Page(1)
	text, err := extractPageText(page)
	if err != nil {
		t.Fatalf("extractPageText returned error: %v", err)
	}
	if !containsSubstring(text, "P/O Number") && !containsSubstring(text, "P / O") {
		t.Errorf("extracted text doesn't look like a reconstructed Coop PO page — got: %.300s", text)
	}
	nl := 0
	for _, c := range text {
		if c == '\n' {
			nl++
		}
	}
	if nl < minPlausibleLines {
		t.Errorf("extracted text has only %d newlines, want at least %d (reconstruction should still produce multiple real lines)", nl, minPlausibleLines)
	}
}

// TestExtractPageText_NBSPNormalizedToRegularSpace covers a real subset
// of archived Coop PDFs (confirmed: 103256391-00, 103340115-00) that
// render field-label spacing using U+00A0 (NBSP) instead of a normal
// space — invisible visually, but Go's RE2 \s is ASCII-only and does not
// match it, silently breaking downstream regex extraction. Requires the
// real fixture PDF; if it's ever regenerated without this quirk, this
// test degrades gracefully to confirming no NBSP survives regardless.
func TestExtractPageText_NBSPNormalizedToRegularSpace(t *testing.T) {
	file, r, err := pdfOpen("coop/testdata/realpdfs/103256391-00.pdf")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	defer file.Close()
	if r.NumPage() < 1 {
		t.Fatal("expected at least 1 page")
	}
	page := r.Page(1)
	text, err := extractPageText(page)
	if err != nil {
		t.Fatalf("extractPageText returned error: %v", err)
	}
	const nbsp = rune(0x00a0)
	for i, ch := range text {
		if ch == nbsp {
			t.Fatalf("extracted text still contains NBSP (U+00A0) at byte offset %d — want it normalized to a regular space", i)
		}
	}
}
