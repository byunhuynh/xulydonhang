package processing

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"

	"order-processor/internal/processing/vendor"
)

// extractPageTexts returns each non-null page's extracted text, plus a
// parallel slice of the real, 1-based PDF page number for each entry.
// The two slices are the same length and the same order; pages[i]
// corresponds to real PDF page pageNumbers[i]. This second slice exists
// because the returned pages slice is COMPACTED — a null page (see the
// page.V.IsNull() check below) is skipped, not appended as an empty
// entry — so pages[i] does NOT in general equal PDF page i+1 once any
// earlier page has been null. Callers that need to re-open this same PDF
// and re-fetch a specific page by its real PDF page number (rather than
// just consuming pages[i] as opaque text) must use pageNumbers[i], not i,
// for that lookup — see extractWinmartPageTextFromFile's caller in
// coop_processor.go for a concrete case this matters for.
func extractPageTexts(path string) ([]string, []int, error) {
	file, r, err := pdfOpen(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	numPages := r.NumPage()
	pages := make([]string, 0, numPages)
	pageNumbers := make([]int, 0, numPages)
	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := extractPageText(page)
		if err != nil {
			return nil, nil, fmt.Errorf("trang %d: %w", i, err)
		}
		pages = append(pages, text)
		pageNumbers = append(pageNumbers, i)
	}
	if len(pages) == 0 {
		return nil, nil, fmt.Errorf("không đọc được nội dung trang nào")
	}
	return pages, pageNumbers, nil
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

// lineCountDivergenceTolerance bounds how far GetPlainText's own newline
// count may drift from trueVisualLineCount's independent, position-based
// line estimate before GetPlainText's text is distrusted outright (see
// the nl >= minPlausibleLines branch in extractPageText). Real,
// correctly-extracted pages measured a diff of -1 in 87/98 sampled
// passing Coop fixtures (a small, consistent, harmless undercount — one
// fewer newline than visual row, most likely a missing trailing blank
// line); the smallest genuinely-broken divergence measured, across both
// of this generator's failure shapes, was +30 (missing line breaks
// entirely — GetPlainText glues two real lines together with no
// separator) and -145 (spurious mid-token line breaks — GetPlainText
// splits a single number like "103226908-00" across 4 separate lines).
// 15 sits with real margin on both sides of that gap.
const lineCountDivergenceTolerance = 15

// trueVisualLineCount estimates the real number of visual text rows on
// a page directly from each text run's own Y position — the same
// Y-bucketing reconstructLinesFromContent uses to rebuild line
// structure, but here only the row COUNT is needed, not the rebuilt
// text itself. Independent of GetPlainText's own (for some generators,
// unreliable — see lineCountDivergenceTolerance) newline count, so
// comparing the two catches cases GetPlainText's newline count alone
// can't: both too FEW newlines (real lines glued together) and too MANY
// (a real line's content spuriously split across several "lines").
func trueVisualLineCount(page pdf.Page) int {
	content := page.Content()
	if len(content.Text) == 0 {
		return 0
	}
	return len(clusterRowsByY(rotateForReading(content.Text, pageRotation(page), false)))
}

// isCoopGeneratedPage reports whether text was produced by Coop's own
// "JDA Software Version 7.7.0 (PDN_DC)" report generator — the same
// signature already used to identify this generator family elsewhere in
// this file's doc comments. Strips ALL whitespace (not just runs of it)
// before matching so a spurious mid-token newline landing inside the
// signature string itself still can't hide it — the same resilience the
// line-count-divergence check needs from its own gate (see the call
// site's doc comment for why vendor.Identify alone isn't robust enough
// here).
func isCoopGeneratedPage(text string) bool {
	stripped := strings.Join(strings.Fields(text), "")
	return strings.Contains(stripped, "JDASoftwareVersion")
}

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
	text, err := extractPageTextRaw(page)
	if err != nil {
		return "", err
	}
	// A real subset of archived Coop PDFs (confirmed: PO numbers around
	// 103256391/103340115) render their field labels' spacing using
	// U+00A0 (NBSP) instead of a normal U+0020 space — invisible to the
	// eye, but Go's RE2 \s is ASCII-only and does not match it, so any
	// downstream regex expecting ordinary whitespace between a label and
	// its value silently fails to match on these specific pages. NBSP
	// carries no meaningful distinction from a normal space for this
	// project's plain-text field extraction, so normalizing it here is
	// safe for every vendor, not just Coop.
	return strings.ReplaceAll(text, "\u00a0", " "), nil
}

