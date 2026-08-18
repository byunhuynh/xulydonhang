package processing

import (
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// isGarbledText reports whether text is dominated by the Unicode
// replacement character (U+FFFD) — the signature of a PDF text-decode
// failure (see extractPageText's doc comment for the confirmed real
// FujiMart case this detects: a malformed embedded ToUnicode CMap). A
// small, fixed sample-size floor (20 non-whitespace runes) avoids
// false-triggering on tiny/near-empty pages where a couple of
// legitimately-unmappable glyphs could otherwise look "mostly garbled"
// by ratio alone.
func isGarbledText(text string) bool {
	var total, garbled int
	for _, r := range text {
		switch r {
		case '\n', '\r', '\t', ' ':
			continue
		}
		total++
		if r == utf8.RuneError {
			garbled++
		}
	}
	return total > 20 && garbled*2 > total
}

// hexTokenPattern matches one PDF hex-string token, e.g. "<0a>" or
// "<00f9>" — used to pull the raw code/destination tokens out of a
// ToUnicode CMap's beginbfrange/beginbfchar blocks without needing a
// full PostScript-CMap parser (this package only ever needs to read
// this one, narrow construct).
var hexTokenPattern = regexp.MustCompile(`<([0-9a-fA-F]+)>`)

// bfBlockPattern captures the body of a beginbfrange/beginbfchar block
// (non-greedy, since a ToUnicode CMap may contain more than one of
// each).
var bfRangeBlockPattern = regexp.MustCompile(`(?s)beginbfrange(.*?)endbfrange`)
var bfCharBlockPattern = regexp.MustCompile(`(?s)beginbfchar(.*?)endbfchar`)

// decodeSimpleFontCmap builds a byte->rune lookup table directly from a
// simple (non-Type0) TrueType/Type1 font's embedded ToUnicode CMap
// stream, deliberately IGNORING that stream's own begincodespacerange
// declaration — see extractPageText's doc comment: on the real FujiMart
// PDFs this fallback exists for, that declaration is wrong (claims
// 2-byte codes) while the actual beginbfrange entries underneath are
// unambiguously 1-byte (matching the PDF spec's own rule that simple
// fonts always use 1-byte character codes, regardless of what a
// malformed CMap claims). Returns ok=false — never a guessed/partial
// table — for any font this narrow parser can't confidently handle:
// Type0 fonts (out of scope; Identity-H FujiMart PDFs already decode
// correctly via the library's own normal path), fonts with a real
// non-Null /Encoding (already decode correctly, don't touch), fonts
// with no ToUnicode stream, or any bfrange/bfchar entry whose code or
// destination token isn't the plain 1-byte-code/single-BMP-rune shape
// confirmed present on the real affected PDFs.
func decodeSimpleFontCmap(font pdf.Font) (map[byte]rune, bool) {
	if font.V.Key("Subtype").Name() == "Type0" {
		return nil, false
	}
	if font.V.Key("Encoding").Kind() != pdf.Null {
		return nil, false
	}
	toUnicode := font.V.Key("ToUnicode")
	if toUnicode.Kind() != pdf.Stream {
		return nil, false
	}
	rdr := toUnicode.Reader()
	if rdr == nil {
		return nil, false
	}
	defer rdr.Close()
	data, err := io.ReadAll(rdr)
	if err != nil {
		return nil, false
	}
	text := string(data)

	table := make(map[byte]rune)
	sawRange := false
	for _, block := range bfRangeBlockPattern.FindAllStringSubmatch(text, -1) {
		tokens := hexTokenPattern.FindAllStringSubmatch(block[1], -1)
		for i := 0; i+3 <= len(tokens); i += 3 {
			loHex, hiHex, dstHex := tokens[i][1], tokens[i+1][1], tokens[i+2][1]
			if len(loHex) != 2 || len(hiHex) != 2 || len(dstHex) != 4 {
				// Not the confirmed 1-byte-code/single-BMP-rune shape —
				// bail on the whole font rather than build a partial,
				// silently-incomplete table.
				return nil, false
			}
			lo, errLo := strconv.ParseUint(loHex, 16, 8)
			hi, errHi := strconv.ParseUint(hiHex, 16, 8)
			dst, errDst := strconv.ParseUint(dstHex, 16, 32)
			if errLo != nil || errHi != nil || errDst != nil || hi < lo {
				return nil, false
			}
			for code := lo; code <= hi; code++ {
				table[byte(code)] = rune(dst + (code - lo))
			}
			sawRange = true
		}
	}
	for _, block := range bfCharBlockPattern.FindAllStringSubmatch(text, -1) {
		tokens := hexTokenPattern.FindAllStringSubmatch(block[1], -1)
		for i := 0; i+2 <= len(tokens); i += 2 {
			srcHex, dstHex := tokens[i][1], tokens[i+1][1]
			if len(srcHex) != 2 || len(dstHex) != 4 {
				return nil, false
			}
			src, errSrc := strconv.ParseUint(srcHex, 16, 8)
			dst, errDst := strconv.ParseUint(dstHex, 16, 32)
			if errSrc != nil || errDst != nil {
				return nil, false
			}
			table[byte(src)] = rune(dst)
			sawRange = true
		}
	}
	if !sawRange || len(table) == 0 {
		return nil, false
	}
	return table, true
}

// extractPageTextViaCorrectedCmap re-extracts a page's text as a flat,
// line-structured string, mirroring the vendored library's own
// Page.GetPlainText operator-by-operator (same "\n" on BT/T*, same
// Tj/TJ/'/" handling, same "TJ position-adjustment numbers don't affect
// text" behavior) — the ONLY difference is that Tj/TJ/'/" text is
// decoded through decodeSimpleFontCmap's corrected, always-1-byte-per-
// rune table instead of that library's own Font.Encoder(), which fails
// on the specific malformed-ToUnicode-CMap PDFs this whole file exists
// for (see extractPageText's doc comment).
//
// Deliberately does NOT reuse the position-based (X/Y-bucketing)
// reconstructLinesFromContent approach: confirmed against a real
// affected PDF (106003302608000751.pdf) that doing so merges same-row
// label+value fields (e.g. "Ngµy ®Æt:" + "10/08/2026" + "14:39", three
// separate BT-delimited text objects sharing one visual row) into a
// single joined line — which breaks ParseOrderInfo's line-position-
// based marker offsets (they assume the SAME per-field line structure
// PyMuPDF's/GetPlainText's own BT/T*-driven line breaks produce, not a
// visual-row grouping). Verified via PyMuPDF's own real extraction of
// this same PDF: "10/08/2026" and "106003302608000751" (the PO number)
// each appear as their OWN separate line, immediately adjacent, exactly
// as ParseOrderInfo's marker-offset rules expect — confirming
// GetPlainText's BT/T*-based line structure (not Y-position bucketing)
// is what this fallback needs to reproduce.
//
// Returns ok=false — unchanged fall-through to the caller's existing
// (still-garbled) behavior — if ANY font used on the page isn't one
// decodeSimpleFontCmap can confidently rebuild (e.g. a genuine Type0/
// Identity-H font mixed in alongside the broken simple fonts), or if the
// reconstructed text ends up empty.
func extractPageTextViaCorrectedCmap(page pdf.Page) (string, bool) {
	if page.V.IsNull() || page.V.Key("Contents").Kind() == pdf.Null {
		return "", false
	}
	fontNames := page.Fonts()
	if len(fontNames) == 0 {
		return "", false
	}
	tables := make(map[string]map[byte]rune, len(fontNames))
	for _, name := range fontNames {
		font := page.Font(name)
		table, ok := decodeSimpleFontCmap(font)
		if !ok {
			return "", false
		}
		tables[name] = table
	}

	var textBuilder strings.Builder
	var curTable map[byte]rune
	haveFont := false

	decodeAndWrite := func(raw string) {
		if !haveFont {
			return
		}
		for i := 0; i < len(raw); i++ {
			if ch, ok := curTable[raw[i]]; ok {
				textBuilder.WriteRune(ch)
			} else {
				textBuilder.WriteRune(utf8.RuneError)
			}
		}
	}

	pdf.Interpret(page.V.Key("Contents"), func(stk *pdf.Stack, op string) {
		n := stk.Len()
		args := make([]pdf.Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}
		switch op {
		case "BT": // matches GetPlainText: unconditional literal newline
			textBuilder.WriteString("\n")

		case "T*": // matches GetPlainText: newline on next-line move
			textBuilder.WriteString("\n")

		case "Tf":
			if len(args) == 2 {
				name := args[0].Name()
				if table, ok := tables[name]; ok {
					curTable = table
					haveFont = true
				} else {
					haveFont = false
				}
			}

		case "\"":
			if len(args) == 3 {
				decodeAndWrite(args[2].RawString())
			}

		case "'":
			if len(args) == 1 {
				decodeAndWrite(args[0].RawString())
			}

		case "Tj":
			if len(args) == 1 {
				decodeAndWrite(args[0].RawString())
			}

		case "TJ":
			if len(args) == 1 {
				v := args[0]
				for i := 0; i < v.Len(); i++ {
					x := v.Index(i)
					if x.Kind() == pdf.String {
						decodeAndWrite(x.RawString())
					}
				}
			}
		}
	})

	text := textBuilder.String()
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}
