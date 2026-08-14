package processing

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"

	"order-processor/internal/processing/vendor"
)

func extractPageTexts(path string) ([]string, error) {
	file, r, err := pdfOpen(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	numPages := r.NumPage()
	pages := make([]string, 0, numPages)
	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := extractPageText(page)
		if err != nil {
			return nil, fmt.Errorf("trang %d: %w", i, err)
		}
		pages = append(pages, text)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("không đọc được nội dung trang nào")
	}
	return pages, nil
}

// minPlausibleLines is the newline-count floor below which
// GetPlainText's output is treated as structurally broken rather than
// a genuinely short page. Every real Coop invoice page has dozens of
// visual lines (header fields, column headers, one line per product,
// totals); a handful of archived PDFs (confirmed: PO numbers around
// 103157888/103231*/103311304 — all sharing the same "JDA Software
// Version 7.7.0 (PDN_DC)" generator string visible in their own text)
// collapse to exactly ONE newline from GetPlainText, because that
// library method only inserts a newline on the PDF content stream's
// "BT" (begin text object) and "T*" (next-line) operators — and this
// particular generator apparently lays out every line of the page
// using direct positioning (Td/Tm) inside a single text object instead
// of "T*", so the naive heuristic never fires. See
// reconstructLinesFromContent for the fallback.
const minPlausibleLines = 5

// extractPageText mirrors process_coop_invoice's page.get_text("text")
// call, with a fallback for the small subset of archived PDFs where
// this Go PDF library's naive BT/T*-based line detection produces a
// single unbroken blob (see minPlausibleLines) instead of real lines.
// Downstream parsing (coop.ExtractProducts in particular) depends on
// one-line-per-visual-row structure the same way the ported Python
// original does against PyMuPDF's much more robust text extraction;
// without this fallback, those PDFs' product lists silently
// mis-parse (SKU-anchor blocks merge together) even though nothing in
// the ported business logic itself is wrong.
func extractPageText(page pdf.Page) (string, error) {
	text, err := page.GetPlainText(nil)
	if err != nil {
		return "", err
	}
	if strings.Count(text, "\n") >= minPlausibleLines {
		return text, nil
	}
	reconstructed, ok := reconstructLinesFromContent(page)
	if !ok {
		return text, nil
	}
	// Guard against the reconstruction itself scrambling character
	// order for PDFs whose content-stream text-positioning this
	// per-character Y-bucketing doesn't handle correctly. Confirmed on
	// a real archived PDF (103231203-00): the same approach that
	// correctly reconstructs 103157888-00 into readable lines instead
	// turned this one's "Vendor: 22856" into scrambled, unrecognizable
	// character soup — apparently some PDF generators lay out text in
	// an order where naive per-character Y/X sorting doesn't reproduce
	// true reading order. GetPlainText's OWN character order is always
	// correct even on the pages this whole fallback exists for (it only
	// fails to insert line breaks, never reorders characters), so only
	// trust the reconstruction when it's at least as recognizable as
	// the original: if the original text identifies as a known vendor
	// but the reconstruction no longer does, prefer the original
	// (accepting that file's line-dependent parsing, e.g.
	// coop.ExtractProducts, stays as limited as it would have been
	// without this fallback at all) over silently feeding corrupted
	// text into every downstream parser.
	if vendor.Identify(text) != "" && vendor.Identify(reconstructed) == "" {
		return text, nil
	}
	return reconstructed, nil
}

// reconstructLinesFromContent rebuilds a line-structured page text
// directly from each text run's own (X, Y) position — bypassing
// GetPlainText's/GetTextByRow's content-stream-operator-based line
// detection (both of which report every run on this PDF family at the
// same Y position, apparently because the library's BT/T*/Td state
// tracking used by walkTextBlocks doesn't handle this generator's
// positioning operators, even though Page.Content()'s lower-level walk
// does compute distinct per-run Y coordinates correctly — confirmed by
// inspecting both against the same real archived PDF). Runs are
// bucketed by truncated Y (rows), rows ordered top-to-bottom (PDF Y
// increases upward), and within a row, runs ordered left-to-right by X
// with a space inserted wherever there's a visible horizontal gap
// (mirrors normal word spacing; a small negative/zero gap between two
// runs already means they're touching, e.g. mid-word kerning splits).
func reconstructLinesFromContent(page pdf.Page) (string, bool) {
	content := page.Content()
	if len(content.Text) == 0 {
		return "", false
	}

	rowsByY := make(map[int64][]pdf.Text)
	for _, t := range content.Text {
		y := int64(t.Y)
		rowsByY[y] = append(rowsByY[y], t)
	}
	ys := make([]int64, 0, len(rowsByY))
	for y := range rowsByY {
		ys = append(ys, y)
	}
	sort.Slice(ys, func(i, j int) bool { return ys[i] > ys[j] })

	// gapThreshold: points; separates real word gaps from touching/kerned
	// runs within the same word. Calibrated against two real archived
	// PDFs with different font-width behavior: one (103157888-00) where
	// this library reports W=0 for every glyph (word gaps then come only
	// from literal " " glyphs, so this threshold is moot there), and one
	// (103311304-00) where real per-glyph widths ARE reported and
	// same-word adjacent-glyph gaps measured up to ~2.0pt while the
	// smallest real word-to-word gap measured ~9.7pt.
	//
	// Note: some archived PDFs render certain field LABELS (seen: "P/O
	// Number", "Sub Total", "Total") with deliberately wide letter
	// tracking baked into the glyph positions themselves — no threshold
	// distinguishes that from a real word gap, since the gap magnitude
	// is genuinely the same either way for those specific labels. Tried
	// raising this to 4.0 to chase that case; it neither fixed nor broke
	// anything across all 155 golden fixtures (verified empirically), so
	// left at the value with the clearer direct justification above.
	// coop.CountPOsOnPage tolerates the result via spacedPattern (see
	// dispatch.go) rather than this threshold trying to fully solve it.
	const gapThreshold = 2.0
	var b strings.Builder
	for _, y := range ys {
		items := rowsByY[y]
		sort.SliceStable(items, func(i, j int) bool { return items[i].X < items[j].X })
		lastX, lastW := -1.0, 0.0
		for _, it := range items {
			if lastX >= 0 && it.X-(lastX+lastW) > gapThreshold {
				b.WriteString(" ")
			}
			b.WriteString(it.S)
			lastX, lastW = it.X, it.W
		}
		b.WriteString("\n")
	}
	return b.String(), true
}
