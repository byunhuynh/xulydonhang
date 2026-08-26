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
// FujiMart case this detects: a malformed embedded ToUnicode CMap).
// Because this ranges over s with `for _, r := range s`, it counts not
// only literal U+FFFD characters already present in the string but also
// every invalid UTF-8 byte sequence encountered along the way — Go's
// range loop yields utf8.RuneError (the same value as U+FFFD) for those
// too. That's desirable here: raw non-UTF-8 byte soup (e.g. from a
// font's nopEncoder fallback path) is exactly the kind of "badly broken"
// extraction this gate exists to catch, not just a decoder that already
// wrote out an explicit replacement character. This is the ONLY gate
// that keeps extractPageTextViaCorrectedCmap from engaging on other
// vendors' already-correctly-decoding PDFs — see this file's own top-of-
// file doc comment. A small, fixed sample-size floor (20 non-whitespace
// runes) avoids false-triggering on tiny/near-empty pages where a couple
// of legitimately-unmappable glyphs could otherwise look "mostly
// garbled" by ratio alone.
func isGarbledText(text string) bool {
	var total, garbled int
	for _, r := range text {
		switch r {
		case '\n', '\r', '\t', ' ':
			continue
		}
		total++
		if r == utf8.RuneError || isControlRune(r) {
			garbled++
		}
	}
	return total > 20 && garbled*2 > total
}

