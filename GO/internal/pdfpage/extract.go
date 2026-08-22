// GO/internal/pdfpage/extract.go

// Package pdfpage extracts a single page from a source PDF into a new,
// standalone single-page PDF file - mirrors xulydonhang.py's
// cat_trang_hien_tai (verified by reading the Python source directly:
// PyMuPDF's `dst.insert_pdf(src_doc, from_page=page_index,
// to_page=page_index)`), real page-level extraction, not a whole-file
// copy and not a sub-page/text-boundary split.
package pdfpage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// ExtractPage extracts pageNumber (1-indexed, matching a PDF's own page
// numbering) from sourcePath into a new temporary single-page PDF file,
// returned as tempPath. The caller MUST call the returned cleanup func
// once done with the file (typically immediately after
// driveupload.Upload's synchronous file read completes, not deferred
// to the end of a long-running function) to remove the temp directory
// created for it.
func ExtractPage(sourcePath string, pageNumber int) (tempPath string, cleanup func(), err error) {
	noop := func() {}

	tempDir, err := os.MkdirTemp("", "driveupload-page-*")
	if err != nil {
		return "", noop, fmt.Errorf("pdfpage: create temp dir: %w", err)
	}
	cleanup = func() { os.RemoveAll(tempDir) }

	pageArg := fmt.Sprintf("%d", pageNumber)
	if err := api.ExtractPagesFile(sourcePath, tempDir, []string{pageArg}, nil); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("pdfpage: extract page %d from %s: %w", pageNumber, sourcePath, err)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		cleanup()
		return "", noop, fmt.Errorf("pdfpage: read output dir after extracting page %d from %s: %w", pageNumber, sourcePath, err)
	}
	if len(entries) == 0 {
		cleanup()
		return "", noop, fmt.Errorf("pdfpage: extract page %d from %s: no output file produced", pageNumber, sourcePath)
	}

	return filepath.Join(tempDir, entries[0].Name()), cleanup, nil
}
