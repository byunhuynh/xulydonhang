package processing

import (
	"archive/zip"
	"encoding/xml"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"order-processor/internal/processing/coop"
)

// writeDocx dong goi body thanh mot file .docx toi thieu: Word chi can
// word/document.xml de doc chu.
func writeDocx(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "HA THANH DDH.docx")
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("tao file docx mau: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("tao word/document.xml: %v", err)
	}
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"><w:body>` +
		body + `<w:sectPr/></w:body></w:document>`
	if _, err := w.Write([]byte(doc)); err != nil {
		t.Fatalf("ghi word/document.xml: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("dong zip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("dong file: %v", err)
	}
	return p
}

// docxParagraphs dung dung hinh dang cua file Coop that: moi dong bao cao la
// mot doan van, mot run Courier New.
func docxParagraphs(t *testing.T, report string) string {
	t.Helper()
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(strings.ReplaceAll(report, "\r\n", "\n"), "\n"), "\n") {
		b.WriteString(`<w:p><w:pPr><w:rPr><w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/><w:sz w:val="12"/></w:rPr></w:pPr>`)
		b.WriteString(`<w:r><w:rPr><w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/><w:sz w:val="12"/></w:rPr><w:t xml:space="preserve">`)
		if err := xml.EscapeText(&b, []byte(line)); err != nil {
			t.Fatalf("escape dong %q: %v", line, err)
		}
		b.WriteString(`</w:t></w:r></w:p>`)
	}
	return b.String()
}

// Truoc day file Coop .docx phai chay macro VBA trong Word (chen ngat trang
// truoc moi POM343, xuat PDF) moi xu ly duoc. Doc thang file Word phai ra
// DUNG nhung don ma chinh bao cao do ra khi luu thanh .txt - ke ca don dai
// tran sang trang hai duoc gop lai.
func TestExtractPageTexts_FileDocxRaDungNhuFileTxt(t *testing.T) {
	report := jdaBlock("103617493-00", "1", " 3558665-1 hang A") +
		jdaBlock("103617493-00", "2", " 3558665-1 hang A tiep") +
		jdaBlock("103617494-00", "1", " 3558666-6 hang B & C")

	txtPages, txtNums, err := extractPageTexts(writeTxt(t, report))
	if err != nil {
		t.Fatalf("extractPageTexts(.txt) = %v", err)
	}
	docxPages, docxNums, err := extractPageTexts(writeDocx(t, docxParagraphs(t, report)))
	if err != nil {
		t.Fatalf("extractPageTexts(.docx) = %v", err)
	}
	if len(docxPages) != 2 {
		t.Fatalf("so don tu .docx = %d, want 2 (hai khoi cung P/O lien nhau phai gop)", len(docxPages))
	}
	if !reflect.DeepEqual(docxPages, txtPages) || !reflect.DeepEqual(docxNums, txtNums) {
		t.Errorf("file Word ra khac file text:\ndocx=%q\ntxt =%q", docxPages, txtPages)
	}
}

func TestDocumentXMLText_ChiLayChuNamTrongRun(t *testing.T) {
	body := `<w:p>` +
		// Diem dung tab khai bao trong w:pPr khong phai ky tu tab.
		`<w:pPr><w:tabs><w:tab w:val="left" w:pos="720"/></w:tabs></w:pPr>` +
		// Word hay cat mot tu ra nhieu run.
		`<w:r><w:t>POM</w:t></w:r><w:r><w:t>343</w:t></w:r>` +
		`<w:r><w:tab/><w:t xml:space="preserve">A &amp; B </w:t></w:r>` +
		// Hai nhanh cua mc:AlternateContent chua cung mot noi dung.
		`<mc:AlternateContent><mc:Choice Requires="wps"><w:r><w:t>X</w:t></w:r></mc:Choice>` +
		`<mc:Fallback><w:r><w:t>X</w:t></w:r></mc:Fallback></mc:AlternateContent>` +
		`<w:r><w:br/><w:t>dong hai</w:t></w:r>` +
		`</w:p><w:p><w:r><w:t>doan sau</w:t></w:r></w:p>`

	got, err := readDocxText(writeDocx(t, body))
	if err != nil {
		t.Fatalf("readDocxText = %v", err)
	}
	want := "POM343\tA & B X\ndong hai\ndoan sau\n"
	if got != want {
		t.Errorf("readDocxText = %q, want %q", got, want)
	}
}