// isControlRune reports whether r is a control character — the second
// shape a failed text decode takes, alongside U+FFFD. When a simple
// font's /Encoding dictionary maps codes to glyph names the vendored
// library's name->rune table doesn't know (typical of a subset font:
// confirmed on the real archived Coop PDF 103346096-00.pdf, whose only
// font is QSWINA+LucidaConsole with a 69-entry /Differences array), its
// dictEncoder falls through to the RAW CODE BYTE for every one of them.
// Those bytes are perfectly valid UTF-8 — "\x01\x02\x03..." — so
// U+FFFD counting alone sees a page that looks fine while the text is
// entirely unreadable. Real extracted text never contains C0 controls:
// the whitespace ones this counter cares about (space, tab, CR, LF) are
// already skipped by the caller before anything is counted.
func isControlRune(r rune) bool {
	return r < 0x20 || r == 0x7f
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

// simpleFontCmapText returns the raw text of a simple (non-Type0)
// font's embedded /ToUnicode CMap stream, or ok=false for any font whose
// shape this package's narrow CMap reasoning was not verified against.
// Shared by decodeSimpleFontCmap (which builds a corrected byte->rune
// table from it) and cmapCodespaceExcludesOwnCodes (which only inspects
// it for self-contradiction) so both agree exactly on which fonts they
// will speak about at all.
func simpleFontCmapText(font pdf.Font) (string, bool) {
	if font.V.Key("Subtype").Name() == "Type0" {
		return "", false
	}
	switch font.V.Key("Encoding").Kind() {
	case pdf.Null:
		// No /Encoding at all — the original FujiMart shape, and also
		// Maxidi's.
	case pdf.Dict:
		// An /Encoding DICTIONARY (/Differences, optionally over a
		// /BaseEncoding). The vendored library takes this branch in
		// preference to /ToUnicode and decodes through glyph NAMES,
		// which for a subset font are frequently names its table does
		// not know — see isControlRune for the confirmed Coop case. The
		// PDF spec makes /ToUnicode the authoritative mapping for text
		// EXTRACTION regardless of how the glyphs are selected for
		// rendering (§9.10.2), so preferring it here is not a guess.
	default:
		// A named base encoding (WinAnsiEncoding, MacRomanEncoding,
		// Identity-H) decodes through a table this parser has no reason
		// to second-guess.
		return "", false
	}
	toUnicode := font.V.Key("ToUnicode")
	if toUnicode.Kind() != pdf.Stream {
		return "", false
	}
	rdr := toUnicode.Reader()
	if rdr == nil {
		return "", false
	}
	defer rdr.Close()
	data, err := io.ReadAll(rdr)
	if err != nil {
		return "", false
	}
	return string(data), true
}

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
	text, ok := simpleFontCmapText(font)
	if !ok {
		return nil, false
	}

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
//     keeps processing the rest of the page instead. The same class of
//     divergence applies to Tj, ', and " as well: each is guarded here
//     by its own `if len(args) == N` check (lines ~343-356 below) that
//     simply does nothing when the argument count doesn't match, while
//     GetPlainText's real operator switch panics on exactly that
//     condition for all three (`panic("bad Tj operator")`,
//     `panic("bad ' operator")`, `panic("bad \" operator")`,
//     confirmed in the vendored library's page.go) — again losing the
//     whole page via its top-level recover, where this function just
//     skips the one malformed call and keeps going. (This is distinct
//     from item 2 above, which covers what happens for a WELL-formed,
//     3-argument " call — a separate, always-triggered bug in the real
//     library, not an argument-count mismatch.)
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

// codespaceBlockPattern captures the body of a begincodespacerange/
// endcodespacerange block — the declaration of how many bytes a
// character code occupies, and which byte values are legal codes at all.
var codespaceBlockPattern = regexp.MustCompile(`(?s)begincodespacerange(.*?)endcodespacerange`)

// cmapCodespaceExcludesOwnCodes reports whether a ToUnicode CMap
// contradicts itself: it declares a 1-byte codespace range that does NOT
// contain every source code its own bfrange/bfchar entries go on to map.
//
// This is not a stylistic quibble — the vendored library honours the
// declaration and emits U+FFFD for every code outside it, silently
// throwing away mappings the same CMap explicitly provides two lines
// further down. Confirmed on every real archived Maxidi delivery note:
// its Arial subset declares "<52> <f9>" while mapping <20>, <2c>, <2f>,
// all ten digits <30>..<39> and most uppercase letters — so the prose
// survives, but the PO number, both dates, the barcode and every
// quantity decode to U+FFFD.
//
// Deliberately narrow — returns false (i.e. "nothing provably wrong
// here") rather than guessing whenever the CMap is not the plain
// 1-byte-code shape this reasoning holds for: a multi-byte codespace
// declaration is the FujiMart shape, already handled by the
// isGarbledText gate, and a CMap with no codespace declaration at all
// makes no claim that could be contradicted. False is also the answer
// for a well-formed CMap, which is the overwhelmingly common case.
func cmapCodespaceExcludesOwnCodes(text string) bool {
	type codespace struct{ lo, hi uint64 }
	var declared []codespace
	for _, block := range codespaceBlockPattern.FindAllStringSubmatch(text, -1) {
		tokens := hexTokenPattern.FindAllStringSubmatch(block[1], -1)
		for i := 0; i+2 <= len(tokens); i += 2 {
			loHex, hiHex := tokens[i][1], tokens[i+1][1]
			if len(loHex) != 2 || len(hiHex) != 2 {
				// Not a 1-byte codespace declaration — out of scope, see
				// this function's own doc comment.
				return false
			}
			lo, errLo := strconv.ParseUint(loHex, 16, 8)
			hi, errHi := strconv.ParseUint(hiHex, 16, 8)
			if errLo != nil || errHi != nil || hi < lo {
				return false
			}
			declared = append(declared, codespace{lo, hi})
		}
	}
	if len(declared) == 0 {
		return false
	}

	covered := func(code uint64) bool {
		for _, r := range declared {
			if code >= r.lo && code <= r.hi {
				return true
			}
		}
		return false
	}

	for _, block := range bfRangeBlockPattern.FindAllStringSubmatch(text, -1) {
		tokens := hexTokenPattern.FindAllStringSubmatch(block[1], -1)
		for i := 0; i+3 <= len(tokens); i += 3 {
			loHex, hiHex := tokens[i][1], tokens[i+1][1]
			if len(loHex) != 2 || len(hiHex) != 2 {
				return false
			}
			lo, errLo := strconv.ParseUint(loHex, 16, 8)
			hi, errHi := strconv.ParseUint(hiHex, 16, 8)
			if errLo != nil || errHi != nil || hi < lo {
				return false
			}
			if !covered(lo) || !covered(hi) {
				return true
			}
		}
	}
	for _, block := range bfCharBlockPattern.FindAllStringSubmatch(text, -1) {
		tokens := hexTokenPattern.FindAllStringSubmatch(block[1], -1)
		for i := 0; i+2 <= len(tokens); i += 2 {
			srcHex := tokens[i][1]
			if len(srcHex) != 2 {
				return false
			}
			src, err := strconv.ParseUint(srcHex, 16, 8)
			if err != nil {
				return false
			}
			if !covered(src) {
				return true
			}
		}
	}
	return false
}

// pageHasSelfContradictoryCmap reports whether ANY simple font used on
// this page carries a ToUnicode CMap that excludes its own mapped codes
// (see cmapCodespaceExcludesOwnCodes). One such font is enough: the
// page's numbers are typically all set in a single font, so a page can
// look mostly fine while everything that matters for order processing is
// destroyed.
//
// Wrapped in a panic boundary for the same reason
// extractPageTextViaCorrectedCmap is: the vendored library panics rather
// than returning an error when a font resource or stream filter is
// malformed, and a single bad PDF must never take down the process.
func pageHasSelfContradictoryCmap(page pdf.Page) (contradictory bool) {
	defer func() {
		if recover() != nil {
			contradictory = false
		}
	}()
	for _, name := range page.Fonts() {
		text, ok := simpleFontCmapText(page.Font(name))
		if !ok {
			continue
		}
		if cmapCodespaceExcludesOwnCodes(text) {
			return true
		}
	}
	return false
}
