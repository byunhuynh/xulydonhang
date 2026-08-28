package processing

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

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
	text := strings.ReplaceAll(decodeReportText(raw), "\r\n", "\n")

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

// decodeReportText giai ma noi dung tho cua mot file bao cao .txt ve UTF-8
// theo BOM o dau file.
//
// Cung mot bao cao JDA co the duoc luu ra "Unicode text" (UTF-16 LE, hoac BE
// neu nguoi dung chon) thay vi ANSI/UTF-8. Truoc day noi dung do bi ep thang
// sang string, nen moi chu cai co mot byte NUL chen giua va pom34Pattern -
// chi khoan dung khoang trang giua cac chu cai, khong khoan NUL - khong con
// nhan ra moc POM343. Mot file doc duoc hoan toan binh thuong vi vay lai bao
// "khong tim thay don nao trong file text".
//
// File khong co BOM duoc giu nguyen: do la duong di cu, va doan mot file
// UTF-16 khong BOM tu noi dung se doan sai nhieu hon la doan dung.
func decodeReportText(raw []byte) string {
	switch {
	case len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE:
		return decodeUTF16(raw[2:], binary.LittleEndian)
	case len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF:
		return decodeUTF16(raw[2:], binary.BigEndian)
	case len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF:
		// BOM UTF-8 khong lam hong regex, nhung neu de lai no se dinh vao dau
		// khoi POM343 dau tien.
		return string(raw[3:])
	default:
		return string(raw)
	}
}

// decodeUTF16 doc tung cap byte theo thu tu da cho. utf16.Decode lo phan cap
// thay the (surrogate pair). Byte le o cuoi - file bi cat cut - bi bo qua
// thay vi doc tran ra ngoai mang.
func decodeUTF16(b []byte, order binary.ByteOrder) string {
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		units = append(units, order.Uint16(b[i:i+2]))
	}
	return string(utf16.Decode(units))
}
