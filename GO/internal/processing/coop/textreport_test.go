package coop

import (
	"strings"
	"testing"
)

// blockJDA dung mot khoi bao cao PO cua JDA rut gon nhung giu dung cac
// moc that: dong dau POM343, dong "P/O Number:", va "Sub Total" ket thuc.
func blockJDA(po, page, body string) string {
	return "POM343" + strings.Repeat(" ", 4) + "Date: 26/08/26\n" +
		"CFDMBCDH02   JDA Software Version 7.7.0 (PDN_DC)   Page:        " + page + "\n" +
		"P/O Number:   " + po + "   Purchase Order   Time:  9:53:45\n" +
		"Vendor:  22856\n" +
		body + "\n" +
		"                Sub Total -   3.00   32.00   .00   2,314,830.00\n"
}

func TestSplitTextReport_MoiKhoiPOM343LaMotDon(t *testing.T) {
	text := blockJDA("103617493-00", "1", " 3558665-1  hang A") +
		blockJDA("103617494-00", "1", " 3558666-6  hang B") +
		blockJDA("103617495-00", "1", " 3564270-4  hang C")

	got := SplitTextReport(text)
	if len(got) != 3 {
		t.Fatalf("SplitTextReport = %d don, want 3", len(got))
	}
	for i, want := range []string{"103617493-00", "103617494-00", "103617495-00"} {
		if info := ParseInvoiceInfo(got[i]); info.PONumber != want {
			t.Errorf("don[%d] P/O = %q, want %q", i, info.PONumber, want)
		}
	}
}

func TestSplitTextReport_DonNhieuTrangGopLaiLamMot(t *testing.T) {
	// Don dai lap lai POM343 voi Page: 2 nhung VAN la mot don. Khong gop
	// thi mot don bi xe thanh hai, va don thu hai thieu phan dau -> ghi
	// vao so dat hang thanh hai chung tu khong co that.
	text := blockJDA("103617493-00", "1", " 3558665-1  hang A") +
		blockJDA("103617493-00", "2", " 3558666-6  hang A tiep") +
		blockJDA("103617494-00", "1", " 3564270-4  hang B")

	got := SplitTextReport(text)
	if len(got) != 2 {
		t.Fatalf("SplitTextReport = %d don, want 2 (hai khoi cung P/O phai gop)", len(got))
	}
	if !strings.Contains(got[0], "hang A") || !strings.Contains(got[0], "hang A tiep") {
		t.Errorf("don gop thieu noi dung trang sau: %q", got[0])
	}
	if info := ParseInvoiceInfo(got[1]); info.PONumber != "103617494-00" {
		t.Errorf("don[1] P/O = %q, want 103617494-00", info.PONumber)
	}
}

func TestSplitTextReport_ChiGopKhiLIENNhau(t *testing.T) {
	// Cung mot P/O nhung bi mot don khac chen giua thi KHONG gop - gop
	// se tron du lieu cua hai lan dat hang khac nhau vao lam mot.
	text := blockJDA("103617493-00", "1", " hang A") +
		blockJDA("103617494-00", "1", " hang B") +
		blockJDA("103617493-00", "1", " hang C")

	if got := SplitTextReport(text); len(got) != 3 {
		t.Fatalf("SplitTextReport = %d don, want 3 (khong gop khoi khong lien nhau)", len(got))
	}
}

func TestSplitTextReport_BoQuaRacTruocKhoiDauTien(t *testing.T) {
	text := "vai dong tieu de khong thuoc don nao\n" + blockJDA("103617493-00", "1", " hang A")
	got := SplitTextReport(text)
	if len(got) != 1 {
		t.Fatalf("SplitTextReport = %d don, want 1", len(got))
	}
	if strings.Contains(got[0], "vai dong tieu de") {
		t.Errorf("rac truoc khoi dau tien bi keo vao don: %q", got[0])
	}
}

func TestSplitTextReport_KhongCoPOM343ThiRong(t *testing.T) {
	if got := SplitTextReport("mot file text khong phai bao cao PO\n"); len(got) != 0 {
		t.Fatalf("SplitTextReport(van ban la) = %d don, want 0", len(got))
	}
}

func TestSplitTextReport_MoiKhoiGiuDuocPOM343DeDemLaiRa1(t *testing.T) {
	// Moi khoi tra ve phai tu dung dem duoc POM343 == SubTotal == 1, vi
	// duong xu ly phia sau (splitPageIntoPOs) dua vao dung phep dem do de
	// quyet dinh khoi nay la mot don hay nhieu don.
	text := blockJDA("103617493-00", "1", " hang A") + blockJDA("103617494-00", "1", " hang B")
	for i, seg := range SplitTextReport(text) {
		c := CountPOsOnPage(seg)
		if c.POM343 != 1 || c.SubTotal != 1 {
			t.Errorf("khoi[%d]: POM343=%d SubTotal=%d, want 1/1", i, c.POM343, c.SubTotal)
		}
	}
}
