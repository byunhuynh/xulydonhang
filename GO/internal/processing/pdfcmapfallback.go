// Package-level note on why this fix lives in SHARED code
// (extractPageText, used by every vendor's dispatch loop and
// vendor.Identify), unlike the closest precedent for a hand-written
// content-stream walker in this codebase: winmart_pdftext.go's
// extractWinmartPageText is deliberately kept OUT of the shared path
// (see that file's own doc comment, lines ~75-81) specifically because
// its line-break-reconstruction heuristic could plausibly turn already-
// CORRECT output into something worse for Coop/Lotte/Satra/BigC's own
// already-passing golden fixtures (155+60+36+29 real fixtures) — it's a
// judgment call about READING ORDER, which has no single objectively
// right answer independent of a specific generator's layout.
//
// This fix is a different architectural class: the same kind of "the
// vendored PDF library's own decode is objectively wrong" bug the
// already-shipped pdfopen.go fixes from the Emart plan address (trailing
// garbage after %%EOF, un-tokenizable inline image data) — a malformed
// ToUnicode CMap makes the library emit U+FFFD for bytes that have an
// unambiguous, spec-correct decoding available. It is placed in shared
// code because the bug it fixes isn't FujiMart-specific either — any
// vendor's PDF generator could in principle emit this same malformed-
// CMap shape.
//
// The safety property that makes this different from Winmart's case,
// and safe to share: this whole fallback is gated behind isGarbledText
// (the call site in extractPageText, pdfextract.go) — it only activates
// when the EXISTING extraction already looks badly broken (>50% U+FFFD).
// It is NOT the internal per-font shape checks below (Type0 check,
// /Encoding-Null check, bfrange-shape check) that keep this from
// engaging on other vendors' PDFs — those checks alone are NOT
// selective: real Coop PDFs have been confirmed to pass every one of
// them (simple fonts, Null /Encoding, well-formed single-byte bfrange
// entries) while still decoding correctly today via the library's own
// normal path. isGarbledText is the only thing that actually keeps this
// fallback from firing on already-working pages. Because of that gating,
// even if this fallback were occasionally wrong on some future page, it
// can only replace ALREADY-GARBLED output with either correct output or
// (at worst) DIFFERENTLY garbled output — it can never make already-good
// extraction worse, since it never runs against text that wasn't already
// mostly U+FFFD to begin with.

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
// FujiMart case this detects: a malformed embedded ToUnicode CMap). This
// is the ONLY gate that keeps extractPageTextViaCorrectedCmap from
// engaging on other vendors' already-correctly-decoding PDFs — see this
// file's own top-of-file doc comment. A small, fixed sample-size floor
// (20 non-whitespace runes) avoids false-triggering on tiny/near-empty
// pages where a couple of legitimately-unmappable glyphs could otherwise
// look "mostly garbled" by ratio alone.
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

// bfRangeBlockPattern and bfCharBlockPattern each capture the body of a
// beginbfrange/endbfrange or beginbfchar/endbfchar block respectively
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
// Type0 fonts, fonts with a real non-Null /Encoding, fonts with no
// ToUnicode stream, a destination-array bfrange entry
// ("<lo><hi>[<d1><d2>...]", not the plain single-string-destination
// shape this parser handles), or any bfrange/bfchar entry whose code or
// destination token isn't the plain 1-byte-code/single-BMP-rune shape
// confirmed present on the real affected PDFs.
//
// IMPORTANT: none of these checks, on their own, are what keeps this
// fallback from engaging on OTHER vendors' PDFs — real Coop PDFs are
// confirmed to pass every one of them (simple fonts, Null /Encoding,
// well-formed single-byte bfrange entries) while still decoding
// correctly today via the library's own normal path. The actual
// selectivity comes entirely from the isGarbledText gate at the call
// site (extractPageText, pdfextract.go) — see this file's own top-of-
// file doc comment for the full safety argument. These checks exist to
// avoid building a WRONG table for a font shape this narrow parser
// wasn't verified against, not to identify FujiMart specifically.
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
	// sawEntry tracks whether at least one bfrange OR bfchar entry was
	// successfully parsed (set by BOTH loops below) — named for what it
	// actually means, not just the bfrange loop it was originally
	// written alongside.
	sawEntry := false
	for _, block := range bfRangeBlockPattern.FindAllStringSubmatch(text, -1) {
		// A destination-ARRAY bfrange entry ("<lo><hi>[<d1><d2>...]", PDF
		// spec §9.10.3) is a real, legal CMap shape this narrow parser
		// does not handle (each source code in [lo,hi] maps to its OWN
		// independent destination string, not a linear offset from a
		// single base value the way the string-destination shape below
		// assumes). Bail explicitly on the presence of "[" in this
		// block's body rather than relying on hexTokenPattern's flat,
		// bracket-blind token extraction to accidentally produce a
		// length mismatch below — a destination array whose element
		// count happens to align with the 3-tokens-per-entry grouping
		// below (e.g. exactly one extra 4-hex-digit destination token)
		// would otherwise silently parse into a WRONG table instead of
		// bailing, which is not an acceptable risk for shared infra.
		if strings.Contains(block[1], "[") {
			return nil, false
		}
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
			sawEntry = true
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
			sawEntry = true
		}
	}
	if !sawEntry || len(table) == 0 {
		return nil, false
	}
	return table, true
}

