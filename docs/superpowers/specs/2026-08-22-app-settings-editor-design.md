# Popup chỉnh sửa cấu hình app (thay settings.ini) — Thiết kế

## Mục tiêu

`settings.ini` hiện là 1 file text dạng tag tự chế (`<gid>...</gid>`,
`<zalo>...</zalo>`, `<reminder>...</reminder>`, không phải XML/INI
chuẩn) nằm ở thư mục gốc repo, đuôi `.ini` — bất kỳ ai cũng có thể vô
tình double-click mở bằng Notepad và sửa sai cú pháp, làm app không
khởi động được (`pricing.LoadGidMap` trả lỗi rõ ràng khi 1 dòng có hơn
1 dấu `=`, nhưng lỗi đó chỉ xuất hiện SAU khi đã hỏng).

Mục tiêu: (1) người dùng không còn lý do/cách thuận tiện để mở file này
bằng tay nữa — chuyển sang định dạng riêng, đuôi file riêng, không phải
`.ini`/`.txt`; (2) xây 1 popup chỉnh sửa ngay trong app (modal đầu tiên
của app), có 3 tab tương ứng 3 khối cấu hình, sửa trực tiếp qua giao
diện thay vì gõ tay vào file text.

## Bối cảnh kỹ thuật hiện tại

- `settings.ini` thật (đọc `git show HEAD:settings.ini` tại thời điểm
  viết spec này) có đúng 3 khối, dạng `<tag>\nKEY = VALUE\n...\n</tag>`:
  - `<gid>`: 18 dòng, map tên hệ thống (COOP, BIGC, LOTTE, ... MAKH,
    SANPHAM) → gid tab Google Sheet. **Đang được dùng thật** bởi
    `pricing.HTTPSource.FetchIndex` (giá/khuyến mãi từng vendor) và
    `productdata.LoadFromSheets` (dữ liệu khách hàng/sản phẩm, vừa
    chuyển sang Google Sheets phiên trước).
  - `<zalo>`: ~20 dòng, map tên nhóm → tên hiển thị nhóm Zalo.
  - `<reminder>`: 7 dòng, map tên nhóm → cờ (hiện toàn giá trị `"1"`).
  - `<zalo>`/`<reminder>` **CHƯA được Go đọc ở bất kỳ đâu** — xác nhận
    bằng cách grep toàn bộ `.go` không có file nào tham chiếu 2 tag này
    ngoài comment. Khớp với việc nút "Gửi Zalo" trên UI hiện đang hiện
    "SẮP RA MẮT" (tính năng Zalo qua Go chưa xây). User đã chọn vẫn đưa
    cả 2 khối này vào popup ngay bây giờ (không đợi tính năng Zalo xây
    xong), để không phải làm lại UI 2 lần.
- `pricing/gid.go` — hàm `LoadGidMap(path string) (map[string]string, error)`
  đọc file bằng regex `<gid>(.*?)</gid>` (flag `(?s)` cho phép xuống
  dòng), rồi tách từng dòng theo dấu `=` đầu tiên. Đây là hàm DUY NHẤT
  hiện đang đọc `settings.ini`.
- `pricing/http_source.go` — `HTTPSource` struct giữ `SettingsPath string`,
  gọi `LoadGidMap(s.SettingsPath)` lại **mỗi lần** `FetchIndex` được gọi
  (không cache, đọc lại file từ đĩa mỗi lần).
- `productdata/sheets_source.go` — `LoadFromSheets(settingsPath string, client *http.Client)`
  cũng gọi thẳng `pricing.LoadGidMap(settingsPath)`.
- `app.go` — `NewApp()` gọi
  `pricing.NewHTTPSource(resolveRepoFile("settings.ini"))` và
  `productdata.LoadFromSheets(resolveRepoFile("settings.ini"), ...)` —
  cả 2 đều tự cầm đường dẫn file, tự đọc lại độc lập.
- `internal/config.Store` là 1 package **khác, không liên quan** — chỉ
  lưu 1 số nguyên (STT) vào `config.txt`, dùng `bufio`/key=value đơn
  giản. Không tái dùng cho tính năng này (trách nhiệm khác hẳn).
- Frontend (`GO/frontend/src/`) **chưa có modal/popup nào** — đây sẽ là
  modal đầu tiên. `App.tsx` hiện có header với 2 tab
  ("Xử lý Đơn hàng"/"Thông tin") và 1 khu vực bên phải hiển thị trạng
  thái xử lý — chưa có chỗ nào đặt icon cài đặt.

