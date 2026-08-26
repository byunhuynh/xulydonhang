package misa

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFetchSession(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		wantErr    string
	}{
		{
			name:   "phiên hợp lệ",
			body:   `{"sid":"s1","tid":"t1","mid":"m1","dbid":"d1","x_device":"dev"}`,
			status: 200,
		},
		{
			// Apps Script trả lỗi kèm HTTP 200 nên phải soi body.
			name:    "endpoint báo lỗi",
			body:    `{"error":"chờ quá 120s mà không thấy mã OTP mới"}`,
			status:  200,
			wantErr: "không thấy mã OTP",
		},
		{
			name:    "thiếu trường",
			body:    `{"sid":"s1","tid":"t1"}`,
			status:  200,
			wantErr: "thiếu trường",
		},
		{
			// Apps Script chưa mở quyền thì trả trang đăng nhập của Google.
			name:    "trả về HTML",
			body:    `<!DOCTYPE html><html><body>Sign in</body></html>`,
			status:  200,
			wantErr: "Anyone with the link",
		},
		{
			name:    "HTTP lỗi",
			body:    `nope`,
			status:  500,
			wantErr: "HTTP 500",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			s, err := FetchSession(context.Background(), srv.URL, srv.Client())
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("FetchSession: %v", err)
				}
				if s.SID != "s1" || s.XDevice != "dev" {
					t.Errorf("phiên = %+v", s)
				}
				return
			}
			if err == nil {
				t.Fatalf("muốn lỗi chứa %q, lại thành công", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("lỗi = %v, muốn chứa %q", err, tc.wantErr)
			}
		})
	}

	if _, err := FetchSession(context.Background(), "", nil); err == nil {
		t.Error("URL rỗng phải báo lỗi")
	}
}

// Phiên trong file chết thì client phải tự xin phiên mới rồi chạy tiếp,
// chứ không bắt người dùng đi lấy tay.
func TestTuXinPhienMoiKhiPhienChet(t *testing.T) {
	var accepted atomic.Value
	accepted.Store("sid-moi")

	var logins int32
	misaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == loginPath {
			atomic.AddInt32(&logins, 1)
			r.ParseForm()
			if r.PostForm.Get("sid") != accepted.Load().(string) {
				io.WriteString(w, `{"Success":false,"Code":4,"UserMessage":"Session không hợp lệ","ErrorsMessage":[]}`)
				return
			}
			io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":{`+
				`"AccessToken":{"Token":"tok","TokenExpired":86400.0},"Context":`+longAnCtx+`}}`)
			return
		}
		io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":{"ok":true}}`)
	}))
	defer misaSrv.Close()

	var fetches int32
	sidSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetches, 1)
		io.WriteString(w, `{"sid":"sid-moi","tid":"t","mid":"m","dbid":"d","x_device":"dev"}`)
	}))
	defer sidSrv.Close()

	var saved *Session
	c := NewClient(misaSrv.URL)
	c.UseSession(&Session{SID: "sid-cu-da-chet", TenantID: "t", MisaID: "m", DatabaseID: "d"})
	c.SetRenewFromURL(sidSrv.URL, func(s *Session) error { saved = s; return nil })

	if _, err := c.Get(context.Background(), "/whatever", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}

	if n := atomic.LoadInt32(&fetches); n != 1 {
		t.Errorf("gọi endpoint cấp phiên %d lần, muốn 1", n)
	}
	if saved == nil || saved.SID != "sid-moi" {
		t.Errorf("phiên mới chưa được trao để lưu: %+v", saved)
	}
	if c.Session() == nil || c.Session().SID != "sid-moi" {
		t.Errorf("client chưa chuyển sang phiên mới: %+v", c.Session())
	}
}

// Không có nguồn cấp phiên thì phiên chết là dừng, không được lặp vô hạn.
func TestPhienChetVaKhongCoNguonCapThiDung(t *testing.T) {
	var logins int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&logins, 1)
		io.WriteString(w, `{"Success":false,"Code":4,"UserMessage":"Session không hợp lệ","ErrorsMessage":[]}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.UseSession(&Session{SID: "chet", TenantID: "t", MisaID: "m", DatabaseID: "d"})

	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("phải báo lỗi")
	}
	if n := atomic.LoadInt32(&logins); n != 1 {
		t.Errorf("thử đăng nhập %d lần, muốn 1", n)
	}
}

// Endpoint cấp phiên hỏng thì lỗi phải nói rõ là hỏng ở khâu xin phiên.
func TestLoiKhiEndpointCapPhienHong(t *testing.T) {
	misaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"Success":false,"Code":4,"UserMessage":"Session không hợp lệ","ErrorsMessage":[]}`)
	}))
	defer misaSrv.Close()

	sidSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"error":"OTP bị từ chối"}`)
	}))
	defer sidSrv.Close()

	c := NewClient(misaSrv.URL)
	c.UseSession(&Session{SID: "chet", TenantID: "t", MisaID: "m", DatabaseID: "d"})
	c.SetRenewFromURL(sidSrv.URL, nil)

	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("phải báo lỗi")
	}
	if !strings.Contains(err.Error(), "xin phiên mới") || !strings.Contains(err.Error(), "OTP bị từ chối") {
		t.Errorf("lỗi không nêu rõ khâu hỏng: %v", err)
	}
}

// Gặp trên MISA thật: sid sai thì server trả Success=true nhưng Data rỗng.
// Phải coi đó là phiên chết, nếu không nhánh xin phiên mới không bao giờ chạy.
func TestSidSaiTraSuccessRongVanPhaiXinPhienMoi(t *testing.T) {
	var logins int32
	misaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != loginPath {
			io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":{"ok":true}}`)
			return
		}
		atomic.AddInt32(&logins, 1)
		r.ParseForm()
		if r.PostForm.Get("sid") == "sid-moi" {
			io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":{`+
				`"AccessToken":{"Token":"tok","TokenExpired":86400.0},"Context":`+longAnCtx+`}}`)
			return
		}
		// Đây là phản hồi thật của MISA cho sid không hợp lệ.
		io.WriteString(w, `{"Success":true,"Code":0,"SubCode":0,"ErrorsMessage":[],"Data":null}`)
	}))
	defer misaSrv.Close()

	sidSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"sid":"sid-moi","tid":"t","mid":"m","dbid":"d","x_device":"dev"}`)
	}))
	defer sidSrv.Close()

	c := NewClient(misaSrv.URL)
	c.UseSession(&Session{SID: "sid-chet", TenantID: "t", MisaID: "m", DatabaseID: "d"})
	c.SetRenewFromURL(sidSrv.URL, nil)

	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if c.Session().SID != "sid-moi" {
		t.Errorf("chưa chuyển sang phiên mới: %+v", c.Session())
	}
}

// Cùng tình huống nhưng không có nguồn cấp phiên: lỗi phải chỉ ra cách khắc phục.
func TestSidSaiKhongCoNguonCapThiBaoRoRang(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":null}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.UseSession(&Session{SID: "chet", TenantID: "t", MisaID: "m", DatabaseID: "d"})

	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("phải báo lỗi")
	}
	if !strings.Contains(err.Error(), "refresh-session") {
		t.Errorf("lỗi nên chỉ cách lấy phiên mới: %v", err)
	}
}
