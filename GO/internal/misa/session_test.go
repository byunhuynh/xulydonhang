package misa

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf16"
)

// encodeAmisSessionID làm ngược lại decodeAmisSessionID, chỉ dùng trong test.
func encodeAmisSessionID(s string) string {
	u := utf16.Encode([]rune(s))
	raw := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(raw[i*2:], v)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func TestDecodeAmisSessionID(t *testing.T) {
	// Giá trị thật lấy từ X-MISA-Context của phiên đã bắt.
	const b64 = "NQBlADEANQBiADAAMABmADkANQAzADYANABjADIANgBiADcAMwA4AGYAOAAzADYAOQA2AGEAYQA1AGUAZQA3ADgAOQBjADQAYgA2ADIAMQAyAGUANgAxADQAOQBjADcAOQAwAGEAMgBjADYAZQAwADAANwBmAGUAYgA4AGEAMQA="
	const want = "5e15b00f95364c26b738f83696aa5ee789c4b6212e6149c790a2c6e007feb8a1"

	got, err := decodeAmisSessionID(b64)
	if err != nil {
		t.Fatalf("decodeAmisSessionID: %v", err)
	}
	if got != want {
		t.Errorf("= %q, muốn %q", got, want)
	}

	if _, err := decodeAmisSessionID("!!!khong-phai-base64"); err == nil {
		t.Error("base64 hỏng phải báo lỗi")
	}
	if _, err := decodeAmisSessionID(base64.StdEncoding.EncodeToString([]byte{1, 2, 3})); err == nil {
		t.Error("độ dài lẻ phải báo lỗi")
	}
}

func TestSessionValid(t *testing.T) {
	s := &Session{SID: "a"}
	err := s.Valid()
	if err == nil {
		t.Fatal("thiếu trường phải báo lỗi")
	}
	for _, f := range []string{"tid", "mid", "dbid"} {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("lỗi không nêu %q: %v", f, err)
		}
	}
	full := &Session{SID: "a", TenantID: "b", MisaID: "c", DatabaseID: "d"}
	if err := full.Valid(); err != nil {
		t.Errorf("đủ trường mà vẫn lỗi: %v", err)
	}
}

func TestSessionFromCaptureVaLuuDoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests.jsonl")

	ctx := `{\"TenantId\":\"tid-1\",\"DatabaseId\":\"db-2\",\"UserId\":\"mid-3\",\"AmisSessionId\":\"` +
		encodeAmisSessionID("sid-xyz") + `\"}`
	lines := []string{
		`{"seq":1,"host":"notify.misa.vn","request_headers":{"X-MISA-Context":"` + ctx + `"}}`,
		`{"seq":2,"host":"actapp.misa.vn","request_headers":{"X-MISA-Context":"{\"TenantId\":\"cu\"}"}}`,
		`{ dòng hỏng`,
		`{"seq":9,"host":"actapp.misa.vn","request_headers":{"X-MISA-Context":"` + ctx + `","X-Device":"dev-9"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := SessionFromCapture(path, "")
	if err != nil {
		t.Fatalf("SessionFromCapture: %v", err)
	}
	if s.SID != "sid-xyz" || s.TenantID != "tid-1" || s.MisaID != "mid-3" ||
		s.DatabaseID != "db-2" || s.XDevice != "dev-9" {
		t.Fatalf("phiên đọc ra sai: %+v", s)
	}

	// Lưu rồi đọc lại phải y nguyên, và file không được chứa mật khẩu.
	out := filepath.Join(dir, "misa-session.json")
	if err := s.Save(out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(raw)), "password") {
		t.Error("file phiên không được chứa mật khẩu")
	}
	back, err := LoadSession(out)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if *back != *s {
		t.Errorf("đọc lại khác lúc lưu:\n%+v\n%+v", back, s)
	}

	// File thiếu trường phải bị từ chối chứ không im lặng chạy tiếp.
	bad := filepath.Join(dir, "thieu.json")
	os.WriteFile(bad, []byte(`{"sid":"a"}`), 0o600)
	if _, err := LoadSession(bad); err == nil {
		t.Error("file phiên thiếu trường phải báo lỗi")
	}
}

// fakeAuth dựng lại endpoint cấp token của MISA.
type fakeAuth struct {
	logins   int32
	form     url.Values
	device   string
	failNext bool
}

func (f *fakeAuth) server(t *testing.T, protected http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(loginPath, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&f.logins, 1)
		r.ParseForm()
		f.form = r.PostForm
		f.device = r.Header.Get("X-Device")
		if f.failNext {
			io.WriteString(w, `{"Success":false,"Code":4,"UserMessage":"Session không hợp lệ","ErrorsMessage":[]}`)
			return
		}
		io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":{`+
			`"AccessToken":{"Token":"tok-`+string(rune('A'+f.logins))+`","TokenExpired":86400.0},`+
			`"Context":`+longAnCtx+`,"Env":"g2"}}`)
	})
	if protected != nil {
		mux.HandleFunc("/protected", protected)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLoginWithSession(t *testing.T) {
	f := &fakeAuth{}
	srv := f.server(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":{}}`)
	})

	c := NewClient(srv.URL)
	s := &Session{SID: "sid-1", TenantID: "tid-1", MisaID: "mid-1", DatabaseID: "db-1", XDevice: "dev-1"}
	if err := c.LoginWithSession(context.Background(), s); err != nil {
		t.Fatalf("LoginWithSession: %v", err)
	}

	for k, want := range map[string]string{"sid": "sid-1", "tid": "tid-1", "mid": "mid-1", "dbid": "db-1", "lang": "vi"} {
		if got := f.form.Get(k); got != want {
			t.Errorf("form %s = %q, muốn %q", k, got, want)
		}
	}
	if f.device != "dev-1" {
		t.Errorf("X-Device gửi lên = %q", f.device)
	}
	if !strings.HasPrefix(c.Headers["Authorization"], "Bearer tok-") {
		t.Errorf("Authorization = %q", c.Headers["Authorization"])
	}
	// Ngữ cảnh phải lấy luôn từ phản hồi, khỏi gọi thêm database-context.
	if c.CurrentDatabaseID() != "b4668d9a-9877-44b9-a9d0-94fa6250d833" {
		t.Errorf("chưa nạp X-MISA-Context: %q", c.Headers["X-MISA-Context"])
	}
	if c.Session() == nil {
		t.Error("phải giữ lại phiên để tự gia hạn")
	}
}