## Kiến trúc

### 1. Package mới `internal/appsettings`

```go
// GO/internal/appsettings/store.go
package appsettings

// Settings là toàn bộ nội dung cấu hình app: 3 map, tương ứng 3 khối
// <gid>/<zalo>/<reminder> của settings.ini cũ. Cả 3 đều map tên khóa
// (string) -> giá trị (string) — Reminder hiện chỉ có giá trị "1"
// nhưng vẫn lưu dạng string để không giả định trước ý nghĩa giá trị
// tương lai (ví dụ sau này có thể là số ngày nhắc thay vì cờ bật/tắt).
type Settings struct {
	Gid      map[string]string `json:"gid"`
	Zalo     map[string]string `json:"zalo"`
	Reminder map[string]string `json:"reminder"`
}
```

### 2. Đọc/ghi file mới

```go
// GO/internal/appsettings/store.go (tiếp)

import (
	"encoding/json"
	"fmt"
	"os"
)

// Store đọc/ghi Settings từ 1 file JSON đuôi .bhconfig (không phải
// .ini/.txt — đổi đuôi có chủ đích để không bị double-click mở nhầm
// bằng Notepad). Nội dung vẫn là text đọc được nếu người dùng CỐ TÌNH
// mở bằng tay (không mã hóa) — quyết định rõ ràng của user, đánh đổi
// lấy khả năng tự debug bằng tay nếu app lỗi.
type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load đọc Settings từ file .bhconfig. Nếu file .bhconfig CHƯA TỒN TẠI
// nhưng oldIniPath (settings.ini cũ) CÓ, tự động đọc file cũ (định
// dạng tag <gid>/<zalo>/<reminder>), ghi ra file .bhconfig mới, rồi
// trả về dữ liệu đã migrate — settings.ini cũ được GIỮ NGUYÊN trên đĩa,
// không xóa, không sửa (quyết định rõ ràng của user). Nếu CẢ 2 file đều
// không tồn tại, trả về Settings với 3 map rỗng (không lỗi — app mới
// cài lần đầu, chưa có cấu hình gì).
func (s *Store) Load(oldIniPath string) (Settings, error) {
	data, err := os.ReadFile(s.path)
	if err == nil {
		var settings Settings
		if err := json.Unmarshal(data, &settings); err != nil {
			return Settings{}, fmt.Errorf("appsettings: parse %s: %w", s.path, err)
		}
		ensureMaps(&settings)
		return settings, nil
	}
	if !os.IsNotExist(err) {
		return Settings{}, fmt.Errorf("appsettings: read %s: %w", s.path, err)
	}

	// .bhconfig chưa tồn tại — thử migrate từ settings.ini cũ.
	settings, migrated, err := migrateFromOldIni(oldIniPath)
	if err != nil {
		return Settings{}, err
	}
	if !migrated {
		// Không có cả 2 file — app mới, chưa từng cấu hình gì.
		return Settings{Gid: map[string]string{}, Zalo: map[string]string{}, Reminder: map[string]string{}}, nil
	}
	if err := s.Save(settings); err != nil {
		return Settings{}, fmt.Errorf("appsettings: write migrated %s: %w", s.path, err)
	}
	return settings, nil
}

// Save ghi Settings ra file .bhconfig, định dạng JSON có thụt lề (đọc
// được nếu người dùng cố mở bằng tay).
func (s *Store) Save(settings Settings) error {
	ensureMaps(&settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("appsettings: encode settings: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("appsettings: write %s: %w", s.path, err)
	}
	return nil
}

// ensureMaps thay mọi map nil bằng map rỗng — JSON marshal của map nil
// ra "null" thay vì "{}", và frontend (TypeScript) cần luôn nhận được
// object thật để render bảng, không phải null.
func ensureMaps(s *Settings) {
	if s.Gid == nil {
		s.Gid = map[string]string{}
	}
	if s.Zalo == nil {
		s.Zalo = map[string]string{}
	}
	if s.Reminder == nil {
		s.Reminder = map[string]string{}
	}
}
```

### 3. Đọc file `settings.ini` cũ (dùng lại lúc migrate)

Tổng quát hoá logic hiện có trong `pricing/gid.go` (vốn chỉ đọc riêng
`<gid>`) thành hàm đọc CẢ 3 tag, đặt trong `appsettings` (không sửa
`pricing/gid.go` — xem mục 6 về việc xóa file này):

