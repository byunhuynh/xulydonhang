# Port tính năng Upload Đơn Hàng lên Google Drive (Python → Go)

## Bối cảnh

Bản Python cũ (`xulydonhang.py`) có 1 tính năng thật, chạy trong production: sau khi xử lý xong MỖI đơn hàng (mọi vendor), file gốc được upload lên Google Drive qua 1 Google Apps Script endpoint, và 1 link "xem" được dựng sẵn ngay lập tức (không đợi upload thật xong). Tính năng này **chưa tồn tại** trong bản Go/Wails rewrite (`GO/`) — đã xác nhận bằng grep toàn bộ `GO/`, không có bất kỳ tham chiếu nào tới Drive/upload.

Bản Python gốc (`ProcessHandler.upload_file_to_drive`, `xulydonhang.py:3072-3168`):
- POST base64 nội dung file + metadata (`filename`/`ext`/`mime`) lên 1 Apps Script endpoint.
- Tên file trên Drive: `[vendor][entry_date][makhachhang][cancle_date][output_name].ext` — mỗi trường bọc ngoặc vuông, rỗng/không parse được → `"NA"`, ngày chuẩn hóa `dd-mm-yyyy`.
- Đọc + encode base64 NGAY (đồng bộ, vì file gốc có thể bị xóa ngay sau khi hàm return); POST thật + retry (3 lần, backoff `2*attempt` giây) chạy trong 1 thread nền — không chặn xử lý tiếp theo. Trả về NGAY 1 URL "xem" dựng sẵn (endpoint Apps Script thứ 2, `.../exec?po=<filename đã encode>`), không đợi upload thật xong/thành công.
- Kết quả (`url`) trong bản Python CHỈ được dùng để log ra khung log (`ghi_message(f"url: {url}")`) — không ghi vào cột nào của `dondathang.xlsx`, không gửi Zalo (đã kiểm chứng qua tất cả các hàm `write_to_dondathang_*`).
- Với PDF nhiều trang/nhiều đơn gộp, Python cắt riêng trang hiện tại thành 1 file PDF tạm để upload — **bản Go KHÔNG làm việc này** (xem Quyết định #3 bên dưới).

## Quyết định đã chốt (qua brainstorming với user)

1. **Endpoint**: dùng lại nguyên 2 URL Apps Script hiện có (upload + view-lookup) — không tạo endpoint mới.
2. **Phạm vi vendor**: áp dụng cho tất cả 9 vendor hiện có trong bản Go (BigC, Coop, Emart, FujiMart, JMart, KingFood, Lotte, Satra, WinMart).
3. **File nhiều trang**: upload NGUYÊN file gốc (`filePath`), không cắt trang — đơn giản hơn nhiều, không cần thêm thư viện ghi/tách PDF cho Go (thư viện đọc PDF hiện tại, `ledongthuc/pdf`, không ghi được). Nhiều đơn cùng 1 file nguồn sẽ trỏ chung 1 link Drive — chấp nhận được, đánh đổi lấy sự đơn giản.
4. **Hiển thị link**: thêm 1 cột "File Drive" trong bảng kết quả (`ResultTable.tsx`) — bấm mở bằng trình duyệt mặc định (`BrowserOpenURL`), KHÔNG chỉ log text như bản cũ.
5. **Cơ chế**: fire-and-forget giống bản cũ — trả URL dựng sẵn ngay, upload thật chạy nền, retry 3 lần không chặn xử lý đơn tiếp theo.
6. **Cải tiến nhỏ so với bản Python** (được user duyệt trong bước trình bày thiết kế): bản Python chỉ `print()` kết quả upload nền ra console ẩn (không ai thấy). Bản Go sẽ log 1 dòng vào khung log thật khi upload nền hoàn tất (thành công hoặc hết retry) — tách biệt với dòng log link hiện ngay lúc xử lý.

## Kiến trúc

### 1. Package mới `GO/internal/driveupload/upload.go`

```go
// Package driveupload uploads a processed order's source file to Google
// Drive via the same Google Apps Script endpoints the old Python app
// used (xulydonhang.py's ProcessHandler.upload_file_to_drive) — fire-
// and-forget: the file is read and base64-encoded synchronously (the
// caller's source file may be deleted right after this call returns),
// but the actual network POST + retry runs in a background goroutine
// so a slow/failing upload never blocks order processing. Returns a
// constructed "view" URL immediately, before the upload is even
// attempted.
package driveupload

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// scriptURL/viewURLBase are the SAME Google Apps Script endpoints the
// old Python app used (xulydonhang.py:3093-3094) — reused deliberately
// so uploads keep landing in the same shared Drive location, per the
// project owner's explicit choice (not a placeholder to fill in).
//
// Deliberately `var`, not `const`: Upload calls scriptURL directly
// (not as a parameter), so a test needs to temporarily repoint it at
// an httptest.Server to avoid ever making a real network call during
// `go test`. A const would make that impossible without changing
// Upload's signature just for tests. Tests that override this MUST
// restore the original value (e.g. via t.Cleanup) and must NOT run in
// parallel with each other while doing so, since it's shared mutable
// package state.
var scriptURL = "https://script.google.com/macros/s/AKfycbx2ZJhdxEAZq_79ibt3g5UeqccNqLT2ScOtRldnwlgRQB2JdquUPSnSebMQoYESNSv2/exec"
const viewURLBase = "https://script.google.com/macros/s/AKfycby9fc3IaX1-EwIb26g34WLs8TbQXNkdxeqpVSYSWddwwxRAFaz9kjsS9yFhypezIaF2/exec?po="

// Metadata is embedded into the uploaded file's name on Drive, in this
// exact order — mirrors upload_file_to_drive's name_parts exactly.
type Metadata struct {
	Vendor       string
	EntryDate    string // any of dateInputLayouts' formats, or "" / a not-found sentinel
	CustomerCode string
	CancelDate   string // same format rules as EntryDate
	OutputName   string // typically the PO/order number
}

var sanitizePattern = regexp.MustCompile(`[\\/:*?"<>|\[\]]+`)

// sanitize mirrors Python's _sanitize: strip characters that would
// break a filename or the bracket-delimited convention itself, empty
// (or now-empty-after-stripping) becomes "NA" so every bracket always
// has content and field position never shifts.
func sanitize(value string) string {
	if value == "" {
		return "NA"
	}
	cleaned := strings.TrimSpace(sanitizePattern.ReplaceAllString(value, ""))
	if cleaned == "" {
		return "NA"
	}
	return cleaned
}

// dateInputLayouts mirrors Python's _format_date try-list exactly
// (xulydonhang.py:3118) — Go's date fields already arrive as strings
// (never a native date/time value the way Python's could), so there is
// no separate "isinstance(datetime)" branch to port.
var dateInputLayouts = []string{
	"02/01/2006",
	"02-01-2006",
	"02/01/06",
	"02-01-06",
	"2006-01-02",
	"2006/01/02",
	// JMart's own entry/cancel date regex allows 1-2 digit day/month
	// with no zero-padding (internal/processing/jmart/extract.go:8,
	// `\d{1,2}/\d{1,2}/\d{4}`) — verified during spec writeup, not
	// assumed. "02/01/2006" alone would fail to parse a real
	// single-digit day or month (e.g. "5/3/2026"); Go's non-padded
	// day/month layout tokens ("2"/"1") cover both 1- and 2-digit
	// input, so this one extra layout is enough — no vendor-specific
	// function needed.
	"2/1/2006",
}

// formatDate mirrors Python's _format_date: try each layout in turn,
// "NA" if none match (covers empty strings and any vendor's
// not-found sentinel, e.g. Coop's "Không tìm thấy"/"Không hợp lệ" -
// neither matches any real date layout, so both correctly fall
// through to "NA", matching Python's behavior for the same inputs).
func formatDate(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "NA"
	}
	for _, layout := range dateInputLayouts {
		if t, err := time.Parse(layout, trimmed); err == nil {
			return t.Format("02-01-2006")
		}
	}
	return "NA"
}

// BuildFilename mirrors upload_file_to_drive's name_parts/filename
// construction exactly (xulydonhang.py:3125-3132).
func BuildFilename(m Metadata) string {
	parts := []string{
		sanitize(m.Vendor),
		formatDate(m.EntryDate),
		sanitize(m.CustomerCode),
		formatDate(m.CancelDate),
		sanitize(m.OutputName),
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("[")
		b.WriteString(p)
		b.WriteString("]")
	}
	return b.String()
}

type uploadPayload struct {
	Filename string `json:"filename"`
	Ext      string `json:"ext"`
	Mime     string `json:"mime"`
	FileB64  string `json:"file_b64"`
}

// Upload reads path synchronously, builds the Drive filename and a
// constructed view URL, starts the real network upload (with retry) in
// a background goroutine, and returns the view URL immediately -
// mirroring upload_file_to_drive's fire-and-forget contract exactly.
// onResult (may be nil) is called exactly once when the background
// goroutine finishes, ok=true on the first successful POST, ok=false
// with the last error after all retries are exhausted - this is the
// ONE deliberate behavior difference from the Python original (which
// only printed to a hidden console): the caller can route this to a
// real, visible log line.
func Upload(client *http.Client, path string, m Metadata, onResult func(ok bool, err error)) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("driveupload: read %s: %w", path, err)
	}

	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	filename := BuildFilename(m)
	viewURL := viewURLBase + url.QueryEscape(filename)

	payload := uploadPayload{
		Filename: filename,
		Ext:      ext,
		Mime:     mimeType,
		FileB64:  base64.StdEncoding.EncodeToString(data),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("driveupload: encode payload: %w", err)
	}

	go func() {
		const maxRetries = 3
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := postOnce(client, body); err != nil {
				lastErr = err
				if attempt < maxRetries {
					time.Sleep(time.Duration(2*attempt) * time.Second)
				}
				continue
			}
			if onResult != nil {
				onResult(true, nil)
			}
			return
		}
		if onResult != nil {
			onResult(false, lastErr)
		}
	}()

	return viewURL, nil
}

