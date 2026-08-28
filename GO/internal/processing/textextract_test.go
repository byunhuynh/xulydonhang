package processing

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func jdaBlock(po, page, body string) string {
	return "POM343    Date: 26/08/26\r\n" +
		"CFDMBCDH02   JDA Software Version 7.7.0 (PDN_DC)   Page:        " + page + "\r\n" +
		"P/O Number:   " + po + "   Purchase Order   Time:  9:53:45\r\n" +
		"Vendor:  22856\r\n" +
		body + "\r\n" +
		"       Sub Total -   3.00   32.00   .00   2,314,830.00\r\n"
}

func writeTxt(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bao-cao.txt")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("ghi file mau: %v", err)
	}
	return p
}

func TestExtractPageTexts_FileTxtRaMoiDonMotTrang(t *testing.T) {
	// Truoc ban sua nay, MOI file .txt deu chet o pdfOpen voi
	// "not a PDF file: invalid header" - du hop thoai chon file va
	// fileset deu nhan .txt.
	p := writeTxt(t, jdaBlock("103617493-00", "1", " 3558665-1 hang A")+
		jdaBlock("103617494-00", "1", " 3558666-6 hang B"))

	pages, nums, err := extractPageTexts(p)
	if err != nil {
		t.Fatalf("extractPageTexts(.txt) = %v, want nil", err)
	}
	if len(pages) != 2 {
		t.Fatalf("so trang = %d, want 2", len(pages))
	}
	if len(nums) != len(pages) {
		t.Fatalf("pageNumbers dai %d, pages dai %d - hai slice phai bang nhau", len(nums), len(pages))
	}
	if !strings.Contains(pages[0], "103617493-00") || !strings.Contains(pages[1], "103617494-00") {
		t.Errorf("noi dung trang khong khop don: %q | %q", pages[0], pages[1])
	}
	// CRLF phai duoc chuan hoa - duong xu ly phia sau gia dinh LF.
	if strings.Contains(pages[0], "\r") {
		t.Errorf("con sot CRLF trong trang tra ve")
	}
}

func TestExtractPageTexts_FileTxtDonNhieuTrangGopLamMot(t *testing.T) {
	p := writeTxt(t, jdaBlock("103617493-00", "1", " hang A")+
		jdaBlock("103617493-00", "2", " hang A tiep")+
		jdaBlock("103617494-00", "1", " hang B"))

	pages, _, err := extractPageTexts(p)
	if err != nil {
		t.Fatalf("extractPageTexts = %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("so trang = %d, want 2 (hai khoi cung P/O phai gop)", len(pages))
	}
}

func TestExtractPageTexts_FileTxtKhongPhaiBaoCaoThiBaoLoiRo(t *testing.T) {
	p := writeTxt(t, "day chi la mot file ghi chu binh thuong\r\n")

	_, _, err := extractPageTexts(p)
	if err == nil {
		t.Fatal("extractPageTexts = nil, want loi vi khong co moc POM343")
	}
	if strings.Contains(err.Error(), "PDF") {
		t.Errorf("thong diep loi noi ve PDF cho mot file .txt: %v", err)
	}
}

func TestExtractPageTexts_MoiTrangTxtTuDemLaiRaMotDon(t *testing.T) {
	// Duong xu ly Coop phia sau quyet dinh mot trang la mot hay nhieu don
	// bang phep dem POM343/Sub Total. Moi trang do file .txt sinh ra phai
	// dem lai ra dung 1/1, neu khong no se roi vao nhanh tach nhieu PO.
	p := writeTxt(t, jdaBlock("103617493-00", "1", " hang A")+
		jdaBlock("103617494-00", "1", " hang B"))

	pages, _, err := extractPageTexts(p)
	if err != nil {
		t.Fatalf("extractPageTexts = %v", err)
	}
	for i, page := range pages {
		segs, ok := splitPageIntoPOs(page)
		if !ok || len(segs) != 1 {
			t.Errorf("trang[%d]: splitPageIntoPOs ok=%v, %d doan, want true/1", i, ok, len(segs))
		}
	}
}

// writeTxtBytes ghi nguyen si mot chuoi byte ra file .txt, de dung duoc cho
// cac ban ma hoa khong phai UTF-8 (UTF-16, hoac UTF-8 co BOM).
func writeTxtBytes(t *testing.T, raw []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bao-cao.txt")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatalf("ghi file mau: %v", err)
	}
	return p
}

// encodeUTF16 dong goi mot chuoi thanh dung byte ma Windows ghi ra khi luu
// "Unicode text": BOM U+FEFF roi tung code unit theo thu tu byte da chon.
func encodeUTF16(t *testing.T, s string, order binary.ByteOrder) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, u := range append([]uint16{0xFEFF}, utf16.Encode([]rune(s))...) {
		unit := make([]byte, 2)
		order.PutUint16(unit, u)
		buf.Write(unit)
	}
	return buf.Bytes()
}

// JDA co the xuat cung mot bao cao ra UTF-16 (Windows "Unicode text") thay vi
// ANSI/UTF-8. Truoc ban sua nay noi dung do bi ep thang sang string, nen moi
// chu cai co mot byte NUL chen giua ("P" NUL "O" NUL "M" NUL "3"...) va pom34Pattern
// - chi khoan dung \s* giua cac chu cai, khong khoan NUL - khong con nhan ra
// moc POM343. Ket qua: mot file doc duoc hoan toan binh thuong lai bao
// "khong tim thay don nao trong file text".
func TestExtractPageTexts_FileTxtUTF16(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order binary.ByteOrder
	}{
		{"UTF-16 LE", binary.LittleEndian},
		{"UTF-16 BE", binary.BigEndian},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := writeTxtBytes(t, encodeUTF16(t, jdaBlock("103408229-00", "1", " 3558665-1 hang A")+
				jdaBlock("103408230-00", "1", " 3558666-6 hang B"), tc.order))

			pages, nums, err := extractPageTexts(p)
			if err != nil {
				t.Fatalf("extractPageTexts(.txt UTF-16) = %v, want nil", err)
			}
			if len(pages) != 2 || len(nums) != 2 {
				t.Fatalf("so trang = %d (nums %d), want 2", len(pages), len(nums))
			}
			for i, want := range []string{"103408229-00", "103408230-00"} {
				if !strings.Contains(pages[i], want) {
					t.Errorf("trang[%d] khong chua P/O %q: %q", i, want, pages[i])
				}
				if strings.ContainsRune(pages[i], 0) {
					t.Errorf("trang[%d] con byte NUL - chua giai ma UTF-16", i)
				}
				if strings.Contains(pages[i], "") {
					t.Errorf("trang[%d] con sot CRLF", i)
				}
			}
		})
	}
}

// BOM UTF-8 khong lam hong regex nhung dinh vao dau khoi POM343 dau tien va
// theo do vao ca truong dau tien doc ra tu don, nen phai duoc cat bo.
func TestExtractPageTexts_FileTxtUTF8CoBOM(t *testing.T) {
	p := writeTxtBytes(t, append([]byte{0xEF, 0xBB, 0xBF},
		[]byte(jdaBlock("103408229-00", "1", " 3558665-1 hang A"))...))

	pages, _, err := extractPageTexts(p)
	if err != nil {
		t.Fatalf("extractPageTexts(.txt UTF-8 BOM) = %v, want nil", err)
	}
	if len(pages) != 1 {
		t.Fatalf("so trang = %d, want 1", len(pages))
	}
	if !strings.HasPrefix(pages[0], "POM343") {
		t.Errorf("trang bat dau bang %q, want moc POM343 sach BOM", pages[0][:20])
	}
}
