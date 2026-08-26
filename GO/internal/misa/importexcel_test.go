package misa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMISA dựng lại đúng các phản hồi đã bắt được từ actapp.misa.vn.
type fakeMISA struct {
	mu sync.Mutex

	uploadFields  map[string]string
	uploadName    string
	uploadCT      string
	step2Query    map[string]string
	step3Query    map[string]string
	step4Query    map[string]string
	step3Body     json.RawMessage
	step4Body     json.RawMessage
	step3Polls    int
	step4Polls    int
	step4Called   bool
	pollsBeforeOK int

	columnsJSON  string
	stepDataJSON string // để trống thì dùng bộ mẫu hợp lệ
}

const okEnvelope = `"Success":true,"Code":0,"SubCode":0,"ErrorsMessage":[]`

// hai cột: một cột bắt buộc đã ghép, một cột tuỳ chọn chưa ghép.
const defaultColumns = `[
 {"import_column_id":"9b225e7e","column_id":"refdate","column_name":"Ngày đơn hàng","column_excel":0,"column_name_excel":"Ngày đơn hàng","not_null":true,"data_type":"timestamp"},
 {"import_column_id":"f6325339","column_id":"refno","column_name":"Số đơn hàng","column_excel":1,"column_name_excel":"Số đơn hàng","not_null":true,"data_type":"nvarchar"},
 {"import_column_id":"53bdcf05","column_id":"master_custom_field3","column_name":"Trường mở rộng 3","column_name_excel":"","not_null":false,"data_type":"nvarchar"}
]`

func (f *fakeMISA) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/g2/api/file/v1/file/multi", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("upload không phải multipart hợp lệ: %v", err)
			http.Error(w, err.Error(), 400)
			return
		}
		f.mu.Lock()
		f.uploadFields = map[string]string{}
		for k, v := range r.MultipartForm.Value {
			f.uploadFields[k] = v[0]
		}
		if fh := r.MultipartForm.File["file"]; len(fh) > 0 {
			f.uploadName = fh[0].Filename
			f.uploadCT = fh[0].Header.Get("Content-Type")
		}
		f.mu.Unlock()

		io.WriteString(w, `{`+okEnvelope+`,"Data":[{"name":"697d3d92.xlsx","ext":".xlsx","size":15006,"index":0,"source":"dondathang.xlsx"}]}`)
	})

	mux.HandleFunc("/g2/api/import/v1/import/sheetname", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["fileName"] != "697d3d92.xlsx" {
			t.Errorf("sheetname nhận fileName = %q, muốn token từ bước upload", body["fileName"])
		}
		io.WriteString(w, `{`+okEnvelope+`,"Data":{"sheets":[{"Index":0,"Name":"Don dat hang","CountRow":9,"MaxDataRow":8,"RowHeaderUltil":7}]}}`)
	})

	mux.HandleFunc("/g2/api/import/v1/import/step2", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.step2Query = flatQuery(r)
		cols := f.columnsJSON
		f.mu.Unlock()
		io.WriteString(w, `{`+okEnvelope+`,"Data":{"excel_columns":["Ngày đơn hàng","Số đơn hàng"],"columns":`+cols+`,"token":"697d3d92.xlsx"}}`)
	})

	mux.HandleFunc("/g2/api/import/v1/import/step3", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.step3Query = flatQuery(r)
		f.step3Body = raw
		f.mu.Unlock()
		io.WriteString(w, `{"Success":true,"Code":210,"SubCode":0,"UserMessage":"Hệ thống đã ghi nhận lệnh nhập khẩu","ErrorsMessage":[],"Data":"sess-3"}`)
	})

	mux.HandleFunc("/g2/api/import/v1/worker/check_step3", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sessionStepID"); got != "sess-3" {
			t.Errorf("check_step3 sessionStepID = %q, muốn sess-3", got)
		}
		f.mu.Lock()
		f.step3Polls++
		done := f.step3Polls > f.pollsBeforeOK
		rows := f.stepDataJSON
		f.mu.Unlock()

		if !done {
			io.WriteString(w, `{`+okEnvelope+`,"Data":{"end":false},"LogStep":["đang xử lý"]}`)
			return
		}
		count := 2
		if rows == "" {
			rows = `[{"refno":"DH001","refdate":"25/08/2026","account_object_id":"KH001","quantity":"3","is_valid":true},` +
				`{"refno":"DH002","refdate":"26/08/2026","account_object_id":"KH002","quantity":"5","is_valid":true}]`
		} else {
			count = 0
		}
		io.WriteString(w, fmt.Sprintf(`{%s,"Data":{"end":true,"import_count":%d,"skip":8,"step_data":%s},"LogStep":["END: Xử lý xong ImportExcelStep3"]}`,
			okEnvelope, count, rows))
	})

	mux.HandleFunc("/g2/api/import/v1/import/step4", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.step4Called = true
		f.step4Query = flatQuery(r)
		f.step4Body = raw
		f.mu.Unlock()
		io.WriteString(w, `{"Success":true,"Code":210,"SubCode":0,"ErrorsMessage":[],"Data":"sess-4"}`)
	})

	mux.HandleFunc("/g2/api/import/v1/worker/check_step4", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.step4Polls++
		f.mu.Unlock()
		io.WriteString(w, `{`+okEnvelope+`,"Data":{"result_file":"","valid":2,"invalid":0,"skip":0,"end":true},"LogStep":["END: Xử lý xong ImportExcelStep4"]}`)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("gọi endpoint ngoài dự kiến: %s %s", r.Method, r.URL.Path)
		http.Error(w, "not found", 404)
	})
	return mux
}

