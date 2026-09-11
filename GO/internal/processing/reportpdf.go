package processing

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"math"
	"strings"
)

// Bo cuc lay dung theo file Word Coop gui (w:pgSz 15840x12240 landscape,
// w:pgMar trai/phai/duoi 432 twip, tren 180 twip, Courier New dam sz 12):
// ban PDF cua tung don tren Drive trong giong trang ma macro VBA cu xuat ra
// tu chinh file do.
const (
	reportPageWidth    = 792.0 // Letter nam ngang, don vi point
	reportPageHeight   = 612.0
	reportMarginLeft   = 21.6
	reportMarginTop    = 9.0
	reportMarginBottom = 21.6
	reportFontSize     = 6.0
	reportLeading      = 6.8 // Word gian dong don cho Courier New 6pt
	reportTabWidth     = 8
)

// reportLinesPerPage la so dong vua mot trang. Mot don JDA that dai 29-37
// dong; don dai tran trang (hai khoi POM343 cung P/O da gop) van co cho.
var reportLinesPerPage = int(math.Floor((reportPageHeight - reportMarginTop - reportMarginBottom) / reportLeading))

// writeReportPDF dung file PDF cho van ban cua MOT don trong bao cao JDA
// (.txt hay .docx), de moi don len Drive thanh mot file PDF rieng - nhu
// cac don PDF khac, va nhu truoc day khi phai chay VBA xuat PDF.
//
// Courier-Bold la font chuan Base-14 cua PDF nen khong can nhung font. Bao
// cao JDA chi co ky tu ASCII (do tren 3 file .docx va 8 file .txt that);
// ky tu nao nam ngoai ASCII in duoc thi thanh "?" thay vi lam hong file.
func writeReportPDF(text string) []byte {
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n"), "\n")

	var pages [][]string
	for len(lines) > reportLinesPerPage {
		pages = append(pages, lines[:reportLinesPerPage])
		lines = lines[reportLinesPerPage:]
	}
	pages = append(pages, lines)

	var out bytes.Buffer
	offsets := []int{0} // object 0 la muc tu do cua xref
	addObject := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, body)
	}

	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	// 1 catalog, 2 cay trang, 3 font; moi trang tiep theo la mot cap
	// (trang, noi dung) - so object cua chung tinh truoc duoc.
	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 4+2*i)
	}
	addObject("<< /Type /Catalog /Pages 2 0 R >>")
	addObject(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 %g %g] >>",
		strings.Join(kids, " "), len(pages), reportPageWidth, reportPageHeight))
	addObject("<< /Type /Font /Subtype /Type1 /BaseFont /Courier-Bold /Encoding /WinAnsiEncoding >>")

	for i, pageLines := range pages {
		addObject(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", 5+2*i))

		var content bytes.Buffer
		// Duong co so cua dong dau = mep tren + phan chu nhu len cua Courier.
		fmt.Fprintf(&content, "BT\n/F1 %g Tf\n%g TL\n%g %g Td\n",
			reportFontSize, reportLeading, reportMarginLeft, reportPageHeight-reportMarginTop-reportFontSize*0.83)
		for j, line := range pageLines {
			if j > 0 {
				content.WriteString("T*\n")
			}
			fmt.Fprintf(&content, "(%s) Tj\n", pdfLiteral(line))
		}
		content.WriteString("ET\n")

		var packed bytes.Buffer
		zw := zlib.NewWriter(&packed)
		zw.Write(content.Bytes())
		zw.Close()
		addObject(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", packed.Len(), packed.Bytes()))
	}

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
}

// pdfLiteral bien mot dong thanh noi dung chuoi PDF: tab thanh khoang trang
// theo cot 8, ky tu dac biet cua chuoi PDF duoc thoat, ky tu ngoai ASCII in
// duoc thanh "?", va bo khoang trang cuoi dong (khong in ra gi).
func pdfLiteral(line string) string {
	var b strings.Builder
	col := 0
	for _, r := range strings.TrimRight(line, " \t") {
		switch {
		case r == '\t':
			n := reportTabWidth - col%reportTabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 32 || r > 126:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
		col++
	}
	return b.String()
}
