package coop

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const notFound = "Không tìm thấy"

var whitespacePattern = regexp.MustCompile(`\s+`)

// StripAllWhitespace mirrors xoakhoangtrang: removes ALL whitespace
// (not just collapsing runs) from text.
func StripAllWhitespace(text string) string {
	return whitespacePattern.ReplaceAllString(text, "")
}

var firstTokenPattern = regexp.MustCompile(`^\s*(\S+)(?:\s+(.*))?$`)

// RemoveLineNumbers mirrors remove_line_numbers: strips a leading "1"
// through "10" token (and the whitespace after it) from each line,
// dropping lines that contain only such a token.
func RemoveLineNumbers(text string) string {
	validNumbers := map[string]bool{}
	for i := 1; i <= 10; i++ {
		validNumbers[fmt.Sprintf("%d", i)] = true
	}

	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			cleaned = append(cleaned, line)
			continue
		}
		m := firstTokenPattern.FindStringSubmatch(line)
		if m == nil {
			cleaned = append(cleaned, line)
			continue
		}
		first, rest := m[1], m[2]
		if validNumbers[first] {
			if rest != "" {
				cleaned = append(cleaned, rest)
			}
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, "\n")
}

// spacedPattern builds a regex fragment matching literal char-by-char,
// tolerating arbitrary whitespace between every pair of characters. Any
// whitespace already present in literal (e.g. the space in "P/O Number")
// is dropped rather than emitted as a required literal character: the
// text this matches against may have that whitespace fully stripped
// (see ParseInvoiceInfo's StripAllWhitespace step), untouched, or split
// arbitrarily by PDF extraction, and in every case a bare \s* between
// the surrounding non-space characters covers it (matching zero, one,
// or many whitespace characters as needed).
func spacedPattern(literal string) string {
	var b strings.Builder
	first := true
	for _, r := range literal {
		if unicode.IsSpace(r) {
			continue
		}
		if !first {
			b.WriteString(`\s*`)
		}
		b.WriteString(regexp.QuoteMeta(string(r)))
		first = false
	}
	return b.String()
}

var (
	poNumberPattern      = regexp.MustCompile(spacedPattern("P/O Number") + `\s*[:-]?\s*([\d-]+)`)
	poLocationPattern    = regexp.MustCompile(spacedPattern("P/O Location") + `\s*:\s*(\d+)`)
	entryDatePattern     = regexp.MustCompile(spacedPattern("Entry Date") + `\s*-\s*([\d/]+)`)
	cancelDatePattern    = regexp.MustCompile(`Cancel\s*Date-\s*([\d/]+)`)
	storeLocationPattern = regexp.MustCompile(`Store-\s*(\d+)`)
)

// InvoiceInfo mirrors extract_info's returned dict.
type InvoiceInfo struct {
	PONumber   string
	POLocation string
	EntryDate  string
	CancelDate string
}

// ParseInvoiceInfo mirrors xulydonhang.py's extract_info. Note that,
// like the Python original, it strips ALL whitespace from the text
// (not just collapsing it) before matching — the `\s*` in every pattern
// above then matches zero characters, so this still works whether the
// source PDF text had normal spacing, character-by-character spacing,
// or none at all.
func ParseInvoiceInfo(text string) InvoiceInfo {
	text = RemoveLineNumbers(text)
	text = whitespacePattern.ReplaceAllString(text, " ")
	text = StripAllWhitespace(text)
	if idx := strings.Index(text, "Currency"); idx >= 0 {
		text = text[:idx]
	}

	info := InvoiceInfo{
		PONumber:   matchGroup(poNumberPattern, text),
		POLocation: matchGroup(poLocationPattern, text),
		EntryDate:  matchGroup(entryDatePattern, text),
		CancelDate: matchGroup(cancelDatePattern, text),
	}
	if info.POLocation == "" {
		info.POLocation = matchGroup(storeLocationPattern, text)
	}

	if info.PONumber == "" {
		info.PONumber = notFound
	}
	if info.POLocation == "" {
		info.POLocation = notFound
	}
	if info.EntryDate == "" {
		info.EntryDate = notFound
	}
	if info.CancelDate == "" {
		info.CancelDate = notFound
	}
	return info
}

func matchGroup(pattern *regexp.Regexp, text string) string {
	m := pattern.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return m[1]
}

