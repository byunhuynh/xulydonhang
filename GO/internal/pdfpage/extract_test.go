// GO/internal/pdfpage/extract_test.go
package pdfpage

import (
	"os"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func TestExtractPage_SinglePageSource(t *testing.T) {
	src := "../processing/coop/testdata/realpdfs/103098619-00.pdf"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not found: %v", err)
	}

	tempPath, cleanup, err := ExtractPage(src, 1)
	defer cleanup()
	if err != nil {
		t.Fatalf("ExtractPage returned error: %v", err)
	}
	if _, err := os.Stat(tempPath); err != nil {
		t.Fatalf("extracted file does not exist: %v", err)
	}
	if err := api.ValidateFile(tempPath, nil); err != nil {
		t.Fatalf("extracted file is not a valid PDF: %v", err)
	}
	count, err := api.PageCountFile(tempPath)
	if err != nil {
		t.Fatalf("PageCountFile returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("extracted file page count = %d, want 1", count)
	}
}

func TestExtractPage_SelectsCorrectPageFromMultiPageSource(t *testing.T) {
	src := "../processing/bigc/testdata/realpdfs/2633058028692.pdf"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	sourceCount, err := api.PageCountFile(src)
	if err != nil {
		t.Fatalf("PageCountFile on source returned error: %v", err)
	}
	if sourceCount < 2 {
		t.Skipf("fixture only has %d page(s), need >= 2 to test page selection", sourceCount)
	}

	tempPath, cleanup, err := ExtractPage(src, 2)
	defer cleanup()
	if err != nil {
		t.Fatalf("ExtractPage returned error: %v", err)
	}
	count, err := api.PageCountFile(tempPath)
	if err != nil {
		t.Fatalf("PageCountFile returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("extracted file page count = %d, want 1", count)
	}
}

func TestExtractPage_CleanupRemovesTempFile(t *testing.T) {
	src := "../processing/coop/testdata/realpdfs/103098619-00.pdf"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not found: %v", err)
	}

	tempPath, cleanup, err := ExtractPage(src, 1)
	if err != nil {
		t.Fatalf("ExtractPage returned error: %v", err)
	}
	cleanup()
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed after cleanup(), stat error = %v", err)
	}
}

func TestExtractPage_NonexistentSourceReturnsError(t *testing.T) {
	tempPath, cleanup, err := ExtractPage("/does/not/exist.pdf", 1)
	defer cleanup()
	if err == nil {
		t.Fatal("expected an error for a nonexistent source file, got nil")
	}
	if tempPath != "" {
		t.Errorf("expected an empty tempPath on error, got %q", tempPath)
	}
}

func TestExtractPage_PageOutOfRangeReturnsError(t *testing.T) {
	src := "../processing/coop/testdata/realpdfs/103098619-00.pdf"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("fixture not found: %v", err)
	}

	tempPath, cleanup, err := ExtractPage(src, 999)
	defer cleanup()
	if err == nil {
		t.Fatal("expected an error for an out-of-range page number, got nil")
	}
	if tempPath != "" {
		t.Errorf("expected an empty tempPath on error, got %q", tempPath)
	}
}
