package processing

import (
	"fmt"
	"os"

	"github.com/ledongthuc/pdf"
)

// pdfOpen wraps pdf.Open with a panic recovery boundary. Most of
// ledongthuc/pdf's malformed-input handling (e.g. inside Page.Content)
// already recovers its own internal panics and turns them into errors,
// but the reader-construction path (NewReaderEncrypted -> readXref)
// does not: a PDF whose trailer's startxref offset doesn't actually
// point at a cross-reference table (seen in the wild on at least one
// real archived Coop PDF whose startxref pointed exactly at EOF instead
// of ~340 bytes earlier, where the real "xref" keyword was) panics all
// the way out of pdf.Open instead of returning an error. A single
// malformed PDF file must never be able to crash the whole process —
// convert that panic into a regular error here, same shape as any other
// pdf.Open failure, so callers (extractPageTexts, and ultimately
// RealProcessor.Process) handle it via their existing "không đọc được
// PDF" / Failed-row error path instead of taking down the app.
func pdfOpen(path string) (f *os.File, r *pdf.Reader, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			if f != nil {
				f.Close()
			}
			f, r = nil, nil
			err = fmt.Errorf("malformed PDF (panic while reading): %v", rec)
		}
	}()
	return pdf.Open(path)
}
