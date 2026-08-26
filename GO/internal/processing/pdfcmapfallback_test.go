package processing

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"

	"order-processor/internal/processing/vendor"
)

// --- isGarbledText -----------------------------------------------------

func TestIsGarbledText_MostlyValidTextDoesNotTrigger(t *testing.T) {
	text := "Sè §¬n: 106003302608000751\nNgµy ®Æt: 10/08/2026 14:39\nFujiMart Ngäc Kh¸nh\n"
	if isGarbledText(text) {
		t.Errorf("isGarbledText(%q) = true, want false (this text is entirely valid, decoded mojibake — not a decode failure)", text)
	}
}

func TestIsGarbledText_MostlyReplacementCharTriggers(t *testing.T) {
	text := strings.Repeat("�", 40) + "\n" + strings.Repeat("�", 40)
	if !isGarbledText(text) {
		t.Errorf("isGarbledText(%q) = false, want true (text is ~100%% U+FFFD, the exact signature confirmed on real broken FujiMart PDFs)", text)
	}
}

func TestIsGarbledText_SmallSampleFloorAvoidsFalsePositive(t *testing.T) {
	// Only a handful of non-whitespace runes, ALL of them U+FFFD — below
	// the 20-rune floor, so this must NOT trigger even though the ratio
	// alone would suggest 100% garbled. Guards against a tiny/near-empty
	// page (a couple of legitimately-unmappable glyphs) being
	// misclassified as a decode failure.
	text := "���"
	if isGarbledText(text) {
		t.Errorf("isGarbledText(%q) = true, want false (sample size %d is below the 20-rune floor)", text, len([]rune(text)))
	}
}

func TestIsGarbledText_MinorityReplacementCharDoesNotTrigger(t *testing.T) {
	// A realistic case: mostly valid ASCII/Vietnamese-mojibake text with
	// only a couple of genuinely-unmappable glyphs mixed in — must not
	// be classified as a wholesale decode failure.
	text := strings.Repeat("A", 30) + "��"
	if isGarbledText(text) {
		t.Errorf("isGarbledText(%q) = true, want false (only 2/32 non-whitespace runes are U+FFFD)", text)
	}
}

// --- synthetic PDF builder ----------------------------------------------

// synthObj is one indirect object ("N 0 obj ... endobj") to assemble
// into a synthetic PDF via buildSynthPDF.
type synthObj struct {
	num  int
	body string
}

// streamObjBody formats dict (the object dictionary's inner key/value
// pairs, WITHOUT the enclosing "<<"/">>") plus data into a complete
// stream object body, computing /Length from the actual byte length of
// data rather than a hand-counted literal — avoiding the exact class of
// off-by-one bug a hardcoded /Length would risk.
func streamObjBody(dict, data string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(data), data)
}

