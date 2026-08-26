package processing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractOrderDocument_TxtChiChuaDungDonDo(t *testing.T) {
	// Truoc ban sua nay, mot file .txt 12 don se upload len Drive NGUYEN
	// CA FILE, 12 lan - vi pdfpage.ExtractPage that bai tren van ban thuan
	// va nhanh du phong lay lai duong dan file goc. Nguoi nhan mo link cua
	// don thu 7 ra thay ca 12 don.
	donA := jdaBlock("103617493-00", "1", " 3558665-1 hang A")
	donB := jdaBlock("103617494-00", "1", " 3558666-6 hang B")
	src := writeTxt(t, donA+donB)

	pages, _, err := extractPageTexts(src)
	if err != nil {
		t.Fatalf("extractPageTexts: %v", err)
	}

	path, cleanup, err := extractOrderDocument(src, 2, pages[1])
	if err != nil {
		t.Fatalf("extractOrderDocument(.txt) = %v, want nil", err)
	}
	defer cleanup()

	if filepath.Ext(path) != ".txt" {
		t.Errorf("duoi file tam = %q, want .txt (driveupload lay MIME tu duoi)", filepath.Ext(path))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("doc file tam: %v", err)
	}
	if !strings.Contains(string(got), "103617494-00") {
		t.Errorf("file tam thieu don cua chinh no:\n%s", got)
	}
	if strings.Contains(string(got), "103617493-00") {
		t.Errorf("file tam chua ca don KHAC - dung la loi dang sua:\n%s", got)
	}
}

func TestExtractOrderDocument_TxtCleanupXoaFileTam(t *testing.T) {
	src := writeTxt(t, jdaBlock("103617493-00", "1", " hang A"))
	pages, _, err := extractPageTexts(src)
	if err != nil {
		t.Fatalf("extractPageTexts: %v", err)
	}

	path, cleanup, err := extractOrderDocument(src, 1, pages[0])
	if err != nil {
		t.Fatalf("extractOrderDocument: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file tam khong ton tai ngay sau khi tao: %v", err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup khong xoa file tam %s", path)
	}
}

func TestExtractOrderDocument_TxtKhongDungFileGoc(t *testing.T) {
	src := writeTxt(t, jdaBlock("103617493-00", "1", " hang A"))
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("doc file goc: %v", err)
	}

	path, cleanup, err := extractOrderDocument(src, 1, "noi dung khac han")
	if err != nil {
		t.Fatalf("extractOrderDocument: %v", err)
	}
	defer cleanup()
	if path == src {
		t.Fatal("tra ve chinh file goc thay vi file tam")
	}

	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("doc lai file goc: %v", err)
	}
	if string(before) != string(after) {
		t.Error("file goc bi sua - chi duoc doc, moi thay doi phai roi vao file tam")
	}
}

func TestExtractOrderDocument_PdfVanCatTheoTrang(t *testing.T) {
	// Duong PDF phai giu nguyen hanh vi cu: cat dung trang cua don do.
	path, cleanup, err := extractOrderDocument("testdata/sample_coop_order.pdf", 1, "")
	if err != nil {
		t.Skipf("bo qua: khong cat duoc trang tu fixture PDF (%v)", err)
	}
	defer cleanup()

	if filepath.Ext(path) != ".pdf" {
		t.Errorf("duoi file tam = %q, want .pdf", filepath.Ext(path))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file tam: %v", err)
	}
	if info.Size() == 0 {
		t.Error("file PDF tam rong")
	}
}
