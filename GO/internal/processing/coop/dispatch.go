package coop

import (
	"regexp"
	"strings"
)

// pom34Pattern and subTotalCountPattern add character-by-character
// whitespace tolerance (via spacedPattern, already used for
// P/O-Number-style fields in invoice.go) on top of a faithful port of
// demsodonhang1trang_coop's two literal regexes. This is NOT a
// behavioral deviation from the Python original — PyMuPDF's text
// extraction never introduces gaps between adjacent letters of a
// single word for these PDFs, so Python's un-tolerant literals always
// matched real "POM343"/"Sub Total" text as intact substrings. But this
// Go port's PDF library sometimes can't fully reconstruct the original
// letter-spacing for certain field labels (confirmed against a real
// archived PDF, 103311304-00: "Sub Total" extracts as "S u b   T o t a
// l", a pure text-extraction-fidelity gap, not a difference in what the
// PDF actually contains) — spacedPattern's \s* between every letter is
// purely additive tolerance that still requires the same letters in
// the same order, so it can only recognize genuine
// "POM343"/"POM346"/"Sub Total" occurrences the original, stricter
// pattern would also have recognized on cleanly-extracted text; it
// cannot introduce a false match that wasn't already the real word.
var (
	pom34Pattern         = regexp.MustCompile(spacedPattern("POM34") + `\s*[36]\b`)
	subTotalCountPattern = regexp.MustCompile(`\b` + spacedPattern("Sub Total") + `\b`)
)

// PageCounts mirrors demsodonhang1trang_coop's returned dict.
type PageCounts struct {
	POM343   int
	SubTotal int
}

// CountPOsOnPage mirrors demsodonhang1trang_coop.
func CountPOsOnPage(text string) PageCounts {
	text = strings.NewReplacer(" ", " ", "pom343", "POM343", "pom346", "POM346").Replace(text)
	return PageCounts{
		POM343:   len(pom34Pattern.FindAllString(text, -1)),
		SubTotal: len(subTotalCountPattern.FindAllString(text, -1)),
	}
}

// SplitMultiPO mirrors catdonra_nhieutrang exactly, including its
// latent case-sensitivity bug (see this plan's Global Constraints): the
// membership check (`keyword in text.lower()`) is case-insensitive, but
// the actual split (`text.split(keyword, 1)`) is case-sensitive against
// the lowercase literal keyword. That preserved bug is only about the
// POM keyword; the "Sub Total" boundary below uses subTotalCountPattern
// (the same whitespace-tolerant matcher CountPOsOnPage uses) instead of
// a literal split, so a page whose extraction shreds "Sub Total" into
// "S u b   T o t a l" — which CountPOsOnPage already tolerates — doesn't
// silently produce zero segments here despite the counts matching.
func SplitMultiPO(text string) []string {
	var segments []string

	before, after, ok := splitOnSubTotal(text)
	if !ok {
		return segments
	}
	segments = append(segments, before)
	text = after

	for containsAny(strings.ToLower(text), "pom343", "pom346") && subTotalCountPattern.MatchString(text) {
		found := false
		for _, keyword := range []string{"pom343", "pom346"} {
			if !strings.Contains(strings.ToLower(text), keyword) {
				continue
			}
			// Case-sensitive on purpose — see the doc comment above.
			splitParts := strings.SplitN(text, keyword, 2)
			if len(splitParts) < 2 {
				// Case-sensitive split failed (bug preserved)
				continue
			}
			found = true
			text = splitParts[1]

			subBefore, subAfter, subOK := splitOnSubTotal(text)
			if subOK {
				segments = append(segments, strings.ToUpper(keyword)+strings.TrimRight(subBefore, "\n"))
				text = subAfter
			} else {
				segments = append(segments, strings.ToUpper(keyword)+text)
				return segments
			}
			break
		}
		if !found {
			// No keyword matched case-sensitively, bug prevents further processing
			break
		}
	}

	return segments
}

// splitOnSubTotal finds the first whitespace-tolerant "Sub Total"
// boundary (per subTotalCountPattern) and returns the text before and
// after it. Mirrors strings.SplitN(text, "Sub Total", 2) but tolerant
// of character-shredded extraction the same way CountPOsOnPage is.
func splitOnSubTotal(text string) (before, after string, ok bool) {
	loc := subTotalCountPattern.FindStringIndex(text)
	if loc == nil {
		return "", "", false
	}
	return text[:loc[0]], text[loc[1]:], true
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
