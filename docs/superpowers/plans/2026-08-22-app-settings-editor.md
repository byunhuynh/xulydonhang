# Popup chỉnh sửa cấu hình app Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Thay `settings.ini` (file text bespoke, dễ bị mở/sửa tay sai cú pháp) bằng 1 file JSON đuôi riêng (`settings.bhconfig`) + 1 popup trong app để chỉnh sửa trực tiếp, có 3 tab (GID/Zalo/Nhắc nhở).

**Architecture:** Package Go mới `internal/appsettings` đọc/ghi file mới, tự động migrate 1 lần từ `settings.ini` cũ. `pricing.HTTPSource`/`productdata.LoadFromSheets` đổi sang nhận `map[string]string` thay vì tự đọc file — `app.go` đọc settings 1 lần lúc khởi động rồi truyền vào. Frontend thêm 1 modal (component React đầu tiên dạng popup trong app) với 3 tab dùng chung 1 bảng key-value có thể sửa.

**Tech Stack:** Go 1.2x, `encoding/json`, Wails v2 bindings, React + TypeScript, Tailwind (theme màu đã có sẵn).

**Spec:** [docs/superpowers/specs/2026-08-22-app-settings-editor-design.md](../specs/2026-08-22-app-settings-editor-design.md)

## Global Constraints

- File cấu hình mới tên `settings.bhconfig`, định dạng JSON có thụt lề (`json.MarshalIndent`, 2 space) — KHÔNG mã hóa, đọc được nếu người dùng cố mở bằng tay.
- `settings.ini` cũ GIỮ NGUYÊN trên đĩa sau khi migrate — không xóa, không sửa.
- Đường dẫn file `.bhconfig` PHẢI tính qua `resolveRepoDir("settings.ini")` + `filepath.Join`, KHÔNG dùng `resolveRepoFile("settings.bhconfig")` trực tiếp (file chưa tồn tại lần đầu chạy nên `resolveRepoFile` sẽ trả sai đường dẫn tùy working directory — xem spec mục 5).
- Thay đổi gid qua popup CHỈ có hiệu lực sau khi khởi động lại app — không reload `RealProcessor`/`HTTPSource` giữa phiên.
- Cả 3 khối `<gid>`/`<zalo>`/`<reminder>` đều có trong popup ngay từ đầu, dù `<zalo>`/`<reminder>` chưa được Go dùng ở tính năng nào (xác nhận qua grep — không có file `.go` nào tham chiếu 2 tag này ngoài `settings.ini` chính nó).
- Validate: khóa trùng (case-sensitive) chặn nút Lưu của TOÀN modal; tab GID validate giá trị là số; tab Nhắc nhở dùng checkbox (giá trị `"1"`/`""`); dòng khóa hoặc giá trị rỗng bị bỏ qua lúc lưu, không báo lỗi.
- Sau khi Task 2 xong, `pricing/gid.go` + `pricing/gid_test.go` bị XÓA HẲN — không giữ lại "phòng khi cần".

---

### Task 1: Package `internal/appsettings`

**Files:**
- Create: `GO/internal/appsettings/store.go`
- Create: `GO/internal/appsettings/migrate.go`
- Test: `GO/internal/appsettings/store_test.go`
- Test: `GO/internal/appsettings/migrate_test.go`

**Interfaces:**
- Consumes: không có (package độc lập, không phụ thuộc code khác trong dự án ngoài thư viện chuẩn).
- Produces: `appsettings.Settings{Gid, Zalo, Reminder map[string]string}`, `appsettings.NewStore(path string) *Store`, `(s *Store) Load(oldIniPath string) (Settings, error)`, `(s *Store) Save(settings Settings) error` — Task 2 và Task 4 dùng đúng các tên/signature này.

- [ ] **Step 1: Verify thực tế `json.Marshal` map nil ra gì**

Trước khi viết `ensureMaps`, xác nhận trực tiếp (không giả định) bằng cách chạy:

```bash
cd GO && cat > /tmp/niltest.go << 'EOF'
package main
import ("encoding/json";"fmt")
type T struct { M map[string]string `json:"m"` }
func main() {
	var t T
	b, _ := json.MarshalIndent(t, "", "  ")
	fmt.Println(string(b))
}
EOF
go run /tmp/niltest.go
```

Expected: in ra `{\n  "m": null\n}` — xác nhận map nil marshal ra `null`, không phải `{}`. Nếu kết quả khác, dừng lại và báo cáo thay vì viết `ensureMaps` theo giả định sai.

- [ ] **Step 2: Đọc `pricing/gid.go` hiện tại để bám sát chính xác logic gốc**

Đọc file `GO/internal/processing/pricing/gid.go` (hàm `LoadGidMap`) — đây là logic sẽ được tổng quát hóa ở Step 4. Không viết lại từ trí nhớ; copy chính xác cấu trúc regex + vòng lặp tách dòng.

- [ ] **Step 3: Viết `store.go`**

