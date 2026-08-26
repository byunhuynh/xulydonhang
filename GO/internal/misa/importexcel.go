package misa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Loại chứng từ hay dùng. Danh sách đầy đủ lấy từ GET /g2/api/import/v1/import/types.
const (
	RefTypeSAOrder = 3520 // Đơn đặt hàng
	TableSAOrder   = "sa_order"

	RefTypePUOrder = 301 // Đơn mua hàng
	TablePUOrder   = "pu_order"
)

// uploadTypeImportExcel là giá trị field `type` khi upload file cho luồng nhập khẩu.
const uploadTypeImportExcel = "11"

const xlsxMime = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// ImportOptions cấu hình một lần nhập khẩu từ Excel.
type ImportOptions struct {
	// FilePath là file .xlsx theo mẫu của MISA. Bỏ qua nếu Data đã có.
	FilePath string
	FileName string // mặc định lấy từ FilePath
	Data     []byte // nội dung file, thay cho FilePath

	RefType   int    // mặc định RefTypeSAOrder
	TableName string // mặc định TableSAOrder

	SheetIndex     int // -1 = sheet đầu tiên
	HeaderRowIndex int // 0 = tự suy từ RowHeaderUltil của sheet
	VoucherType    int
	ImportType     int
	IsFullTemplate bool
	CustomParam    string

	Take int // số dòng mỗi lô, mặc định 500

	// Commit = false thì dừng sau bước kiểm tra dữ liệu, KHÔNG ghi vào sổ.
	Commit bool
	// Force cho phép ghi sổ dù còn dòng không hợp lệ. MISA sẽ bỏ qua các dòng đó.
	Force bool

	PollInterval time.Duration // mặc định 1s
	PollTimeout  time.Duration // mặc định 5 phút
}

func (o *ImportOptions) applyDefaults() {
	if o.RefType == 0 {
		o.RefType = RefTypeSAOrder
	}
	if o.TableName == "" {
		o.TableName = TableSAOrder
	}
	if o.Take == 0 {
		o.Take = 500
	}
	if o.PollInterval == 0 {
		o.PollInterval = time.Second
	}
	if o.PollTimeout == 0 {
		o.PollTimeout = 5 * time.Minute
	}
	if o.FileName == "" && o.FilePath != "" {
		o.FileName = filepath.Base(o.FilePath)
	}
}

// SheetInfo mô tả một sheet trong file Excel vừa tải lên.
type SheetInfo struct {
	Index          int    `json:"Index"`
	Name           string `json:"Name"`
	CountRow       int    `json:"CountRow"`
	MaxDataRow     int    `json:"MaxDataRow"`
	RowHeaderUltil int    `json:"RowHeaderUltil"` // chính tả của MISA, giữ nguyên
}

// MappedColumn là một cột của chứng từ đã được MISA ghép với cột trong Excel.
// ColumnExcel = nil nghĩa là cột chưa ghép được với cột nào.
type MappedColumn struct {
	ColumnID        string `json:"column_id"`
	ColumnName      string `json:"column_name"`
	ColumnExcel     *int   `json:"column_excel"`
	ColumnNameExcel string `json:"column_name_excel"`
	NotNull         bool   `json:"not_null"`
	DataType        string `json:"data_type"`
}

// RowError là một dòng bị MISA từ chối ở bước kiểm tra dữ liệu.
type RowError struct {
	Row         int    `json:"row"`
	RefNo       string `json:"refno,omitempty"`
	Description string `json:"description"`
}

func (e RowError) String() string {
	if e.RefNo != "" {
		return fmt.Sprintf("dòng %d (%s): %s", e.Row, e.RefNo, e.Description)
	}
	return fmt.Sprintf("dòng %d: %s", e.Row, e.Description)
}

// ImportResult gom lại mọi thứ biết được sau một lần chạy.
type ImportResult struct {
	Token        string         `json:"token"`
	Sheet        SheetInfo      `json:"sheet"`
	ExcelColumns []string       `json:"excel_columns"`
	Columns      []MappedColumn `json:"columns"`
	// UnmappedRequired là các cột bắt buộc chưa ghép được — nguyên nhân lỗi phổ biến nhất.
	UnmappedRequired []string `json:"unmapped_required"`
	// RawColumns là mảng cột nguyên bản từ step2, được bắn lại y hệt sang step3/step4.
	RawColumns json.RawMessage `json:"-"`

	Step3SessionID string           `json:"step3_session_id"`
	Preview        []map[string]any `json:"preview"`
	RowsParsed     int              `json:"rows_parsed"`
	ValidRows      int              `json:"valid_rows"`
	RowErrors      []RowError       `json:"row_errors,omitempty"`
	Step3Log       []string         `json:"step3_log"`

	Committed      bool     `json:"committed"`
	Step4SessionID string   `json:"step4_session_id,omitempty"`
	Valid          int      `json:"valid"`
	Invalid        int      `json:"invalid"`
	Skipped        int      `json:"skipped"`
	ResultFile     string   `json:"result_file,omitempty"`
	Step4Log       []string `json:"step4_log,omitempty"`
}

