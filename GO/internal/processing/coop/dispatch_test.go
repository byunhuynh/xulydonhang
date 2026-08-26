package coop

import "testing"

func TestCountPOsOnPage(t *testing.T) {
	text := "POM343 first order\nSub Total\nPOM343 second order\nSub Total"
	got := CountPOsOnPage(text)
	if got.POM343 != 2 || got.SubTotal != 2 {
		t.Fatalf("CountPOsOnPage = %+v, want POM343=2 SubTotal=2", got)
	}
}

func TestCountPOsOnPage_LowercaseNormalized(t *testing.T) {
	text := "pom343 order\nSub Total"
	got := CountPOsOnPage(text)
	if got.POM343 != 1 || got.SubTotal != 1 {
		t.Fatalf("CountPOsOnPage(lowercase) = %+v, want POM343=1 SubTotal=1", got)
	}
}

func TestSplitMultiPO_UppercaseInputSplitsCorrectly(t *testing.T) {
	// Dữ liệu Coop THẬT viết hoa "POM343". Trước đây hàm này tách bằng
	// strings.SplitN với chuỗi chữ thường nên không khớp gì, và mọi đơn
	// sau đơn đầu tiên biến mất KHÔNG BÁO GÌ — port nguyên si một bug của
	// bản Python. Người dùng quyết sửa (2026-08-26): đây là mất đơn im
	// lặng trên mọi trang PDF Coop có nhiều hơn một PO.
	text := "header stuff\nSub Total\nPOM343 second order text\nSub Total\nfooter"
	segments := SplitMultiPO(text)
	if len(segments) != 2 {
		t.Fatalf("SplitMultiPO(uppercase POM343) = %d segments, want 2: %v", len(segments), segments)
	}
	if segments[1] != "POM343 second order text" {
		t.Fatalf("segments[1] = %q, want %q", segments[1], "POM343 second order text")
	}
}

func TestSplitMultiPO_BaDonVietHoaRaDuBaDoan(t *testing.T) {
	// Đúng hình dạng dữ liệu thật: nhiều PO liên tiếp, mỗi PO kết thúc
	// bằng "Sub Total". Ca 2-đoạn ở trên vẫn xanh cả khi vòng lặp chỉ
	// chạy được một lần; ba đoạn mới chứng minh nó chạy tới cùng.
	text := "POM343 don A Sub Total x\nPOM343 don B Sub Total y\nPOM343 don C Sub Total z\n"
	segments := SplitMultiPO(text)
	if len(segments) != 3 {
		t.Fatalf("SplitMultiPO(3 don viet hoa) = %d doan, want 3: %v", len(segments), segments)
	}
}

func TestSplitMultiPO_TronHoaThuong(t *testing.T) {
	// Không phân biệt hoa thường ở CẢ hai chiều: CountPOsOnPage vốn chuẩn
	// hoá "pom343" -> "POM343" trước khi đếm, nên một trang trộn hai kiểu
	// viết phải ra đủ đoạn, không lệch với số đếm.
	text := "POM343 don A Sub Total x\npom346 don B Sub Total y\n"
	segments := SplitMultiPO(text)
	if len(segments) != 2 {
		t.Fatalf("SplitMultiPO(tron hoa/thuong) = %d doan, want 2: %v", len(segments), segments)
	}
}

func TestSplitMultiPO_LowercaseInputSplitsCorrectly(t *testing.T) {
	text := "header stuff\nSub Total\npom343 second order text\nSub Total\nfooter"
	segments := SplitMultiPO(text)
	if len(segments) != 2 {
		t.Fatalf("SplitMultiPO(lowercase pom343) = %d segments, want 2: %v", len(segments), segments)
	}
	if segments[1] != "POM343 second order text" {
		t.Fatalf("segments[1] = %q, want %q", segments[1], "POM343 second order text")
	}
}