```go
// GO/internal/appsettings/store.go
package appsettings

import (
	"encoding/json"
	"fmt"
	"os"
)

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
// không xóa, không sửa. Nếu CẢ 2 file đều không tồn tại, trả về
// Settings với 3 map rỗng (không lỗi — app mới cài lần đầu, chưa có
// cấu hình gì).
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

	settings, migrated, err := migrateFromOldIni(oldIniPath)
	if err != nil {
		return Settings{}, err
	}
	if !migrated {
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
// ra "null" thay vì "{}" (xác nhận thực nghiệm ở Step 1), và frontend
// (TypeScript) cần luôn nhận được object thật để render bảng, không
// phải null.
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

- [ ] **Step 4: Viết `migrate.go`**

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
// <tag>\nKEY = VALUE\n...\n</tag>, xem pricing/gid.go's LoadGidMap bản
// gốc — hàm này thay thế nó, tổng quát hóa tên tag thành tham số thay
// vì chỉ đọc riêng <gid>) và trả về Settings + true nếu file tồn tại
// và đọc được, hoặc Settings{} + false nếu file không tồn tại (KHÔNG
// phải lỗi — app có thể chưa từng có settings.ini, ví dụ cài mới hoàn
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
// (comment, ví dụ 2 dòng "# MAKH/SANPHAM..." đã có sẵn trong <gid>);
// dòng có NHIỀU HƠN 1 dấu "=" là lỗi rõ ràng, không âm thầm lấy phần
// đầu/cuối. Không tìm thấy khối tag → trả map rỗng, không lỗi (ví dụ
// file cũ không có <reminder> vì được thêm sau).
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

- [ ] **Step 5: Chạy build để xác nhận package biên dịch được**

Run: `cd GO && go build ./internal/appsettings/...`
Expected: không lỗi (chưa có test nào chạy ở bước này).

- [ ] **Step 6: Viết test cho `migrate.go`**

```go
// GO/internal/appsettings/migrate_test.go
package appsettings

import (
	"os"
	"path/filepath"
	"testing"
)

func writeIniFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.ini")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed writing ini file: %v", err)
	}
	return path
}

func TestMigrateFromOldIni_ParsesAllThreeBlocks(t *testing.T) {
	path := writeIniFile(t, "[GoogleSheets]\n<gid>\nCOOP = 1741405320\n# a comment line\nMAKH = 0\n</gid>\n<zalo>\nMNCOOPMART = Đơn hàng Co-op Miền Nam\n</zalo>\n<reminder>\nMNKINGFOOD = 1\n</reminder>\n")

	settings, migrated, err := migrateFromOldIni(path)
	if err != nil {
		t.Fatalf("migrateFromOldIni returned error: %v", err)
	}
	if !migrated {
		t.Fatal("migrateFromOldIni returned migrated=false for an existing file")
	}
	if settings.Gid["COOP"] != "1741405320" {
		t.Errorf("Gid[COOP] = %q, want %q", settings.Gid["COOP"], "1741405320")
	}
	if settings.Gid["MAKH"] != "0" {
		t.Errorf("Gid[MAKH] = %q, want %q", settings.Gid["MAKH"], "0")
	}
	if len(settings.Gid) != 2 {
		t.Errorf("len(Gid) = %d, want 2 (comment line must be skipped, not parsed as a key)", len(settings.Gid))
	}
	if settings.Zalo["MNCOOPMART"] != "Đơn hàng Co-op Miền Nam" {
		t.Errorf("Zalo[MNCOOPMART] = %q, want %q", settings.Zalo["MNCOOPMART"], "Đơn hàng Co-op Miền Nam")
	}
	if settings.Reminder["MNKINGFOOD"] != "1" {
		t.Errorf("Reminder[MNKINGFOOD] = %q, want %q", settings.Reminder["MNKINGFOOD"], "1")
	}
}

func TestMigrateFromOldIni_FileDoesNotExist(t *testing.T) {
	_, migrated, err := migrateFromOldIni(filepath.Join(t.TempDir(), "does-not-exist.ini"))
	if err != nil {
		t.Fatalf("migrateFromOldIni returned error for a missing file: %v", err)
	}
	if migrated {
		t.Fatal("migrateFromOldIni returned migrated=true for a missing file")
	}
}

func TestMigrateFromOldIni_MissingBlockReturnsEmptyMap(t *testing.T) {
	path := writeIniFile(t, "[GoogleSheets]\n<gid>\nCOOP = 123\n</gid>\n")

	settings, migrated, err := migrateFromOldIni(path)
	if err != nil {
		t.Fatalf("migrateFromOldIni returned error: %v", err)
	}
	if !migrated {
		t.Fatal("migrateFromOldIni returned migrated=false")
	}
	if len(settings.Zalo) != 0 {
		t.Errorf("Zalo = %v, want empty (no <zalo> block in this file)", settings.Zalo)
	}
	if len(settings.Reminder) != 0 {
		t.Errorf("Reminder = %v, want empty (no <reminder> block in this file)", settings.Reminder)
	}
}

func TestParseTagBlock_MalformedLineReturnsError(t *testing.T) {
	path := writeIniFile(t, "<gid>\nCOOP = 123 = 456\n</gid>\n")

	if _, _, err := migrateFromOldIni(path); err == nil {
		t.Fatal("migrateFromOldIni expected error for a <gid> line with more than one '=', got nil")
	}
}
```

- [ ] **Step 7: Chạy test `migrate.go`, xác nhận pass**

Run: `cd GO && go test ./internal/appsettings/... -run TestMigrateFromOldIni -v`
Run: `cd GO && go test ./internal/appsettings/... -run TestParseTagBlock -v`
Expected: cả 4 test PASS.

- [ ] **Step 8: Viết test cho `store.go`**

```go
// GO/internal/appsettings/store_test.go
package appsettings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_Load_MigratesFromOldIni(t *testing.T) {
	dir := t.TempDir()
	oldIniPath := filepath.Join(dir, "settings.ini")
	if err := os.WriteFile(oldIniPath, []byte("<gid>\nCOOP = 1741405320\n</gid>\n<zalo>\nMNCOOPMART = Đơn hàng Co-op Miền Nam\n</zalo>\n<reminder>\nMNKINGFOOD = 1\n</reminder>\n"), 0o644); err != nil {
		t.Fatalf("failed writing old ini file: %v", err)
	}
	newPath := filepath.Join(dir, "settings.bhconfig")
	store := NewStore(newPath)

	settings, err := store.Load(oldIniPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if settings.Gid["COOP"] != "1741405320" {
		t.Errorf("Gid[COOP] = %q, want %q", settings.Gid["COOP"], "1741405320")
	}
	if settings.Zalo["MNCOOPMART"] != "Đơn hàng Co-op Miền Nam" {
		t.Errorf("Zalo[MNCOOPMART] = %q, want %q", settings.Zalo["MNCOOPMART"], "Đơn hàng Co-op Miền Nam")
	}

	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("settings.bhconfig was not created on disk: %v", err)
	}
	if _, err := os.Stat(oldIniPath); err != nil {
		t.Errorf("old settings.ini was removed or is inaccessible — must be left untouched: %v", err)
	}
}