func TestLoginWithSessionPhienChet(t *testing.T) {
	f := &fakeAuth{failNext: true}
	srv := f.server(t, nil)

	c := NewClient(srv.URL)
	s := &Session{SID: "cu", TenantID: "t", MisaID: "m", DatabaseID: "d"}
	err := c.LoginWithSession(context.Background(), s)
	if err == nil {
		t.Fatal("phiên chết phải báo lỗi")
	}
	if !strings.Contains(err.Error(), "misasniff") {
		t.Errorf("lỗi nên chỉ cách khắc phục: %v", err)
	}
}

// Token 24h có thể hết hạn giữa lúc đang chạy: client phải tự cấp lại một lần
// rồi gửi lại đúng request đó, không bắt người dùng chạy lại từ đầu.
func TestTuCapLaiTokenKhiGap401(t *testing.T) {
	var hits int32
	f := &fakeAuth{}
	srv := f.server(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"Success":false,"Code":401}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":`+
			string(mustJSON(map[string]any{"echo": string(body), "auth": r.Header.Get("Authorization")}))+`}`)
	})

	c := NewClient(srv.URL)
	c.UseSession(&Session{SID: "s", TenantID: "t", MisaID: "m", DatabaseID: "d"})

	env, err := c.PostJSON(context.Background(), "/protected", nil, map[string]string{"a": "b"})
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	var out struct{ Echo, Auth string }
	if err := env.Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Echo != `{"a":"b"}` {
		t.Errorf("body không được gửi lại nguyên vẹn sau khi cấp token: %q", out.Echo)
	}
	if n := atomic.LoadInt32(&f.logins); n != 2 {
		t.Errorf("số lần cấp token = %d, muốn 2 (1 lần đầu + 1 lần gia hạn)", n)
	}
	if hits != 2 {
		t.Errorf("request đích gọi %d lần, muốn 2", hits)
	}
}

// Không được đệ quy: phiên chết thì báo lỗi chứ đừng cấp token vô hạn.
func TestKhongCapTokenVoHanKhiPhienChet(t *testing.T) {
	f := &fakeAuth{failNext: true}
	srv := f.server(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"Success":false}`)
	})

	c := NewClient(srv.URL)
	c.UseSession(&Session{SID: "s", TenantID: "t", MisaID: "m", DatabaseID: "d"})

	if _, err := c.Get(context.Background(), "/protected", nil); err == nil {
		t.Fatal("phải báo lỗi")
	}
	if n := atomic.LoadInt32(&f.logins); n > 2 {
		t.Errorf("gọi cấp token %d lần — có vòng lặp", n)
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