// buildSynthPDF assembles a minimal, syntactically valid PDF from the
// given objects, computing each object's real byte offset and emitting
// a correct xref table + trailer automatically — deliberately not
// hand-computing byte offsets (see TestPdfOpen_TrimsGarbageAfterEOF's
// own comment on why a real xref table, not just a nominal PDF
// skeleton, is required for pdf.NewReader to resolve objects at all).
func buildSynthPDF(objs []synthObj, rootNum int) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")

	maxNum := 0
	for _, o := range objs {
		if o.num > maxNum {
			maxNum = o.num
		}
	}
	offsetByNum := make(map[int]int, len(objs))
	for _, o := range objs {
		offsetByNum[o.num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", o.num, o.body)
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", maxNum+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= maxNum; i++ {
		off, ok := offsetByNum[i]
		if !ok {
			buf.WriteString("0000000000 00000 f \n")
			continue
		}
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d 0 R >>\n", maxNum+1, rootNum)
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return buf.Bytes()
}

// buildFontTestPage assembles a single-page synthetic PDF with one font
// resource named "F1" (object 10) whose body is fontDict, plus whatever
// extra objects (e.g. a ToUnicode stream) fontDict references. Returns
// the parsed pdf.Font ready to pass directly to decodeSimpleFontCmap.
func buildFontTestPage(t *testing.T, fontDict string, extraObjs ...synthObj) pdf.Font {
	t.Helper()
	objs := []synthObj{
		{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 10 0 R >> >> /Contents 4 0 R >>"},
		{4, streamObjBody("", "")},
		{10, fontDict},
	}
	objs = append(objs, extraObjs...)

	data := buildSynthPDF(objs, 1)
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("failed constructing synthetic test PDF: %v\n--- PDF bytes ---\n%s", err, data)
	}
	page := r.Page(1)
	if page.V.IsNull() {
		t.Fatalf("synthetic test PDF's page 1 did not resolve\n--- PDF bytes ---\n%s", data)
	}
	return page.Font("F1")
}

// --- decodeSimpleFontCmap: bail cases -----------------------------------

func TestDecodeSimpleFontCmap_BailsOnType0Font(t *testing.T) {
	// Deliberately gives this font a VALID, well-formed single-byte
	// ToUnicode stream (the same shape
	// TestDecodeSimpleFontCmap_BuildsTableFromValidSingleByteBfrange
	// proves decodeSimpleFontCmap can successfully parse) — otherwise
	// this test would bail 3 checks later at the missing-/ToUnicode
	// check without ever reaching the Type0 guard it's named after,
	// making it green-by-construction rather than a real proof. With a
	// real ToUnicode present, only the Type0 check (which runs first)
	// can be what stops this from succeeding.
	cmapData := "1 beginbfrange\n<00><00><0041>\nendbfrange"
	font := buildFontTestPage(t,
		"<< /Type /Font /Subtype /Type0 /BaseFont /F1 /ToUnicode 20 0 R >>",
		synthObj{20, streamObjBody("", cmapData)},
	)
	if table, ok := decodeSimpleFontCmap(font); ok {
		t.Errorf("decodeSimpleFontCmap(Type0 font with a valid ToUnicode) = (%v, true), want ok=false — Type0/Identity-H FujiMart PDFs already decode correctly via the library's own normal path and must never be touched by this fallback", table)
	}
}

func TestDecodeSimpleFontCmap_BailsOnNonNullEncoding(t *testing.T) {
	// Same reasoning as TestDecodeSimpleFontCmap_BailsOnType0Font above:
	// includes a VALID ToUnicode stream so this test is actually stopped
	// by the non-Null-/Encoding check (which runs before the ToUnicode
	// check), not by the unrelated missing-/ToUnicode check.
	cmapData := "1 beginbfrange\n<00><00><0041>\nendbfrange"
	font := buildFontTestPage(t,
		"<< /Type /Font /Subtype /TrueType /BaseFont /F1 /Encoding /WinAnsiEncoding /ToUnicode 20 0 R >>",
		synthObj{20, streamObjBody("", cmapData)},
	)
	if table, ok := decodeSimpleFontCmap(font); ok {
		t.Errorf("decodeSimpleFontCmap(font with /Encoding /WinAnsiEncoding and a valid ToUnicode) = (%v, true), want ok=false — a font with a real, non-Null /Encoding already decodes correctly and must not be touched", table)
	}
}

func TestDecodeSimpleFontCmap_BailsOnNoToUnicodeStream(t *testing.T) {
	font := buildFontTestPage(t, "<< /Type /Font /Subtype /TrueType /BaseFont /F1 >>")
	if table, ok := decodeSimpleFontCmap(font); ok {
		t.Errorf("decodeSimpleFontCmap(font with no /ToUnicode) = (%v, true), want ok=false", table)
	}
}

func TestDecodeSimpleFontCmap_BailsOnTwoByteCodeShape(t *testing.T) {
	// The malformed real shape's INVERSE: here the bfrange entries
	// themselves genuinely use 2-byte (4-hex-digit) codes, which is a
	// real, legal CMap shape this narrow single-byte-only parser is not
	// verified against — must bail, not silently misinterpret half the
	// bytes.
	cmapData := "1 beginbfrange\n<0000><0000><0020>\nendbfrange"
	font := buildFontTestPage(t,
		"<< /Type /Font /Subtype /TrueType /BaseFont /F1 /ToUnicode 20 0 R >>",
		synthObj{20, streamObjBody("", cmapData)},
	)
	if table, ok := decodeSimpleFontCmap(font); ok {
		t.Errorf("decodeSimpleFontCmap(2-byte-code bfrange) = (%v, true), want ok=false", table)
	}
}

func TestDecodeSimpleFontCmap_BailsOnDestinationArrayShape(t *testing.T) {
	// "<lo><hi>[<d1><d2>]" — a destination-ARRAY bfrange entry (PDF spec
	// §9.10.3): each source code in [lo,hi] maps to its OWN independent
	// destination, not a linear offset from one base value. Without the
	// explicit "[" check in decodeSimpleFontCmap, hexTokenPattern's flat
	// token extraction would see 4 tokens here (00, 01, 0020, 0041) and
	// could accidentally group the first 3 into a syntactically-valid-
	// looking (but semantically WRONG) single-string-destination entry —
	// this test exists specifically to prove that accident can't happen.
	cmapData := "1 beginbfrange\n<00><01>[<0020><0041>]\nendbfrange"
	font := buildFontTestPage(t,
		"<< /Type /Font /Subtype /TrueType /BaseFont /F1 /ToUnicode 20 0 R >>",
		synthObj{20, streamObjBody("", cmapData)},
	)
	if table, ok := decodeSimpleFontCmap(font); ok {
		t.Errorf("decodeSimpleFontCmap(destination-array bfrange) = (%v, true), want ok=false", table)
	}
}

func TestDecodeSimpleFontCmap_BailsOnEmptyCmap(t *testing.T) {
	font := buildFontTestPage(t,
		"<< /Type /Font /Subtype /TrueType /BaseFont /F1 /ToUnicode 20 0 R >>",
		synthObj{20, streamObjBody("", "not a cmap at all, no bfrange or bfchar blocks")},
	)
	if table, ok := decodeSimpleFontCmap(font); ok {
		t.Errorf("decodeSimpleFontCmap(no bfrange/bfchar blocks) = (%v, true), want ok=false", table)
	}
}

// --- decodeSimpleFontCmap: positive case ---------------------------------

func TestDecodeSimpleFontCmap_BuildsTableFromValidSingleByteBfrange(t *testing.T) {
	cmapData := "1 beginbfrange\n<00><00><0041>\nendbfrange"
	font := buildFontTestPage(t,
		"<< /Type /Font /Subtype /TrueType /BaseFont /F1 /ToUnicode 20 0 R >>",
		synthObj{20, streamObjBody("", cmapData)},
	)
	table, ok := decodeSimpleFontCmap(font)
	if !ok {
		t.Fatalf("decodeSimpleFontCmap(valid single-byte bfrange) returned ok=false, want true — this proves the synthetic-PDF construction itself is sound, not just that bad shapes get rejected")
	}
	if got, want := table[0x00], rune('A'); got != want {
		t.Errorf("table[0x00] = %q, want %q", got, want)
	}
}

func TestDecodeSimpleFontCmap_BuildsTableFromBfchar(t *testing.T) {
	cmapData := "1 beginbfchar\n<05><0042>\nendbfchar"
	font := buildFontTestPage(t,
		"<< /Type /Font /Subtype /TrueType /BaseFont /F1 /ToUnicode 20 0 R >>",
		synthObj{20, streamObjBody("", cmapData)},
	)
	table, ok := decodeSimpleFontCmap(font)
	if !ok {
		t.Fatalf("decodeSimpleFontCmap(valid bfchar) returned ok=false, want true")
	}
	if got, want := table[0x05], rune('B'); got != want {
		t.Errorf("table[0x05] = %q, want %q", got, want)
	}
}

// --- extractPageTextViaCorrectedCmap: recover() guard ---------------------

func TestExtractPageTextViaCorrectedCmap_RecoversFromUnsupportedFilterPanic(t *testing.T) {
	// A ToUnicode stream declaring an unrecognized /Filter makes the
	// vendored library's own Value.Reader() panic outright
	// (read.go:818's "unsupported filter" / applyFilter's "unknown
	// filter" paths) — confirmed by direct inspection of the vendored
	// module. decodeSimpleFontCmap calls Reader() for EVERY font
	// resource on the page (not just ones a Tf operator actually
	// selects), so this is reachable even for an unused font resource.
	// This must degrade to a clean ok=false, never crash the caller.
	objs := []synthObj{
		{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 10 0 R >> >> /Contents 4 0 R >>"},
		{4, streamObjBody("", "")},
		{10, "<< /Type /Font /Subtype /TrueType /BaseFont /F1 /ToUnicode 20 0 R >>"},
		{20, streamObjBody("/Filter /BogusFilter", "ABCD")},
	}
	data := buildSynthPDF(objs, 1)
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("failed constructing synthetic test PDF: %v", err)
	}
	page := r.Page(1)
	if page.V.IsNull() {
		t.Fatalf("synthetic test PDF's page 1 did not resolve")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("extractPageTextViaCorrectedCmap panicked instead of returning ok=false: %v", r)
		}
	}()
	text, ok := extractPageTextViaCorrectedCmap(page)
	if ok {
		t.Errorf("extractPageTextViaCorrectedCmap(page with unsupported-filter ToUnicode stream) = (%q, true), want ok=false", text)
	}
}