func TestStore_Load_PrefersNewFileOverOldIni(t *testing.T) {
	dir := t.TempDir()
	oldIniPath := filepath.Join(dir, "settings.ini")
	if err := os.WriteFile(oldIniPath, []byte("<gid>\nCOOP = old-value\n</gid>\n"), 0o644); err != nil {
		t.Fatalf("failed writing old ini file: %v", err)
	}
	newPath := filepath.Join(dir, "settings.bhconfig")
	store := NewStore(newPath)
	if err := store.Save(Settings{Gid: map[string]string{"COOP": "new-value"}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	settings, err := store.Load(oldIniPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if settings.Gid["COOP"] != "new-value" {
		t.Errorf("Gid[COOP] = %q, want %q (Load must prefer the .bhconfig file over settings.ini once it exists)", settings.Gid["COOP"], "new-value")
	}
}

func TestStore_Load_NeitherFileExists(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "settings.bhconfig"))

	settings, err := store.Load(filepath.Join(dir, "settings.ini"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if settings.Gid == nil || len(settings.Gid) != 0 {
		t.Errorf("Gid = %v, want empty non-nil map", settings.Gid)
	}
	if settings.Zalo == nil || len(settings.Zalo) != 0 {
		t.Errorf("Zalo = %v, want empty non-nil map", settings.Zalo)
	}
	if settings.Reminder == nil || len(settings.Reminder) != 0 {
		t.Errorf("Reminder = %v, want empty non-nil map", settings.Reminder)
	}
}

func TestStore_Save_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.bhconfig")
	store := NewStore(path)
	want := Settings{
		Gid:      map[string]string{"COOP": "123", "BIGC": "456"},
		Zalo:     map[string]string{"MNCOOPMART": "Đơn hàng Co-op Miền Nam"},
		Reminder: map[string]string{"MNKINGFOOD": "1"},
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	got, err := store.Load(filepath.Join(dir, "settings.ini"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got.Gid["COOP"] != "123" || got.Gid["BIGC"] != "456" {
		t.Errorf("Gid = %v, want %v", got.Gid, want.Gid)
	}
	if got.Zalo["MNCOOPMART"] != "Đơn hàng Co-op Miền Nam" {
		t.Errorf("Zalo = %v, want %v", got.Zalo, want.Zalo)
	}
	if got.Reminder["MNKINGFOOD"] != "1" {
		t.Errorf("Reminder = %v, want %v", got.Reminder, want.Reminder)
	}
}
```

- [ ] **Step 9: Chạy toàn bộ test package, xác nhận pass**

Run: `cd GO && go test ./internal/appsettings/... -v`
Expected: tất cả 8 test (4 ở Step 7 + 4 ở Step 8) PASS.

- [ ] **Step 10: Commit**

```bash
cd GO && git add internal/appsettings/
git commit -m "feat(go): add internal/appsettings package (settings.bhconfig store + migration from settings.ini)"
```

---

### Task 2: Nối lại `pricing`/`productdata`/`app.go` dùng `appsettings` (task gộp — đổi đồng thời để build không đỏ giữa chừng)

**Files:**
- Modify: `GO/internal/processing/pricing/http_source.go`
- Delete: `GO/internal/processing/pricing/gid.go`
- Delete: `GO/internal/processing/pricing/gid_test.go`
- Modify: `GO/internal/processing/productdata/sheets_source.go`
- Modify: `GO/internal/processing/productdata/sheets_source_test.go`
- Modify: `GO/app.go`

**Interfaces:**
- Consumes: `appsettings.Settings`, `appsettings.NewStore(path string) *Store`, `(s *Store) Load(oldIniPath string) (Settings, error)` (Task 1).
- Produces: `pricing.NewHTTPSource(gidMap map[string]string) *HTTPSource`, `productdata.LoadFromSheets(gidMap map[string]string, client *http.Client) (*Store, error)`, `App.appSettingsStore *appsettings.Store` field, `App.GetAppSettings() (appsettings.Settings, error)`, `App.SaveAppSettings(settings appsettings.Settings) error` — Task 4 (frontend) gọi 2 method cuối qua Wails binding.

**Vì sao gộp thành 1 task**: `pricing.NewHTTPSource`/`productdata.LoadFromSheets` đổi signature đồng thời làm `app.go` (nơi DUY NHẤT gọi cả 2 hàm này) không biên dịch được cho đến khi CŨNG được sửa — tách nhỏ theo file sẽ để lại trạng thái build đỏ giữa các bước, không review được độc lập.

- [ ] **Step 1: Đổi `pricing/http_source.go`**

Thay toàn bộ nội dung file bằng:

```go
// GO/internal/processing/pricing/http_source.go
package pricing

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

const spreadsheetID = "1yvxE_SPYXKhofcZdhv1CSKAyiwdY1Mf4pFlsiMbtOr4"

// HTTPSource fetches a vendor's live pricing/promotion sheet over HTTP.
// Every vendor's sheet lives in the same Google Sheet (spreadsheetID),
// on a different tab selected by gid — confirmed in xulydonhang.py's
// find_price_by_sku/find_all_promotions_by_sku_and_time family, which
// all hardcode the same sheet_id and vary only the sheet_name param used
// to resolve gid. It is the production PricingSource; tests substitute a
// fixture-backed implementation instead of hitting the network.
//
// GidMap is a snapshot read once at app startup (see appsettings.Store)
// rather than a file path re-read on every FetchIndex call — the
// previous version re-read settings.ini from disk on every single call,
// which was always wasted work (the file never changes mid-batch).
// Changing a gid via the in-app settings popup takes effect on the next
// app restart, not live mid-session — a deliberate simplicity choice.
type HTTPSource struct {
	GidMap map[string]string
	Client *http.Client
}

func NewHTTPSource(gidMap map[string]string) *HTTPSource {
	return &HTTPSource{GidMap: gidMap, Client: &http.Client{Timeout: 30 * time.Second}}
}

// FetchIndex mirrors find_price_by_sku/find_all_promotions_by_sku_and_time's
// URL construction: sheetKey is the same value as their sheet_name
// parameter (e.g. "COOP", "LOTTE" — must match a key in GidMap).
func (s *HTTPSource) FetchIndex(sheetKey string) (*Index, error) {
	gid, ok := s.GidMap[sheetKey]
	if !ok {
		return nil, fmt.Errorf("pricing: no %s gid configured", sheetKey)
	}

	url := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", spreadsheetID, gid)
	resp, err := s.Client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("pricing: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricing: fetch %s: HTTP %d", url, resp.StatusCode)
	}

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1 // rows can have varying column counts
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("pricing: parse CSV from %s: %w", url, err)
	}

	return ParseIndex(rows), nil
}
```

- [ ] **Step 2: Xóa `pricing/gid.go` và `pricing/gid_test.go`**

```bash
cd GO && rm internal/processing/pricing/gid.go internal/processing/pricing/gid_test.go
```

- [ ] **Step 3: Đổi `productdata/sheets_source.go`**

Đổi hàm `LoadFromSheets` (giữ nguyên phần còn lại của file — `productDataSpreadsheetID`, `fetchSheetRows` thân hàm, `NewHTTPClient`):

Tìm:
```go
import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"

	"order-processor/internal/processing/pricing"
)
```
Thay bằng (bỏ import `pricing` — không còn gọi `pricing.LoadGidMap` nữa):
```go
import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)
```

Tìm:
```go
// LoadFromSheets builds a Store from the live Google Sheet, replacing
// the local data.xlsx file as the production source — so that updating
// customer/product data takes effect on every machine running this app
// without needing to distribute an updated file, the same reasoning
// that already applies to pricing.HTTPSource's own live fetch. settings
// resolves to settings.ini's own <gid> block (see pricing.LoadGidMap),
// which must contain "MAKH" and "SANPHAM" keys naming each tab's gid
// within productDataSpreadsheetID.
//
// There is deliberately no offline fallback to a local file: if the
// network is unreachable, this returns an error and the caller (NewApp)
// fails to start, exactly like the local data.xlsx path already did on
// a read failure — a conscious choice discussed with the project owner,
// matching this app's existing full dependency on the same network for
// live pricing/promotion lookups mid-processing (see pricing.HTTPSource),
// rather than adding asymmetric resilience only for this one data
// source.
func LoadFromSheets(settingsPath string, client *http.Client) (*Store, error) {
	gidMap, err := pricing.LoadGidMap(settingsPath)
	if err != nil {
		return nil, err
	}

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
```

Thay bằng:
```go
// LoadFromSheets builds a Store from the live Google Sheet, replacing
// the local data.xlsx file as the production source — so that updating
// customer/product data takes effect on every machine running this app
// without needing to distribute an updated file, the same reasoning
// that already applies to pricing.HTTPSource's own live fetch. gidMap
// is a snapshot read once at app startup by the caller (see
// appsettings.Store) — must contain "MAKH" and "SANPHAM" keys naming
// each tab's gid within productDataSpreadsheetID.
//
// There is deliberately no offline fallback to a local file: if the
// network is unreachable, this returns an error and the caller (NewApp)
// fails to start, exactly like the local data.xlsx path already did on
// a read failure — a conscious choice discussed with the project owner,
// matching this app's existing full dependency on the same network for
// live pricing/promotion lookups mid-processing (see pricing.HTTPSource),
// rather than adding asymmetric resilience only for this one data
// source.
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
```

`fetchSheetRows` đã nhận `gidMap map[string]string` trực tiếp từ trước (xem file hiện tại) — KHÔNG cần đổi gì thêm ở hàm đó.

- [ ] **Step 4: Đổi `productdata/sheets_source_test.go`**

Tìm hàm `TestLoadFromSheets_MissingGidReturnsClearError` hiện tại (dùng `settingsPath` + file `settings.ini` giả), thay toàn bộ thân hàm bằng:

```go
func TestLoadFromSheets_MissingGidReturnsClearError(t *testing.T) {
	_, err := LoadFromSheets(map[string]string{"COOP": "123"}, NewHTTPClient())
	if err == nil {
		t.Fatal("LoadFromSheets returned nil error for a gid map with no MAKH/SANPHAM key")
	}
	if !containsSubstring(err.Error(), "MAKH") {
		t.Errorf("LoadFromSheets error = %q, want it to mention the missing MAKH key", err.Error())
	}
}
```

Xóa import `"os"` và `"path/filepath"` khỏi đầu file NẾU không còn hàm nào khác trong file dùng tới (kiểm tra bằng cách đọc lại toàn bộ file trước khi xóa import — `readCSVFixture` trong cùng file dùng `os.Open`/`filepath.Join`, nên 2 import này VẪN CẦN GIỮ LẠI — chỉ bỏ đoạn code viết `settings.ini` giả bên trong hàm test, không đổi import).

- [ ] **Step 5: Đổi `app.go`**

Thêm import (đầu file, cùng khối `order-processor/internal/...`):
```go
"order-processor/internal/appsettings"
```

Đổi struct `App` — tìm:
```go
type App struct {
	ctx          context.Context
	cfg          *config.Store
	processor    processing.Processor
	emitter      Emitter
	orderDir     string
	excelPath    string
	resolvedRows map[int]bool
	resolvedMu   sync.Mutex
	processing   atomic.Bool
}
```
Thay bằng:
```go
type App struct {
	ctx              context.Context
	cfg              *config.Store
	appSettingsStore *appsettings.Store
	processor        processing.Processor
	emitter          Emitter
	orderDir         string
	excelPath        string
	resolvedRows     map[int]bool
	resolvedMu       sync.Mutex
	processing       atomic.Bool
}
```

Đổi `NewApp()` — tìm:
```go
// NewApp creates a new App application struct
func NewApp() (*App, error) {
	// Customer/product data now comes from a live Google Sheet, not the
	// local data.xlsx file — a prior update to just this one machine's
	// data.xlsx never reached any OTHER machine running this app, the
	// exact class of staleness pricing/promotion data already avoids via
	// its own live fetch (pricing.HTTPSource). No offline fallback to
	// data.xlsx: a network failure here fails NewApp entirely, matching
	// this app's already-full dependency on the same network for live
	// pricing lookups mid-processing — a deliberate choice, not an
	// oversight (see productdata.LoadFromSheets' own doc comment).
	// data.xlsx itself is left on disk, untouched; nothing reads it
	// anymore.
	store, err := productdata.LoadFromSheets(resolveRepoFile("settings.ini"), productdata.NewHTTPClient())
	if err != nil {
		return nil, fmt.Errorf("app: load customer/product data from Google Sheets: %w", err)
	}

	excelPath := resolveRepoFile("dondathang_test.xlsx")

	return &App{
		cfg: config.NewStore(configFileName),
		processor: &processing.RealProcessor{
			Store:     store,
			Pricing:   pricing.NewHTTPSource(resolveRepoFile("settings.ini")),
			ExcelPath: excelPath,
		},
		orderDir:  orderFolderName,
		excelPath: excelPath,
	}, nil
}
```
Thay bằng:
```go
// NewApp creates a new App application struct
func NewApp() (*App, error) {
	// Cấu hình (gid Google Sheets/Zalo/nhắc nhở) giờ đọc từ
	// settings.bhconfig thay vì settings.ini — appSettingsPath PHẢI tính
	// qua resolveRepoDir, KHÔNG dùng resolveRepoFile trực tiếp: file
	// .bhconfig chưa tồn tại ở lần chạy đầu tiên, resolveRepoFile chỉ
	// tìm file ĐÃ CÓ SẴN nên sẽ trả về đường dẫn tương đối sai chỗ (phụ
	// thuộc working directory hiện tại, khác nhau giữa `wails dev` và
	// bản .exe đã build). resolveRepoDir("settings.ini") lấy đúng thư
	// mục chứa settings.ini (file chắc chắn tồn tại vì app đã và đang
	// chạy dựa vào nó), cho ra đường dẫn ổn định dùng được cho cả đọc
	// lẫn ghi, mọi lần chạy.
	appSettingsPath := filepath.Join(resolveRepoDir("settings.ini"), "settings.bhconfig")
	appSettingsStore := appsettings.NewStore(appSettingsPath)
	settings, err := appSettingsStore.Load(resolveRepoFile("settings.ini"))
	if err != nil {
		return nil, fmt.Errorf("app: load app settings: %w", err)
	}

	// Customer/product data now comes from a live Google Sheet, not the
	// local data.xlsx file — a prior update to just this one machine's
	// data.xlsx never reached any OTHER machine running this app, the
	// exact class of staleness pricing/promotion data already avoids via
	// its own live fetch (pricing.HTTPSource). No offline fallback to
	// data.xlsx: a network failure here fails NewApp entirely, matching
	// this app's already-full dependency on the same network for live
	// pricing lookups mid-processing — a deliberate choice, not an
	// oversight (see productdata.LoadFromSheets' own doc comment).
	// data.xlsx itself is left on disk, untouched; nothing reads it
	// anymore.
	store, err := productdata.LoadFromSheets(settings.Gid, productdata.NewHTTPClient())
	if err != nil {
		return nil, fmt.Errorf("app: load customer/product data from Google Sheets: %w", err)
	}

	excelPath := resolveRepoFile("dondathang_test.xlsx")

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
}
```

Thêm 2 method mới, đặt ngay sau `SetSTT` hiện có (theo đúng pattern method đơn giản đã có):
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

- [ ] **Step 6: Build toàn bộ, xác nhận sạch**

Run: `cd GO && go build ./... && go vet ./...`
Expected: không lỗi.

- [ ] **Step 7: Chạy test liên quan**

Run: `cd GO && go test ./internal/processing/pricing/... ./internal/processing/productdata/... . -v`
Expected: tất cả PASS — không còn `TestLoadGidMap_*` (đã xóa cùng file), `TestLoadFromSheets_MissingGidReturnsClearError` pass với logic mới.

- [ ] **Step 8: Chạy toàn bộ test suite, so với baseline đã biết**

Run: `cd GO && go test ./... 2>&1 | tail -30`
Expected: CHỈ còn 2 fixture Coop lỗi từ trước (`103229379-00.pdf`, `103346096-00.pdf`, không liên quan việc này — đã ghi trong bộ nhớ dự án) — không có regression mới ở bất kỳ package nào khác.

- [ ] **Step 9: Commit**

```bash
cd GO && git add app.go internal/processing/pricing/ internal/processing/productdata/sheets_source.go internal/processing/productdata/sheets_source_test.go
git commit -m "refactor(go): wire pricing/productdata through appsettings.Store instead of settings.ini paths"
```

---

### Task 3: Frontend — `types.ts` + `KeyValueEditor.tsx`

**Files:**
- Modify: `GO/frontend/src/types.ts`
- Create: `GO/frontend/src/components/KeyValueEditor.tsx`

**Interfaces:**
- Consumes: theme Tailwind đã có (`bg`, `panel`, `border`, `ink`, `muted`, `accent`, `danger` — xem `GO/frontend/tailwind.config.js`).
- Produces: `AppSettings` type, `KeyValueEditor` component (props `entries`, `onChange`, `keyLabel`, `valueLabel`, `valueType`) — Task 4 dùng cả 2 để dựng `SettingsModal.tsx`.

- [ ] **Step 1: Thêm type `AppSettings` vào `types.ts`**

Đọc `GO/frontend/src/types.ts` hiện tại trước, thêm vào cuối file:

```typescript
export interface AppSettings {
  gid: Record<string, string>
  zalo: Record<string, string>
  reminder: Record<string, string>
}
```

- [ ] **Step 2: Viết `KeyValueEditor.tsx`**

```tsx
// GO/frontend/src/components/KeyValueEditor.tsx
import { useState } from 'react'
import { FaTrash, FaPlus } from 'react-icons/fa6'

interface KeyValueEditorProps {
  entries: Record<string, string>
  onChange: (entries: Record<string, string>) => void
  keyLabel: string
  valueLabel: string
  valueType: 'text' | 'number' | 'toggle'
}

interface Row {
  id: number
  key: string
  value: string
}

let nextRowId = 0

function toRows(entries: Record<string, string>): Row[] {
  return Object.entries(entries).map(([key, value]) => ({ id: nextRowId++, key, value }))
}

// KeyValueEditor là bảng key-value dùng chung cho cả 3 tab của
// SettingsModal (GID/Zalo/Nhắc nhở) — mỗi tab chỉ khác nhau ở nhãn cột
// và valueType. Dòng có khóa hoặc giá trị rỗng bị BỎ QUA khi gọi
// onChange (không tính vào entries, không báo lỗi) — cho phép người
// dùng gõ dở dang mà không bị validate ngay lập tức.
export function KeyValueEditor({ entries, onChange, keyLabel, valueLabel, valueType }: KeyValueEditorProps) {
  const [rows, setRows] = useState<Row[]>(() => toRows(entries))

  function commit(next: Row[]) {
    setRows(next)
    const result: Record<string, string> = {}
    for (const row of next) {
      if (row.key.trim() === '' || row.value.trim() === '') continue
      result[row.key] = row.value
    }
    onChange(result)
  }

  function updateKey(id: number, key: string) {
    commit(rows.map((r) => (r.id === id ? { ...r, key } : r)))
  }

  function updateValue(id: number, value: string) {
    commit(rows.map((r) => (r.id === id ? { ...r, value } : r)))
  }

  function removeRow(id: number) {
    commit(rows.filter((r) => r.id !== id))
  }

  function addRow() {
    setRows([...rows, { id: nextRowId++, key: '', value: '' }])
  }

  const keyCounts = new Map<string, number>()
  for (const row of rows) {
    if (row.key.trim() === '') continue
    keyCounts.set(row.key, (keyCounts.get(row.key) ?? 0) + 1)
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="grid grid-cols-[1fr_1fr_auto] gap-2 px-1 font-sans text-[10px] font-bold uppercase tracking-wider text-muted">
        <span>{keyLabel}</span>
        <span>{valueLabel}</span>
        <span></span>
      </div>
      {rows.map((row) => {
        const isDuplicate = row.key.trim() !== '' && (keyCounts.get(row.key) ?? 0) > 1
        return (
          <div key={row.id} className="grid grid-cols-[1fr_1fr_auto] items-center gap-2">
            <input
              value={row.key}
              onChange={(e) => updateKey(row.id, e.target.value)}
              className={`rounded border bg-bg px-2 py-1.5 font-mono text-xs text-ink outline-none ${
                isDuplicate ? 'border-danger' : 'border-border focus:border-accent'
              }`}
            />
            {valueType === 'toggle' ? (
              <input
                type="checkbox"
                checked={row.value === '1'}
                onChange={(e) => updateValue(row.id, e.target.checked ? '1' : '')}
                className="h-4 w-4 accent-accent"
              />
            ) : (
              <input
                value={row.value}
                onChange={(e) => {
                  if (valueType === 'number' && e.target.value !== '' && !/^\d*$/.test(e.target.value)) return
                  updateValue(row.id, e.target.value)
                }}
                className="rounded border border-border bg-bg px-2 py-1.5 font-mono text-xs text-ink outline-none focus:border-accent"
              />
            )}
            <button
              type="button"
              onClick={() => removeRow(row.id)}
              className="rounded p-1.5 text-muted transition-colors hover:text-danger"
            >
              <FaTrash size={11} />
            </button>
          </div>
        )
      })}
      <button
        type="button"
        onClick={addRow}
        className="mt-1 inline-flex items-center gap-1.5 self-start rounded border border-border px-2.5 py-1 font-sans text-[11px] font-semibold text-muted transition-colors hover:border-accent hover:text-accent"
      >
        <FaPlus size={9} /> Thêm dòng
      </button>
    </div>
  )
}
```

- [ ] **Step 3: Type-check**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: không lỗi.

- [ ] **Step 4: Commit**

```bash
cd GO && git add frontend/src/types.ts frontend/src/components/KeyValueEditor.tsx
git commit -m "feat(frontend): add AppSettings type + KeyValueEditor component"
```

---

### Task 4: Frontend — `SettingsModal.tsx` + nối vào `App.tsx`

**Files:**
- Create: `GO/frontend/src/components/SettingsModal.tsx`
- Modify: `GO/frontend/src/App.tsx`

**Interfaces:**
- Consumes: `AppSettings`/`KeyValueEditor` (Task 3); `GetAppSettings(): Promise<AppSettings>`, `SaveAppSettings(settings: AppSettings): Promise<void>` (Wails binding tự sinh từ `App.GetAppSettings`/`App.SaveAppSettings`, Task 2); `appendLog` (từ `useAppStore`, đã có sẵn).
- Produces: không có task nào sau — đây là task cuối.

- [ ] **Step 1: Regenerate Wails bindings**

Run: `cd GO && wails dev -browser -loglevel Error` — chạy, chờ tới khi thấy "Vite Server URL" xuất hiện trong log (xác nhận binding đã sinh), sau đó dừng (Ctrl+C hoặc kill process). Bắt buộc trước Step 3 — nếu không, `import { GetAppSettings, SaveAppSettings } from '../../wailsjs/go/main/App'` sẽ không resolve được.

Verify: `grep -n "GetAppSettings\|SaveAppSettings" GO/frontend/wailsjs/go/main/App.d.ts`
Expected: 2 dòng khai báo hàm tương ứng.

- [ ] **Step 2: Viết `SettingsModal.tsx`**

```tsx
// GO/frontend/src/components/SettingsModal.tsx
import { useEffect, useState } from 'react'
import { FaXmark } from 'react-icons/fa6'
import { GetAppSettings, SaveAppSettings } from '../../wailsjs/go/main/App'
import { useAppStore } from '../store/appStore'
import type { AppSettings } from '../types'
import { KeyValueEditor } from './KeyValueEditor'

type SettingsTab = 'gid' | 'zalo' | 'reminder'

interface SettingsModalProps {
  onClose: () => void
}

export function SettingsModal({ onClose }: SettingsModalProps) {
  const [tab, setTab] = useState<SettingsTab>('gid')
  const [settings, setSettings] = useState<AppSettings | null>(null)
  const [saved, setSaved] = useState(false)
  const appendLog = useAppStore((s) => s.appendLog)

  useEffect(() => {
    GetAppSettings()
      .then((s) => setSettings(s))
      .catch((err) => appendLog(`❌ Lỗi tải cấu hình: ${String(err)}`))
  }, [appendLog])

  if (!settings) {
    return (
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
        <div className="rounded-xl border border-border bg-panel p-6 text-sm text-muted">Đang tải...</div>
      </div>
    )
  }

  const allKeys = [
    ...Object.keys(settings.gid),
    ...Object.keys(settings.zalo),
    ...Object.keys(settings.reminder),
  ]
  const hasDuplicates =
    new Set(allKeys.filter((k) => k.trim() !== '')).size !==
    allKeys.filter((k) => k.trim() !== '').length

  async function handleSave() {
    if (!settings) return
    try {
      await SaveAppSettings(settings)
      setSaved(true)
    } catch (err) {
      appendLog(`❌ Lỗi lưu cấu hình: ${String(err)}`)
    }
  }

  const tabs: { key: SettingsTab; label: string }[] = [
    { key: 'gid', label: 'Google Sheets (GID)' },
    { key: 'zalo', label: 'Zalo' },
    { key: 'reminder', label: 'Nhắc nhở' },
  ]

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60" onClick={onClose}>
      <div
        className="flex max-h-[80vh] w-[560px] flex-col rounded-xl border border-border bg-panel p-4"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-sans text-sm font-bold text-ink">Cấu hình app</h2>
          <button type="button" onClick={onClose} className="text-muted hover:text-ink">
            <FaXmark size={16} />
          </button>
        </div>
        <div className="mb-3 flex gap-1 border-b border-border">
          {tabs.map((t) => (
            <button
              key={t.key}
              type="button"
              onClick={() => setTab(t.key)}
              className={`px-3 py-2 font-sans text-xs font-semibold transition-colors ${
                tab === t.key ? 'border-b-2 border-accent text-accent' : 'text-muted hover:text-ink'
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>
        <div className="flex-1 overflow-y-auto">
          {tab === 'gid' && (
            <KeyValueEditor
              entries={settings.gid}
              onChange={(gid) => setSettings({ ...settings, gid })}
              keyLabel="Hệ thống"
              valueLabel="Gid"
              valueType="number"
            />
          )}
          {tab === 'zalo' && (
            <KeyValueEditor
              entries={settings.zalo}
              onChange={(zalo) => setSettings({ ...settings, zalo })}
              keyLabel="Nhóm"
              valueLabel="Tên hiển thị"
              valueType="text"
            />
          )}
          {tab === 'reminder' && (
            <KeyValueEditor
              entries={settings.reminder}
              onChange={(reminder) => setSettings({ ...settings, reminder })}
              keyLabel="Nhóm"
              valueLabel="Bật"
              valueType="toggle"
            />
          )}
        </div>
        <div className="mt-3 flex items-center justify-between border-t border-border pt-3">
          {saved ? (
            <span className="font-sans text-xs text-success">Đã lưu. Khởi động lại app để áp dụng thay đổi.</span>
          ) : (
            <span />
          )}
          <button
            type="button"
            onClick={handleSave}
            disabled={hasDuplicates}
            className="rounded-lg bg-accent px-4 py-2 font-sans text-xs font-bold text-[#0a1620] transition-opacity disabled:cursor-not-allowed disabled:opacity-40"
          >
            Lưu
          </button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Nối vào `App.tsx`**

Đọc `GO/frontend/src/App.tsx` hiện tại trước. Thêm import:

```typescript
import { FaGear } from 'react-icons/fa6'
import { SettingsModal } from './components/SettingsModal'
```

(gộp vào dòng import `react-icons/fa6` đã có: `import { FaGears, FaCircleInfo, FaGear } from 'react-icons/fa6'`)

Thêm state, ngay dưới `const isProcessing = useAppStore((s) => s.isProcessing)`:

```typescript
  const [isSettingsOpen, setIsSettingsOpen] = useState(false)
```

Thêm icon bánh răng vào header, ngay TRƯỚC thẻ `<div className="ml-auto ...">` (khu vực trạng thái xử lý hiện có):

```tsx
        <button
          type="button"
          onClick={() => setIsSettingsOpen(true)}
          className="mb-3.5 ml-2 rounded-lg border border-border p-2 text-muted transition-colors hover:border-accent hover:text-accent"
          title="Cấu hình"
        >
          <FaGear size={14} />
        </button>
```

Thêm render modal, ngay TRƯỚC thẻ đóng `</div>` cuối cùng của component (sau `<footer>`):

```tsx
      {isSettingsOpen && <SettingsModal onClose={() => setIsSettingsOpen(false)} />}
```

- [ ] **Step 4: Type-check**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: không lỗi.

- [ ] **Step 5: Verify UI thật qua `wails dev`**

Run: `cd GO && wails dev -browser -loglevel Error`

Trong trình duyệt mở ra:
1. Bấm icon bánh răng — modal mở, 3 tab hiện đúng dữ liệu đã migrate từ `settings.ini` (18 dòng GID, ~20 dòng Zalo, 7 dòng Nhắc nhở).
2. Tab GID: sửa 1 giá trị thành chữ (không phải số) — xác nhận KHÔNG gõ được (input chặn ký tự không phải số).
3. Thêm 1 dòng mới ở tab Zalo với khóa trùng 1 khóa đã có — xác nhận viền đỏ hiện ra VÀ nút "Lưu" bị disable.
4. Xóa dòng vừa thêm — nút "Lưu" hết disable.
5. Tab Nhắc nhở: xác nhận hiện checkbox, không phải input text.
6. Bấm "Lưu" — xác nhận hiện dòng "Đã lưu. Khởi động lại app để áp dụng thay đổi."
7. Mở file `settings.bhconfig` (đã sinh ra ở thư mục gốc, cùng chỗ `settings.ini`) bằng cách đọc trực tiếp — xác nhận nội dung JSON đúng, có đủ 3 khối, đúng dữ liệu vừa sửa.
8. Dừng `wails dev`, chạy lại — xác nhận app khởi động lại bình thường, đọc đúng `settings.bhconfig` (không migrate lại từ `settings.ini`, xác nhận qua log không có lỗi và dữ liệu đúng như đã lưu).
9. Đóng modal bằng click ra ngoài overlay (không bấm Lưu) — xác nhận modal đóng, không lưu gì.

Ghi lại kết quả — đây là thay đổi UI, KHÔNG coi là hoàn thành nếu chỉ dựa vào `tsc --noEmit` xanh.

- [ ] **Step 6: Commit**

```bash
cd GO && git add frontend/src/components/SettingsModal.tsx frontend/src/App.tsx
git commit -m "feat(frontend): add settings popup (gear icon, 3-tab GID/Zalo/reminder editor)"
```

## Self-Review Notes (từ người viết plan, trước khi bàn giao)

- **Bao phủ spec**: cả 7 mục trong spec đều có task tương ứng — mục 1-3 (package `appsettings`) → Task 1; mục 4-6 (đổi pricing/productdata/app.go) → Task 2; mục 7 (frontend) → Task 3+4.
- **Không có placeholder**: mọi bước code đều có nội dung literal đầy đủ, không có "TODO"/"tương tự Task N".
- **Nhất quán kiểu/tên**: `Settings`/`Store`/`NewStore`/`Load`/`Save` dùng xuyên suốt Task 1→2→4 không đổi tên. `KeyValueEditor` props (`entries`/`onChange`/`keyLabel`/`valueLabel`/`valueType`) khớp giữa Task 3 (định nghĩa) và Task 4 (dùng).
- **Validate khóa trùng**: `SettingsModal` (Task 4) tự tính `hasDuplicates` bằng cách gộp key của cả 3 map lại và so sánh kích thước `Set` — không cần `KeyValueEditor.tsx` (Task 3) export thêm hàm riêng cho việc này; `KeyValueEditor` tự lo phần viền đỏ TỪNG Ô (dựa trên khóa trùng NGAY TRONG bảng của chính nó), còn việc disable nút Lưu (cần biết trùng khóa CẢ 3 tab) là trách nhiệm của `SettingsModal`.
