package fujimart

import (
	"regexp"
	"strings"
	"time"
)

// fujimartMojibakeMap decodes this PDF template's legacy 8-bit Vietnamese
// font encoding artifacts (misread as Latin-1/Windows-1252 by this
// repo's PDF text extraction, same as PyMuPDF's) to correct Unicode.
// Verified by running Python's REAL pytesseract OCR output side-by-side
// with the plain text layer across all 15 real FujiMart PDFs available
// during planning, character-aligning every mismatch — zero conflicts
// found across 11 distinct real store branch names. This table covers
// every character actually observed; it is NOT the full legacy font's
// character set. decodeFujimartMojibake (below) passes through any
// unmapped rune unchanged rather than guessing.
var fujimartMojibakeMap = map[rune]rune{
	'§': 'Đ', '©': 'â', 'ª': 'ê', '«': 'ô', '¬': 'ơ',
	'µ': 'à', '¸': 'á', '¹': 'ạ', 'Ç': 'ầ', 'È': 'ẩ',
	'Ô': 'ễ', 'ä': 'ọ', 'ó': 'ú', 'ô': 'ụ', 'ú': 'ỳ',
}

// decodeFujimartMojibake applies fujimartMojibakeMap rune-by-rune. Any
// rune not in the map is passed through unchanged — never guessed. This
// is the ONLY place FujiMart's port needs a decode step; every other
// field either comes from a database lookup (product names via
// timten_sanpham) or is purely numeric/date (unaffected by the font
// encoding issue).
func decodeFujimartMojibake(s string) string {
	var b strings.Builder
	for _, r := range s {
		if mapped, ok := fujimartMojibakeMap[r]; ok {
			b.WriteRune(mapped)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var validDatePattern = regexp.MustCompile(`^\d{2}/\d{2}/\d{4}$`)

// ParseOrderInfo mirrors process_file's FujiMart branch
// (xulydonhang.py:8831-8930), minus the OCR step — see the design spec's
// "Không cần OCR" section for the full verification that storeInfo is
// reconstructable from the plain text layer alone.
//
// entryDate (xulydonhang.py:8853-8857): the line 3 positions BEFORE the
// line containing "Sè §¬n:" — a positional offset within this PDF
// template's fixed "values-then-labels" block, NOT a marker-adjacent
// value. Confirmed identical relative line ordering in both PyMuPDF's
// and this repo's own extractPageTexts output across multiple real
// FujiMart PDFs during planning.
//
// poNumber (xulydonhang.py:8885-8887): the line immediately AFTER the
// line whose content exactly equals the entryDate value.
//
// cancelDate (xulydonhang.py:8859): Python assumes the "Ngµy giao:"
// label and its value sit on the SAME line and splits on the literal
// marker. Confirmed this repo's own extractPageTexts instead puts them
// on two SEPARATE lines for real FujiMart PDFs (same class of layout
// mismatch already fixed for Emart's ParseOrderInfo) — valueAfterMarker
// tolerates BOTH shapes.
//
// Cross-validate/fallback ±2 days (xulydonhang.py:8862-8884): ported
// exactly, no simplification.
//
// storeInfo (xulydonhang.py:8895-8899, via OCR of "Nơi nhận:"): the
// 5-digit store code (the line right after "N¬i nhËn:") + " " + the
// line starting with "FujiMart " (decoded via decodeFujimartMojibake).
// Best-effort — matches Python's tenstore defaulting to "" when its OCR
// regex doesn't match (xulydonhang.py:8895) — does NOT gate ok.
//
// ok=false when poNumber, entryDate, or cancelDate fails to resolve to a
// real value (including the "Không tìm thấy" fallback-exhausted case) —
// Python's real code would carry an undefined/garbage value into
// several downstream operations in that case (entry_date could even be
// a genuine NameError, xulydonhang.py:8857's print(entry_date) if the
// marker line is never found at all), which this port treats as a clean
// failure instead, per this codebase's established policy.
func ParseOrderInfo(text string) (poNumber, entryDate, cancelDate, storeInfo string, ok bool) {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			lines = append(lines, t)
		}
	}

	for i, l := range lines {
		if strings.Contains(l, "Sè §¬n:") && i >= 3 {
			entryDate = lines[i-3]
			break
		}
	}

	for i, l := range lines {
		if l == entryDate && i+1 < len(lines) {
			poNumber = lines[i+1]
			break
		}
	}

	cancelDate = valueAfterMarker(lines, "Ngµy giao:")

	if !validDatePattern.MatchString(cancelDate) {
		cancelDate = "Không tìm thấy"
		if validDatePattern.MatchString(entryDate) {
			if t, err := time.Parse("02/01/2006", entryDate); err == nil {
				cancelDate = t.AddDate(0, 0, 2).Format("02/01/2006")
			}
		}
	}
	if !validDatePattern.MatchString(entryDate) {
		entryDate = "Không tìm thấy"
		if validDatePattern.MatchString(cancelDate) {
			if t, err := time.Parse("02/01/2006", cancelDate); err == nil {
				entryDate = t.AddDate(0, 0, -2).Format("02/01/2006")
			}
		}
	}

	storeCode := ""
	for i, l := range lines {
		if strings.Contains(l, "N¬i nhËn:") && i+1 < len(lines) {
			storeCode = lines[i+1]
			break
		}
	}
	branchLine := ""
	for _, l := range lines {
		if strings.HasPrefix(l, "FujiMart ") {
			branchLine = l
			break
		}
	}
	if storeCode != "" && branchLine != "" {
		storeInfo = storeCode + " " + decodeFujimartMojibake(branchLine)
	}

	ok = poNumber != "" &&
		entryDate != "" && entryDate != "Không tìm thấy" &&
		cancelDate != "" && cancelDate != "Không tìm thấy"
	return poNumber, entryDate, cancelDate, storeInfo, ok
}

// valueAfterMarker finds the line containing marker, then returns either
// the remainder of that same line (after the marker text, trimmed) if
// non-empty, or the next line if the marker line has nothing left after
// it — tolerating both the same-line layout Python's own split-based
// extraction assumes and the two-line layout this repo's actual Go PDF
// extraction produces for real FujiMart PDFs.
func valueAfterMarker(lines []string, marker string) string {
	for i, l := range lines {
		idx := strings.Index(l, marker)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(l[idx+len(marker):])
		if rest != "" {
			return rest
		}
		if i+1 < len(lines) {
			return lines[i+1]
		}
		return ""
	}
	return ""
}
