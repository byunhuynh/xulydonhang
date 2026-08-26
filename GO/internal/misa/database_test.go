package misa

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var sampleDBs = []Database{
	{DatabaseID: "c05e12ab-76aa-48e9-a211-956c08d07387", DatabaseName: "2025-NỘI BỘ- HÀ THÀNH"},
	{DatabaseID: "b4668d9a-9877-44b9-a9d0-94fa6250d833", DatabaseName: "2025-Thuế-Long An"},
	{DatabaseID: "aaaa1111-0000-0000-0000-000000000000", DatabaseName: "2024-Thuế-Long An cũ"},
}

func TestFindDatabase(t *testing.T) {
	if d, err := FindDatabase(sampleDBs, "b4668d9a-9877-44b9-a9d0-94fa6250d833"); err != nil ||
		d.DatabaseName != "2025-Thuế-Long An" {
		t.Errorf("tra theo id: %+v, %v", d, err)
	}
	if d, err := FindDatabase(sampleDBs, "hà thành"); err != nil || !strings.Contains(d.DatabaseName, "HÀ THÀNH") {
		t.Errorf("tra theo tên không phân biệt hoa thường: %+v, %v", d, err)
	}
	if _, err := FindDatabase(sampleDBs, "Long An"); err == nil {
		t.Error("khớp 2 bộ dữ liệu thì phải báo mơ hồ, không được chọn bừa")
	} else if !strings.Contains(err.Error(), "2025-Thuế-Long An") {
		t.Errorf("lỗi mơ hồ phải liệt kê các lựa chọn: %v", err)
	}
	if _, err := FindDatabase(sampleDBs, "Đà Nẵng"); err == nil {
		t.Error("không khớp gì thì phải báo lỗi")
	}
	if _, err := FindDatabase(sampleDBs, ""); err == nil {
		t.Error("chuỗi rỗng phải báo lỗi")
	}
}

const haThanhCtx = `{"TenantId":"tid","TenantCode":"3HJRAPAH","DatabaseId":"c05e12ab-76aa-48e9-a211-956c08d07387","BranchId":"a026647f","UserId":"uid-1"}`
const longAnCtx = `{"TenantId":"tid","TenantCode":"3HJRAPAH","DatabaseId":"b4668d9a-9877-44b9-a9d0-94fa6250d833","BranchId":"0e910c8f","UserId":"uid-1"}`

// Endpoint đổi ngữ cảnh chỉ chạy khi có header X-Device; thiếu nó MISA trả
// "Error while process request." — lỗi phải chỉ thẳng nguyên nhân đó.
func TestSwitchDatabase(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		if r.Header.Get("X-Device") == "" {
			io.WriteString(w, `{"Success":false,"Code":4,"UserMessage":"Error while process request.","ErrorsMessage":[]}`)
			return
		}
		io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":{"IsSwitchTenant":false,"Context":`+longAnCtx+`}}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetHeader("Authorization", "Bearer test")
	c.SetHeader("X-MISA-Context", haThanhCtx)

	// Chưa có X-Device: lỗi phải nêu đúng nguyên nhân.
	err := c.SwitchDatabase(context.Background(), "b4668d9a-9877-44b9-a9d0-94fa6250d833")
	if err == nil {
		t.Fatal("thiếu X-Device thì phải báo lỗi")
	}
	if !strings.Contains(err.Error(), "X-Device") {
		t.Errorf("lỗi không nêu X-Device: %v", err)
	}
	if c.CurrentDatabaseID() != "c05e12ab-76aa-48e9-a211-956c08d07387" {
		t.Error("đổi thất bại thì không được thay ngữ cảnh")
	}

	// Có X-Device: đổi thành công và header được thay.
	c.SetHeader("X-Device", "0add708bdedb113360b7ba9610e0faab")
	if err := c.SwitchDatabase(context.Background(), "b4668d9a-9877-44b9-a9d0-94fa6250d833"); err != nil {
		t.Fatalf("SwitchDatabase: %v", err)
	}
	if c.CurrentDatabaseID() != "b4668d9a-9877-44b9-a9d0-94fa6250d833" {
		t.Errorf("DatabaseId sau khi đổi = %q", c.CurrentDatabaseID())
	}
	if want := "/g2/api/auth/v1/account/database-context/b4668d9a-9877-44b9-a9d0-94fa6250d833/uid-1"; gotPath != want {
		t.Errorf("path = %q, muốn %q", gotPath, want)
	}
	if gotQuery != "isContinueAccessDBBackup=false" {
		t.Errorf("query = %q", gotQuery)
	}

	// Ngữ cảnh mới phải là JSON hợp lệ, không còn bản cũ sót lại.
	var ctx map[string]any
	if err := json.Unmarshal([]byte(c.Headers["X-MISA-Context"]), &ctx); err != nil {
		t.Fatalf("X-MISA-Context mới không phải JSON: %v", err)
	}
	if ctx["BranchId"] != "0e910c8f" {
		t.Errorf("BranchId phải đổi theo bộ dữ liệu, có %v", ctx["BranchId"])
	}
	n := 0
	for k := range c.Headers {
		if strings.EqualFold(k, "x-misa-context") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("có %d biến thể header X-MISA-Context, muốn 1", n)
	}
}