// --- extractPageTextViaCorrectedCmap: end-to-end inertness -----------------

func TestExtractPageTextViaCorrectedCmap_BailsWhenAnyFontIsType0(t *testing.T) {
	// A page mixing a normal simple font (with a valid, decodable
	// ToUnicode CMap) alongside one Type0/CID font must bail entirely —
	// never return a partially-corrected result for only some of the
	// page's fonts.
	objs := []synthObj{
		{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 10 0 R /F2 11 0 R >> >> /Contents 4 0 R >>"},
		{4, streamObjBody("", "")},
		{10, "<< /Type /Font /Subtype /TrueType /BaseFont /F1 /ToUnicode 20 0 R >>"},
		{20, streamObjBody("", "1 beginbfrange\n<00><00><0041>\nendbfrange")},
		{11, "<< /Type /Font /Subtype /Type0 /BaseFont /F2 >>"},
	}
	data := buildSynthPDF(objs, 1)
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("failed constructing synthetic test PDF: %v", err)
	}
	page := r.Page(1)
	if page.V.IsNull() {
		t.Fatalf("synthetic test PDF's page 1 did not resolve")
	}

	text, ok := extractPageTextViaCorrectedCmap(page)
	if ok {
		t.Errorf("extractPageTextViaCorrectedCmap(page with a Type0 font mixed in) = (%q, true), want ok=false", text)
	}
}

