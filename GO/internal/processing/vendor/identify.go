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
	// Lotte's two tax-ID forms, mirroring identify_vendor's second
	// branch (xulydonhang.py:102-103): either of two ID pairs appearing
	// anywhere in the (whitespace-normalized) page text.
	lottePattern = regexp.MustCompile(`0107889783\s*009333|1102018142\s*010544`)
	// Satra's two VD-code forms, mirroring identify_vendor's third
	// branch (xulydonhang.py:105-109): either literal substring
	// appearing anywhere in the (whitespace-normalized) page text.
	satraPattern = regexp.MustCompile(`VD-00002345|VD-00002547`)
)

// Identify tries to recognize which retail vendor produced this
// page/PO text, mirroring xulydonhang.py's identify_vendor. Coop, Lotte,
// and Satra are implemented; every other vendor is a later phase's work, so
// Identify returns "" for anything that isn't one of those three.
func Identify(text string) string {
	cleaned := strings.TrimSpace(whitespacePattern.ReplaceAllString(text, " "))
	if coopPattern.MatchString(cleaned) {
		return "Coop"
	}
	if lottePattern.MatchString(cleaned) {
		return "Lotte"
	}
	if satraPattern.MatchString(cleaned) {
		return "Satra"
	}
	return ""
}