func postOnce(client *http.Client, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, scriptURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// NewHTTPClient matches this project's other HTTP clients
// (pricing.NewHTTPSource, productdata.NewHTTPClient, applock.Check) -
// a bare-timeout client, no cookies/retries at the transport level
// (retry is handled explicitly in Upload's goroutine, since a 30s
// per-attempt timeout needs to coexist with 3 attempts + backoff, not
// a single client-wide timeout covering all of them).
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
```

**Ghi chú quan trọng cho implementer**: `formatDate`'s layout list được suy ra từ Coop (`coop.ConvertDateFormat` trả về `dd/mm/yyyy`, đã verify trực tiếp trong code — `internal/processing/coop/invoice.go:155-164`). Các vendor khác (BigC, Emart, FujiMart, JMart, KingFood, Lotte, Satra, WinMart) có thể có định dạng ngày khác nhau ở điểm gọi — **phải đọc code từng vendor để xác nhận định dạng thật trước khi wiring**, không giả định. Nếu 1 vendor có format không nằm trong `dateInputLayouts`, thêm layout đó vào danh sách (không tạo hàm riêng cho từng vendor) — cùng 1 hàm `formatDate` phải xử lý được mọi vendor.

### 2. `RealProcessor` (Go) — thêm 2 field

`GO/internal/processing/coop_processor.go` (struct định nghĩa chính, dùng chung cho mọi vendor vì tất cả nằm trong cùng package `processing`):

```go
type RealProcessor struct {
	Store       *productdata.Store
	Pricing     PricingSource
	ExcelPath   string
	DriveClient *http.Client   // driveupload.NewHTTPClient() trong production
	LogFunc     func(string)   // optional (nil-safe) - route "process:log" nền
}
```

Cần thêm import `"net/http"` vào `coop_processor.go`.

### 3. Nối vào `app.go`

```go
// Trong NewApp(), sau khi RealProcessor{...} được tạo, TRƯỚC return:
```

Cụ thể, sửa đoạn tạo `processor` trong `NewApp()` (hiện đang là `&processing.RealProcessor{Store: store, Pricing: ..., ExcelPath: excelPath}`):

```go
	processor := &processing.RealProcessor{
		Store:       store,
		Pricing:     pricing.NewHTTPSource(settings.Gid),
		ExcelPath:   excelPath,
		DriveClient: driveupload.NewHTTPClient(),
	}

	app := &App{
		cfg:              config.NewStore(configFileName),
		appSettingsStore: appSettingsStore,
		processor:        processor,
		orderDir:         orderFolderName,
		excelPath:        excelPath,
	}

	processor.LogFunc = func(msg string) {
		if app.emitter != nil {
			app.emitter.Emit("process:log", msg)
		}
	}

	return app, nil
```

**Lý do `LogFunc` kiểm tra `app.emitter != nil`**: `NewApp()` chạy TRƯỚC `startup(ctx)` — `app.emitter` chưa được set lúc `NewApp()` return. `LogFunc` là 1 closure, được GỌI muộn hơn (khi goroutine nền của `driveupload.Upload` hoàn tất, luôn sau khi `startup()` đã chạy vì xử lý đơn chỉ có thể bắt đầu sau khi UI đã lên) — nhưng thêm nil-check cho an toàn tuyệt đối, tránh panic trong trường hợp cực hiếm upload nền hoàn tất trước `startup()`.

Thêm import `"order-processor/internal/driveupload"` vào `app.go`.

### 4. `OrderRow` (Go) — thêm field

`GO/internal/processing/types.go`:

```go
type OrderRow struct {
	FileName    string `json:"fileName"`
	Page        string `json:"page"`
	System      string `json:"system"`
	MaKhachHang string `json:"maKhachHang"`
	PO          string `json:"po"`
	DonGia      string `json:"donGia"`
	Status      string `json:"status"`
	StatusKind  string `json:"statusKind"`

	// DriveURL is the constructed "view" link from driveupload.Upload -
	// populated the moment a row is built (fire-and-forget: the real
	// upload may still be in progress or even fail in the background,
	// this URL is a best-effort placeholder from the start). Empty
	// string if the row's file was never uploaded (e.g. a Failed row
	// with no successfully-written Excel data to link to).
	DriveURL string `json:"driveUrl"`

	PriceMismatchCount int `json:"priceMismatchCount"`
	SkuLog []string `json:"-"`
	PriceMismatchDetails []PriceMismatchDetail `json:"priceMismatchDetails"`
}
```

(Giữ nguyên toàn bộ field/comment hiện có, chỉ thêm `DriveURL` — vị trí chèn ngay sau `StatusKind`, trước `PriceMismatchCount`, theo đúng khối code hiển thị ở trên.)

### 5. Điểm gọi trong vendor processor — VÍ DỤ ĐẦY ĐỦ: Coop

`GO/internal/processing/coop_processor.go`'s `processSegment` (dòng 209-433 hiện tại) là nơi đơn Coop được viết vào Excel và trả về `OrderRow`. Điểm chèn: **ngay sau `excelwriter.WriteOrderRows` thành công, trước khi build `OrderRow` return** (dòng 413-432 hiện tại):

```go
	headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
	startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, rows, headerDescription)
	if err != nil {
		return OrderRow{}, err
	}
	for i := range mismatchDetails {
		mismatchDetails[i].ExcelRow += startRow
	}

	driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
		Vendor:       "COOP",
		EntryDate:    entryDate,
		CustomerCode: customerCode,
		CancelDate:   cancelDate,
		OutputName:   info.PONumber,
	}, func(ok bool, err error) {
		if p.LogFunc == nil {
			return
		}
		if ok {
			p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
		} else {
			p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
		}
	})
	if uploadErr != nil && p.LogFunc != nil {
		p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
	}

	statusKind := StatusKindDone
	statusText := StatusDone
	if saigia > 0 {
		statusKind = StatusKindWarning
		statusText = fmt.Sprintf("%s - Có %d mã sai giá", StatusWarning, saigia)
	}

	return OrderRow{
		FileName: filepath.Base(filePath), Page: pageLabel, System: system, MaKhachHang: customerCode,
		PO: info.PONumber, DonGia: fmt.Sprintf("%.0f", totalValue), Status: statusText, StatusKind: statusKind,
		DriveURL: driveURL,
		SkuLog: skuLog, PriceMismatchCount: saigia, PriceMismatchDetails: mismatchDetails,
	}, nil