```go
// GO/internal/appsettings/migrate.go
package appsettings

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// migrateFromOldIni đọc settings.ini cũ (định dạng
// <tag>\nKEY = VALUE\n...\n</tag>, xem pricing/gid.go bản gốc — hàm
// này thay thế nó) và trả về Settings + true nếu file tồn tại và đọc
// được, hoặc Settings{} + false nếu file không tồn tại (KHÔNG phải
// lỗi — app có thể chưa từng có settings.ini, ví dụ cài mới hoàn
// toàn).
func migrateFromOldIni(path string) (Settings, bool, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: read %s: %w", path, err)
	}

	gid, err := parseTagBlock(string(content), "gid")
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: migrate <gid>: %w", err)
	}
	zalo, err := parseTagBlock(string(content), "zalo")
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: migrate <zalo>: %w", err)
	}
	reminder, err := parseTagBlock(string(content), "reminder")
	if err != nil {
		return Settings{}, false, fmt.Errorf("appsettings: migrate <reminder>: %w", err)
	}
	return Settings{Gid: gid, Zalo: zalo, Reminder: reminder}, true, nil
}

// parseTagBlock đọc khối <tagName>...</tagName>, mỗi dòng bên trong
// dạng "KEY = VALUE" — logic giống hệt pricing.LoadGidMap bản gốc,
// tổng quát hoá tên tag thành tham số. Dòng không có dấu "=" bị bỏ qua
// (comment, ví dụ 2 dòng "# MAKH/SANPHAM..." đã thêm vào <gid> phiên
// trước); dòng có NHIỀU HƠN 1 dấu "=" là lỗi rõ ràng, không âm thầm
// lấy phần đầu/cuối. Không tìm thấy khối tag → trả map rỗng, không
// lỗi (ví dụ file cũ không có <reminder> vì được thêm sau).
func parseTagBlock(content, tagName string) (map[string]string, error) {
	pattern := regexp.MustCompile(`(?s)<` + tagName + `>(.*?)</` + tagName + `>`)
	match := pattern.FindStringSubmatch(content)
	if match == nil {
		return map[string]string{}, nil
	}

	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(match[1]), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.Split(line, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed <%s> line (expected exactly one '='): %q", tagName, line)
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result, nil
}
```

### 4. Đổi `pricing.HTTPSource` và `productdata.LoadFromSheets` nhận map thay vì đường dẫn file

`HTTPSource`/`LoadFromSheets` không cần biết settings được lưu ở đâu
hay định dạng gì nữa — `app.go` đọc `appsettings.Store` **một lần** lúc
khởi động, truyền thẳng `Settings.Gid` vào cả 2 nơi.

```go
// GO/internal/processing/pricing/http_source.go — ĐỔI
type HTTPSource struct {
	GidMap map[string]string
	Client *http.Client
}

func NewHTTPSource(gidMap map[string]string) *HTTPSource {
	return &HTTPSource{GidMap: gidMap, Client: &http.Client{Timeout: 30 * time.Second}}
}

func (s *HTTPSource) FetchIndex(sheetKey string) (*Index, error) {
	gid, ok := s.GidMap[sheetKey]
	if !ok {
		return nil, fmt.Errorf("pricing: no %s gid configured", sheetKey)
	}
	url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", spreadsheetID, gid)
	// ... phần còn lại (Get/đọc CSV/ParseIndex) giữ nguyên không đổi.
}
```

```go
// GO/internal/processing/productdata/sheets_source.go — ĐỔI
func LoadFromSheets(gidMap map[string]string, client *http.Client) (*Store, error) {
	customerRows, err := fetchSheetRows(client, gidMap, "MAKH")
	if err != nil {
		return nil, err
	}
	productRows, err := fetchSheetRows(client, gidMap, "SANPHAM")
	if err != nil {
		return nil, err
	}
	return newStore(customerRows, productRows), nil
}

func fetchSheetRows(client *http.Client, gidMap map[string]string, sheetKey string) ([][]string, error) {
	gid, ok := gidMap[sheetKey]
	if !ok {
		return nil, fmt.Errorf("productdata: no %s gid configured", sheetKey)
	}
	// ... phần còn lại giữ nguyên không đổi.
}
```

### 5. `App` — đọc settings 1 lần, expose 2 method mới cho frontend