func extractPageTextRaw(page pdf.Page) (string, error) {
	text, err := page.GetPlainText(nil)
	if err != nil {
		return "", err
	}
	// Some real FujiMart PDFs (confirmed: a subset using the ".VnArial"/
	// ".VnTime" simple TrueType font family instead of the CID/Identity-H
	// fonts most real FujiMart PDFs use) embed a malformed ToUnicode CMap
	// whose /CMapType 2 begincodespacerange declares 2-byte character
	// codes (e.g. "<0020><00f9>") while every actual beginbfrange entry
	// underneath uses 1-byte codes (e.g. "<00><00><0020>") — a real bug
	// in whatever tool generated these specific PDFs, confirmed present
	// identically across all font resources on the affected pages. This
	// vendored PDF library's own CMap reader trusts the (wrong) 2-byte
	// declaration, so its codespace-range membership check never matches
	// a real 1-byte code, and every single character — not just
	// Vietnamese diacritics, ALL of it, digits and ASCII letters
	// included — decodes to U+FFFD. See pdfcmapfallback.go for the fix:
	// a from-scratch content-stream walk (mirroring this library's own
	// GetPlainText operator-by-operator, but built on this package's
	// exported API) that ignores the bad codespacerange declaration and
	// just trusts each simple font's bfrange/bfchar entries' own 1-byte
	// code width — correct per the PDF spec, since simple (non-Type0)
	// fonts always use 1-byte codes regardless of what a malformed CMap
	// claims.
	if isGarbledText(text) {
		if corrected, ok := extractPageTextViaCorrectedCmap(page); ok {
			return corrected, nil
		}
	}
	nl := strings.Count(text, "\n")
	if nl >= minPlausibleLines {
		// The line-count-divergence check below is scoped to pages from
		// Coop's own "JDA Software" report generator only. Tried applying
		// it to every vendor's pages; a real archived JMart PDF
		// (testdata/sample_jmart_order.pdf) has real diff=-54 (86 raw
		// newlines, 32 true visual rows — this generator wraps a single
		// field's value across several GetPlainText lines, not a bug),
		// and reconstructLinesFromContent's Y/X-based reading order does
		// NOT reproduce this specific generator's true reading order
		// (same known failure class already documented below for
		// 103231203-00) — switching to reconstruction for it turned a
		// working extraction into zero products found. Scoping to this
		// generator keeps every other vendor's extraction byte-for-byte
		// unchanged from before this check existed.
		//
		// Deliberately NOT gated via vendor.Identify(text): this exact
		// generator's own spurious-newline bug (see the "\n" skip inside
		// reconstructLinesFromContent) can land mid-token in the Vendor-ID
		// digits vendor.Identify's own coopPattern requires contiguous
		// (confirmed: 103226984-00's raw text fails vendor.Identify
		// entirely because of this, even though it's a genuine Coop page)
		// — the exact unreliability this whole check exists to route
		// around would then make the check whether to APPLY the check
		// unreliable too. isCoopGeneratedPage strips ALL whitespace
		// (spaces and any newline, spurious or real) before matching, so
		// a spurious mid-token break can't hide the signature either.
		if !isCoopGeneratedPage(text) {
			return text, nil
		}
		trueLines := trueVisualLineCount(page)
		diff := trueLines - nl
		if diff < 0 {
			diff = -diff
		}
		if trueLines == 0 || diff <= lineCountDivergenceTolerance {
			return text, nil
		}
		// nl clears the old bare line-count floor but still doesn't look
		// plausible against the position-based estimate — same "GetPlainText's
		// newline count can't be trusted for this page" situation
		// minPlausibleLines exists to catch, just the opposite shape (too
		// many/too few relative to trueLines rather than too few in
		// absolute terms). Fall through to the same reconstruction attempt
		// and safety net used below.
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

// rowYJitterTolerance: points. Two text runs belong to the same visual
// row if their Y positions are within this distance of the row's own
// reference Y (its first, topmost run). Confirmed necessary on a real
// archived Coop PDF (103145712-00): individual glyphs of the SAME word
// ("SKU", "S"/"K"/"U") reported Y values only ~0.05-0.15pt apart, but
// straddling different sides of an integer boundary (608.966/609.015/
// 609.065) — hard int64-truncated bucketing (the original approach)
// split them into 2 different "rows", scrambling reading order once
// those fragments got sorted alongside every other row on the page. Real
// row-to-row spacing on this same PDF measured ~6.8pt (591.24 -> 584.4),
// nearly 50x this tolerance, so genuinely distinct rows are never at
// risk of merging.
const rowYJitterTolerance = 1.5

// clusterRowsByY groups text runs into visual rows by Y proximity (see
// rowYJitterTolerance), ordered top-to-bottom (PDF Y increases upward).
// Shared by reconstructLinesFromContent (which also needs each row's own
// members, in original run order) and trueVisualLineCount (which only
// needs the row count) so both agree on what counts as one visual row.
func clusterRowsByY(texts []pdf.Text) [][]pdf.Text {
	sorted := make([]pdf.Text, len(texts))
	copy(sorted, texts)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Y > sorted[j].Y })

	var rows [][]pdf.Text
	var rowRefY float64
	for _, t := range sorted {
		if len(rows) == 0 || rowRefY-t.Y > rowYJitterTolerance {
			rows = append(rows, nil)
			rowRefY = t.Y
		}
		last := len(rows) - 1
		rows[last] = append(rows[last], t)
	}
	return rows
}

