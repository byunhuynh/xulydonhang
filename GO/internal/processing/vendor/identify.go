package vendor

import (
	"regexp"
	"strings"
)

var (
	whitespacePattern = regexp.MustCompile(`\s+`)
	// Coop's two internal vendor IDs, exactly as in
	// xulydonhang.py:identify_vendor's first branch (checked first
	// because it's checked first there — order matters for the other
	// ~18 vendor branches that exist in Python but aren't ported here).
	coopPattern = regexp.MustCompile(`Vendor\s*[-:]\s*(21569|22856)`)
	// BigC's identify pattern (xulydonhang.py:99): either a literal
	// "3005382" substring, or "CTY TNHH DV EB" case-insensitive, in the
	// whitespace-normalized page text. Confirmed on real BigC PDFs that
	// "CTY TNHH DV EB" alone appears on EVERY page (not just page 0) —
	// "3005382" is the page-0-exclusive one. Both are checked via one
	// combined regex here, matching Python's `or`.
	bigcPattern = regexp.MustCompile(`(?i)3005382|CTY TNHH DV EB`)
	// Lotte's two tax-ID forms, mirroring identify_vendor's second
	// branch (xulydonhang.py:102-103): either of two ID pairs appearing
	// anywhere in the (whitespace-normalized) page text.
	lottePattern = regexp.MustCompile(`0107889783\s*009333|1102018142\s*010544`)
	// Satra's two VD-code forms, mirroring identify_vendor's third
	// branch (xulydonhang.py:105-109): either literal substring
	// appearing anywhere in the (whitespace-normalized) page text.
	satraPattern = regexp.MustCompile(`VD-00002345|VD-00002547`)
	// Winmart's identify pattern (xulydonhang.py:121-122): a single
	// literal regex against the whitespace-normalized page text, no
	// alternation, no case-insensitivity flag in Python (the supplier
	// code string itself is fixed, so case sensitivity is moot).
	winmartPattern = regexp.MustCompile(`Nhà cung cấp \(Supplier\): 0002011398`)
)

// Identify tries to recognize which retail vendor produced this
// page/PO text, mirroring xulydonhang.py's identify_vendor. Coop, BigC,
// Lotte, Satra, and Winmart are implemented in that order (order is load-bearing and
// mirrors Python's real identify_vendor precedence). Python's real order has several
// more vendors between Satra and Winmart (Emart, Kingfood, CN-HCM) that aren't ported
// yet — a future implementer adding one of those must insert it at the correct relative
// position, not simply append. Identify returns "" for anything that isn't one of the
// five implemented vendors. Future vendor additions must insert their case at the correct
// position in this sequence, not simply append.
func Identify(text string) string {
	cleaned := strings.TrimSpace(whitespacePattern.ReplaceAllString(text, " "))
	if coopPattern.MatchString(cleaned) {
		return "Coop"
	}
	if bigcPattern.MatchString(cleaned) {
		return "BigC"
	}
	if lottePattern.MatchString(cleaned) {
		return "Lotte"
	}
	if satraPattern.MatchString(cleaned) {
		return "Satra"
	}
	if winmartPattern.MatchString(cleaned) {
		return "Winmart"
	}
	return ""
}
