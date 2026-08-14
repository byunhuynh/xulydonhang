package lotte

import "testing"

func TestLinesBetween_ReturnsLinesStrictlyBetweenMarkers(t *testing.T) {
	text := "before\nSTART here\nline1\nline2\nEND\nafter"
	got := LinesBetween(text, "START", "END")
	want := []string{"line1", "line2"}
	if len(got) != len(want) {
		t.Fatalf("LinesBetween returned %d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LinesBetween()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLinesBetween_AdjacentMarkersReturnEmptySlice(t *testing.T) {
	got := LinesBetween("START\nEND", "START", "END")
	if len(got) != 0 {
		t.Fatalf("LinesBetween with adjacent markers = %v, want empty", got)
	}
}

func TestLinesBetween_MissingStartReturnsNil(t *testing.T) {
	if got := LinesBetween("nothing\nEND", "START", "END"); got != nil {
		t.Fatalf("LinesBetween with no start match = %v, want nil", got)
	}
}

func TestLinesBetween_MissingEndReturnsNil(t *testing.T) {
	if got := LinesBetween("START\nline1", "START", "END"); got != nil {
		t.Fatalf("LinesBetween with no end match = %v, want nil", got)
	}
}

func TestLinesBetween_EndMarkerBeforeStartReturnsNil(t *testing.T) {
	// Mirrors the Python source's single-pass-with-break behavior: once
	// end_marker is found, the scan stops immediately, so a start match
	// that would only appear LATER in the text is never reached.
	if got := LinesBetween("END\nSTART\nline1", "START", "END"); got != nil {
		t.Fatalf("LinesBetween with end before start = %v, want nil", got)
	}
}