// pageRotation reads a page's inherited /Rotate entry (PDF spec: MAY be
// inherited from an ancestor Pages node, not always present directly on
// the page dict), normalized to one of 0/90/180/270. Returns 0 (no
// rotation) if absent anywhere in the chain — the overwhelmingly common
// case, and the correct default per spec.
func pageRotation(page pdf.Page) int {
	for v := page.V; !v.IsNull(); v = v.Key("Parent") {
		r := v.Key("Rotate")
		if r.IsNull() {
			continue
		}
		n := int(r.Int64()) % 360
		if n < 0 {
			n += 360
		}
		return n
	}
	return 0
}

// rotateForReading returns texts with X/Y replaced by reading-order
// coordinates (increasing X = rightward in the DISPLAYED page, and — to
// reuse clusterRowsByY's existing "sort Y descending, cluster by
// proximity" logic unchanged — increasing Y = further UP the displayed
// page, matching Rotate 0's own already-correct convention) — bypassing
// the CONTENT STREAM's raw, unrotated coordinate system a page's
// /Rotate entry asks a viewer to rotate for display. Only the relative
// ordering and point-scale distances matter here (this feeds sorting
// and a small absolute gap threshold, never absolute page positions),
// so a simple axis swap + negation is enough — no page width/height
// needed, unlike a full display-coordinate transform.
//
// Confirmed empirically against a real archived Coop PDF with /Rotate
// 90 (103145712-00): "POM343"'s own 6 characters, read in that exact
// order, had CONSTANT raw X and raw Y increasing by the font's own
// per-glyph advance — i.e. in raw content-stream space, moving right in
// reading order is moving along raw Y, not raw X, and successive
// visual ROWS (confirmed via distinct field labels appearing later in
// the content stream) had raw X increasing by roughly one line height
// per row. Before this transform, clusterRowsByY bucketed by raw Y
// directly, treating what is actually the WITHIN-row axis for a rotated
// page as if it were the ROW axis — every row split into fragments
// interleaved with fragments of every OTHER row once naively sorted,
// scrambling reading order (confirmed: output like "KU Discounts -\nS"
// for what should read "SKU Discounts -" on one line).
//
// mirror flips the within-row (reading-order-X) axis only, NOT the row
// axis. Needed because a second real archived Coop PDF with the SAME
// /Rotate 90 (103269932-00) had "POM343"'s own characters read in order
// with raw Y DEcreasing — the opposite sign from 103145712-00 — while
// its row axis (raw X, increasing for later rows) matched exactly. Two
// PDFs sharing one /Rotate value but disagreeing on reading-order sign
// for one axis only is not something /Rotate alone predicts; the caller
// tries both and keeps whichever one the vendor-identification check
// (see reconstructLinesFromContent) accepts, rather than this function
// guessing a single "correct" sign that would silently mis-handle
// whichever of the two page shapes it guessed wrong for.
func rotateForReading(texts []pdf.Text, rotation int, mirror bool) []pdf.Text {
	if rotation == 0 {
		return texts
	}
	sign := 1.0
	if mirror {
		sign = -1.0
	}
	out := make([]pdf.Text, len(texts))
	copy(out, texts)
	for i, t := range out {
		switch rotation {
		case 90:
			out[i].X, out[i].Y = sign*t.Y, -t.X
		case 180:
			out[i].X, out[i].Y = sign*-t.X, -t.Y
		case 270:
			out[i].X, out[i].Y = sign*-t.Y, t.X
		}
	}
	return out
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
// clustered into rows by Y proximity (see rowYJitterTolerance), rows
// ordered top-to-bottom (PDF Y increases upward), and within a row, runs
// ordered left-to-right by X with a space inserted wherever there's a
// visible horizontal gap (mirrors normal word spacing; a small
// negative/zero gap between two runs already means they're touching,
// e.g. mid-word kerning splits).
func reconstructLinesFromContent(page pdf.Page) (string, bool) {
	content := page.Content()
	if len(content.Text) == 0 {
		return "", false
	}

	rotation := pageRotation(page)
	primary := buildTextFromRows(clusterRowsByY(rotateForReading(content.Text, rotation, false)))
	if rotation != 90 && rotation != 270 {
		return primary, true
	}
	// Try the within-row-mirrored candidate too (see rotateForReading's
	// doc comment for why: two real /Rotate 90 Coop PDFs disagreed on
	// which sign reproduces true reading order for this axis) and keep
	// whichever one is actually identifiable as a real vendor page —
	// vendor.Identify is a coarse check, but a mirrored reconstruction
	// of real text reads as reversed word fragments that essentially
	// never happen to match any vendor's real signature phrase, while a
	// correctly-oriented one does. Prefer primary on a tie or when
	// neither identifies (matches this function's existing contract:
	// callers apply their own, stronger safety net against whatever
	// this returns).
	if vendor.Identify(primary) != "" {
		return primary, true
	}
	mirrored := buildTextFromRows(clusterRowsByY(rotateForReading(content.Text, rotation, true)))
	if vendor.Identify(mirrored) != "" {
		return mirrored, true
	}
	return primary, true
}

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

// buildTextFromRows concatenates each row's runs left-to-right by X,
// inserting a space at visible horizontal gaps (see gapThreshold),
// joining rows with a newline. Shared by reconstructLinesFromContent's
// primary and rotation-mirrored candidates so both go through identical
// gap/skip logic.
func buildTextFromRows(rows [][]pdf.Text) string {
	var b strings.Builder
	for _, items := range rows {
		sort.SliceStable(items, func(i, j int) bool { return items[i].X < items[j].X })
		lastX, lastW := -1.0, 0.0
		for _, it := range items {
			// Some real archived PDFs from this same generator family embed
			// a literal U+000A (newline) as its own zero-width glyph run —
			// confirmed present mid-token, e.g. between the "1" and "0" of
			// a P/O number's digit run in 103226908-00 — apparently an
			// artifact of whatever internal text-substitution process
			// generated the report, not a real drawn character. Trusting
			// it (as both GetPlainText and an earlier version of this
			// function did) splits a single token across output lines; it
			// carries no visual position of its own worth preserving, so
			// skip it entirely rather than writing it into the row or
			// letting it perturb the next glyph's gap calculation.
			if it.S == "\n" {
				continue
			}
			if lastX >= 0 && it.X-(lastX+lastW) > gapThreshold {
				b.WriteString(" ")
			}
			b.WriteString(it.S)
			lastX, lastW = it.X, it.W
		}
		b.WriteString("\n")
	}
	return b.String()
}
