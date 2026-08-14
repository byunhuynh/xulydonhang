package coop

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	cfParenPattern      = regexp.MustCompile(`(?i)\(([^)]*cf[^)]*)\)`)
	cfLinePattern       = regexp.MustCompile(`(?i)cf[^\n]*`)
	cmLinePattern       = regexp.MustCompile(`(?i)cm[^\n]*`)
	cmFromPattern       = regexp.MustCompile(`(?is)cm.*`)
	discountPattern     = regexp.MustCompile(`(\d+(?:\.\d+)?)%`)
	braceContentPattern = regexp.MustCompile(`(?s)\{(.*?)\}`)
	moneyKPattern       = regexp.MustCompile(`(?i)\b(\d{1,3})\s*k\b`)
	moneyFullPattern    = regexp.MustCompile(`\b\d{5,6}\b`)
)

// SplitPromoText mirrors tachkhuyenmai_coop: a promo-text cell can
// bundle both a Coopmart ("cm...") and Coopfood ("cf...") variant in
// one string; returns whichever matches `system`, or text unchanged
// for any other system.
func SplitPromoText(text, system string) string {
	text = strings.TrimSpace(text)
	system = strings.TrimSpace(system)

	cfResult := ""
	cfMatchStart := -1
	if loc := cfParenPattern.FindStringSubmatchIndex(text); loc != nil {
		cfResult = strings.TrimSpace(text[loc[2]:loc[3]])
		cfMatchStart = loc[0] // start of the full "(...)" match, not just the inner group
	} else if loc := cfLinePattern.FindStringIndex(text); loc != nil {
		cfResult = strings.TrimSpace(text[loc[0]:loc[1]])
		cfMatchStart = loc[0]
	}

	cmResult := ""
	if cfResult != "" {
		if cfMatchStart >= 0 {
			cmCandidate := text[:cfMatchStart]
			if m := cmFromPattern.FindString(cmCandidate); m != "" {
				cmResult = strings.TrimSpace(m)
			}
		}
	} else if m := cmLinePattern.FindString(text); m != "" {
		cmResult = strings.TrimSpace(m)
	}

	switch strings.ToUpper(system) {
	case "COOPMART":
		if cmResult != "" {
			return cmResult
		}
	case "COOPFOOD":
		if cfResult != "" {
			return cfResult
		}
	default:
		return text
	}
	return text
}

// ExtractDiscount mirrors extract_discount.
func ExtractDiscount(value string) float64 {
	m := discountPattern.FindStringSubmatch(value)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	return v
}

// ExtractBraceContent mirrors laycachbo_khuyenmai.
func ExtractBraceContent(value string) string {
	m := braceContentPattern.FindStringSubmatch(value)
	if m == nil {
		return ""
	}
	return m[1]
}

// ExtractMoneyAmount mirrors tachtien_khuyenmai: "199k"/"199 K" -> 199000,
// or a bare 5-6 digit number as itself.
func ExtractMoneyAmount(text string) (int, bool) {
	if m := moneyKPattern.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n * 1000, true
		}
	}
	if m := moneyFullPattern.FindString(text); m != "" {
		if n, err := strconv.Atoi(m); err == nil {
			return n, true
		}
	}
	return 0, false
}

// LastFourDigits mirrors layduoi_mahang: text before the first "_",
// last 4 runes of that (or the whole thing if shorter).
func LastFourDigits(text string) string {
	base := strings.SplitN(text, "_", 2)[0]
	runes := []rune(base)
	if len(runes) <= 4 {
		return base
	}
	return string(runes[len(runes)-4:])
}

// FormatWeightKg mirrors format_weight_kg: < 1000kg shows kg,
// >= 1000kg converts to tấn, both rounded to 2 decimals.
func FormatWeightKg(kg float64) string {
	if kg >= 1000 {
		return fmt.Sprintf("%s tấn", trimFloat(kg/1000, 2))
	}
	return fmt.Sprintf("%s kg", trimFloat(kg, 2))
}

// trimFloat mirrors Python's f"{round(value, decimals)}": round(x, 2)
// returns a float, and Python's default float-to-str always shows a
// fractional part with at least one digit (round(70.0, 2) -> 70.0 ->
// "70.0", never "70"). Format to `decimals` places, trim trailing
// zeros, but always leave exactly one digit after the decimal point —
// unlike a bare TrimRight(s, "0.") which would strip a whole-number
// result ("70.00") down to "70" and silently drop the fractional part
// real fixtures always show (e.g. "COOPFOOD PO... (Tổng trọng lượng:
// 70.0 kg)", not "70 kg").
func trimFloat(v float64, decimals int) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return s
	}
	end := len(s)
	for end > dot+2 && s[end-1] == '0' {
		end--
	}
	return s[:end]
}
