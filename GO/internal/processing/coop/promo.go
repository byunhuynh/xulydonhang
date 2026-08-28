package coop

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// cfMarkerPattern / cmMarkerPattern match a "cf"/"cm" SYSTEM MARKER,
	// not merely the letters. The token must START on a boundary (start
	// of text, or right after something that is neither a letter nor a
	// digit) and must not run straight into another letter — otherwise an
	// ordinary promo description like "chai cao 20cm" would read as a
	// Coopmart marker and silently stop that CTKM from applying to
	// Coopfood. Only a following LETTER is excluded, not a digit: real
	// sheet cells write the glued form ("CF20%") as well as the spaced
	// one ("CF 1+1"), and both are markers.
	cfMarkerPattern   = regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])(cf)([^\p{L}]|$)`)
	cmMarkerPattern   = regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])(cm)([^\p{L}]|$)`)
	parenGroupPattern = regexp.MustCompile(`\(([^)]*)\)`)

	discountPattern     = regexp.MustCompile(`(\d+(?:\.\d+)?)%`)
	braceContentPattern = regexp.MustCompile(`(?s)\{(.*?)\}`)
	moneyKPattern       = regexp.MustCompile(`(?i)\b(\d{1,3})\s*k\b`)
	moneyFullPattern    = regexp.MustCompile(`\b\d{5,6}\b`)
)

// SplitPromoText returns the part of a promo cell that applies to
// `system`. A cell can bundle a Coopmart ("cm...") and a Coopfood
// ("cf...") variant in one string, and the Coop sheet also uses a
// single-system cell (only "cf...", or only "cm...") to mean the CTKM
// belongs to that system ALONE.
//
// The rule, confirmed with the sheet's owner (2026-08-28):
//
//   - no cf/cm marker at all            -> whole text applies to BOTH systems
//   - a marker for this system          -> that system's part
//   - a marker, but not for this system -> "" (CTKM does not apply here)
//
// The last case deliberately DIVERGES from Python's tachkhuyenmai_coop
// (xulydonhang.py:747-781), which ends both branches with `return ... if
// ... else text` — falling back to the WHOLE cell. That fallback is a
// real bug: a cell reading "CF 1+1 | CF 2+1 tặng NLS 1L TP30565"
// (verbatim from the sheet) was handed back intact for a Coopmart order,
// so Coopfood-only promotions were applied to Coopmart, and vice versa.
//
// Column-level scoping is a separate rule handled by ColumnSystem;
// PromoForSystem combines the two.
func SplitPromoText(text, system string) string {
	text = strings.TrimSpace(text)
	system = strings.TrimSpace(system)

	// CF first, preferring a parenthesised variant, mirroring
	// tachkhuyenmai_coop's ordering.
	cfResult := ""
	cfMatchStart := -1
	if start, inner := findParenWithMarker(text, cfMarkerPattern); start >= 0 {
		cfResult = strings.TrimSpace(inner)
		cfMatchStart = start // start of the full "(...)" match, not just the inner group
	} else if i := findMarker(text, cfMarkerPattern); i >= 0 {
		cfResult = strings.TrimSpace(lineFrom(text, i))
		cfMatchStart = i
	}

	cmResult := ""
	if cfResult != "" {
		// CM runs from its own marker up to where CF starts (Python's
		// `(?is)cm.*` over text[:cf_start] — newlines included).
		cmCandidate := text[:cfMatchStart]
		if i := findMarker(cmCandidate, cmMarkerPattern); i >= 0 {
			cmResult = strings.TrimSpace(cmCandidate[i:])
		}
	} else if i := findMarker(text, cmMarkerPattern); i >= 0 {
		cmResult = strings.TrimSpace(lineFrom(text, i))
	}

	switch strings.ToUpper(system) {
	case "COOPMART":
		if cmResult != "" {
			return cmResult
		}
		if cfResult != "" {
			return ""
		}
	case "COOPFOOD":
		if cfResult != "" {
			return cfResult
		}
		if cmResult != "" {
			return ""
		}
	}
	return text
}

// ColumnSystem reports which Coop system a whole promo COLUMN belongs
// to, read from a standalone "CF"/"CM" token in the campaign name that
// precedes the date range — e.g. "CTKM CF", "CNMS CF", "CNMS 11+12 CM",
// "CNMS 14+15 Dành riêng CF" (all verbatim from the real sheet). Such a
// column applies to that system only; the other system skips it
// entirely, whatever its individual cells say.
//
// Returns "" when the name carries no marker (the column applies to both
// systems) and also when it somehow carries both, since there is no
// sensible way to pick one and excluding both would silently drop a live
// campaign.
func ColumnSystem(column string) string {
	hasCF := findMarker(column, cfMarkerPattern) >= 0
	hasCM := findMarker(column, cmMarkerPattern) >= 0
	switch {
	case hasCF && !hasCM:
		return "COOPFOOD"
	case hasCM && !hasCF:
		return "COOPMART"
	}
	return ""
}

// PromoForSystem applies both scoping levels to one promotion and returns
// the text to use. ok=false means this CTKM does not apply to `system` at
// all, and the caller must DROP it from the candidate list rather than
// treat it as an empty promo — an excluded column has to be
// indistinguishable from a column that was never there, or the SKU ends
// up with a blank AQ cell and a spurious price mismatch.
func PromoForSystem(column, value, system string) (string, bool) {
	if scope := ColumnSystem(column); scope != "" && !strings.EqualFold(scope, strings.TrimSpace(system)) {
		return "", false
	}
	applied := SplitPromoText(value, system)
	if strings.TrimSpace(applied) == "" {
		return "", false
	}
	return applied, true
}

// findMarker returns the byte index at which the first system marker
// matched by pattern starts, or -1.
func findMarker(s string, pattern *regexp.Regexp) int {
	loc := pattern.FindStringSubmatchIndex(s)
	if loc == nil {
		return -1
	}
	return loc[4] // start of the "(cf|cm)" group, past any boundary char
}

// findParenWithMarker finds the first "(...)" group whose contents carry
// the given marker, returning the index of its opening parenthesis and
// the inner text, or (-1, "").
func findParenWithMarker(s string, pattern *regexp.Regexp) (int, string) {
	for _, loc := range parenGroupPattern.FindAllStringSubmatchIndex(s, -1) {
		inner := s[loc[2]:loc[3]]
		if findMarker(inner, pattern) >= 0 {
			return loc[0], inner
		}
	}
	return -1, ""
}

// lineFrom returns s from index i up to (not including) the next newline,
// mirroring the `[^\n]*` marker patterns Python used.
func lineFrom(s string, i int) string {
	rest := s[i:]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		return rest[:nl]
	}
	return rest
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
