package coop

import "testing"

func TestCountPOsOnPage(t *testing.T) {
	text := "POM343 first order\nSub Total\nPOM343 second order\nSub Total"
	got := CountPOsOnPage(text)
	if got.POM343 != 2 || got.SubTotal != 2 {
		t.Fatalf("CountPOsOnPage = %+v, want POM343=2 SubTotal=2", got)
	}
}

func TestCountPOsOnPage_LowercaseNormalized(t *testing.T) {
	text := "pom343 order\nSub Total"
	got := CountPOsOnPage(text)
	if got.POM343 != 1 || got.SubTotal != 1 {
		t.Fatalf("CountPOsOnPage(lowercase) = %+v, want POM343=1 SubTotal=1", got)
	}
}

func TestSplitMultiPO_UppercaseInputFindsNoSegments(t *testing.T) {
	// Regression test for the preserved bug documented in this plan's
	// Global Constraints: catdonra_nhieutrang's split uses a lowercase
	// literal keyword against text that, on real Coop PDFs, is
	// uppercase ("POM343") — so on real data this returns no
	// mid-document segments beyond the first "Sub Total" boundary.
	text := "header stuff\nSub Total\nPOM343 second order text\nSub Total\nfooter"
	segments := SplitMultiPO(text)
	if len(segments) != 1 {
		t.Fatalf("SplitMultiPO(uppercase POM343) = %d segments, want 1 (bug preserved): %v", len(segments), segments)
	}
}

func TestSplitMultiPO_LowercaseInputSplitsCorrectly(t *testing.T) {
	text := "header stuff\nSub Total\npom343 second order text\nSub Total\nfooter"
	segments := SplitMultiPO(text)
	if len(segments) != 2 {
		t.Fatalf("SplitMultiPO(lowercase pom343) = %d segments, want 2: %v", len(segments), segments)
	}
	if segments[1] != "POM343 second order text" {
		t.Fatalf("segments[1] = %q, want %q", segments[1], "POM343 second order text")
	}
}
