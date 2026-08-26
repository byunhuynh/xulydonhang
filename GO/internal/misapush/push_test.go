package misapush

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"order-processor/internal/misa"
)

// fakeMISA dựng lại đúng chuỗi phản hồi mà AMIS Kế toán trả về, ghi lại
// thứ tự các endpoint đã bị gọi.
type fakeMISA struct {
	mu      sync.Mutex
	calls   []string
	invalid bool // step3 trả về một dòng không hợp lệ
}

const misaContextJSON = `{"TenantId":"t-1","TenantCode":"CODE","DatabaseId":"db-cu","UserId":"u-1"}`

func (f *fakeMISA) record(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, path)
}

func (f *fakeMISA) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeMISA) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case p == "/g2/api/auth/v1/account/login/misa_id":
			f.record("login")
			w.Write([]byte(`{"Success":true,"Data":{"AccessToken":{"Token":"tok","TokenExpired":86400},"Context":` + misaContextJSON + `}}`))

		case strings.HasPrefix(p, "/g2/api/system/v1/database/databases_user_can_see"):
			f.record("databases")
			w.Write([]byte(`{"Success":true,"Data":[{"database_id":"db-htla","database_name":"2025-Thuế-Long An"},{"database_id":"db-ht","database_name":"2025-NỘI BỘ- HÀ THÀNH"}]}`))

		case strings.HasPrefix(p, "/g2/api/auth/v1/account/database-context/"):
			f.record("database-context:" + strings.Split(p, "/")[7])
			w.Write([]byte(`{"Success":true,"Data":{"IsSwitchTenant":false,"Context":` + misaContextJSON + `}}`))

		case p == "/g2/api/file/v1/file/multi":
			f.record("upload")
			w.Write([]byte(`{"Success":true,"Data":[{"name":"tok.xlsx","ext":".xlsx","size":1,"index":0,"source":"htla.xlsx"}]}`))

		case p == "/g2/api/import/v1/import/sheetname":
			f.record("sheetname")
			w.Write([]byte(`{"Success":true,"Data":{"sheets":[{"Index":0,"Name":"Don dat hang","CountRow":10,"MaxDataRow":2,"RowHeaderUltil":7}]}}`))

		case p == "/g2/api/import/v1/import/step2":
			f.record("step2")
			w.Write([]byte(`{"Success":true,"Data":{"excel_columns":["Số đơn hàng"],"columns":[{"column_id":"refno","column_name":"Số đơn hàng","column_excel":0,"not_null":true}],"token":"tok.xlsx"}}`))

		case p == "/g2/api/import/v1/import/step3":
			f.record("step3")
			w.Write([]byte(`{"Success":true,"Data":"sess-3"}`))

		case p == "/g2/api/import/v1/worker/check_step3":
			f.record("check_step3")
			if f.invalid {
				w.Write([]byte(`{"Success":true,"Data":{"end":true,"step_data":[{"refno":"PO-1","is_valid":false,"validate_description":"Mã khách hàng <X> không có trong danh mục"}]}}`))
				return
			}
			w.Write([]byte(`{"Success":true,"Data":{"end":true,"step_data":[{"refno":"PO-1","is_valid":true}]}}`))

		case p == "/g2/api/import/v1/import/step4":
			f.record("step4")
			w.Write([]byte(`{"Success":true,"Data":"sess-4"}`))

		case p == "/g2/api/import/v1/worker/check_step4":
			f.record("check_step4")
			w.Write([]byte(`{"Success":true,"Data":{"end":true,"valid":1,"invalid":0,"skip":0,"result_file":""}}`))

		default:
			f.record("KHÔNG RÕ:" + p)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func writeSession(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "misa-session.json")
	body, err := json.Marshal(map[string]string{
		"sid": "s-1", "tid": "t-1", "mid": "u-1", "dbid": "db-cu", "x_device": "dev-1",
	})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("ghi session: %v", err)
	}
	return path
}

func writeXLSX(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "htla.xlsx")
	if err := os.WriteFile(path, []byte("PK\x03\x04 giả lập"), 0o600); err != nil {
		t.Fatalf("ghi xlsx: %v", err)
	}
	return path
}

