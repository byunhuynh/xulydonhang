package processing

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func writePDFFile(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "don.pdf")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("ghi PDF: %v", err)
	}
	return p
}

// PDF tu dung phai la file PDF hop le theo pdfcpu (thu vien cat trang cua
// app), va doc nguoc lai bang chinh bo trich chu cua app phai ra dung chu.
func TestWriteReportPDF_HopLeVaDocLaiDuocChu(t *testing.T) {
	report := jdaBlock("103617494-00", "1", " 3558666-6 NUOC GIAT (TUI) 3.6KG \\ khuyen mai")
	data := writeReportPDF(report)

	if err := api.Validate(bytes.NewReader(data), nil); err != nil {
		t.Fatalf("pdfcpu Validate = %v, want PDF hop le", err)
	}
	pages, _, err := extractPageTexts(writePDFFile(t, data))
	if err != nil {
		t.Fatalf("doc lai PDF: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("so trang = %d, want 1", len(pages))
	}
	for _, want := range []string{"POM343", "103617494-00", "3558666-6", "(TUI)", "\\ khuyen mai", "Sub Total"} {
		if !strings.Contains(pages[0], want) {
			t.Errorf("PDF doc lai thieu %q:\n%s", want, pages[0])
		}
	}
}

// Don dai hon mot trang phai sang trang moi, khong duoc ve tran ra ngoai kho
// giay (nua duoi don se khong ai thay).
func TestWriteReportPDF_DonDaiSangTrangMoi(t *testing.T) {
	var b strings.Builder
	total := reportLinesPerPage*2 + 5
	for i := 1; i <= total; i++ {
		b.WriteString("dong so ")
		b.WriteString(strings.Repeat("x", i%40))
		b.WriteString("\n")
	}
	data := writeReportPDF(b.String())
	if err := api.Validate(bytes.NewReader(data), nil); err != nil {
		t.Fatalf("pdfcpu Validate = %v", err)
	}
	pages, _, err := extractPageTexts(writePDFFile(t, data))
	if err != nil {
		t.Fatalf("doc lai PDF: %v", err)
	}
	if len(pages) != 3 {
		t.Errorf("%d dong ra %d trang, want 3 (%d dong/trang)", total, len(pages), reportLinesPerPage)
	}
}

func TestPdfLiteral(t *testing.T) {
	for in, want := range map[string]string{
		"a(b)c\\d":     "a\\(b\\)c\\\\d",
		"ab\tc":        "ab      c",
		"hàng Việt   ": "h?ng Vi?t",
		"":             "",
	} {
		if got := pdfLiteral(in); got != want {
			t.Errorf("pdfLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}
