package processing

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// isWordReport bao file co phai bao cao don dat hang JDA luu bang Word
// (.docx) hay khong.
//
// Coop gui mot so dot don duoi dang .docx: chinh la bao cao van ban JDA
// (font Courier New, moi dong mot doan van, moc POM343/POM346 dau moi don).
// Truoc day nguoi dung phai chay macro VBA trong Word - chen ngat trang
// truoc moi POM343 roi xuat PDF - moi dua duoc vao app. Doc thang chu tu
// file Word thi khong can Word, khong can PDF, va chu giu dung nhu bao cao
// goc thay vi phai boc lai tu PDF.
func isWordReport(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".docx")
}

const (
	wordprocessingNS      = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	markupCompatibilityNS = "http://schemas.openxmlformats.org/markup-compatibility/2006"
)

// extractDocxFilePages doc mot bao cao JDA luu bang Word va tra ve tung don
// duoi dang "trang" - dung y nhu extractTextFilePages, vi sau khi lay chu ra
// thi hai loai file la mot.
func extractDocxFilePages(path string) ([]string, []int, error) {
	text, err := readDocxText(path)
	if err != nil {
		return nil, nil, err
	}
	return splitReportPages(text, "file Word")
}

func readDocxText(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("khong mo duoc file Word: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("khong doc duoc noi dung file Word: %w", err)
		}
		defer rc.Close()
		return documentXMLText(rc)
	}
	return "", fmt.Errorf("file Word thieu word/document.xml")
}

// documentXMLText lay chu tu word/document.xml, moi doan van (w:p) thanh
// mot dong.
//
// Chi lay nhung gi nam TRONG mot run (w:r): w:tab con dung de khai bao diem
// dung tab trong w:pPr, dem ca cai do se chen tab gia vao giua dong. Nhanh
// mc:Fallback bi bo qua vi no lap lai y nguyen noi dung cua nhanh mc:Choice
// ngay truoc no, doc ca hai thi chu bi nhan doi.
func documentXMLText(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var b strings.Builder
	runDepth := 0
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("noi dung file Word bi hong: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == markupCompatibilityNS && t.Name.Local == "Fallback" {
				if err := dec.Skip(); err != nil {
					return "", fmt.Errorf("noi dung file Word bi hong: %w", err)
				}
				continue
			}
			if t.Name.Space != wordprocessingNS {
				continue
			}
			switch t.Name.Local {
			case "r":
				runDepth++
			case "t":
				inText = runDepth > 0
			case "tab":
				if runDepth > 0 {
					b.WriteByte('\t')
				}
			case "br", "cr":
				if runDepth > 0 {
					b.WriteByte('\n')
				}
			}
		case xml.EndElement:
			if t.Name.Space != wordprocessingNS {
				continue
			}
			switch t.Name.Local {
			case "r":
				runDepth--
			case "t":
				inText = false
			case "p":
				b.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}
	return b.String(), nil
}