func flatQuery(r *http.Request) map[string]string {
	out := map[string]string{}
	for k, v := range r.URL.Query() {
		out[k] = v[0]
	}
	return out
}

func newTestClient(t *testing.T, f *fakeMISA) (*Client, *httptest.Server) {
	t.Helper()
	if f.columnsJSON == "" {
		f.columnsJSON = defaultColumns
	}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL)
	c.SetHeader("Authorization", "Bearer test")
	c.SetHeader("X-MISA-Context", `{"TenantId":"tid","TenantCode":"TCODE"}`)
	return c, srv
}

func runImport(t *testing.T, c *Client, commit bool) (*ImportResult, error) {
	t.Helper()
	return c.ImportExcel(context.Background(), ImportOptions{
		FileName:     "dondathang.xlsx",
		Data:         []byte("PK\x03\x04 gia lap file xlsx"),
		Commit:       commit,
		PollInterval: time.Millisecond,
		PollTimeout:  5 * time.Second,
	})
}

func TestImportExcelDryRunKhongGhiSo(t *testing.T) {
	f := &fakeMISA{}
	c, _ := newTestClient(t, f)

	res, err := runImport(t, c, false)
	if err != nil {
		t.Fatalf("ImportExcel: %v", err)
	}

	if f.step4Called {
		t.Error("chế độ kiểm tra vẫn gọi step4 — dữ liệu bị ghi vào sổ")
	}
	if res.Committed {
		t.Error("Committed phải là false khi chưa -commit")
	}
	if res.Token != "697d3d92.xlsx" {
		t.Errorf("Token = %q", res.Token)
	}
	if res.Sheet.Name != "Don dat hang" || res.Sheet.MaxDataRow != 8 {
		t.Errorf("Sheet = %+v", res.Sheet)
	}
	if res.RowsParsed != 2 || len(res.Preview) != 2 {
		t.Errorf("RowsParsed = %d, preview = %d dòng", res.RowsParsed, len(res.Preview))
	}
	if res.Preview[0]["refno"] != "DH001" {
		t.Errorf("preview đầu = %+v", res.Preview[0])
	}
	if len(res.UnmappedRequired) != 0 {
		t.Errorf("không nên báo thiếu cột: %v", res.UnmappedRequired)
	}
}

func TestImportExcelGuiDungThamSo(t *testing.T) {
	f := &fakeMISA{}
	c, _ := newTestClient(t, f)

	if _, err := runImport(t, c, true); err != nil {
		t.Fatalf("ImportExcel: %v", err)
	}

	if f.uploadFields["type"] != "11" {
		t.Errorf("upload field type = %q, muốn 11", f.uploadFields["type"])
	}
	if f.uploadName != "dondathang.xlsx" {
		t.Errorf("upload filename = %q", f.uploadName)
	}
	if !strings.Contains(f.uploadCT, "spreadsheetml") {
		t.Errorf("upload content-type = %q", f.uploadCT)
	}

	want2 := map[string]string{
		"token": "697d3d92.xlsx", "sheetIndex": "0", "headerRowIndex": "8",
		"refType": "3520", "tableName": "sa_order", "voucherType": "0",
		"importType": "0", "isFullTemplate": "false", "isAutoMergeColumnWithMISAAVA": "false",
	}
	for k, v := range want2 {
		if f.step2Query[k] != v {
			t.Errorf("step2 query %s = %q, muốn %q", k, f.step2Query[k], v)
		}
	}

	if f.step3Query["take"] != "500" || f.step3Query["skip"] != "0" || f.step3Query["option"] != "true" {
		t.Errorf("step3 query = %+v", f.step3Query)
	}
	if f.step4Query["token"] != "697d3d92.xlsx" || f.step4Query["timeImport"] == "" {
		t.Errorf("step4 query = %+v", f.step4Query)
	}
	if _, err := time.Parse(http.TimeFormat, f.step4Query["timeImport"]); err != nil {
		t.Errorf("timeImport %q không đúng định dạng RFC1123 GMT: %v", f.step4Query["timeImport"], err)
	}
}