```

Thêm import `"order-processor/internal/driveupload"` vào `coop_processor.go`.

**QUAN TRỌNG - đây là ĐIỂM QUYẾT ĐỊNH THIẾT KẾ, không phải chi tiết vặt**: `uploadErr` (lỗi ĐỌC FILE đồng bộ - hiếm, path không tồn tại hoặc không đọc được) KHÔNG được làm fail toàn bộ order (không `return OrderRow{}, uploadErr`) - đơn hàng đã ghi thành công vào Excel rồi, việc upload Drive thất bại chỉ là mất tính năng phụ, không phải lỗi nghiệp vụ. Chỉ log cảnh báo, `DriveURL` rỗng, `OrderRow` vẫn trả về bình thường với `Status`/`StatusKind` như cũ.

### 6. Áp dụng cho 8 vendor còn lại

Cùng pattern chính xác như Coop ở trên: gọi `driveupload.Upload` NGAY SAU `excelwriter.WriteOrderRows` thành công, TRƯỚC khi build `OrderRow` trả về, log kết quả nền qua `p.LogFunc` giống hệt khối code ở mục 5, chỉ khác `Metadata` map vào đúng biến cục bộ của từng vendor. Đã đọc trực tiếp code thật của cả 8 file để xác định chính xác — bảng dưới đây là kết quả xác nhận (không suy đoán), dùng làm căn cứ literal cho plan:

| Vendor | File | Hàm | EntryDate (biến) | CancelDate (biến) | CustomerCode (biến) | OutputName (biến) | Metadata.Vendor | Định dạng ngày thật đã verify |
|---|---|---|---|---|---|---|---|---|
| Lotte | `lotte_processor.go` | `processLotteSegment` | `info.EntryDate` | `cancelDate` | `customerCode` | `info.PONumber` | `"LOTTE"` | dd/mm/yyyy (`lotte/extract.go:61`). **Lưu ý**: `cancelDate` (`lotte.ExtractCancelDate`) có thể là chuỗi NHIỀU DÒNG nối bằng "\n" (khi nhiều dòng khớp pattern ngày) — `formatDate` sẽ không parse được trường hợp này và trả "NA", đây là hành vi CHẤP NHẬN ĐƯỢC (khớp thiết kế fallback-to-NA), không phải lỗi cần sửa. |
| Satra | `satra_processor.go` | `processSatraSegment` | `entryDate` | `cancelDate` | `customerCode` | `poNumber` | `"SATRA"` | dd/mm/yyyy, đã zero-pad (`satra/extract.go:107`, `formatMDYtoDMYChecked`) |
| Emart | `emart_processor.go` | `processEmartSegment` | `entryDate` | `cancelDate` | `emartCustomerCode` (hằng số package, KHÔNG phải biến cục bộ) | `poNumber` | `"EMART"` | dấu "." được thay bằng "/" (`emart/extract.go:114`), thứ tự day/month CHƯA xác nhận 100% từ nguồn PDF thật — đã có sẵn 2 layout (`"02/01/2006"` và `"2006/01/02"`) trong `dateInputLayouts` để phủ cả 2 khả năng, không cần thêm |
| Kingfood | `kingfood_processor.go` | `processKingfoodSegment` | `entryDate` | `cancelDate` | `kingfoodCustomerCode` (hằng số) | `poNumber` | `"KINGFOOD"` | dd/mm/yyyy, đã zero-pad (`kingfood/extract.go:84`) |
| Winmart | `winmart_processor.go` | `processWinmartSegment` | `entryDate` | `cancelDate` | `customerCode` | `poNumber` | `"WINMART"` | dấu "." thay bằng "/" (`winmart/extract.go:34,44`), cùng tình huống như Emart — 2 layout hiện có đã đủ phủ |
| FujiMart | `fujimart_processor.go` | `processFujimartSegment` | `entryDate` | `cancelDate` | `fujimartCustomerCode` (hằng số) | `poNumber` | `"FUJIMART"` | dd/mm/yyyy, đã zero-pad (`fujimart/extract.go:108`, dùng trực tiếp layout `"02/01/2006"`) |
| JMart | `jmart_processor.go` | `processJMartSegment` | `entryDate` | `cancelDate` | `jmartCustomerCode` (hằng số) | `poNumber` | `"JMART"` | **d/m/yyyy KHÔNG zero-pad** (`jmart/extract.go:8`, regex `\d{1,2}/\d{1,2}/\d{4}`) — đây là lý do `dateInputLayouts` (mục 1) đã có thêm layout `"2/1/2006"`, bắt buộc phải có layout này thì JMart mới parse được ngày 1 chữ số |
| BigC | `bigc_processor.go` | `processBigcDocument` | `entryDate` | `cancelDate` | `customerCode` | `poNumber` | `"BIGC"` | dd/mm/yyyy, đã zero-pad (`bigc/extract.go:98`, `convertEntryDate`) — **KIẾN TRÚC KHÁC HẲN, xem chi tiết ngay dưới** |

**BigC — điểm khác biệt kiến trúc (đã xác định rõ, không còn là quyết định mở)**: `processBigcDocument` xử lý CẢ FILE 1 lần (nhiều store page), gom TẤT CẢ store page thành công vào 1 `allRows` rồi gọi `excelwriter.WriteOrderRows` DUY NHẤT 1 LẦN cho cả file (dòng ~155-166 hiện tại) — SAU khi từng `OrderRow` riêng của mỗi store page đã được append vào slice `orderRows` (dòng ~148-152, bên trong vòng lặp, TRƯỚC lần gọi `WriteOrderRows` đó). Vì file nguồn là DUY NHẤT (`filePath`, giống nhau cho mọi store page) và quyết định đã chốt là "upload nguyên file gốc" — chỉ gọi `driveupload.Upload` **ĐÚNG 1 LẦN**, ngay sau `WriteOrderRows` thành công, rồi gán CÙNG 1 `driveURL` ngược lại cho MỌI phần tử trong `orderRows` (cùng cách mà code hiện tại đã backfill `ExcelRow` offset ngược lại cho `PriceMismatchDetails` ở đúng vị trí này). Code chính xác cần chèn vào `processBigcDocument`, thay thế khối `if len(allRows) > 0 { ... }` hiện tại:

```go
	if len(allRows) > 0 {
		headerDescription := fmt.Sprintf("%s (Tổng trọng lượng: %s)", description, coop.FormatWeightKg(totalWeight))
		startRow, err := excelwriter.WriteOrderRows(p.ExcelPath, allRows, headerDescription)
		if err != nil {
			return nil, err
		}
		for i := range orderRows {
			for j := range orderRows[i].PriceMismatchDetails {
				orderRows[i].PriceMismatchDetails[j].ExcelRow += startRow
			}
		}

		driveURL, uploadErr := driveupload.Upload(p.DriveClient, filePath, driveupload.Metadata{
			Vendor:       "BIGC",
			EntryDate:    entryDate,
			CustomerCode: customerCode,
			CancelDate:   cancelDate,
			OutputName:   poNumber,
		}, func(ok bool, err error) {
			if p.LogFunc == nil {
				return
			}
			if ok {
				p.LogFunc(fmt.Sprintf("✅ Đã upload file lên Drive: %s", filepath.Base(filePath)))
			} else {
				p.LogFunc(fmt.Sprintf("❌ Upload Drive thất bại (%s): %v", filepath.Base(filePath), err))
			}
		})
		if uploadErr != nil && p.LogFunc != nil {
			p.LogFunc(fmt.Sprintf("⚠️ Không đọc được file để upload Drive: %v", uploadErr))
		}
		for i := range orderRows {
			orderRows[i].DriveURL = driveURL
		}
	}