type uploadedFile struct {
	Name   string `json:"name"`
	Ext    string `json:"ext"`
	Size   int64  `json:"size"`
	Index  int    `json:"index"`
	Source string `json:"source"`
}

type sheetList struct {
	Sheets []SheetInfo `json:"sheets"`
}

type step2Data struct {
	ExcelColumns []string        `json:"excel_columns"`
	Columns      json.RawMessage `json:"columns"`
	Token        string          `json:"token"`
}

type stepStatus struct {
	End         bool             `json:"end"`
	StepData    []map[string]any `json:"step_data"`
	ImportCount int              `json:"import_count"`
	Skip        int              `json:"skip"`
	ResultFile  string           `json:"result_file"`
	Valid       int              `json:"valid"`
	Invalid     int              `json:"invalid"`
}

// ImportExcel chạy trọn luồng nhập khẩu chứng từ từ file Excel:
// upload → đọc sheet → lấy bản đồ cột → kiểm tra dữ liệu → (tuỳ chọn) ghi vào sổ.
//
// Với Commit = false, hàm dừng ngay sau bước kiểm tra: không có gì được ghi vào
// sổ kế toán, kết quả trả về là dữ liệu MISA đọc được để bạn đối chiếu.
func (c *Client) ImportExcel(ctx context.Context, opts ImportOptions) (*ImportResult, error) {
	opts.applyDefaults()

	data := opts.Data
	if data == nil {
		if opts.FilePath == "" {
			return nil, fmt.Errorf("cần FilePath hoặc Data")
		}
		var err error
		if data, err = os.ReadFile(opts.FilePath); err != nil {
			return nil, err
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("file rỗng")
	}

	res := &ImportResult{}

	// Bước 1: tải file lên, lấy token (chính là tên file server đặt lại).
	c.logf("1/5 tải lên %s (%d bytes)", opts.FileName, len(data))
	env, err := c.PostMultipart(ctx, "/g2/api/file/v1/file/multi", nil,
		map[string]string{"type": uploadTypeImportExcel},
		[]FileUpload{{Field: "file", FileName: opts.FileName, ContentType: xlsxMime, Data: data}})
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	if err := env.Err(); err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	var uploaded []uploadedFile
	if err := env.Decode(&uploaded); err != nil {
		return nil, fmt.Errorf("upload: đọc phản hồi: %w", err)
	}
	if len(uploaded) == 0 {
		return nil, fmt.Errorf("upload: server không trả về file nào")
	}
	res.Token = uploaded[0].Name
	c.logf("    token = %s", res.Token)

	// Bước 2: hỏi danh sách sheet để biết dòng tiêu đề nằm ở đâu.
	c.logf("2/5 đọc danh sách sheet")
	env, err = c.PostJSON(ctx, "/g2/api/import/v1/import/sheetname", nil,
		map[string]string{"fileName": res.Token, "customParam": opts.CustomParam})
	if err != nil {
		return nil, fmt.Errorf("sheetname: %w", err)
	}
	if err := env.Err(); err != nil {
		return nil, fmt.Errorf("sheetname: %w", err)
	}
	var sheets sheetList
	if err := env.Decode(&sheets); err != nil {
		return nil, fmt.Errorf("sheetname: đọc phản hồi: %w", err)
	}
	sheet, err := pickSheet(sheets.Sheets, opts.SheetIndex)
	if err != nil {
		return nil, err
	}
	res.Sheet = sheet

	headerRow := opts.HeaderRowIndex
	if headerRow == 0 {
		headerRow = sheet.RowHeaderUltil + 1
	}
	c.logf("    sheet %d %q — %d dòng, tiêu đề ở dòng %d", sheet.Index, sheet.Name, sheet.MaxDataRow, headerRow)

	// Bước 3: lấy bản đồ cột mà MISA tự ghép giữa mẫu và file Excel.
	c.logf("3/5 lấy bản đồ cột (refType=%d, tableName=%s)", opts.RefType, opts.TableName)
	q := url.Values{}
	q.Set("token", res.Token)
	q.Set("sheetIndex", strconv.Itoa(sheet.Index))
	q.Set("headerRowIndex", strconv.Itoa(headerRow))
	q.Set("importType", strconv.Itoa(opts.ImportType))
	q.Set("refType", strconv.Itoa(opts.RefType))
	q.Set("tableName", opts.TableName)
	q.Set("voucherType", strconv.Itoa(opts.VoucherType))
	q.Set("isFullTemplate", strconv.FormatBool(opts.IsFullTemplate))
	q.Set("customParam", opts.CustomParam)
	q.Set("isAutoMergeColumnWithMISAAVA", "false")

	env, err = c.PostJSON(ctx, "/g2/api/import/v1/import/step2", q, nil)
	if err != nil {
		return nil, fmt.Errorf("step2: %w", err)
	}
	if err := env.Err(); err != nil {
		return nil, fmt.Errorf("step2: %w", err)
	}
	var s2 step2Data
	if err := env.Decode(&s2); err != nil {
		return nil, fmt.Errorf("step2: đọc phản hồi: %w", err)
	}
	if len(s2.Columns) == 0 {
		return nil, fmt.Errorf("step2: không có bản đồ cột nào — kiểm tra lại refType/tableName")
	}

	res.ExcelColumns = s2.ExcelColumns
	res.RawColumns = s2.Columns
	if err := json.Unmarshal(s2.Columns, &res.Columns); err != nil {
		return nil, fmt.Errorf("step2: đọc bản đồ cột: %w", err)
	}
	for _, col := range res.Columns {
		if col.NotNull && col.ColumnExcel == nil {
			res.UnmappedRequired = append(res.UnmappedRequired, col.ColumnName)
		}
	}
	c.logf("    ghép được %d cột; %d cột bắt buộc chưa ghép", len(res.Columns), len(res.UnmappedRequired))
	if len(res.UnmappedRequired) > 0 {
		return res, fmt.Errorf("thiếu cột bắt buộc trong file Excel: %s",
			strings.Join(res.UnmappedRequired, ", "))
	}

	// Bước 4: gửi bản đồ cột để MISA đọc và kiểm tra dữ liệu (chưa ghi sổ).
	c.logf("4/5 kiểm tra dữ liệu")
	q3 := url.Values{}
	q3.Set("token", res.Token)
	q3.Set("voucherType", strconv.Itoa(opts.VoucherType))
	q3.Set("option", "true")
	q3.Set("skip", "0")
	q3.Set("take", strconv.Itoa(opts.Take))
	q3.Set("isAutoGetItemCombo", "false")

	env, err = c.PostRawJSON(ctx, "/g2/api/import/v1/import/step3", q3, res.RawColumns)
	if err != nil {
		return res, fmt.Errorf("step3: %w", err)
	}
	if err := env.Err(); err != nil {
		return res, fmt.Errorf("step3: %w", err)
	}
	if err := json.Unmarshal(env.Data, &res.Step3SessionID); err != nil {
		return res, fmt.Errorf("step3: không đọc được sessionStepID: %w", err)
	}

	status, log, err := c.pollStep(ctx, "/g2/api/import/v1/worker/check_step3", res.Step3SessionID, opts)
	if err != nil {
		return res, fmt.Errorf("step3: %w", err)
	}
	res.Preview = status.StepData
	res.RowsParsed = len(status.StepData)
	res.RowErrors = collectRowErrors(status.StepData)
	// Không tin import_count: đã gặp trường hợp MISA trả import_count = 1 trong khi
	// chính dòng đó có is_valid = false ("Số đơn hàng đã tồn tại trên phần mềm").
	res.ValidRows = res.RowsParsed - len(res.RowErrors)
	res.Step3Log = log
	c.logf("    MISA đọc được %d chứng từ, %d hợp lệ", res.RowsParsed, res.ValidRows)
	for _, e := range res.RowErrors {
		c.logf("    ✗ %s", e)
	}

	if !opts.Commit {
		c.logf("5/5 bỏ qua ghi sổ (Commit = false)")
		return res, nil
	}
	// Ghi sổ khi còn dòng lỗi thì MISA bỏ qua đúng những dòng đó — dễ tưởng
	// đã đẩy xong mà thực ra thiếu. Chặn lại trừ khi người dùng cố ý.
	if len(res.RowErrors) > 0 && !opts.Force {
		return res, fmt.Errorf("%d/%d chứng từ không hợp lệ, không ghi sổ: %s (sửa file rồi chạy lại, hoặc bật Force để ghi phần hợp lệ)",
			len(res.RowErrors), res.RowsParsed, res.RowErrors[0])
	}

	// Bước 5: ghi vào sổ.
	c.logf("5/5 ghi vào sổ")
	q4 := url.Values{}
	q4.Set("token", res.Token)
	q4.Set("skip", "0")
	q4.Set("take", strconv.Itoa(opts.Take))
	q4.Set("timeImport", time.Now().UTC().Format(http.TimeFormat))
	q4.Set("isSumDetailVoucherData", "false")
	q4.Set("isAutoGetUnitPrice", "false")

	env, err = c.PostRawJSON(ctx, "/g2/api/import/v1/import/step4", q4, res.RawColumns)
	if err != nil {
		return res, fmt.Errorf("step4: %w", err)
	}
	if err := env.Err(); err != nil {
		return res, fmt.Errorf("step4: %w", err)
	}
	if err := json.Unmarshal(env.Data, &res.Step4SessionID); err != nil {
		return res, fmt.Errorf("step4: không đọc được sessionStepID: %w", err)
	}

	status, log, err = c.pollStep(ctx, "/g2/api/import/v1/worker/check_step4", res.Step4SessionID, opts)
	if err != nil {
		return res, fmt.Errorf("step4: %w", err)
	}
	res.Committed = true
	res.Valid, res.Invalid, res.Skipped = status.Valid, status.Invalid, status.Skip
	res.ResultFile = status.ResultFile
	res.Step4Log = log
	c.logf("    xong: %d hợp lệ, %d lỗi, %d bỏ qua", res.Valid, res.Invalid, res.Skipped)

	return res, nil
}

// pollStep hỏi worker cho tới khi bước xử lý nền báo end = true.
func (c *Client) pollStep(ctx context.Context, path, sessionID string, opts ImportOptions) (stepStatus, []string, error) {
	q := url.Values{}
	q.Set("sessionStepID", sessionID)
	q.Set("source", "socket")

	deadline := time.Now().Add(opts.PollTimeout)
	lastData := ""

	for attempt := 1; ; attempt++ {
		env, err := c.Get(ctx, path, q)
		if err != nil {
			return stepStatus{}, nil, err
		}
		if err := env.Err(); err != nil {
			return stepStatus{}, env.LogStep, err
		}

		// Giao diện web chờ WebSocket báo xong rồi mới hỏi worker đúng một lần.
		// Hỏi sớm hơn thì Data về dạng chuỗi chứ không phải đối tượng trạng thái —
		// coi như "chưa xong" và hỏi lại, thay vì bỏ cuộc.
		var st stepStatus
		if len(env.Data) > 0 {
			lastData = truncate(env.Data, 200)
			if err := json.Unmarshal(env.Data, &st); err != nil {
				c.logf("    …worker chưa xong (Data = %s)", lastData)
			} else if st.End {
				return st, env.LogStep, nil
			}
		}

		if time.Now().After(deadline) {
			return st, env.LogStep, fmt.Errorf(
				"quá %s mà tiến trình nền chưa xong (session %s, phản hồi cuối: %s)",
				opts.PollTimeout, sessionID, lastData)
		}
		c.logf("    …đang xử lý (lần hỏi %d)", attempt)

		select {
		case <-ctx.Done():
			return st, env.LogStep, ctx.Err()
		case <-time.After(opts.PollInterval):
		}
	}
}

func pickSheet(sheets []SheetInfo, want int) (SheetInfo, error) {
	if len(sheets) == 0 {
		return SheetInfo{}, fmt.Errorf("file không có sheet nào đọc được")
	}
	if want < 0 {
		return sheets[0], nil
	}
	for _, s := range sheets {
		if s.Index == want {
			return s, nil
		}
	}
	names := make([]string, 0, len(sheets))
	for _, s := range sheets {
		names = append(names, fmt.Sprintf("%d:%s", s.Index, s.Name))
	}
	return SheetInfo{}, fmt.Errorf("không có sheet index %d; file có: %s", want, strings.Join(names, ", "))
}

// ImportType là một loại chứng từ nhập khẩu được.
type ImportType struct {
	RefType     int    `json:"reftype"`
	Description string `json:"description"`
	TableMaster string `json:"table_master"`
}

// ImportTypes liệt kê các loại chứng từ nhập khẩu được, để tra refType/tableName.
func (c *Client) ImportTypes(ctx context.Context) ([]ImportType, error) {
	env, err := c.Get(ctx, "/g2/api/import/v1/import/types", nil)
	if err != nil {
		return nil, err
	}
	if err := env.Err(); err != nil {
		return nil, err
	}
	var out []ImportType
	if err := env.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// collectRowErrors nhặt các dòng bị MISA đánh dấu không hợp lệ ở bước kiểm tra.
func collectRowErrors(rows []map[string]any) []RowError {
	var out []RowError
	for i, r := range rows {
		valid, ok := r["is_valid"].(bool)
		if !ok || valid {
			continue
		}
		desc, _ := r["validate_description"].(string)
		if desc == "" {
			desc = "không rõ nguyên nhân"
		}
		refno, _ := r["refno"].(string)
		out = append(out, RowError{Row: i + 1, RefNo: refno, Description: desc})
	}
	return out
}