func TestExtractPageTexts_FileDocxKhongPhaiBaoCaoThiBaoLoiRo(t *testing.T) {
	_, _, err := extractPageTexts(writeDocx(t, docxParagraphs(t, "Giay gioi thieu nhan vien\r\n")))
	if err == nil || !strings.Contains(err.Error(), "file Word") || !strings.Contains(err.Error(), "POM343") {
		t.Errorf("extractPageTexts(docx khong co don) = %v, want loi noi ro file Word thieu moc POM343", err)
	}

	broken := filepath.Join(t.TempDir(), "hong.docx")
	if err := os.WriteFile(broken, []byte("khong phai zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := extractPageTexts(broken); err == nil || !strings.Contains(err.Error(), "khong mo duoc file Word") {
		t.Errorf("extractPageTexts(docx hong) = %v, want loi khong mo duoc file Word", err)
	}
}

// Moi don tu file Word len Drive thanh mot PDF rieng chi chua don do - nhu
// truoc day khi VBA chen ngat trang roi xuat PDF, moi don mot trang.
func TestExtractOrderDocument_DonTuFileWordThanhPDFRieng(t *testing.T) {
	donA := jdaBlock("103617493-00", "1", " 3558665-1 hang A")
	donB := jdaBlock("103617494-00", "1", " 3558666-6 hang B")
	src := writeDocx(t, docxParagraphs(t, donA+donB))
	pages, _, err := extractPageTexts(src)
	if err != nil {
		t.Fatalf("extractPageTexts(.docx) = %v", err)
	}

	path, cleanup, err := extractOrderDocument(src, 2, pages[1])
	defer cleanup()
	if err != nil {
		t.Fatalf("extractOrderDocument(.docx) = %v", err)
	}
	if filepath.Ext(path) != ".pdf" {
		t.Errorf("file tam = %q, want duoi .pdf", path)
	}
	gotPages, _, err := extractPageTexts(path)
	if err != nil {
		t.Fatalf("doc lai PDF tam: %v", err)
	}
	got := strings.Join(gotPages, "\n")
	if !strings.Contains(got, "103617494-00") || !strings.Contains(got, "3558666-6") {
		t.Errorf("PDF cua don thieu noi dung cua chinh no:\n%s", got)
	}
	if strings.Contains(got, "103617493-00") {
		t.Errorf("PDF cua don lan ca don khac:\n%s", got)
	}
}

// Ba file Coop .docx that ngay 10/09 (dot CN40_41). Cung ba file nay da
// duoc chay qua dung quy trinh VBA cu (chen ngat trang, xuat PDF) va xu ly
// ca hai ban: 28/9/5 don, trung tung PO, don gia, trong luong, va 0 o Excel
// khac nhau. Test nay giu lai phan khong can mang: so don va moi don tu dem
// lai ra dung mot PO.
func TestExtractPageTexts_FileDocxCoopThat(t *testing.T) {
	for name, want := range map[string]struct {
		orders  int
		firstPO string
	}{
		"HA THANH DDH CN40_41 01 10.09.docx": {28, "104077137-00"},
		"HA THANH DDH CN40_41 02 10.09.docx": {9, "104077509-00"},
		"HA THANH DDH CN40_41 03 10.09.docx": {5, "104076407-00"},
	} {
		path := filepath.Join("..", "..", "..", "đơn hàng", name)
		if _, err := os.Stat(path); err != nil {
			t.Logf("bo qua %s: %v", name, err)
			continue
		}
		pages, _, err := extractPageTexts(path)
		if err != nil {
			t.Fatalf("extractPageTexts(%s) = %v", name, err)
		}
		if len(pages) != want.orders {
			t.Errorf("%s: %d don, want %d", name, len(pages), want.orders)
		}
		seen := map[string]bool{}
		for i, page := range pages {
			if segments, ok := splitPageIntoPOs(page); !ok || len(segments) != 1 {
				t.Errorf("%s don %d: splitPageIntoPOs = %d doan (ok=%v), want dung 1", name, i+1, len(segments), ok)
			}
			po := coop.ParseInvoiceInfo(page).PONumber
			if po == "" || seen[po] {
				t.Errorf("%s don %d: P/O %q rong hoac trung", name, i+1, po)
			}
			seen[po] = true
		}
		if len(pages) > 0 {
			if po := coop.ParseInvoiceInfo(pages[0]).PONumber; po != want.firstPO {
				t.Errorf("%s: P/O dau tien = %q, want %q", name, po, want.firstPO)
			}
		}
	}
}