```

Thêm import `"order-processor/internal/driveupload"` vào `bigc_processor.go`.

### 7. Frontend — `types.ts`

Thêm vào `OrderRow` interface (giữ nguyên các field khác):

```typescript
export interface OrderRow {
  fileName: string
  page: string
  system: string
  maKhachHang: string
  po: string
  donGia: string
  status: string
  statusKind: string
  driveUrl: string
  priceMismatchCount: number
  priceMismatchDetails: PriceMismatchDetail[]
}
```

### 8. Frontend — `ResultTable.tsx`

Thêm cột mới vào mảng `columns` (dòng 15-24 hiện tại), sau `status`:

```typescript
const columns: { key: Exclude<keyof OrderRow, 'priceMismatchDetails'>; label: string }[] = [
  { key: 'fileName', label: 'Tên file' },
  { key: 'page', label: 'Trang' },
  { key: 'system', label: 'Hệ thống' },
  { key: 'maKhachHang', label: 'Mã khách hàng' },
  { key: 'po', label: 'PO' },
  { key: 'donGia', label: 'Đơn giá' },
  { key: 'priceMismatchCount', label: 'Đối soát giá' },
  { key: 'status', label: 'Trạng thái' },
  { key: 'driveUrl', label: 'File Drive' },
]
```

Import thêm `BrowserOpenURL` và 1 icon (`FaUpRightFromSquare` từ `react-icons/fa6`, đã có sẵn trong dependency):

```typescript
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { FaUpRightFromSquare } from 'react-icons/fa6'
```

Thêm 1 nhánh xử lý trong chuỗi `c.key === '...' ? ... : ...` (trong khối render `<td>`, sau nhánh `c.key === 'donGia'`, trước nhánh `else` cuối cùng `row[c.key]`):

```tsx
) : c.key === 'driveUrl' ? (
  row.driveUrl ? (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation()
        BrowserOpenURL(row.driveUrl)
      }}
      className="inline-flex items-center gap-1.5 rounded-full border border-border px-2.5 py-0.5 font-sans font-semibold text-accent transition-colors hover:border-accent"
    >
      <FaUpRightFromSquare size={9} /> Mở file
    </button>
  ) : (
    <span className="text-muted">—</span>
  )
) : (
```

**Lưu ý**: cột này KHÔNG dùng cơ chế click-để-copy chung của các cột khác (dòng 145 `onClick={() => handleCopy(...)}` trên `<td>`) — nút "Mở file" bên trong phải `e.stopPropagation()` để không kích hoạt copy của `<td>` cha, giống hệt cách nút "Dùng giá PO"/"Dùng giá hệ thống" đã làm ở bảng chi tiết đối soát giá bên dưới (dòng 166-167 hiện tại).

## Testing

### Go unit test — `GO/internal/driveupload/upload_test.go`

- `TestSanitize_ReplacesEmptyWithNA`, `TestSanitize_StripsForbiddenCharacters` (test trực tiếp các ký tự trong `sanitizePattern`).
- `TestFormatDate_ParsesAllListedLayouts` (1 case cho mỗi layout trong `dateInputLayouts`).
- `TestFormatDate_UnrecognizedInputReturnsNA` (bao gồm case sentinel kiểu "Không tìm thấy"/"Không hợp lệ").
- `TestBuildFilename_MatchesPythonBracketConvention` — so với 1 case cụ thể lấy từ code Python thật (vd vendor="COOP", entry="15/03/2026", customer="MNCOOP001", cancel="", output="103229379-00" → verify chuỗi kết quả đúng định dạng `[COOP][15-03-2026][MNCOOP001][NA][103229379-00]`).
- `TestUpload_ReturnsURLImmediatelyWithoutWaitingForNetwork` — dùng `httptest.Server` trả lời CHẬM (vd `time.Sleep` vài trăm ms trước khi respond), verify `Upload` return trước khi response đó tới (dùng channel/timing đơn giản, không cần chính xác tuyệt đối).
- `TestUpload_RetriesOnFailureThenCallsOnResult` — `httptest.Server` trả lỗi 2 lần đầu, thành công lần 3, verify `onResult(true, nil)` được gọi, verify server nhận đúng 3 request.
- `TestUpload_AllRetriesFailedCallsOnResultFalse` — server luôn lỗi, verify `onResult(false, err)` sau đúng 3 lần thử.
- `TestUpload_FileNotFoundReturnsErrorSynchronously` — path không tồn tại, verify lỗi trả về NGAY (không phải qua `onResult`), verify KHÔNG có goroutine nào được spawn (không có network call nào xảy ra).

**KHÔNG test thật lên `scriptURL` production trong bất kỳ test nào.** Mọi test cần gọi mạng phải tạm gán `scriptURL = testServer.URL` (dùng `t.Cleanup` khôi phục lại giá trị gốc), trỏ vào 1 `httptest.Server` riêng của test đó — không được để sót 1 test nào gọi ra URL thật. `viewURLBase` không cần override (chỉ dùng để DỰNG chuỗi URL trả về ngay, không thực sự gọi mạng tới đó trong `Upload`).

### UI test — `wails dev` thật (bắt buộc, không chỉ tsc)

- Xử lý 1 đơn hàng thật (1 vendor bất kỳ), xác nhận cột "File Drive" xuất hiện, nút "Mở file" bấm được, mở đúng link trong trình duyệt mặc định.
- Xác nhận dòng log "✅ Đã upload..." hoặc "❌ Upload Drive thất bại..." xuất hiện trong khung log sau vài giây (đợi goroutine nền hoàn tất) — verify cơ chế `LogFunc` hoạt động thật, không chỉ đọc code.
- Xác nhận xử lý đơn TIẾP THEO không bị chặn/chậm trong lúc đơn trước đang upload nền (verify fire-and-forget thật sự không block).

## Phạm vi KHÔNG làm (out of scope)

- Không cắt trang PDF cho file nhiều đơn gộp (Quyết định #3) — mọi đơn từ cùng 1 file nguồn dùng chung 1 link Drive.
- Không ghi `DriveURL` vào `dondathang.xlsx` (bản Python cũ cũng không làm việc này).
- Không gửi link qua Zalo.
- Không thêm cấu hình bật/tắt tính năng này qua UI settings — luôn bật, giống bản Python cũ luôn upload không hỏi.
