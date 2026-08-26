package misa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// googleErrorPage la hinh dang that cua trang loi Google tra ve khi URL
// Apps Script sai deployment - chinh thu nguoi dung gap.
const googleErrorPage = `<!DOCTYPE html><html lang="vi"><head><script nonce="ofwJ8BX2pGlrJkKZD1veNg">` +
	`window['ppConfig'] = {productName: '26981ed0d57bbad37e728ff58134270c', deleteIsEnforced: false,` +
	` sealIsEnforced: false, heartbeatRate: 0.5, periodicReportingRateMillis: 60000.0,` +
	` disableAllReporting: false};(function(){'use strict';function k(a){var b=0;return function(){` +
	`return b<a.length?{done:!1,value:a[b++]}:{done:!0}}}</script></head><body>Not Found</body></html>`

func fetchSessionErr(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	_, err := FetchSession(context.Background(), srv.URL, srv.Client())
	if err == nil {
		t.Fatalf("FetchSession(HTTP %d) = nil, want loi", status)
	}
	return err.Error()
}

func TestFetchSession_HTML404KhongDoTrangHTMLRaManHinh(t *testing.T) {
	// Loi that nguoi dung gap: URL Apps Script sai deployment -> Google
	// tra 404 kem mot trang HTML. Truoc day nhanh nhan dien HTML nam SAU
	// kiem tra status nen khong bao gio chay, va 400 ky tu HTML bi do
	// thang vao giao dien lan nhat ky he thong.
	got := fetchSessionErr(t, http.StatusNotFound, googleErrorPage)

	if strings.Contains(got, "<!DOCTYPE") || strings.Contains(got, "<script") || strings.Contains(got, "ppConfig") {
		t.Errorf("thong diep loi con do HTML tho ra:\n%s", got)
	}
	if !strings.Contains(got, "404") {
		t.Errorf("thong diep loi khong neu ma HTTP:\n%s", got)
	}
	if !strings.Contains(strings.ToLower(got), "exec") {
		t.Errorf("thong diep loi khong chi cho nguoi dung kiem tra gi:\n%s", got)
	}
}

func TestFetchSession_HTML200VanGiuGoiYQuyenTruyCap(t *testing.T) {
	// Truong hop cu (HTTP 200 kem trang dang nhap Google) phai giu nguyen
	// goi y ve quyen truy cap.
	got := fetchSessionErr(t, http.StatusOK, googleErrorPage)

	if strings.Contains(got, "<!DOCTYPE") {
		t.Errorf("thong diep loi con do HTML tho ra:\n%s", got)
	}
	if !strings.Contains(got, "Anyone with the link") {
		t.Errorf("mat goi y ve quyen truy cap:\n%s", got)
	}
}

func TestFetchSession_LoiKhongPhaiHTMLVanGiuNguyenBody(t *testing.T) {
	// Body khong phai HTML thi van dang ra: no thuong la thong diep that
	// cua Apps Script, doc duoc va huu ich.
	got := fetchSessionErr(t, http.StatusInternalServerError, "Exception: Khong doc duoc hop thu")

	if !strings.Contains(got, "Khong doc duoc hop thu") {
		t.Errorf("mat noi dung loi that tu endpoint:\n%s", got)
	}
	if !strings.Contains(got, "500") {
		t.Errorf("thong diep loi khong neu ma HTTP:\n%s", got)
	}
}