// Bản đồ cột từ step2 phải được bắn lại nguyên vẹn sang step3 và step4.
func TestImportExcelBanLaiBanDoCotNguyenVen(t *testing.T) {
	f := &fakeMISA{}
	c, _ := newTestClient(t, f)

	if _, err := runImport(t, c, true); err != nil {
		t.Fatalf("ImportExcel: %v", err)
	}

	var want, got3, got4 any
	if err := json.Unmarshal([]byte(defaultColumns), &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(f.step3Body, &got3); err != nil {
		t.Fatalf("step3 body không phải JSON: %v", err)
	}
	if err := json.Unmarshal(f.step4Body, &got4); err != nil {
		t.Fatalf("step4 body không phải JSON: %v", err)
	}
	if !jsonEqual(want, got3) {
		t.Errorf("step3 body khác bản đồ cột của step2:\n%s", f.step3Body)
	}
	if !jsonEqual(want, got4) {
		t.Errorf("step4 body khác bản đồ cột của step2:\n%s", f.step4Body)
	}
}

func TestImportExcelCommitTraVeSoLieu(t *testing.T) {
	f := &fakeMISA{}
	c, _ := newTestClient(t, f)

	res, err := runImport(t, c, true)
	if err != nil {
		t.Fatalf("ImportExcel: %v", err)
	}
	if !res.Committed || res.Valid != 2 || res.Invalid != 0 {
		t.Errorf("kết quả ghi sổ = %+v", res)
	}
	if res.Step3SessionID != "sess-3" || res.Step4SessionID != "sess-4" {
		t.Errorf("sessionStepID = %q / %q", res.Step3SessionID, res.Step4SessionID)
	}
}

func TestImportExcelChoWorkerXongMoiSangBuocSau(t *testing.T) {
	f := &fakeMISA{pollsBeforeOK: 3}
	c, _ := newTestClient(t, f)

	res, err := runImport(t, c, true)
	if err != nil {
		t.Fatalf("ImportExcel: %v", err)
	}
	if f.step3Polls < 4 {
		t.Errorf("chỉ hỏi worker %d lần — không chờ end=true", f.step3Polls)
	}
	if res.RowsParsed != 2 {
		t.Errorf("RowsParsed = %d", res.RowsParsed)
	}
}

func TestImportExcelBaoLoiKhiThieuCotBatBuoc(t *testing.T) {
	f := &fakeMISA{columnsJSON: `[
	 {"column_id":"refdate","column_name":"Ngày đơn hàng","column_excel":0,"not_null":true},
	 {"column_id":"refno","column_name":"Số đơn hàng","column_name_excel":"","not_null":true}
	]`}
	c, _ := newTestClient(t, f)

	res, err := runImport(t, c, true)
	if err == nil {
		t.Fatal("phải báo lỗi khi cột bắt buộc chưa ghép được")
	}
	if !strings.Contains(err.Error(), "Số đơn hàng") {
		t.Errorf("thông báo lỗi không nêu tên cột: %v", err)
	}
	if f.step3Query != nil || f.step4Called {
		t.Error("không được gửi step3/step4 khi thiếu cột bắt buộc")
	}
	if len(res.UnmappedRequired) != 1 {
		t.Errorf("UnmappedRequired = %v", res.UnmappedRequired)
	}
}

func TestImportExcelBaoLoiKhiChuaNapHeader(t *testing.T) {
	c := NewClient("https://example.invalid")
	_, err := runImport(t, c, false)
	if err == nil || !strings.Contains(err.Error(), "phiên đăng nhập") {
		t.Errorf("muốn lỗi thiếu xác thực, có: %v", err)
	}
}

func TestLoadHeadersFromCapture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests.jsonl")

	lines := []string{
		`{"seq":1,"host":"notify.misa.vn","request_headers":{"Authorization":"Bearer CU","X-MISA-Context":"{}"}}`,
		`{"seq":2,"host":"actapp.misa.vn","request_headers":{"Authorization":"Bearer A","X-MISA-Context":"{\"TenantCode\":\"OLD\"}","User-Agent":"x"}}`,
		`{ dòng hỏng`,
		`{"seq":9,"host":"actapp.misa.vn","request_headers":{"Authorization":"Bearer MOI","X-MISA-Context":"{\"TenantId\":\"tid\",\"TenantCode\":\"3HJRAPAH\"}","X-MISA-BranchId":"br-1","accept":"*/*"}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewClient("")
	if err := c.LoadHeadersFromCapture(path, ""); err != nil {
		t.Fatalf("LoadHeadersFromCapture: %v", err)
	}
	if c.Headers["Authorization"] != "Bearer MOI" {
		t.Errorf("phải lấy request mới nhất, có %q", c.Headers["Authorization"])
	}
	if c.Headers["X-MISA-BranchId"] != "br-1" {
		t.Errorf("thiếu X-MISA-BranchId: %+v", c.Headers)
	}
	if _, ok := c.Headers["accept"]; ok {
		t.Error("không nên mang theo header không liên quan tới xác thực")
	}

	tenant, err := c.Tenant()
	if err != nil {
		t.Fatalf("Tenant: %v", err)
	}
	if tenant.TenantCode != "3HJRAPAH" {
		t.Errorf("TenantCode = %q", tenant.TenantCode)
	}
}

func TestEnvelopeErr(t *testing.T) {
	// step3/step4 trả Code 210 kèm Success=true — không được coi là lỗi.
	ok := &Envelope{Success: true, Code: 210, UserMessage: "Hệ thống đã ghi nhận lệnh nhập khẩu"}
	if err := ok.Err(); err != nil {
		t.Errorf("Code 210 với Success=true không phải lỗi, có: %v", err)
	}

	bad := &Envelope{Success: false, Code: 900, UserMessage: "Không có quyền"}
	if err := bad.Err(); err == nil || !strings.Contains(err.Error(), "Không có quyền") {
		t.Errorf("muốn lỗi kèm UserMessage, có: %v", err)
	}
}

func jsonEqual(a, b any) bool {
	ba, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ba) == string(bb)
}

// Gặp trên MISA thật: giao diện web chờ WebSocket báo xong rồi mới hỏi worker
// một lần. Client poll sớm hơn nên nhận Data dạng chuỗi (id phiên) thay vì đối
// tượng trạng thái — phải coi là "chưa xong" và hỏi lại, không được bỏ cuộc.
func TestPollChiuDuocDataDangChuoi(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls++
		if polls < 3 {
			io.WriteString(w, `{`+okEnvelope+`,"Data":"112d0356-1c90-4618-a137-0e01894e0b30"}`)
			return
		}
		io.WriteString(w, `{`+okEnvelope+`,"Data":{"end":true,"import_count":1,"valid":1},"LogStep":["END"]}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetHeader("Authorization", "Bearer test")

	st, log, err := c.pollStep(context.Background(), "/check", "sess-1",
		ImportOptions{PollInterval: time.Millisecond, PollTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("pollStep: %v", err)
	}
	if polls != 3 {
		t.Errorf("hỏi %d lần, muốn 3", polls)
	}
	if !st.End || st.ImportCount != 1 {
		t.Errorf("trạng thái cuối = %+v", st)
	}
	if len(log) == 0 {
		t.Error("thiếu LogStep")
	}
}

// Nếu worker không bao giờ xong, lỗi phải nêu phản hồi cuối để còn lần ra.
func TestPollHetGioThiBaoPhanHoiCuoi(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{`+okEnvelope+`,"Data":"van-dang-cho"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetHeader("Authorization", "Bearer test")

	_, _, err := c.pollStep(context.Background(), "/check", "sess-1",
		ImportOptions{PollInterval: time.Millisecond, PollTimeout: 30 * time.Millisecond})
	if err == nil {
		t.Fatal("muốn lỗi hết giờ")
	}
	if !strings.Contains(err.Error(), "van-dang-cho") {
		t.Errorf("lỗi không nêu phản hồi cuối: %v", err)
	}
}