**Lưu ý về đường dẫn file mới**: `resolveRepoFile("settings.bhconfig")`
KHÔNG dùng được để xác định nơi TẠO file mới — hàm này chỉ tìm file đã
tồn tại sẵn (đi ngược lên 5 cấp thư mục cha, trả về đường dẫn bare
filename nếu không tìm thấy ở đâu cả, xem doc comment gốc của hàm).
Lần đầu chạy, `settings.bhconfig` chưa tồn tại ở bất kỳ đâu → sẽ trả về
chuỗi tương đối "settings.bhconfig", ghi ra sai chỗ (phụ thuộc working
directory hiện tại — khác nhau giữa `wails dev` và bản `.exe` đã build,
đúng vấn đề mà `resolveRepoFile`'s doc comment đã cảnh báo). Thay vào
đó, dùng `resolveRepoDir("settings.ini")` (hàm đã có sẵn, cùng file
`app.go`) để lấy THƯ MỤC chứa `settings.ini` (file CHẮC CHẮN tồn tại,
vì app đã và đang chạy dựa vào nó), rồi ghép tên file mới vào — cho ra
đường dẫn ổn định, đúng chỗ, dùng được cho CẢ đọc lẫn ghi, mọi lần
chạy:

```go
// GO/app.go — trong NewApp()
appSettingsPath := filepath.Join(resolveRepoDir("settings.ini"), "settings.bhconfig")
appSettingsStore := appsettings.NewStore(appSettingsPath)
settings, err := appSettingsStore.Load(resolveRepoFile("settings.ini"))
if err != nil {
	return nil, fmt.Errorf("app: load app settings: %w", err)
}

store, err := productdata.LoadFromSheets(settings.Gid, productdata.NewHTTPClient())
if err != nil {
	return nil, fmt.Errorf("app: load customer/product data from Google Sheets: %w", err)
}

// ... (giữ nguyên excelPath, các phần khác)

return &App{
	cfg:              config.NewStore(configFileName),
	appSettingsStore: appSettingsStore,
	processor: &processing.RealProcessor{
		Store:     store,
		Pricing:   pricing.NewHTTPSource(settings.Gid),
		ExcelPath: excelPath,
	},
	orderDir:  orderFolderName,
	excelPath: excelPath,
}, nil
```

Thêm field `appSettingsStore *appsettings.Store` vào `App` struct, và 2
method mới (theo đúng pattern `GetSTT`/`SetSTT` hiện có):

```go
// GetAppSettings trả về toàn bộ cấu hình hiện tại (gid/zalo/reminder)
// để popup cài đặt hiển thị.
func (a *App) GetAppSettings() (appsettings.Settings, error) {
	return a.appSettingsStore.Load(resolveRepoFile("settings.ini"))
}

// SaveAppSettings ghi cấu hình mới. Thay đổi gid CHỈ có hiệu lực ở lần
// khởi động app tiếp theo — Pricing/Store hiện tại (RealProcessor) đã
// được tạo sẵn với gid map cũ lúc NewApp chạy, không tự động cập nhật
// lại giữa phiên làm việc (quyết định rõ ràng của user, tránh phức tạp
// reload).
func (a *App) SaveAppSettings(settings appsettings.Settings) error {
	return a.appSettingsStore.Save(settings)
}
```

Wails tự sinh `wailsjs/go/main/App.GetAppSettings` /
`wailsjs/go/main/App.SaveAppSettings` khi build/dev, giống các method
hiện có.

### 6. Xóa `pricing/gid.go` + `pricing/gid_test.go`

Sau khi `HTTPSource` không còn gọi `LoadGidMap` nữa (mục 4), không còn
nơi nào trong codebase gọi hàm này — xóa hẳn 2 file, không giữ lại
"phòng khi cần" (logic tương đương đã có trong
`appsettings/migrate.go`, dùng cho đúng 1 mục đích: đọc file cũ 1 lần
lúc migrate).

### 7. Frontend

**Vị trí mở popup**: icon bánh răng (⚙, `FaGear` từ `react-icons/fa6`
đã có sẵn trong dependency, các icon khác trong app đều lấy từ đây)
đặt cạnh 2 `TabButton` hiện có trong header của `App.tsx`, KHÔNG phải 1
tab thứ 3 (đây là 1 popup/modal, không phải nội dung điều hướng
chính).

**`SettingsModal.tsx`** (component mới, modal đầu tiên của app — dùng
overlay cố định `fixed inset-0 bg-black/60` + panel `bg-panel` giữa
màn hình, khớp theme tối hiện có của app):

- 3 tab con bên trong modal: "Google Sheets (GID)" / "Zalo" /
  "Nhắc nhở" — dùng lại pattern `TabButton` đã có trong `App.tsx` (kéo
  ra thành component dùng chung nếu hợp lý, hoặc copy — quyết định lúc
  code, không ảnh hưởng thiết kế).
- Mỗi tab render 1 `KeyValueEditor` (component dùng chung, xem bên
  dưới) với dữ liệu tương ứng `settings.gid` / `settings.zalo` /
  `settings.reminder`.
- Load dữ liệu: gọi `GetAppSettings()` lúc modal mở (không phải lúc
  app khởi động — tránh giữ state cũ nếu modal mở lại nhiều lần trong
  1 phiên).
- Nút "Lưu" ở cuối modal: validate (xem bên dưới) → gọi
  `SaveAppSettings(settings)` → thành công: hiện dòng chữ
  "Đã lưu. Khởi động lại app để áp dụng thay đổi." (không tự đóng
  modal ngay, để người dùng đọc được thông báo) → thất bại: hiện lỗi
  qua `appendLog` (dùng lại cơ chế log đã có) + không đóng modal.
- Nút "Đóng"/click ra ngoài overlay: đóng modal, KHÔNG lưu (không cảnh
  báo "có thay đổi chưa lưu" — YAGNI, xem Phạm vi).

**`KeyValueEditor.tsx`** (component dùng chung cho cả 3 tab):

```tsx
interface KeyValueEditorProps {
  entries: Record<string, string>
  onChange: (entries: Record<string, string>) => void
  keyLabel: string
  valueLabel: string
  valueType: 'text' | 'number' | 'toggle'
}
```

- Render bảng: cột khóa (input text), cột giá trị (input text/số, hoặc
  checkbox nếu `valueType === 'toggle'` — dùng cho tab Reminder, giá
  trị `"1"`/`""` map với checked/unchecked), nút xóa dòng (icon thùng
  rác) mỗi dòng, 1 dòng trống + nút "Thêm dòng" ở cuối bảng.
- `valueType === 'number'` (dùng cho tab GID): input giá trị chỉ nhận
  chữ số, hiện lỗi ngay dưới ô nếu gõ ký tự không phải số.
- **Validate khóa trùng**: so sánh case-sensitive (khớp cách map Go
  hoạt động — 2 khóa khác hoa/thường là 2 khóa khác nhau thật sự, ví
  dụ nếu tồn tại cả `"BigC"` và `"BIGC"`). Khóa trùng → viền đỏ ô đó +
  disable nút "Lưu" của TOÀN modal (không chỉ tab đang xem — tránh lưu
  1 tab đang có lỗi ở tab khác chưa xem tới).
- Khóa hoặc giá trị rỗng: cho phép gõ dở khi đang sửa, nhưng KHÔNG tính
  vào `entries` khi gọi `onChange` (dòng rỗng bị bỏ qua lúc lưu, không
  báo lỗi).

**`types.ts`**: thêm

```ts
export interface AppSettings {
  gid: Record<string, string>
  zalo: Record<string, string>
  reminder: Record<string, string>
}
```

## Phạm vi

### Làm thật

- Package `internal/appsettings`: `Settings`, `Store.Load`/`Store.Save`,
  migrate tự động từ `settings.ini` cũ (giữ nguyên file cũ, không xóa).
- Đổi `pricing.HTTPSource`/`productdata.LoadFromSheets` nhận
  `map[string]string` thay vì đường dẫn file.
- Xóa `pricing/gid.go` + `gid_test.go` (không còn nơi nào gọi).
- `App.GetAppSettings`/`App.SaveAppSettings`.
- Frontend: `SettingsModal.tsx`, `KeyValueEditor.tsx` dùng chung 3 tab,
  icon mở modal trong header.
- Cả 3 khối `<gid>`/`<zalo>`/`<reminder>` đều có trong popup ngay từ
  đầu (quyết định của user — không đợi tính năng Zalo xây xong).

### Không làm (YAGNI)

- Không mã hóa/làm rối nội dung file `.bhconfig` — vẫn là JSON đọc
  được nếu cố mở tay (quyết định rõ ràng của user, đánh đổi lấy khả
  năng tự debug).
- Không tự động áp dụng thay đổi gid ngay khi đang chạy (không reload
  `RealProcessor`/`HTTPSource` giữa phiên) — yêu cầu khởi động lại app.
- Không di chuyển file `.bhconfig` sang thư mục khác/ẩn — vẫn cùng thư
  mục gốc như `settings.ini` cũ (chỉ đổi CÁCH xác định đường dẫn, xem
  mục 5 — không đổi VỊ TRÍ thư mục).
- Không xóa `settings.ini` cũ sau khi migrate — giữ nguyên trên đĩa.
- Không có cảnh báo "có thay đổi chưa lưu" khi đóng modal mà chưa bấm
  Lưu.
- Không xây tính năng Zalo/reminder THẬT (gửi tin nhắn, nhắc nhở) —
  chỉ xây popup chỉnh SỐ LIỆU CẤU HÌNH cho 2 tính năng đó, bản thân
  tính năng vẫn "SẮP RA MẮT" như hiện tại.

## Rủi ro / lưu ý

- **Thứ tự parse trong `migrateFromOldIni`**: nếu `settings.ini` cũ có
  lỗi cú pháp thật (ví dụ dòng có 2 dấu `=`) ở BẤT KỲ khối nào trong 3
  khối, `Load()` trả lỗi và app không khởi động được — GIỐNG HỆT hành
  vi hiện tại (`LoadGidMap` cũng trả lỗi tương tự). Không phải rủi ro
  mới, chỉ là hành vi cũ được giữ nguyên qua lần chuyển đổi.
- **Chạy migrate nhiều lần**: nếu vì lý do gì đó `settings.bhconfig` bị
  xóa sau khi đã migrate 1 lần, `Load()` sẽ migrate LẠI từ
  `settings.ini` cũ (vẫn còn nguyên trên đĩa) — có thể ghi đè mất các
  thay đổi đã lưu qua popup từ trước nếu chúng không có trong file cũ.
  Chấp nhận được (trường hợp hiếm, người dùng tự xóa file cấu hình thì
  tự chịu mất thay đổi gần nhất, không phải luồng chính).
- **`ensureMaps` xử lý map nil**: cần verify thực tế `json.Marshal` map
  nil ra `"null"` (không phải `"{}"`) trước khi viết test — đây là hành
  vi chuẩn của `encoding/json` nhưng nên verify trực tiếp thay vì giả
  định, theo đúng kỷ luật dự án.
- **`HTTPSource.GidMap`/`LoadFromSheets`'s gidMap giờ là snapshot tĩnh**,
  không còn đọc lại file mỗi lần gọi như bản cũ — hành vi CŨ vốn đã
  lãng phí (đọc lại cùng 1 file không đổi hàng chục lần trong 1 lần xử
  lý), đổi sang đọc 1 lần là cải thiện, không phải rủi ro.

## Kiểm thử

- `appsettings` package (test mới hoàn toàn):
  - `TestStore_Load_MigratesFromOldIni`: dựng file `settings.ini` giả
    (3 khối, có cả dòng comment `#` như file thật hiện tại) trong
    `t.TempDir()`, gọi `Load`, assert `Settings` đúng cả 3 map, assert
    file `.bhconfig` mới đã được tạo ra trên đĩa.
  - `TestStore_Load_PrefersNewFileOverOldIni`: cả 2 file cùng tồn tại,
    nội dung khác nhau — assert `Load` đọc từ `.bhconfig`, không đọc lại
    `settings.ini`.
  - `TestStore_Load_NeitherFileExists`: trả `Settings` với 3 map rỗng,
    không lỗi.
  - `TestParseTagBlock_MalformedLineReturnsError`: dòng có 2 dấu `=` →
    lỗi rõ ràng (mirror `TestLoadGidMap_MalformedLineReturnsError` cũ).
  - `TestStore_Save_RoundTrips`: `Save` rồi `Load` lại, dữ liệu giống
    hệt.
- `pricing`/`productdata`: cập nhật các test hiện có gọi
  `NewHTTPSource`/`LoadFromSheets` sang signature mới (`map[string]string`
  thay vì đường dẫn file) — không đổi assertion, chỉ đổi cách khởi tạo.
- Frontend: verify bằng `wails dev` thật (theo quy ước dự án cho thay
  đổi UI) — mở modal, thêm/sửa/xóa dòng ở cả 3 tab, validate khóa
  trùng chặn nút Lưu, lưu thành công, khởi động lại app xác nhận gid
  mới có hiệu lực (`pricing`/`productdata` dùng đúng gid mới).