// extractPageTextViaCorrectedCmap re-extracts a page's text as a flat,
// line-structured string, mirroring the vendored library's own
// Page.GetPlainText operator-by-operator (same "\n" on BT/T*, same
// Tj/TJ/'/" handling, same "TJ position-adjustment numbers don't affect
// text" behavior), with Tj/TJ/'/" text decoded through
// decodeSimpleFontCmap's corrected, always-1-byte-per-rune table instead
// of the library's own Font.Encoder() (which fails on the specific
// malformed-ToUnicode-CMap PDFs this whole file exists for — see
// extractPageText's doc comment). This is the MAIN difference, but not
// the only one — confirmed by direct comparison against GetPlainText's
// real operator handling (page.go), this function also differs in four
// smaller ways, none of which are bugs, but all worth stating precisely
// rather than claiming a single clean difference:
//  1. T* here writes a literal "\n" directly; GetPlainText instead
//     routes even T*'s "\n" through the font's own encoder
//     (showEncodedText("\n")) — functionally equivalent for any encoder
//     that passes ASCII through unchanged, which every encoder relevant
//     here does, but not literally the same code path.
//  2. The real GetPlainText has a genuine bug in its own operator
//     switch: the `"` case's args (3 of them) fall through into the `'`
//     case's arg-count guard (`if len(args) != 1`), which always fails
//     for `"` and panics ("bad ' operator") — caught by GetPlainText's
//     own top-level recover, which then discards ALL accumulated text
//     for that page and returns an error. This function instead handles
//     `"` correctly on its own terms (reads args[2], the actual show-
//     text argument per the PDF spec) — a real behavioral difference,
//     and arguably more correct, not merely different.
//  3. Text shown before any Tf, or after a Tf naming a font missing from
//     the page's own resource dict, is silently DROPPED here (haveFont
//     stays false). GetPlainText's real behavior for a missing font is
//     to fall back to a pass-through nopEncoder and write the RAW,
//     un-decoded bytes instead. Both are "wrong" for that edge case in
//     different ways; this function's choice avoids ever emitting a
//     byte under the pretense that it's a real decoded character.
//  4. A malformed Tf (wrong argument count) is silently ignored here,
//     leaving whatever font was already active in effect. GetPlainText
//     panics on this too (`panic("bad TL")`), again losing the whole
//     page's accumulated text via its top-level recover. This function
//     keeps processing the rest of the page instead.
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
// Identity-H font mixed in alongside the broken simple fonts), if the
// reconstructed text ends up empty, or if anything below panics (see the
// recover() immediately below — decodeSimpleFontCmap calls
// toUnicode.Reader() for EVERY font resource on the page, including any
// font never actually referenced by a Tf operator; unlike GetPlainText's
// own lazy per-use font resolution (page.go:577-578), that eager
// per-resource read means Reader() — which panics outright on an
// unsupported /Filter, read.go:818 — could be reached for a font this
// page's content stream never even shows text with. Never confirmed on
// a real PDF today, but cheap and important to guard against in shared
// code, matching pdfOpen's (pdfopen.go:33-41) and
// extractWinmartPageText's (winmart_pdftext.go:82-88) own precedent: a
// single malformed PDF must never be able to crash the whole process.)
func extractPageTextViaCorrectedCmap(page pdf.Page) (result string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			result, ok = "", false
		}
	}()

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
