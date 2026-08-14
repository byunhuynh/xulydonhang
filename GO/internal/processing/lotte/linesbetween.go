package lotte

import "strings"

// LinesBetween finds the first line starting with startPrefix, then —
// continuing the SAME single pass — the first line (from the very
// beginning of text, not from the start match) whose trimmed content
// exactly equals endMarker, stopping the scan the instant that line is
// found. Returns every line strictly between the two matches (excluding
// both), or nil if no valid (start, end) pair with start before end was
// found.
//
// Mirrors the identical "find a start line, find an end line, take
// what's between" scan duplicated across three Lotte functions in
// xulydonhang.py: tachcancledate_lotte (:6051-6071, used by
// ExtractCancelDate) and lamsachdonhang_lotte (:6405-6423, used by
// ExtractProducts's cleanup step). laytenstore_lotte (:6565-6584) has
// the same scan shape but needs the raw start/end indices (not just the
// slice between them) for a case this helper's return value can't
// represent — see ExtractStoreName's own comment for why it doesn't use
// this helper.
func LinesBetween(text, startPrefix, endMarker string) []string {
	lines := strings.Split(text, "\n")
	startIndex := -1
	endIndex := -1
	for i, line := range lines {
		if startIndex == -1 && strings.HasPrefix(line, startPrefix) {
			startIndex = i
		}
		if strings.TrimSpace(line) == endMarker {
			endIndex = i
			break
		}
	}
	if startIndex == -1 || endIndex == -1 || startIndex >= endIndex {
		return nil
	}
	return lines[startIndex+1 : endIndex]
}