// ConvertDateFormat mirrors process_coop_invoice's inline
// convert_date_format: "dd/mm/yy" -> "dd/mm/yyyy". Uses the unpadded
// reference layout "2/1/06" (not "02/01/06"): Python's
// datetime.strptime(date_str, "%d/%m/%y") accepts both zero-padded and
// single-digit day/month components, and real archived Coop PDFs
// contain both ("23/07/26" and "3/10/26" both occur) — go's time.Parse
// requires an exact digit-count match against "02"/"01" placeholders,
// so a strict zero-padded layout wrongly rejected every unpadded date
// as "Không hợp lệ" where Python parsed it fine. "2/1/06" parses both
// forms identically (confirmed: "3/10/26", "03/10/26" and "23/07/26"
// all parse correctly against it).
func ConvertDateFormat(dateStr string) string {
	if dateStr == "" || dateStr == notFound {
		return notFound
	}
	t, err := time.Parse("2/1/06", dateStr)
	if err != nil {
		return "Không hợp lệ"
	}
	return t.Format("02/01/2006")
}

// ResolveCancelDate mirrors "if cancle_date == Không tìm thấy: entry_date + 65 days".
func ResolveCancelDate(entryDate, cancelDate string) (string, error) {
	if cancelDate != notFound {
		return cancelDate, nil
	}
	if entryDate == notFound {
		return notFound, nil
	}
	t, err := time.Parse("02/01/2006", entryDate)
	if err != nil {
		return "", fmt.Errorf("coop: entry date không hợp lệ: %q", entryDate)
	}
	return t.AddDate(0, 0, 65).Format("02/01/2006"), nil
}

var (
	notesPattern         = regexp.MustCompile(`(?is)` + spacedPattern("Notes") + `\s*-\s*(.*?)\s*FOB`)
	notesCleanupPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)` + spacedPattern("Xin vui long kem DDH khi giao hang")),
		regexp.MustCompile(`(?i)\*\s*=\s*` + spacedPattern("This SKU Discounted")),
		regexp.MustCompile(`(?i)` + spacedPattern("Mot Hoa Don chi xuat cho mot PO")),
		regexp.MustCompile(`(?i)` + spacedPattern("mua1TANG1CUNGLOAI")),
		regexp.MustCompile(`(?i)` + spacedPattern("1TANG1CUNGLOAI")),
	}
)

// ExtractNotes mirrors the "Notes-...FOB" extraction + boilerplate
// stripping + word-dedup block inside process_coop_invoice.
func ExtractNotes(text string) string {
	notes := ""
	if m := notesPattern.FindStringSubmatch(text); m != nil {
		notes = strings.TrimSpace(m[1])
	}
	for _, p := range notesCleanupPatterns {
		notes = p.ReplaceAllString(notes, "")
	}
	notes = strings.TrimSpace(whitespacePattern.ReplaceAllString(notes, " "))

	words := strings.Fields(notes)
	seen := make(map[string]bool, len(words))
	var deduped []string
	for _, w := range words {
		if !seen[w] {
			seen[w] = true
			deduped = append(deduped, w)
		}
	}
	return strings.Join(deduped, " ")
}

// shipToStatusPattern and shipToStorePattern mirror process_coop_invoice's
// two Ship To regexes (xulydonhang.py:5407,5415) verbatim, INCLUDING the
// `\s*` the Python literals put between the word and the following
// literal "-" (Python: r"...Status\s*-\s*\d+..." / r"...Store\s*-\s*...").
// A first Go port dropped that `\s*` (spacedPattern only inserts \s*
// *between* a word's own letters, not after the whole word), so it
// required "Status-"/"Store-" with zero space before the hyphen — but
// real archived PDFs render "Status - 3 RELEASED" / normalize to
// "Contact -" with a space before the hyphen, which never matched,
// silently falling through to ExtractShipTo's "" default for every
// order. Confirmed against real extracted text via a throwaway debug
// probe before fixing.
var (
	shipToStatusPattern = regexp.MustCompile(`(?is)` + spacedPattern("Ship To") + `:\s*` + spacedPattern("Status") + `\s*-\s*\d+\s*` + spacedPattern("RELEASED") + `\s*(.*?)\s*` + spacedPattern("Contact") + `\s*-`)
	shipToStorePattern  = regexp.MustCompile(`(?is)` + spacedPattern("Store") + `\s*-\s*(.*?)\s*` + spacedPattern("Vendor"))
)

// ExtractShipTo mirrors process_coop_invoice's Ship To extraction: try
// "Ship To: Status-... RELEASED ... Contact-" first, then "Store- ...
// Vendor", then "" if neither matches.
func ExtractShipTo(text string) string {
	normalized := whitespacePattern.ReplaceAllString(text, " ")
	if m := shipToStatusPattern.FindStringSubmatch(normalized); m != nil {
		return strings.TrimSpace(m[1])
	}
	if m := shipToStorePattern.FindStringSubmatch(normalized); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}
