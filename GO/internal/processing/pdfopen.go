package processing

import (
	"os"

	"github.com/ledongthuc/pdf"
)

func pdfOpen(path string) (*os.File, *pdf.Reader, error) {
	return pdf.Open(path)
}
