package processing

import (
	"fmt"
	"math"

	"github.com/ledongthuc/pdf"
)

// yBreakEpsilon is the minimum change in the text matrix's Y translation
// (in PDF points) between two consecutive text-showing operators that
// extractWinmartPageText treats as evidence of a real new visual line —
// calibrated against 4194002858.pdf's real "Ghi chú" note field, where
// consecutive Tm calls for its two wrapped sub-lines are 8pt apart
// (555.07 then 547.07, see extractWinmartPageText's doc comment), a
// typical single line-height step for this generator's 8pt body font.
// A small epsilon (well under one line height) distinguishes a real new
// line from any floating-point noise in repeated same-line Tm calls.
const yBreakEpsilon = 1.0

// extractWinmartPageText re-extracts a single page's text with the same
// reading order as the shared extractPageText/GetPlainText path, but
// additionally inserts a line break whenever the text matrix's Y
// translation changes between two consecutive text-showing operators
// with NO intervening BT/T* (the only two operators GetPlainText itself
// reacts to for line breaks — see the vendored
// github.com/ledongthuc/pdf page.go:557-607 GetPlainText, which does not
// track Tm/Td/TD positioning at all).
//
// Root cause (confirmed directly against 4194002858.pdf's raw content
// stream via Interpret): most fields in this PDF generator's real
// output are each their own BT...ET text object, so GetPlainText's
// "case BT: showText(\"\\n\")" already inserts a correct line break
// between every field — this is why, for example, "Số đơn hàng (PO
// No.)" and its value "4194002858" already come out on separate lines
// today (verified: both sit at the SAME Y=639.07 but in DIFFERENT
// BT/ET blocks, so the existing BT-triggered break already handles
// this case correctly and this function must not change it). But the
// "Ghi chú" (note) field's text — which wraps across 2-3 sub-lines
// when it doesn't fit its allotted width — is emitted as MULTIPLE Tj
// calls inside a SINGLE BT...ET block, using a bare "Tm" (not T*) to
// reposition between each wrapped sub-line: confirmed via direct
// content-stream trace: "Tm[1 0 0 1 446 555.07] Tf Tj('Nguyễn Quang')
// Tm[1 0 0 1 446 547.07] Tj('Phi_0396035541_phinq@winmart.m...')" with
// NO BT or T* between the two Tj calls. GetPlainText has no way to
// detect this Tm-only repositioning at all, so it silently concatenates
// the two runs with zero separator, producing
// "Nguyễn QuangPhi_0396035541_phinq@winmart.masangroup.com..." instead
// of two separate lines — which, in turn, made
// winmart.ParseOrderInfo's note (ghichu) extraction, which relies on
// splitlines()-based line counting exactly mirroring Python's
// xulydonhang.py:8994-9000, drop the entire note (Python's real
// PyMuPDF-based extraction DOES treat these as separate lines, matching
// the real frozen fixtures' L/S column values). The same root cause
// (missing separator at a same-BT/ET Tm-only line wrap) also explains
// the "CátHải"/"Cát Hải" and similar E-column ship-to-address
// word-gap mismatches seen across every real fixture — the delivery
// address's wrapped lines hit the identical pattern.
//
// This is intentionally NOT a wholesale swap to position-sorted
// reconstruction (compare reconstructLinesFromContent, used only as a
// last-resort fallback for a handful of archived Coop PDFs whose
// GetPlainText output has almost no newlines at all): that Y-bucketing
// approach was tried against this exact PDF during investigation and
// found to scramble this generator's real two-column header layout
// (Billing Information column interleaved with Order Information
// column when sorted purely by Y), which would break
// winmart.ParseOrderInfo's marker-line-then-next-line field scanning
// entirely. This function instead walks the SAME content-stream
// operator sequence, in the SAME order, as GetPlainText — it only adds
// one extra signal (a same-BT/ET Y jump) that GetPlainText's own
// BT/T*-only heuristic happens to miss, so multi-column reading order
// is preserved exactly as already-working.
//
// Scoped to Winmart only (not merged into the shared extractPageText
// used by every vendor's dispatch loop and vendor.Identify): a change
// to line-break heuristics in code shared by Coop/Lotte/Satra/BigC
// risks regressing their own already-passing golden fixtures (155+60+
// 36+29 real fixtures respectively) for a generator-specific quirk
// confirmed only in real Winmart PDFs.
func extractWinmartPageText(page pdf.Page) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = ""
			err = fmt.Errorf("%v", r)
		}
	}()

	if page.V.IsNull() || page.V.Key("Contents").Kind() == pdf.Null {
		return "", nil
	}
	strm := page.V.Key("Contents")
	var enc pdf.TextEncoding = nopEncoderInstance()

	fonts := make(map[string]*pdf.Font)
	for _, name := range page.Fonts() {
		f := page.Font(name)
		fonts[name] = &f
	}

	var textBuilder []byte
	appendString := func(s string) {
		textBuilder = append(textBuilder, s...)
	}
	appendEncoded := func(s string) {
		textBuilder = append(textBuilder, enc.Decode(s)...)
	}

	haveY := false
	lastY := 0.0
	justBroke := true // true right after a break was already inserted (BT/T*/Tm-jump), so a second Tm at the same spot doesn't double it

	pdf.Interpret(strm, func(stk *pdf.Stack, op string) {
		n := stk.Len()
		args := make([]pdf.Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}

		switch op {
		case "BT":
			appendString("\n")
			justBroke = true
		case "T*":
			appendEncoded("\n")
			justBroke = true
		case "Tm":
			// Tm sets the text matrix directly; args[5] is its Y
			// translation component (the 6-number [a b c d e f] form,
			// matching the vendored library's own Tm handling in both
			// GetPlainText's sibling Content()/walkTextBlocks methods).
			if len(args) == 6 {
				ty := args[5].Float64()
				if haveY && !justBroke && math.Abs(ty-lastY) > yBreakEpsilon {
					appendString("\n")
					justBroke = true
				}
				lastY = ty
				haveY = true
			}
		case "Td", "TD":
			// Relative move: approximate the new absolute Y as
			// lastY+ty (correct for the common non-rotated case these
			// business-document generators use; see this function's
			// doc comment for the scope of what's handled).
			if len(args) == 2 {
				ty := args[1].Float64()
				newY := lastY + ty
				if haveY && !justBroke && math.Abs(newY-lastY) > yBreakEpsilon {
					appendString("\n")
					justBroke = true
				}
				lastY = newY
				haveY = true
			}
		case "Tf":
			// Deliberate fix vs. the vendored library: GetPlainText
			// assigns font.Encoder() into its encoder variable
			// unconditionally, even when Encoder() returns nil (page.go
			// ~line 578) — this guards against that and falls back to
			// the passthrough nopEncoder instead, the same way this
			// function already does for an unrecognized font name.
			if len(args) == 2 {
				if font, ok := fonts[args[0].Name()]; ok {
					if e := font.Encoder(); e != nil {
						enc = e
					} else {
						enc = nopEncoderInstance()
					}
				} else {
					enc = nopEncoderInstance()
				}
			}
		case "\"":
			// The vendored library's own GetPlainText has a bug here: its
			// case "\"" falls through into case "'"'s len(args) != 1
			// check, so it panics on every valid 3-operand '"' call. This
			// reimplementation fixes that by handling '"' with its own
			// 3-arg check and using args[2] (the string operand — '"'
			// syntax is `aw ac string "`), matching the PDF spec.
			if len(args) != 3 {
				panic("bad \" operator")
			}
			appendEncoded(args[2].RawString())
			justBroke = false
		case "'":
			if len(args) != 1 {
				panic("bad ' operator")
			}
			appendEncoded(args[0].RawString())
			justBroke = false
		case "Tj":
			if len(args) != 1 {
				panic("bad Tj operator")
			}
			appendEncoded(args[0].RawString())
			justBroke = false
		case "TJ":
			if n == 0 {
				panic("bad TJ operator")
			}
			v := args[0]
			for i := 0; i < v.Len(); i++ {
				x := v.Index(i)
				if x.Kind() == pdf.String {
					appendEncoded(x.RawString())
				}
			}
			justBroke = false
		}
	})
	return string(textBuilder), nil
}

