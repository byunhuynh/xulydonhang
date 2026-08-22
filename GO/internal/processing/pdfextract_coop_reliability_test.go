package processing

import (
	"testing"

	"order-processor/internal/processing/vendor"
)

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

// TestPageRotation_ReadsInheritedRotateEntry covers pageRotation against
// two real fixtures: 103145712-00 (a real archived Coop PDF confirmed via
// direct inspection of its raw /Rotate entry to declare 90) and
// 103226908-00 (a real Coop PDF with no rotation, used elsewhere in this
// file — the overwhelmingly common case). Locks in that pageRotation
// reads the real value rather than defaulting to 0 unconditionally.
func TestPageRotation_ReadsInheritedRotateEntry(t *testing.T) {
	cases := []struct {
		file string
		want int
	}{
		{"coop/testdata/realpdfs/103145712-00.pdf", 90},
		{"coop/testdata/realpdfs/103226908-00.pdf", 0},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			file, r, err := pdfOpen(c.file)
			if err != nil {
				t.Skipf("fixture not available: %v", err)
			}
			defer file.Close()
			if r.NumPage() < 1 {
				t.Fatal("expected at least 1 page")
			}
			got := pageRotation(r.Page(1))
			if got != c.want {
				t.Errorf("pageRotation(%s) = %d, want %d", c.file, got, c.want)
			}
		})
	}
}

// TestExtractPageText_RotatedCoopPage_ReadsInCorrectOrder is the core
// regression test for the rotation-handling fix: 103145712-00.pdf has a
// real /Rotate 90 entry, and before this fix, reconstructLinesFromContent
// bucketed rows by raw (unrotated) Y — the WITHIN-row axis for this
// page, not the row axis — scrambling every row into fragments
// interleaved with fragments of every other row (confirmed: output like
// "KU Discounts -\nS" for what should read "SKU Discounts -" on one
// line). Asserts the PO number, a product description, and the totals
// label all appear as intact, correctly-ordered substrings.
func TestExtractPageText_RotatedCoopPage_ReadsInCorrectOrder(t *testing.T) {
	file, r, err := pdfOpen("coop/testdata/realpdfs/103145712-00.pdf")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	defer file.Close()
	if r.NumPage() < 1 {
		t.Fatal("expected at least 1 page")
	}
	page := r.Page(1)
	if rot := pageRotation(page); rot != 90 {
		t.Fatalf("fixture's own rotation = %d, want 90 (fixture assumption changed?)", rot)
	}
	text, err := extractPageText(page)
	if err != nil {
		t.Fatalf("extractPageText returned error: %v", err)
	}
	for _, want := range []string{
		"P/O Number:", "103145712-00",
		"3547985-0", "NG BLUE huong nuoc hoa 3kg",
		"Sub Total -",
	} {
		if !containsSubstring(text, want) {
			t.Errorf("extracted text missing intact substring %q — got: %.500s", want, text)
		}
	}
}

// TestExtractPageText_RotatedCoopPage_StreamOrderHandlesOppositeAxisSign
// covers the OTHER real /Rotate 90 shape found in this Coop generator's
// corpus: 103269932-00.pdf's reading order runs the OPPOSITE direction
// along the same raw axis as 103145712-00 (confirmed: "POM343"'s own raw
// Y values DEcrease in reading order here, vs increase there — the same
// /Rotate 90 value does not by itself predict this sign). An earlier,
// since-replaced version of this fix sorted by a transformed X value and
// had to guess that sign, trying a "mirrored" candidate and keeping
// whichever vendor.Identify accepted; reconstructRotatedPage instead
// orders each row by ORIGINAL CONTENT-STREAM index, which is correct
// regardless of which raw-axis direction happens to be reading order for
// a given file — no sign to guess. This test also covers a second, at
// the time separate bug this same fixture exposed: this generator can
// draw SPURIOUS padding-space runs positioned earlier in Y than the
// field's real content, which spatial X-sorting placed mid-token
// (producing "3 5 47984-5" instead of "3547984-5"); stream order places
// them in their correct position since they're emitted in the right
// place in the content stream itself.
func TestExtractPageText_RotatedCoopPage_StreamOrderHandlesOppositeAxisSign(t *testing.T) {
	file, r, err := pdfOpen("coop/testdata/realpdfs/103269932-00.pdf")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	defer file.Close()
	if r.NumPage() < 1 {
		t.Fatal("expected at least 1 page")
	}
	page := r.Page(1)
	if rot := pageRotation(page); rot != 90 {
		t.Fatalf("fixture's own rotation = %d, want 90 (fixture assumption changed?)", rot)
	}
	text, err := extractPageText(page)
	if err != nil {
		t.Fatalf("extractPageText returned error: %v", err)
	}
	if got := vendor.Identify(text); got != "Coop" {
		t.Fatalf("vendor.Identify(extracted text) = %q, want %q", got, "Coop")
	}
	for _, want := range []string{
		"P/O Number:", "103269932-00",
		"3547984-5", "NG BLUE huong thao moc 3kg",
		"Sub Total -",
	} {
		if !containsSubstring(text, want) {
			t.Errorf("extracted text missing intact substring %q (reversed reading order or spurious mid-token padding space) — got: %.500s", want, text)
		}
	}
}

