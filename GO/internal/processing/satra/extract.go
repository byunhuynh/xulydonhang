package satra

import (
	"regexp"
	"strings"
	"time"
)

var poNumberPattern = regexp.MustCompile(`\*(P-[^*]+)\*`)

// ParsePONumber mirrors the PO-number extraction at the top of
// process_file's Satra branch (xulydonhang.py:9309-9310): the PO number
// is whatever sits between two literal "*" characters, prefixed "P-".
// Python captures the WHOLE "*...*" match then strips the first/last
// character; this uses a capture group directly instead, equivalent.
func ParsePONumber(text string) (string, bool) {
	m := poNumberPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

var entryDateBlockPattern = regexp.MustCompile(`(?s)(.*?)\nNgày đặt hàng:`)
var printDateBlockPattern = regexp.MustCompile(`(?s)(.*?)\nNgày in:`)

const placeholderDate = "01/01/0001"

// ParseEntryDate mirrors xulydonhang.py:9326-9336: takes everything
// before the first "Ngày đặt hàng:" marker, uses its LAST line as the
// raw date, parses "MM/DD/YYYY" and reformats "DD/MM/YYYY". If the RAW
// extracted date string is the literal placeholder "01/01/0001" (the PDF
// template renders this when the entry-date field itself is unset in the
// source system — not a parse failure), retries the same shape against
// "Ngày in:" instead. Returns false if the "Ngày đặt hàng:" marker isn't
// found at all, or if a fallback is triggered but "Ngày in:" isn't found
// either, or if the date ultimately used doesn't parse as "MM/DD/YYYY".
//
// The fallback only triggers when the "Ngày đặt hàng:" marker WAS found
// and its raw value is exactly the placeholder — mirroring Python's
// control flow, where the "Ngày in:" retry sits inside the `if
// entry_date:` block and is never reached when the first marker is
// simply absent from the text.
//
// Deliberately checks the RAW pre-format string against the placeholder,
// not the formatted "DD/MM/YYYY" output: comparing formatted strings
// requires re-deriving the formatted placeholder via a second format
// call, which invites exactly the kind of stray literal comparison this
// implementation avoids.
func ParseEntryDate(text string) (string, bool) {
	raw, ok := rawDateBeforeMarker(text, entryDateBlockPattern)
	if !ok {
		return "", false
	}
	if raw == placeholderDate {
		raw, ok = rawDateBeforeMarker(text, printDateBlockPattern)
		if !ok {
			return "", false
		}
	}
	return formatMDYtoDMYChecked(raw)
}

// rawDateBeforeMarker returns the last line of the text preceding
// blockPattern's marker, trimmed, without parsing or formatting it.
func rawDateBeforeMarker(text string, blockPattern *regexp.Regexp) (string, bool) {
	m := blockPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	lines := strings.Split(m[1], "\n")
	return strings.TrimSpace(lines[len(lines)-1]), true
}

func formatMDYtoDMYChecked(raw string) (string, bool) {
	t, err := time.Parse("01/02/2006", raw)
	if err != nil {
		return "", false
	}
	return t.Format("02/01/2006"), true
}

var cancelDateBlockPattern = regexp.MustCompile(`(?s)Ngày giao hàng:\s*(.*?)\s*Địa chỉ giao hàng:`)
var cancelDateLinePattern = regexp.MustCompile(`\d{1,2}/\d{1,2}/\d{4}`)

// ParseCancelDate mirrors xulydonhang.py:9339-9347: within the block
// between "Ngày giao hàng:" and "Địa chỉ giao hàng:", finds the FIRST
// line containing a d/d/dddd-shaped date, parses "MM/DD/YYYY", reformats
// "DD/MM/YYYY". Returns false if the block or no date-shaped line within
// it is found.
func ParseCancelDate(text string) (string, bool) {
	m := cancelDateBlockPattern.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	for _, line := range strings.Split(m[1], "\n") {
		if cancelDateLinePattern.MatchString(line) {
			return formatMDYtoDMYChecked(strings.TrimSpace(line))
		}
	}
	return "", false
}