func TestSwitchDatabaseByNameBoQuaKhiDaDungCho(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "databases_user_can_see") {
			io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":[`+
				`{"database_id":"c05e12ab-76aa-48e9-a211-956c08d07387","database_name":"2025-NỘI BỘ- HÀ THÀNH"},`+
				`{"database_id":"b4668d9a-9877-44b9-a9d0-94fa6250d833","database_name":"2025-Thuế-Long An"}]}`)
			return
		}
		calls++
		io.WriteString(w, `{"Success":true,"Code":0,"ErrorsMessage":[],"Data":{"Context":`+longAnCtx+`}}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetHeader("Authorization", "Bearer test")
	c.SetHeader("X-Device", "dev")
	c.SetHeader("X-MISA-Context", longAnCtx)

	db, err := c.SwitchDatabaseByName(context.Background(), "Long An")
	if err != nil {
		t.Fatalf("SwitchDatabaseByName: %v", err)
	}
	if db.DatabaseName != "2025-Thuế-Long An" {
		t.Errorf("db = %+v", db)
	}
	if calls != 0 {
		t.Error("đang ở đúng bộ dữ liệu rồi thì không cần gọi database-context")
	}
}

func TestLoadHeadersFromCaptureLayXDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.jsonl")
	line := `{"seq":1,"host":"actapp.misa.vn","request_headers":{"Authorization":"Bearer A","X-MISA-Context":"{}","X-Device":"0add708b","Referer":"x"}}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewClient("")
	if err := c.LoadHeadersFromCapture(path, ""); err != nil {
		t.Fatal(err)
	}
	if c.Headers["X-Device"] != "0add708b" {
		t.Errorf("thiếu X-Device: %+v", c.Headers)
	}
	if _, ok := c.Headers["Referer"]; ok {
		t.Error("không nên mang theo Referer")
	}
}

func TestCollectRowErrors(t *testing.T) {
	rows := []map[string]any{
		{"refno": "DH001", "is_valid": true, "validate_description": ""},
		{"refno": "Test11", "is_valid": false, "validate_description": "Mã khách hàng <MN_TMDT_00015> không có trong danh mục"},
		{"refno": "DH003", "is_valid": false},
	}
	errs := collectRowErrors(rows)
	if len(errs) != 2 {
		t.Fatalf("muốn 2 lỗi, có %d: %+v", len(errs), errs)
	}
	if errs[0].Row != 2 || errs[0].RefNo != "Test11" ||
		!strings.Contains(errs[0].Description, "MN_TMDT_00015") {
		t.Errorf("lỗi đầu = %+v", errs[0])
	}
	if errs[1].Description != "không rõ nguyên nhân" {
		t.Errorf("thiếu validate_description thì phải có mô tả thay thế: %+v", errs[1])
	}
	if s := errs[0].String(); !strings.Contains(s, "dòng 2") || !strings.Contains(s, "Test11") {
		t.Errorf("String() = %q", s)
	}
}

// Gặp trên MISA thật: đẩy file có mã khách hàng không tồn tại ở bộ dữ liệu đích.
// MISA vẫn nhận lệnh nhưng bỏ qua dòng lỗi — dễ tưởng đã đẩy xong mà thực ra không.
func TestKhongGhiSoKhiConDongLoi(t *testing.T) {
	invalid := `[{"refno":"Test11","is_valid":false,"validate_description":"Mã khách hàng <MN_TMDT_00015> không có trong danh mục"}]`

	f := &fakeMISA{stepDataJSON: invalid}
	c, _ := newTestClient(t, f)

	res, err := runImport(t, c, true)
	if err == nil {
		t.Fatal("còn dòng lỗi mà vẫn ghi sổ")
	}
	if f.step4Called {
		t.Error("không được gọi step4 khi còn dòng lỗi")
	}
	if !strings.Contains(err.Error(), "MN_TMDT_00015") {
		t.Errorf("lỗi phải nêu lý do MISA trả về: %v", err)
	}
	if len(res.RowErrors) != 1 || res.ValidRows != 0 || res.RowsParsed != 1 {
		t.Errorf("res = %+v", res)
	}
}

func TestForceVanGhiSoDuConDongLoi(t *testing.T) {
	invalid := `[{"refno":"Test11","is_valid":false,"validate_description":"Mã khách hàng không có trong danh mục"}]`

	f := &fakeMISA{stepDataJSON: invalid}
	c, _ := newTestClient(t, f)

	res, err := c.ImportExcel(context.Background(), ImportOptions{
		FileName:     "dondathang.xlsx",
		Data:         []byte("PK\x03\x04"),
		Commit:       true,
		Force:        true,
		PollInterval: time.Millisecond,
		PollTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Force bật thì phải chạy tiếp: %v", err)
	}
	if !f.step4Called || !res.Committed {
		t.Error("Force bật mà không ghi sổ")
	}
	if len(res.RowErrors) != 1 {
		t.Errorf("vẫn phải báo cáo dòng lỗi: %+v", res.RowErrors)
	}
}
