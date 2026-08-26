package processing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"order-processor/internal/processing/coop"
)

// isTextReport bao file co phai bao cao don dat hang dang van ban thuan
// hay khong. Xet theo duoi file: hop thoai chon file va fileset chi cho
// qua .pdf/.xlsx/.txt, va trong so do chi .txt la van ban thuan.
func isTextReport(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".txt")
}

// extractTextFilePages doc mot file .txt bao cao don dat hang cua JDA
// (Coop/Coop Food) va tra ve tung don duoi dang "trang", de duong xu ly
// phia sau dung y nguyen vong lap theo trang cua PDF.
//
// Truoc day moi file deu di qua pdfOpen, nen mot file .txt luon chet voi
// "not a PDF file: invalid header" - du hop thoai chon file VA fileset
// deu nhan .txt. Nghia la app moi file vao roi bao khong doc duoc.
//
// Moi "trang" tra ve la mot don da tach san (xem coop.SplitTextReport),
// nen CountPOsOnPage tren no ra 1/1 va splitPageIntoPOs tra thang mot
// doan - khong dung toi duong tach nhieu PO.
func extractTextFilePages(path string) ([]string, []int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	// Chuan hoa CRLF ve LF: file JDA xuat ra tu he thong Windows luon la
	// CRLF, trong khi van ban boc tu PDG chi co LF. Duong xu ly phia sau
	// (va ca strings.TrimRight(..., "\n")) gia dinh LF.
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")

	pages := coop.SplitTextReport(text)
	if len(pages) == 0 {
		return nil, nil, fmt.Errorf("khong tim thay don nao trong file text (thieu moc POM343/POM346)")
	}

	pageNumbers := make([]int, len(pages))
	for i := range pageNumbers {
		pageNumbers[i] = i + 1
	}
	return pages, pageNumbers, nil
}
