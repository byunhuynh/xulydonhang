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
	// Emart's identify pattern (xulydonhang.py:111-112): either a literal
	// ASCII company-name substring, or "THISO RETAIL COMPANY LIMITED"
	// (the actual PO issuer's letterhead name), in the whitespace-
	// normalized page text. Confirmed on a real sample (4501866956.PDF):
	// the real PDF text uses the ACCENTED Vietnamese form of the first
	// company name ("CÔNG TY TNHH TMDV XNK HÀ THÀNH  (101017)"), so
	// Python's plain-ASCII first alternative never actually matches real
	// PDFs — only "THISO RETAIL COMPANY LIMITED" does the real work.
	// Both are mirrored here for fidelity with Python (the ASCII form
	// costs nothing to keep and guards against a future PDF that happens
	// to use it).
	emartPattern = regexp.MustCompile(`CONG TY TNHH TMDV XNK HA THANH \(101017\)|THISO RETAIL COMPANY LIMITED`)
	// Kingfood's identify pattern (xulydonhang.py:114-115): a single
	// literal numeric substring (the vendor's own tax code), no
	// alternation. Real Python order places Kingfood immediately after
	// Emart and before CN-HCM (unported)/Winmart — see Identify's own
	// doc comment for the full chain.
	kingfoodPattern = regexp.MustCompile(`0313403198`)
	// Winmart's identify pattern (xulydonhang.py:121-122): a single
	// literal regex against the whitespace-normalized page text, no
	// alternation, no case-insensitivity flag in Python (the supplier
	// code string itself is fixed, so case sensitivity is moot).
	winmartPattern = regexp.MustCompile(`Nhà cung cấp \(Supplier\): 0002011398`)
	// FujiMart's identify pattern (xulydonhang.py:128-129): a single
	// literal numeric substring (the vendor's own tax code), no
	// alternation.
	fujimartPattern = regexp.MustCompile(`251000000161`)
)

// Identify tries to recognize which retail vendor produced this
// page/PO text, mirroring xulydonhang.py's identify_vendor. Coop, BigC,
// Lotte, Satra, Emart, Kingfood, Winmart, and FujiMart are implemented in
// that order (order is load-bearing and mirrors Python's real
// identify_vendor precedence). Python's real order still has CN-HCM between
// Kingfood and Winmart, and SHOPEE-CHOICE between Winmart and FujiMart,
// that aren't ported yet — a future implementer adding one of those must
// insert it at the correct relative position, not simply append. Identify
// returns "" for anything that isn't one of the eight implemented vendors.
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
	if emartPattern.MatchString(cleaned) {
		return "Emart"
	}
	if kingfoodPattern.MatchString(cleaned) {
		return "Kingfood"
	}
	if winmartPattern.MatchString(cleaned) {
		return "Winmart"
	}
	if fujimartPattern.MatchString(cleaned) {
		return "FujiMart"
	}
	return ""
}