// TestExtractPageText_RotatedCoopPage_GluedFieldsStillGetASpace covers a
// regression the stream-order fix (above) introduced and then had to fix
// again: without ANY gap detection, stream order alone glues together
// fields this generator draws with NO glyph — not even a space — between
// them despite a real visual gap (confirmed on 103145712-00: "...Time:
// 18:16:19Purchase OrderP/O Number:..." — "Time:"'s value and "Purchase
// Order" have zero characters, drawn or otherwise, between them). This
// broke coop.CountPOsOnPage's own regex on the glued "OrderP/O" sequence
// (the whole real-fixture-corpus golden test regressed before this gap
// detection was added back, scoped to STREAM-adjacent runs rather than
// spatially-sorted-adjacent ones). Locks in that a real space character
// still separates these two fields in the final output.
func TestExtractPageText_RotatedCoopPage_GluedFieldsStillGetASpace(t *testing.T) {
	file, r, err := pdfOpen("coop/testdata/realpdfs/103145712-00.pdf")
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
	if containsSubstring(text, "OrderP/O") {
		t.Error("extracted text glues \"Purchase Order\" and \"P/O Number\" together with no separator — gap detection between stream-adjacent runs did not fire")
	}
}

// TestExtractPageText_RotatedCoopPage_AdaptiveThresholdHandlesWideFont
// covers a THIRD real archived Coop /Rotate 90 shape found in this
// generator's corpus: 103231217-00.pdf has a uniform per-character
// advance of ~4.21pt (confirmed via raw per-run gap dumps — the same
// value repeats for hundreds of consecutive stream-adjacent runs on this
// page), well ABOVE buildTextFromRows' fixed gapThreshold (2.0pt,
// calibrated against two DIFFERENT real PDFs whose own normal advance is
// smaller). Using that fixed threshold for this page's own word-gap
// detection synthesized a space between every single character
// ("P O M 3 4 3" instead of "POM343"). rotatedPageGapThreshold computes
// a threshold specific to THIS page (a multiple of its own median
// stream-adjacent gap) instead of trusting one fixed value across every
// page this generator produces. Confirms both extremes stay correct on
// real data: normal same-word character spacing does NOT get a
// synthesized space, and a real field-to-field gap (P/O Number to
// Purchase Order, drawn with zero glyphs between them on this same
// page) still does.
func TestExtractPageText_RotatedCoopPage_AdaptiveThresholdHandlesWideFont(t *testing.T) {
	file, r, err := pdfOpen("coop/testdata/realpdfs/103231217-00.pdf")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	defer file.Close()
	if r.NumPage() < 1 {
		t.Fatal("expected at least 1 page")
	}
	page := r.Page(1)
	if rot := pageRotation(page); rot != 90 {
		t.Fatalf("fixture's own rotation = %d, want 90 (fixture assumption changed?)", rot)
	}
	// Test reconstructLinesFromContent's OWN return value directly, not
	// extractPageText's — extractPageText has a safety net (vendor.Identify
	// rejects an unreadable reconstruction and falls back to the original,
	// already-decent-enough-for-this-specific-file raw text), which would
	// silently mask a broken reconstruction here: this particular fixture's
	// raw GetPlainText text already keeps "POM343"/"103231217-00" intact on
	// its own, so asserting only against extractPageText's final output
	// cannot tell "reconstruction worked" apart from "reconstruction failed
	// and the safety net quietly covered for it" — confirmed by mutation
	// (temporarily shrinking gapThresholdMultiplier to 0.1 still passed an
	// earlier, weaker version of this test that asserted against
	// extractPageText instead of reconstructLinesFromContent).
	recon, ok := reconstructLinesFromContent(page)
	if !ok {
		t.Fatal("reconstructLinesFromContent returned ok=false")
	}
	if containsSubstring(recon, "P O M") {
		t.Error("reconstructed text spaces out individual characters (\"P O M\" instead of \"POM\") — the fixed gapThreshold was used instead of a per-page adaptive one")
	}
	for _, want := range []string{"POM343", "103231217-00", "3591312-3", "NX BLUE Huong Thanh Xuan T3.2L"} {
		if !containsSubstring(recon, want) {
			t.Errorf("reconstructed text missing intact substring %q — got: %.500s", want, recon)
		}
	}
	// Also confirm extractPageText's own output stays correct end-to-end.
	text, err := extractPageText(page)
	if err != nil {
		t.Fatalf("extractPageText returned error: %v", err)
	}
	if got := vendor.Identify(text); got != "Coop" {
		t.Errorf("vendor.Identify(extractPageText result) = %q, want %q", got, "Coop")
	}
}