func TestHTTPPusher_ĐúngThứTựBướcVàGhiSổ(t *testing.T) {
	fake := &fakeMISA{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	dir := t.TempDir()
	res, err := (&HTTPPusher{}).Push(context.Background(), Request{
		BaseURL:     srv.URL,
		SessionPath: writeSession(t, dir),
		Database:    "Long An",
		FilePath:    writeXLSX(t, dir),
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !res.Committed || res.Valid != 1 {
		t.Errorf("res.Committed=%v res.Valid=%d, want true/1", res.Committed, res.Valid)
	}

	want := []string{"login", "databases", "database-context:db-htla", "upload", "sheetname", "step2", "step3", "check_step3", "step4", "check_step4"}
	got := fake.Calls()
	if len(got) != len(want) {
		t.Fatalf("thứ tự gọi = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("thứ tự gọi = %v, want %v", got, want)
		}
	}
}

func TestHTTPPusher_CònDòngLỗiThìKhôngGọiStep4(t *testing.T) {
	fake := &fakeMISA{invalid: true}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	dir := t.TempDir()
	res, err := (&HTTPPusher{}).Push(context.Background(), Request{
		BaseURL:     srv.URL,
		SessionPath: writeSession(t, dir),
		Database:    "Long An",
		FilePath:    writeXLSX(t, dir),
	})
	if err == nil {
		t.Fatal("Push = nil, want lỗi vì còn dòng không hợp lệ")
	}
	// Kết quả vẫn phải về được để bên gọi liệt kê ĐỦ các dòng lỗi, chứ
	// không chỉ dòng đầu nằm trong thông điệp lỗi.
	if res == nil || len(res.RowErrors) != 1 {
		t.Fatalf("res.RowErrors = %#v, want đúng 1 dòng lỗi", res)
	}
	for _, c := range fake.Calls() {
		if c == "step4" || c == "check_step4" {
			t.Error("đã gọi step4 dù còn dòng không hợp lệ — cả nhánh phải không ghi gì")
		}
	}
}

func TestHTTPPusher_ThiếuPhiênVàThiếuSidURLThìDừngTrướcKhiUpload(t *testing.T) {
	fake := &fakeMISA{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	dir := t.TempDir()
	_, err := (&HTTPPusher{}).Push(context.Background(), Request{
		BaseURL:     srv.URL,
		SessionPath: filepath.Join(dir, "không-có.json"),
		Database:    "Long An",
		FilePath:    writeXLSX(t, dir),
	})
	if err == nil {
		t.Fatal("Push = nil, want lỗi vì không có phiên lẫn nguồn cấp phiên")
	}
	for _, c := range fake.Calls() {
		if c == "upload" {
			t.Error("đã upload file dù chưa có phiên đăng nhập")
		}
	}
}

func TestHTTPPusher_GhiNhậtKýQuaRequestLog(t *testing.T) {
	fake := &fakeMISA{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	dir := t.TempDir()
	var lines []string
	_, err := (&HTTPPusher{}).Push(context.Background(), Request{
		BaseURL:     srv.URL,
		SessionPath: writeSession(t, dir),
		Database:    "Long An",
		FilePath:    writeXLSX(t, dir),
		Log:         func(s string) { lines = append(lines, s) },
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if len(lines) == 0 {
		t.Error("Request.Log không nhận được dòng nào — người dùng sẽ không thấy tiến độ")
	}
}

// TestHTTPPusher_KhôngCóFilePhiênThìXinTừSidURL khoá lại ĐƯỜNG CHÍNH của người
// dùng khi đã khai sid_url: máy chưa từng có misa-session.json vẫn đẩy được, vì
// client tự xin phiên ở endpoint cấp phiên (Google Apps Script) rồi GHI LẠI ra
// file cho những lần sau.
//
// Ba test phiên khác đều bắt đầu bằng một file phiên có sẵn, nên không cái nào
// đi qua nhánh này — mà đây lại đúng là nhánh chạy trên máy mới cài.
func TestHTTPPusher_KhôngCóFilePhiênThìXinTừSidURL(t *testing.T) {
	fake := &fakeMISA{}
	misaSrv := httptest.NewServer(fake.handler())
	defer misaSrv.Close()

	var sidCalls int
	sidSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sidCalls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"sid":"s-1","tid":"t-1","mid":"u-1","dbid":"db-cu","x_device":"dev-1"}`))
	}))
	defer sidSrv.Close()

	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "misa-session.json")

	res, err := (&HTTPPusher{}).Push(context.Background(), Request{
		BaseURL:     misaSrv.URL,
		SessionPath: sessionPath, // CỐ Ý chưa tồn tại
		SidURL:      sidSrv.URL,
		Database:    "Long An",
		FilePath:    writeXLSX(t, dir),
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if !res.Committed {
		t.Errorf("res.Committed = false, want true")
	}
	if sidCalls != 1 {
		t.Errorf("gọi endpoint cấp phiên %d lần, want đúng 1", sidCalls)
	}

	// Phiên vừa xin phải được ghi lại, nếu không thì mỗi lần đẩy lại tốn một
	// lượt gọi Apps Script (và có thể một mã OTP).
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("không ghi lại phiên vừa xin ra %s: %v", sessionPath, err)
	}
	if s, err := misa.LoadSession(sessionPath); err != nil {
		t.Errorf("file phiên vừa ghi không đọc lại được: %v", err)
	} else if s.SID != "s-1" || s.XDevice != "dev-1" {
		t.Errorf("phiên đã ghi = %+v, want sid=s-1 x_device=dev-1", s)
	}
}