// extractWinmartPageTextFromFile reopens filePath and re-extracts a
// single page's text via extractWinmartPageText. pageIdx is 0-based and
// must be the REAL, uncompacted PDF page number minus one — NOT the
// shared dispatch loop's pageTexts index. extractPageTexts (in
// pdfextract.go) skips null pages without appending a placeholder, so
// its returned pageTexts slice is compacted; coop_processor.go's Process
// passes extractPageTexts's parallel pageNumbers slice
// (pageNumbers[pageIdx]-1) here for exactly this reason. Winmart pages
// are already re-parsed a second time on purpose here (the shared
// extractPageTexts pass, used for vendor.Identify and every other
// vendor, stays exactly as-is) — see extractWinmartPageText's own doc
// comment for why this isn't instead folded into that shared pass.
func extractWinmartPageTextFromFile(filePath string, pageIdx int) (string, error) {
	file, r, err := pdfOpen(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if pageIdx < 0 || pageIdx >= r.NumPage() {
		return "", fmt.Errorf("trang %d không tồn tại", pageIdx+1)
	}
	page := r.Page(pageIdx + 1)
	if page.V.IsNull() {
		return "", fmt.Errorf("trang %d rỗng", pageIdx+1)
	}
	return extractWinmartPageText(page)
}

// nopEncoderInstance returns a TextEncoding that passes bytes through
// unchanged — matches the vendored library's own unexported nopEncoder
// used as GetPlainText's initial/fallback encoder (page.go:534,580),
// reimplemented here since that type isn't exported.
func nopEncoderInstance() pdf.TextEncoding {
	return nopEncoder{}
}

type nopEncoder struct{}

func (nopEncoder) Decode(raw string) string {
	return raw
}
