package processing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
