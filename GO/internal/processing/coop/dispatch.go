package coop

import (
	"regexp"
	"strings"
)

var (
	pom34Pattern         = regexp.MustCompile(`POM34[36]\b`)
	subTotalCountPattern = regexp.MustCompile(`\bSub\s*(?:Total|Tot\s*al)\b`)
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
// the lowercase literal keyword.
func SplitMultiPO(text string) []string {
	var segments []string

	parts := strings.SplitN(text, "Sub Total", 2)
	if len(parts) < 2 {
		return segments
	}
	segments = append(segments, parts[0])
	text = parts[1]

	for containsAny(strings.ToLower(text), "pom343", "pom346") && strings.Contains(text, "Sub Total") {
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

			subParts := strings.SplitN(text, "Sub Total", 2)
			if len(subParts) > 1 {
				segments = append(segments, strings.ToUpper(keyword)+strings.TrimRight(subParts[0], "\n"))
				text = subParts[1]
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

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
