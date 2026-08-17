package processing

import "testing"

func TestExtractPageTexts_ReturnsOnePerPage(t *testing.T) {
	// Use any real single-page Coop PDF from the repo's đơn hàng/08-2026/
	// folder as a smoke-test fixture — copy one into testdata/ (see
	// Step 2) rather than depending on the live folder, so this test
	// doesn't break if that folder's contents change.
	pages, _, err := extractPageTexts("testdata/sample_coop_order.pdf")
	if err != nil {
		t.Fatalf("extractPageTexts returned error: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("extractPageTexts returned no pages")
	}
	if !containsSubstring(pages[0], "POM343") && !containsSubstring(pages[0], "P/O Number") {
		t.Fatalf("page 0 text doesn't look like a Coop PO, got: %.200s", pages[0])
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