func TestExtractPageTexts_DecodesDifferencesEncodedSubsetFontViaToUnicode(t *testing.T) {
	// Real archived Coop PDF whose only font is a subset TrueType
	// (QSWINA+LucidaConsole) carrying BOTH an /Encoding dictionary with a
	// 69-entry /Differences array AND a well-formed 1-byte /ToUnicode
	// CMap. The vendored library picks the /Differences path whenever
	// /Encoding is a dictionary and never looks at /ToUnicode; the glyph
	// names in this subset's Differences array are not in its
	// name->rune table, so every character fell through to its own raw
	// code byte and the page extracted as C0 control-character soup
	// ("\x01\x02\x03..."), which vendor.Identify cannot classify.
	path := filepath.Join("coop", "testdata", "realpdfs", "103346096-00.pdf")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Skipf("real sample PDF not found at %s: %v", path, statErr)
	}
	pages, _, err := extractPageTexts(path)
	if err != nil {
		t.Fatalf("extractPageTexts: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no pages extracted")
	}
	if got := vendor.Identify(pages[0]); got != "Coop" {
		t.Fatalf("vendor.Identify = %q, want %q\nextracted text:\n%q", got, "Coop", pages[0])
	}
}

func TestExtractPageTexts_DecodesCodesBelowADeclaredCodespaceRange(t *testing.T) {
	// Real archived Maxidi delivery note. Its Arial subset font's
	// ToUnicode CMap is self-contradictory: it declares
	// "1 begincodespacerange <52> <f9> endcodespacerange" while its own
	// 75 bfrange entries map codes far BELOW 0x52 — every digit
	// (<30>..<39>), the space (<20>), the comma, the slash and most
	// uppercase letters. The vendored library honours the declared range
	// and emits U+FFFD for each of those codes, so the page decodes with
	// its prose intact but every NUMBER destroyed: PO number, both
	// dates, barcode, PLU and all quantities. isGarbledText alone cannot
	// catch this — only ~40% of the page's runes are affected, under its
	// >50% threshold — which is why a contradictory codespacerange is
	// its own, separate trigger for the corrected-CMap walk.
	path := filepath.Join("maxidi", "testdata", "realpdfs", "00000000054823.pdf")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Skipf("real sample PDF not found at %s: %v", path, statErr)
	}
	pages, _, err := extractPageTexts(path)
	if err != nil {
		t.Fatalf("extractPageTexts: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no pages extracted")
	}
	for _, want := range []string{"HO-PO00085936", "26/08/2026", "24/09/2026", "8935355302344", "10,800.00"} {
		if !strings.Contains(pages[0], want) {
			t.Errorf("extracted page 1 does not contain %q\nextracted text:\n%s", want, pages[0])
		}
	}
}
