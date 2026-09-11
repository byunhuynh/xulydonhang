package processing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"order-processor/internal/pdfpage"
)

// extractOrderDocument tach phan tai lieu cua DUNG MOT don ra mot file
// tam, de link Drive cua don do chi mo ra chinh don do.
//
// Voi PDF thi day la cat dung trang (pdfpage.ExtractPage) - hanh vi da co
// tu truoc. Voi bao cao JDA dang van ban (.txt, .docx) thi khong co "trang"
// nao de cat: ca file la mot khoi van ban lien tuc, va segmentText chinh la
// phan van ban cua don nay ma coop.SplitTextReport da tach san. Phan do duoc
// dung thanh mot file PDF rieng (writeReportPDF), de moi don len Drive la
// mot PDF nhu don cua moi he thong khac - va nhu luc con phai chay VBA xuat
// PDF tu file Word.
//
// Thieu nhanh van ban thi pdfpage.ExtractPage that bai, ben goi lui ve
// upload NGUYEN CA FILE - nen mot bao cao 12 don se upload ca 12 don len
// Drive 12 lan, va link cua tung don deu mo ra toan bo file.
//
// Duoi file tam phai dung loai that: driveupload.Upload lay ca duoi lan
// MIME tu duong dan nay.
//
// Luon tra ve cleanup goi duoc, ke ca khi loi - ben goi defer no ngay.
func extractOrderDocument(filePath string, realPageNum int, segmentText string) (path string, cleanup func(), err error) {
	if !isTextReport(filePath) && !isWordReport(filePath) {
		return pdfpage.ExtractPage(filePath, realPageNum)
	}

	noop := func() {}

	tempDir, err := os.MkdirTemp("", "driveupload-don-*")
	if err != nil {
		return "", noop, fmt.Errorf("orderdoc: tao thu muc tam: %w", err)
	}
	cleanup = func() { os.RemoveAll(tempDir) }

	// Dat ten theo file goc de log va thu muc tam con doc duoc, nhung ten
	// hien tren Drive khong lay tu day - driveupload.BuildFilename dung
	// metadata (nha cung cap, ngay, ma khach, so don).
	base := filepath.Base(filePath)
	tempPath := filepath.Join(tempDir, strings.TrimSuffix(base, filepath.Ext(base))+".pdf")

	if err := os.WriteFile(tempPath, writeReportPDF(segmentText), 0o600); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("orderdoc: ghi file tam: %w", err)
	}
	return tempPath, cleanup, nil
}
