# Khung sườn Go + Wails + React (Phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dựng khung sườn ứng dụng desktop Go + Wails v2 + React (thư mục `GO/` ở gốc repo) tái hiện luồng UI hiện có (chọn/kéo-thả file, STT, log realtime, bảng kết quả) với xử lý đơn hàng chạy qua `MockProcessor`, sẵn sàng để Phase 2 cắm engine parse PDF thật vào mà không phải sửa lại UI hay event contract.

**Architecture:** Backend Go (Wails v2) expose các method qua `App` struct; xử lý file chạy trong goroutine, phát sự kiện (`process:log`, `process:row`, `process:done`, `files:dropped`) qua `runtime.EventsEmit`. Frontend React+TypeScript nhận sự kiện qua hook `useWailsEvents`, lưu state trong Zustand store, render qua các component theo section (FileListPanel, LogPanel, ResultTable, ControlPanel). `processing.Processor` là interface duy nhất — Phase 1 chỉ có `MockProcessor`.

**Tech Stack:** Go 1.26 (đã cài, `go version` = go1.26.5), Wails v2.13.0 (đã cài, `wails version` = v2.13.0), Node v24.13.0 / npm 11.15.0 (đã cài), React 18 + TypeScript + Vite (template `react-ts` của Wails), Tailwind CSS, Zustand, react-icons (bộ `fa6` = Font Awesome 6).

**Spec:** [docs/superpowers/specs/2026-08-13-go-wails-skeleton-design.md](../specs/2026-08-13-go-wails-skeleton-design.md)

## Global Constraints

- Toàn bộ code mới nằm trong `GO/` ở gốc repo; không sửa/xoá file Python hiện có (`xulydonhang.py`, `App.py`, ...).
- Backend: Go 1.26.5 (toolchain đã cài sẵn), Wails v2.13.0 — **không** dùng Wails v3.
- Frontend: React + TypeScript qua template `react-ts` của Wails, Tailwind CSS, state qua Zustand, icon **chỉ** dùng `react-icons/fa6` (Font Awesome) — không dùng icon emoji trong UI (emoji chỉ còn trong chuỗi `Status` do backend mock trả về, dùng để phân loại màu/icon, không hiển thị trực tiếp).
- Design tokens cố định (đã chốt với người dùng — theme "Trạm điều hành", nền tối):
  - `bg` = `#0B0F14`, `panel` = `#131A22`, `border` = `#232E3A`
  - `ink` (chữ chính) = `#E8EDF2`, `muted` (chữ phụ) = `#8A97A6`
  - `accent` = `#3DD9FF`, `success` = `#3ED598`, `warning` = `#FFC24B`, `danger` = `#FF5D5D`
  - Font UI/tiêu đề: **Be Vietnam Pro** (self-host qua `@fontsource/be-vietnam-pro`)
  - Font dữ liệu/log/bảng: **JetBrains Mono** (self-host qua `@fontsource/jetbrains-mono`)
  - Fonts self-host bắt buộc (không dùng Google Fonts CDN) — app desktop không được phụ thuộc mạng để hiển thị chữ.
- Mặc định `user-select: none` toàn UI chrome; riêng nội dung `LogPanel` và ô dữ liệu `ResultTable`/`FileListPanel` dùng class `.selectable` để cho phép bôi đen/copy.
- Không viết test tự động cho frontend ở Phase 1 (đã chốt trong spec, YAGNI) — các task frontend dùng bước "xác minh thủ công" cụ thể thay cho test tự động.
- Backend: mọi package logic thuần (`internal/config`, `internal/fileset`, `internal/processing`, hàm `runBatch` trong `app.go`) đều phải có unit test theo TDD; các lệnh gọi trực tiếp Wails runtime (dialog, EventsEmit thật) không unit-test được nên chỉ xác minh thủ công qua `wails dev`.

---

### Task 1: Scaffold dự án Wails v2 (template react-ts)

**Files:**
- Create: toàn bộ cây thư mục `GO/` (sinh ra bởi `wails init`)

**Interfaces:**
- Produces: dự án Wails chạy được ở trạng thái mặc định (chưa có logic riêng), làm nền cho các task sau.

- [ ] **Step 1: Scaffold dự án**

Chạy từ thư mục gốc repo (`c:/Users/Admin/Desktop/code py/Xử lý đơn hàng`):

```bash
wails init -n order-processor -t react-ts -d GO
```

Lệnh này tạo thư mục `GO/` chứa `main.go`, `app.go` mẫu (có sẵn hàm `Greet`), `wails.json`, `go.mod` (module `order-processor`), và `GO/frontend/` (Vite + React + TypeScript).

- [ ] **Step 2: Cài dependency frontend mặc định**

```bash
cd "GO/frontend" && npm install
```

- [ ] **Step 3: Build thử để xác nhận toolchain hoạt động**

```bash
cd GO && wails build
```

Expected: build thành công, sinh ra `GO/build/bin/order-processor.exe` không lỗi.

- [ ] **Step 4: Xác nhận `wails dev` mở được cửa sổ**

```bash
cd GO && wails dev
```

Expected: cửa sổ desktop mở lên hiển thị giao diện mặc định của template (logo Wails + nút Greet). Đóng cửa sổ / Ctrl+C để dừng sau khi xác nhận.

- [ ] **Step 5: Commit**

```bash
git add GO/
git commit -m "chore(go): scaffold Wails v2 react-ts project skeleton"
```

---

### Task 2: `internal/config` — đọc/ghi STT

**Files:**
- Create: `GO/internal/config/config.go`
- Test: `GO/internal/config/config_test.go`

**Interfaces:**
- Produces: `config.NewStore(path string) *config.Store`, `(*Store).GetSTT() (int, error)`, `(*Store).SetSTT(v int) error`. `GetSTT` trả về `1` nếu file chưa tồn tại (giống hành vi `xulydonhang.lay_gia_tri_G1() + 1` khi chưa có dữ liệu).

- [ ] **Step 1: Viết test thất bại**

Tạo `GO/internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetSTT_MissingFileReturnsDefaultOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.txt")
	store := NewStore(path)

	got, err := store.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if got != 1 {
		t.Fatalf("GetSTT = %d, want 1", got)
	}
}

func TestSetSTTThenGetSTT_RoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.txt")
	store := NewStore(path)

	if err := store.SetSTT(108); err != nil {
		t.Fatalf("SetSTT returned error: %v", err)
	}

	got, err := store.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if got != 108 {
		t.Fatalf("GetSTT = %d, want 108", got)
	}
}

func TestGetSTT_InvalidValueReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.txt")
	if err := os.WriteFile(path, []byte("current_row=abc\n"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	store := NewStore(path)

	if _, err := store.GetSTT(); err == nil {
		t.Fatal("GetSTT expected error for invalid value, got nil")
	}
}
```

- [ ] **Step 2: Chạy test, xác nhận thất bại**

```bash
cd GO && go test ./internal/config/... -v
```

Expected: FAIL — `NewStore`/`Store` chưa tồn tại (lỗi compile).

- [ ] **Step 3: Viết implementation tối thiểu**

Tạo `GO/internal/config/config.go`:

```go
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const sttKey = "current_row"

// Store đọc/ghi số thứ tự đơn hàng (STT) từ một file dạng key=value,
// tương thích với config.txt (`current_row=N`) của bản Python hiện tại.
type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) GetSTT() (int, error) {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != sttKey {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("config: invalid %s value %q: %w", sttKey, value, err)
		}
		return v, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 1, nil
}

func (s *Store) SetSTT(v int) error {
	content := fmt.Sprintf("%s=%d\n", sttKey, v)
	return os.WriteFile(s.path, []byte(content), 0o644)
}
```

- [ ] **Step 4: Chạy test, xác nhận qua**

```bash
cd GO && go test ./internal/config/... -v
```

Expected: PASS cả 3 test.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/config/
git commit -m "feat(go): add config store for order STT persistence"
```

---

### Task 3: `internal/fileset` — quét thư mục & lọc file hợp lệ

**Files:**
- Create: `GO/internal/fileset/fileset.go`
- Test: `GO/internal/fileset/fileset_test.go`

**Interfaces:**
- Produces: `fileset.IsAllowed(path string) bool`, `fileset.FilterValid(paths []string) []string`, `fileset.EnsureMonthlyFolder(baseDir string, now time.Time) (string, error)`, `fileset.ListFiles(dir string) ([]string, error)`.

- [ ] **Step 1: Viết test thất bại**

Tạo `GO/internal/fileset/fileset_test.go`:

```go
package fileset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilterValid_KeepsOnlyAllowedExtensions(t *testing.T) {
	input := []string{"a.pdf", "b.xlsx", "c.txt", "d.docx", "e.PDF"}
	got := FilterValid(input)
	want := []string{"a.pdf", "b.xlsx", "c.txt", "e.PDF"}

	if len(got) != len(want) {
		t.Fatalf("FilterValid(%v) = %v, want %v", input, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FilterValid(%v) = %v, want %v", input, got, want)
		}
	}
}

func TestEnsureMonthlyFolder_CreatesBaseAndMonthlyDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "đơn hàng")
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)

	got, err := EnsureMonthlyFolder(base, now)
	if err != nil {
		t.Fatalf("EnsureMonthlyFolder returned error: %v", err)
	}

	wantSuffix := filepath.Join("đơn hàng", "08-2026")
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("EnsureMonthlyFolder = %q, want suffix %q", got, wantSuffix)
	}
	if info, err := os.Stat(got); err != nil || !info.IsDir() {
		t.Fatalf("expected %q to be a directory, stat err: %v", got, err)
	}
}

func TestListFiles_ReturnsOnlyAllowedFilesNotDirs(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "order1.pdf"))
	mustWrite(t, filepath.Join(dir, "order2.xlsx"))
	mustWrite(t, filepath.Join(dir, "notes.docx"))
	if err := os.Mkdir(filepath.Join(dir, "08-2026"), 0o755); err != nil {
		t.Fatalf("setup mkdir failed: %v", err)
	}

	got, err := ListFiles(dir)
	if err != nil {
		t.Fatalf("ListFiles returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListFiles returned %d files, want 2: %v", len(got), got)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("failed writing %q: %v", path, err)
	}
}
```

- [ ] **Step 2: Chạy test, xác nhận thất bại**

```bash
cd GO && go test ./internal/fileset/... -v
```

Expected: FAIL — package chưa có implementation (lỗi compile).

- [ ] **Step 3: Viết implementation tối thiểu**

Tạo `GO/internal/fileset/fileset.go`:

```go
package fileset

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

var allowedExtensions = map[string]bool{
	".pdf":  true,
	".xlsx": true,
	".txt":  true,
}

// IsAllowed báo file có đuôi được phép xử lý (.pdf, .xlsx, .txt) hay không.
func IsAllowed(path string) bool {
	return allowedExtensions[strings.ToLower(filepath.Ext(path))]
}

// FilterValid giữ lại các đường dẫn có đuôi hợp lệ, giữ nguyên thứ tự.
func FilterValid(paths []string) []string {
	valid := make([]string, 0, len(paths))
	for _, p := range paths {
		if IsAllowed(p) {
			valid = append(valid, p)
		}
	}
	return valid
}

// EnsureMonthlyFolder đảm bảo baseDir và baseDir/MM-YYYY (theo `now`) tồn
// tại, tự tạo nếu thiếu, trả về đường dẫn tuyệt đối tới thư mục tháng-năm.
func EnsureMonthlyFolder(baseDir string, now time.Time) (string, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}
	monthly := filepath.Join(baseDir, now.Format("01-2006"))
	if err := os.MkdirAll(monthly, 0o755); err != nil {
		return "", err
	}
	return filepath.Abs(monthly)
}

// ListFiles trả về đường dẫn tuyệt đối các file (không gồm thư mục con)
// nằm trực tiếp trong dir có đuôi hợp lệ.
func ListFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !IsAllowed(entry.Name()) {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, abs)
	}
	return files, nil
}
```

- [ ] **Step 4: Chạy test, xác nhận qua**

```bash
cd GO && go test ./internal/fileset/... -v
```

Expected: PASS cả 3 test.

- [ ] **Step 5: Commit**

```bash
git add GO/internal/fileset/
git commit -m "feat(go): add fileset scanning and extension filtering"
```

---

### Task 4: `internal/processing` — `OrderRow`, `Processor`, `MockProcessor`

**Files:**
- Create: `GO/internal/processing/types.go`
- Create: `GO/internal/processing/processor.go`
- Test: `GO/internal/processing/processor_test.go`

**Interfaces:**
- Produces: `processing.OrderRow{FileName, Page, System, MaKhachHang, PO, DonGia, Status string}` (JSON tags: `fileName,page,system,maKhachHang,po,donGia,status`), hằng số `processing.StatusDone/StatusWarning/StatusFailed`, interface `processing.Processor{ Process(ctx, filePath string, stt int) (OrderRow, error) }`, `processing.NewMockProcessor() *MockProcessor`.

- [ ] **Step 1: Viết `types.go`**

```go
package processing

// OrderRow là một dòng trong bảng kết quả, ánh xạ đúng các cột của bảng
// gốc: Tên file, Trang, Hệ thống, Mã khách hàng, PO, Đơn giá, Trạng thái.
type OrderRow struct {
	FileName    string `json:"fileName"`
	Page        string `json:"page"`
	System      string `json:"system"`
	MaKhachHang string `json:"maKhachHang"`
	PO          string `json:"po"`
	DonGia      string `json:"donGia"`
	Status      string `json:"status"`
}

// Các giá trị Status giữ nguyên ký hiệu (emoji) của bản gốc để frontend
// phân loại màu/icon theo đúng ngữ nghĩa cũ.
const (
	StatusDone    = "✅ Hoàn Thành"
	StatusWarning = "⚠️ Hoàn Thành"
	StatusFailed  = "❌ Thất bại"
)
```

- [ ] **Step 2: Viết test thất bại cho `MockProcessor`**

Tạo `GO/internal/processing/processor_test.go`:

```go
package processing

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

func TestMockProcessor_ReturnsRowWithKnownVendorAndPO(t *testing.T) {
	p := &MockProcessor{Rand: rand.New(rand.NewSource(1)), Delay: 0}

	row, err := p.Process(context.Background(), "/tmp/order1.pdf", 108)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if row.FileName != "order1.pdf" {
		t.Fatalf("FileName = %q, want %q", row.FileName, "order1.pdf")
	}
	if row.PO != "PO000108" {
		t.Fatalf("PO = %q, want %q", row.PO, "PO000108")
	}

	found := false
	for _, v := range mockVendors {
		if v == row.System {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("System = %q, not in known vendor list", row.System)
	}
}

func TestMockProcessor_ContextCancelledReturnsError(t *testing.T) {
	p := &MockProcessor{Rand: rand.New(rand.NewSource(1)), Delay: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := p.Process(ctx, "/tmp/order1.pdf", 1); err == nil {
		t.Fatal("Process expected error when context is already cancelled, got nil")
	}
}
```

- [ ] **Step 3: Chạy test, xác nhận thất bại**

```bash
cd GO && go test ./internal/processing/... -v
```

Expected: FAIL — `MockProcessor` chưa tồn tại (lỗi compile).

- [ ] **Step 4: Viết implementation tối thiểu**

Tạo `GO/internal/processing/processor.go`:

```go
package processing

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"time"
)

// Processor biến một file đầu vào thành một OrderRow. Phase 1 chỉ có
// MockProcessor; phase sau sẽ thêm RealProcessor implement cùng interface
// này để parse PDF thật theo từng vendor — App.ProcessFiles và frontend
// không cần đổi khi đó xảy ra.
type Processor interface {
	Process(ctx context.Context, filePath string, stt int) (OrderRow, error)
}

var mockVendors = []string{
	"Coop", "BigC", "Lotte", "Satra", "Emart", "Kingfood", "Winmart",
	"Fujimart", "BHX", "Farmer", "CN-HCM", "MR.DIY", "JIT", "JV-Mart", "JMART", "BC MART",
}

var mockStatuses = []string{StatusDone, StatusDone, StatusDone, StatusWarning, StatusFailed}

// MockProcessor giả lập xử lý đơn hàng: delay ngắn + dữ liệu mẫu ngẫu
// nhiên, để dựng và xác minh pipeline UI/event trước khi có parser PDF
// thật.
type MockProcessor struct {
	Rand  *rand.Rand
	Delay time.Duration
}

func NewMockProcessor() *MockProcessor {
	return &MockProcessor{
		Rand:  rand.New(rand.NewSource(time.Now().UnixNano())),
		Delay: 800 * time.Millisecond,
	}
}

func (m *MockProcessor) Process(ctx context.Context, filePath string, stt int) (OrderRow, error) {
	select {
	case <-time.After(m.Delay):
	case <-ctx.Done():
		return OrderRow{}, ctx.Err()
	}

	system := mockVendors[m.Rand.Intn(len(mockVendors))]
	status := mockStatuses[m.Rand.Intn(len(mockStatuses))]

	return OrderRow{
		FileName:    filepath.Base(filePath),
		Page:        "1",
		System:      system,
		MaKhachHang: fmt.Sprintf("MN_KH%04d", m.Rand.Intn(9999)),
		PO:          fmt.Sprintf("PO%06d", stt),
		DonGia:      fmt.Sprintf("%d", 10000+m.Rand.Intn(90000)),
		Status:      status,
	}, nil
}
```

- [ ] **Step 5: Chạy test, xác nhận qua**

```bash
cd GO && go test ./internal/processing/... -v
```

Expected: PASS cả 2 test.

- [ ] **Step 6: Commit**

```bash
git add GO/internal/processing/
git commit -m "feat(go): add OrderRow type, Processor interface, MockProcessor"
```

---

### Task 5: `App` struct — nối backend, sự kiện, drag-and-drop

**Files:**
- Modify: `GO/app.go` (thay nội dung mẫu bằng implementation thật)
- Modify: `GO/main.go` (thay nội dung mẫu: bind App thật, bật DragAndDrop)
- Test: `GO/app_test.go`

**Interfaces:**
- Consumes: `config.NewStore`, `config.Store{GetSTT,SetSTT}`, `fileset.{FilterValid,EnsureMonthlyFolder,ListFiles}`, `processing.{Processor,NewMockProcessor,OrderRow}`.
- Produces: `App.SelectFiles() ([]string, error)`, `App.ScanOrderFolder() ([]string, error)`, `App.GetSTT() (int, error)`, `App.SetSTT(v int) error`, `App.ProcessFiles(files []string, stt int)`. Sự kiện phát ra: `process:log` (string), `process:row` (`processing.OrderRow`), `process:done` (không data), `files:dropped` (`[]string`).

- [ ] **Step 1: Viết test thất bại cho `runBatch`**

Tạo `GO/app_test.go`:

```go
package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"order-processor/internal/config"
	"order-processor/internal/processing"
)

type fakeEmitter struct {
	events []emittedEvent
}

type emittedEvent struct {
	name string
	data []interface{}
}

func (f *fakeEmitter) Emit(name string, data ...interface{}) {
	f.events = append(f.events, emittedEvent{name: name, data: data})
}

type stubProcessor struct {
	failOn string
}

func (s *stubProcessor) Process(ctx context.Context, filePath string, stt int) (processing.OrderRow, error) {
	if filePath == s.failOn {
		return processing.OrderRow{}, errors.New("stub failure")
	}
	return processing.OrderRow{FileName: filePath, PO: "PO1", Status: processing.StatusDone}, nil
}

func TestRunBatch_EmitsLogRowPerFileThenDone(t *testing.T) {
	cfg := config.NewStore(filepath.Join(t.TempDir(), "config.txt"))
	a := &App{cfg: cfg, processor: &stubProcessor{}}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"a.pdf", "b.pdf"}, 10)

	wantNames := []string{"process:log", "process:row", "process:log", "process:row", "process:done"}
	if len(emitter.events) != len(wantNames) {
		t.Fatalf("got %d events, want %d: %+v", len(emitter.events), len(wantNames), emitter.events)
	}
	for i, want := range wantNames {
		if emitter.events[i].name != want {
			t.Fatalf("event[%d] = %q, want %q", i, emitter.events[i].name, want)
		}
	}

	gotSTT, err := cfg.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if gotSTT != 12 {
		t.Fatalf("STT after batch = %d, want 12", gotSTT)
	}
}

func TestRunBatch_FileErrorEmitsLogAndContinues(t *testing.T) {
	cfg := config.NewStore(filepath.Join(t.TempDir(), "config.txt"))
	a := &App{cfg: cfg, processor: &stubProcessor{failOn: "bad.pdf"}}
	emitter := &fakeEmitter{}

	a.runBatch(emitter, []string{"bad.pdf", "good.pdf"}, 1)

	wantNames := []string{"process:log", "process:log", "process:log", "process:row", "process:done"}
	if len(emitter.events) != len(wantNames) {
		t.Fatalf("got %d events, want %d: %+v", len(emitter.events), len(wantNames), emitter.events)
	}

	gotSTT, err := cfg.GetSTT()
	if err != nil {
		t.Fatalf("GetSTT returned error: %v", err)
	}
	if gotSTT != 3 {
		t.Fatalf("STT after batch = %d, want 3", gotSTT)
	}
}
```

- [ ] **Step 2: Chạy test, xác nhận thất bại**

```bash
cd GO && go test . -run TestRunBatch -v
```

Expected: FAIL — `App`, `runBatch` chưa có field/hàm đúng như test cần (lỗi compile).

- [ ] **Step 3: Viết implementation `app.go`**

Thay toàn bộ nội dung `GO/app.go`:

```go
package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"order-processor/internal/config"
	"order-processor/internal/fileset"
	"order-processor/internal/processing"
)

// Emitter trừu tượng hoá runtime.EventsEmit để logic của App test được mà
// không cần một Wails context thật.
type Emitter interface {
	Emit(eventName string, data ...interface{})
}

type wailsEmitter struct {
	ctx context.Context
}

func (e *wailsEmitter) Emit(eventName string, data ...interface{}) {
	runtime.EventsEmit(e.ctx, eventName, data...)
}

const orderFolderName = "đơn hàng"
const configFileName = "config.txt"

// App struct
type App struct {
	ctx       context.Context
	cfg       *config.Store
	processor processing.Processor
	emitter   Emitter
	orderDir  string
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		cfg:       config.NewStore(configFileName),
		processor: processing.NewMockProcessor(),
		orderDir:  orderFolderName,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emitter = &wailsEmitter{ctx: ctx}

	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		valid := fileset.FilterValid(paths)
		if len(valid) > 0 {
			a.emitter.Emit("files:dropped", valid)
		}
	})
}

// GetSTT trả về số thứ tự đơn hàng bắt đầu hiện tại.
func (a *App) GetSTT() (int, error) {
	return a.cfg.GetSTT()
}

// SetSTT ghi lại số thứ tự đơn hàng bắt đầu.
func (a *App) SetSTT(v int) error {
	return a.cfg.SetSTT(v)
}

// ScanOrderFolder quét thư mục "đơn hàng/MM-YYYY" hiện tại (tự tạo nếu
// thiếu) và trả về danh sách file hợp lệ.
func (a *App) ScanOrderFolder() ([]string, error) {
	dir, err := fileset.EnsureMonthlyFolder(a.orderDir, time.Now())
	if err != nil {
		return nil, err
	}
	return fileset.ListFiles(dir)
}

// SelectFiles mở dialog chọn nhiều file, lọc theo đuôi hợp lệ.
func (a *App) SelectFiles() ([]string, error) {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn file đơn hàng",
		Filters: []runtime.FileFilter{
			{DisplayName: "Đơn hàng (*.pdf;*.xlsx;*.txt)", Pattern: "*.pdf;*.xlsx;*.txt"},
		},
	})
	if err != nil {
		return nil, err
	}
	return fileset.FilterValid(paths), nil
}

// ProcessFiles chạy xử lý các file đã chọn trong nền, phát sự kiện
// process:log / process:row / process:done về frontend.
func (a *App) ProcessFiles(files []string, stt int) {
	go a.runBatch(a.emitter, files, stt)
}

func (a *App) runBatch(emitter Emitter, files []string, stt int) {
	defer func() {
		if r := recover(); r != nil {
			emitter.Emit("process:log", fmt.Sprintf("❌ Lỗi không mong muốn: %v", r))
		}
		emitter.Emit("process:done")
	}()

	current := stt
	for _, f := range files {
		emitter.Emit("process:log", fmt.Sprintf("Đang xử lý %s...", filepath.Base(f)))
		row, err := a.processOne(f, current)
		if err != nil {
			emitter.Emit("process:log", fmt.Sprintf("❌ Lỗi xử lý %s: %v", filepath.Base(f), err))
			current++
			continue
		}
		emitter.Emit("process:row", row)
		current++
	}
	_ = a.cfg.SetSTT(current)
}

func (a *App) processOne(f string, stt int) (row processing.OrderRow, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return a.processor.Process(context.Background(), f, stt)
}
```

- [ ] **Step 4: Chạy test, xác nhận qua**

```bash
cd GO && go test . -run TestRunBatch -v
```

Expected: PASS cả 2 test.

- [ ] **Step 5: Cập nhật `main.go`**

Thay toàn bộ nội dung `GO/main.go`:

```go
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Blue Hà Thành - Order System v3.0",
		Width:     1100,
		Height:    850,
		MinWidth:  900,
		MinHeight: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
```

- [ ] **Step 6: Sinh lại Wails bindings cho frontend**

```bash
cd GO && wails dev
```

Đợi vài giây cho cửa sổ mở lên (xác nhận không lỗi build Go), sau đó đóng cửa sổ / Ctrl+C. Kiểm tra:

```bash
cat "GO/frontend/wailsjs/go/main/App.d.ts"
```

Expected: file này liệt kê đủ `SelectFiles`, `ScanOrderFolder`, `GetSTT`, `SetSTT`, `ProcessFiles` (không còn `Greet` mẫu).

- [ ] **Step 7: Commit**

```bash
git add GO/app.go GO/main.go GO/app_test.go GO/frontend/wailsjs
git commit -m "feat(go): wire App methods, batch processing events, and file drop"
```

---

### Task 6: Design system, app shell, tab Thông tin

**Files:**
- Modify: `GO/frontend/package.json` (thêm dependency)
- Create: `GO/frontend/tailwind.config.js`
- Create: `GO/frontend/postcss.config.js`
- Modify: `GO/frontend/src/index.css` (hoặc `App.css` tuỳ template — thay bằng nội dung dưới)
- Create: `GO/frontend/src/lib/desktopFeel.ts`
- Modify: `GO/frontend/src/main.tsx`
- Modify: `GO/frontend/src/App.tsx`
- Create: `GO/frontend/src/components/InfoTab.tsx`
- Create: `GO/frontend/src/components/ProcessTab.tsx`
- Create: `GO/frontend/public/qr.jpg` (copy từ file gốc ở repo root)
- Modify: `GO/build/windows/icon.ico` (copy từ `blue.ico` ở repo root, dùng làm icon file .exe)

**Interfaces:**
- Produces: class Tailwind `bg-bg/bg-panel/border-border/text-ink/text-muted/text-accent/bg-success/bg-warning/bg-danger`, `font-sans`/`font-mono`; class tiện ích `.selectable`; hàm `installDesktopFeel()`; component `<ProcessTab />`, `<InfoTab />` dùng trong `App.tsx`.

- [ ] **Step 1: Cài dependency**

```bash
cd "GO/frontend" && npm install -D tailwindcss@3 postcss autoprefixer
npm install @fontsource/be-vietnam-pro @fontsource/jetbrains-mono zustand react-icons
```

- [ ] **Step 2: Khởi tạo Tailwind config**

Tạo `GO/frontend/postcss.config.js`:

```js
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
}
```

Tạo `GO/frontend/tailwind.config.js`:

```js
/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: '#0B0F14',
        panel: '#131A22',
        border: '#232E3A',
        ink: '#E8EDF2',
        muted: '#8A97A6',
        accent: '#3DD9FF',
        success: '#3ED598',
        warning: '#FFC24B',
        danger: '#FF5D5D',
      },
      fontFamily: {
        sans: ['"Be Vietnam Pro"', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'monospace'],
      },
    },
  },
  plugins: [],
}
```

- [ ] **Step 3: Thay `src/index.css`**

Thay toàn bộ nội dung `GO/frontend/src/index.css` (nếu template dùng tên khác, ví dụ `src/style.css`, đổi tên file cho khớp và cập nhật import trong `main.tsx` ở Step 5):

```css
@import '@fontsource/be-vietnam-pro/400.css';
@import '@fontsource/be-vietnam-pro/500.css';
@import '@fontsource/be-vietnam-pro/700.css';
@import '@fontsource/jetbrains-mono/400.css';
@import '@fontsource/jetbrains-mono/600.css';

@tailwind base;
@tailwind components;
@tailwind utilities;

html, body, #root {
  height: 100%;
  margin: 0;
}

body {
  @apply bg-bg text-ink font-sans;
  user-select: none;
  -webkit-user-select: none;
}

.selectable {
  user-select: text;
  -webkit-user-select: text;
}

img, .no-drag {
  -webkit-user-drag: none;
  user-drag: none;
}
```

- [ ] **Step 4: Tạo `src/lib/desktopFeel.ts`**

```ts
// Vô hiệu hoá các hành vi "trang web" của webview (menu chuột phải mặc
// định, zoom bằng Ctrl+cuộn/Ctrl +/-) để ứng dụng cảm giác như desktop app.
export function installDesktopFeel(): void {
  window.addEventListener('contextmenu', (e) => {
    e.preventDefault()
  })

  window.addEventListener(
    'wheel',
    (e) => {
      if (e.ctrlKey) {
        e.preventDefault()
      }
    },
    { passive: false }
  )

  window.addEventListener('keydown', (e) => {
    const isZoomKey = e.key === '+' || e.key === '-' || e.key === '=' || e.key === '0'
    if ((e.ctrlKey || e.metaKey) && isZoomKey) {
      e.preventDefault()
    }
  })
}
```

- [ ] **Step 5: Cập nhật `src/main.tsx`**

```tsx
import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'
import { installDesktopFeel } from './lib/desktopFeel'

installDesktopFeel()

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)
```

- [ ] **Step 6: Tạo `src/components/InfoTab.tsx`**

```tsx
export function InfoTab() {
  return (
    <div className="mx-auto flex h-full max-w-2xl flex-col items-center justify-center gap-6 overflow-auto text-center">
      <div className="rounded-2xl border border-border bg-panel p-8">
        <h2 className="font-mono text-2xl font-semibold text-accent">
          AUTOMATED ORDER PROCESSING SYSTEM
        </h2>
        <div className="mt-4 space-y-2 text-left text-sm text-ink">
          <p className="font-medium text-muted">Chức năng chính:</p>
          <ul className="list-disc space-y-1 pl-5">
            <li>Phân tích đơn hàng PDF/XLSX/TXT từ hệ thống MT (BigC, Lotte, Satra...)</li>
            <li>Tự động đối soát giá bán và chương trình khuyến mãi.</li>
            <li>Xuất dữ liệu chuẩn hóa phục vụ kế toán.</li>
          </ul>
          <p className="pt-2">
            <span className="text-muted">Tác giả:</span> HUYNH DAT THANH
          </p>
          <p>
            <span className="text-muted">Liên hệ:</span> 0947.940.391 · byun.huynh@gmail.com
          </p>
        </div>
        <img
          src="/qr.jpg"
          alt="QR liên hệ"
          className="no-drag mx-auto mt-6 h-40 w-40 rounded-lg border border-border object-cover"
          draggable={false}
        />
      </div>
    </div>
  )
}
```

- [ ] **Step 7: Tạo `src/components/ProcessTab.tsx` (placeholder, sẽ thay dần ở Task 8-11)**

```tsx
function Placeholder({ label }: { label: string }) {
  return (
    <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-border text-sm text-muted">
      {label}
    </div>
  )
}

export function ProcessTab() {
  return (
    <div className="grid h-full grid-rows-[minmax(0,2fr)_minmax(0,3fr)] gap-4">
      <div className="grid grid-cols-[3fr_1fr] gap-4 overflow-hidden">
        <Placeholder label="FileListPanel (Task 8)" />
        <Placeholder label="ControlPanel (Task 11)" />
      </div>
      <div className="grid grid-rows-[1fr_1fr] gap-4 overflow-hidden">
        <Placeholder label="LogPanel (Task 9)" />
        <Placeholder label="ResultTable (Task 10)" />
      </div>
    </div>
  )
}
```

- [ ] **Step 8: Thay `src/App.tsx`**

```tsx
import { useState } from 'react'
import { FaGears, FaCircleInfo } from 'react-icons/fa6'
import { ProcessTab } from './components/ProcessTab'
import { InfoTab } from './components/InfoTab'

type TabKey = 'process' | 'info'

function App() {
  const [tab, setTab] = useState<TabKey>('process')

  return (
    <div className="flex h-screen flex-col">
      <header className="flex items-center gap-1 border-b border-border bg-panel px-4 pt-3">
        <TabButton active={tab === 'process'} onClick={() => setTab('process')}>
          <FaGears /> Xử lý Đơn hàng
        </TabButton>
        <TabButton active={tab === 'info'} onClick={() => setTab('info')}>
          <FaCircleInfo /> Thông tin
        </TabButton>
      </header>
      <main className="flex-1 overflow-hidden p-4">
        {tab === 'process' ? <ProcessTab /> : <InfoTab />}
      </main>
      <footer className="border-t border-border px-4 py-2 text-center text-xs text-muted">
        © 2026 Blue Hà Thành. All rights reserved.
      </footer>
    </div>
  )
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      onClick={onClick}
      className={`inline-flex items-center gap-2 rounded-t-lg px-4 py-2 text-sm font-medium transition-colors ${
        active ? 'bg-bg text-accent' : 'text-muted hover:text-ink'
      }`}
    >
      {children}
    </button>
  )
}

export default App
```

- [ ] **Step 9: Copy asset ảnh QR và icon ứng dụng**

```bash
cp "qr.jpg" "GO/frontend/public/qr.jpg"
cp "blue.ico" "GO/build/windows/icon.ico"
```

(`GO/build/windows/icon.ico` điều khiển icon của file .exe khi `wails build` trên Windows — nền tảng duy nhất app này chạy theo spec.)

- [ ] **Step 10: Xác minh thủ công**

```bash
cd GO && wails dev
```

Kiểm tra: cửa sổ mở lên nền tối (#0B0F14), 2 tab "Xử lý Đơn hàng"/"Thông tin" có icon Font Awesome, tab "Thông tin" hiển thị đúng nội dung + ảnh QR, tab "Xử lý Đơn hàng" hiển thị 4 ô placeholder viền nét đứt. Bôi đen thử một đoạn text ở vùng tiêu đề/nút — không chọn được (do `user-select: none`). Đóng cửa sổ / Ctrl+C.

- [ ] **Step 11: Commit**

```bash
git add GO/frontend GO/build
git commit -m "feat(frontend): design system, desktop feel, app shell, info tab"
```

---

### Task 7: Zustand store, types, `useWailsEvents` hook

**Files:**
- Create: `GO/frontend/src/types.ts`
- Create: `GO/frontend/src/store/appStore.ts`
- Create: `GO/frontend/src/hooks/useWailsEvents.ts`
- Modify: `GO/frontend/src/App.tsx` (gọi `useWailsEvents()`)

**Interfaces:**
- Consumes: sự kiện Wails `process:log`, `process:row`, `process:done`, `files:dropped` (Task 5); binding `EventsOn` từ `wailsjs/runtime/runtime`.
- Produces: hook `useAppStore` (Zustand) với state `{files, stt, isProcessing, logLines, rows}` và action `{setFiles, addFiles, removeFiles, setStt, setProcessing, appendLog, clearLog, appendRow, resetRows}`; hook `useWailsEvents()` — các component ở Task 8-11 dùng `useAppStore` để đọc/ghi state.

- [ ] **Step 1: Tạo `src/types.ts`**

```ts
export interface OrderRow {
  fileName: string
  page: string
  system: string
  maKhachHang: string
  po: string
  donGia: string
  status: string
}
```

- [ ] **Step 2: Tạo `src/store/appStore.ts`**

```ts
import { create } from 'zustand'
import type { OrderRow } from '../types'

interface AppState {
  files: string[]
  stt: number
  isProcessing: boolean
  logLines: string[]
  rows: OrderRow[]
  setFiles: (files: string[]) => void
  addFiles: (files: string[]) => void
  removeFiles: (files: string[]) => void
  setStt: (stt: number) => void
  setProcessing: (processing: boolean) => void
  appendLog: (line: string) => void
  clearLog: () => void
  appendRow: (row: OrderRow) => void
  resetRows: () => void
}

export const useAppStore = create<AppState>((set) => ({
  files: [],
  stt: 1,
  isProcessing: false,
  logLines: [],
  rows: [],
  setFiles: (files) => set({ files }),
  addFiles: (newFiles) =>
    set((state) => ({
      files: Array.from(new Set([...state.files, ...newFiles])),
    })),
  removeFiles: (toRemove) =>
    set((state) => ({
      files: state.files.filter((f) => !toRemove.includes(f)),
    })),
  setStt: (stt) => set({ stt }),
  setProcessing: (isProcessing) => set({ isProcessing }),
  appendLog: (line) => set((state) => ({ logLines: [...state.logLines, line] })),
  clearLog: () => set({ logLines: [] }),
  appendRow: (row) => set((state) => ({ rows: [...state.rows, row] })),
  resetRows: () => set({ rows: [] }),
}))
```

- [ ] **Step 3: Tạo `src/hooks/useWailsEvents.ts`**

```ts
import { useEffect } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../store/appStore'
import type { OrderRow } from '../types'

export function useWailsEvents() {
  const appendLog = useAppStore((s) => s.appendLog)
  const appendRow = useAppStore((s) => s.appendRow)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const addFiles = useAppStore((s) => s.addFiles)

  useEffect(() => {
    const offLog = EventsOn('process:log', (line: string) => appendLog(line))
    const offRow = EventsOn('process:row', (row: OrderRow) => appendRow(row))
    const offDone = EventsOn('process:done', () => setProcessing(false))
    const offDrop = EventsOn('files:dropped', (paths: string[]) => addFiles(paths))

    return () => {
      offLog()
      offRow()
      offDone()
      offDrop()
    }
  }, [appendLog, appendRow, setProcessing, addFiles])
}
```

- [ ] **Step 4: Gọi hook trong `App.tsx`**

Trong `GO/frontend/src/App.tsx`, thêm import và gọi hook đầu hàm `App`:

```tsx
import { useWailsEvents } from './hooks/useWailsEvents'
```

```tsx
function App() {
  useWailsEvents()
  const [tab, setTab] = useState<TabKey>('process')
  // ... phần còn lại giữ nguyên
```

- [ ] **Step 5: Xác minh thủ công**

```bash
cd GO && wails dev
```

Mở DevTools (chỉ khả dụng khi chạy `wails dev`, không có ở bản build production) và xác nhận không có lỗi JS nào liên quan tới `EventsOn` hoặc import `wailsjs/runtime` trong console. Đóng cửa sổ / Ctrl+C.

- [ ] **Step 6: Commit**

```bash
git add GO/frontend/src/types.ts GO/frontend/src/store GO/frontend/src/hooks GO/frontend/src/App.tsx
git commit -m "feat(frontend): add Zustand store and useWailsEvents hook"
```

---

### Task 8: `FileListPanel` — danh sách file, chọn/kéo-thả, xoá

**Files:**
- Create: `GO/frontend/src/components/FileListPanel.tsx`
- Modify: `GO/frontend/src/components/ProcessTab.tsx` (thay placeholder đầu tiên)

**Interfaces:**
- Consumes: `useAppStore` (`files, setFiles, addFiles, removeFiles, appendLog, isProcessing`), Wails binding `SelectFiles()`, `ScanOrderFolder()` từ `wailsjs/go/main/App`.
- Produces: `<FileListPanel />`.

- [ ] **Step 1: Tạo `src/components/FileListPanel.tsx`**

```tsx
import { useState } from 'react'
import { FaArrowsRotate, FaFolderOpen } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { SelectFiles, ScanOrderFolder } from '../../wailsjs/go/main/App'

export function FileListPanel() {
  const files = useAppStore((s) => s.files)
  const setFiles = useAppStore((s) => s.setFiles)
  const addFiles = useAppStore((s) => s.addFiles)
  const removeFiles = useAppStore((s) => s.removeFiles)
  const appendLog = useAppStore((s) => s.appendLog)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const [selected, setSelected] = useState<Set<string>>(new Set())

  async function reload() {
    try {
      const found = await ScanOrderFolder()
      setFiles(found)
      setSelected(new Set())
      appendLog(`Đã load ${found.length} file từ thư mục đơn hàng.`)
    } catch (err) {
      appendLog(`❌ Lỗi tải thư mục: ${String(err)}`)
    }
  }

  async function pickFiles() {
    try {
      const picked = await SelectFiles()
      if (picked.length === 0) return
      addFiles(picked)
      appendLog(`Đã thêm ${picked.length} file.`)
    } catch (err) {
      appendLog(`❌ Lỗi chọn file: ${String(err)}`)
    }
  }

  function toggleSelect(f: string, e: React.MouseEvent) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (!e.ctrlKey && !e.metaKey) next.clear()
      if (next.has(f)) next.delete(f)
      else next.add(f)
      return next
    })
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Delete' && selected.size > 0) {
      removeFiles([...selected])
      appendLog(`Đã xóa ${selected.size} file khỏi danh sách.`)
      setSelected(new Set())
    }
  }

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3">
      <h2 className="mb-2 text-sm font-semibold text-muted">1. Danh sách file đầu vào</h2>
      <ul
        tabIndex={0}
        onKeyDown={handleKeyDown}
        className="selectable flex-1 overflow-auto rounded-lg border border-border bg-bg font-mono text-xs"
      >
        {files.length === 0 && (
          <li className="p-3 text-muted">
            Chưa có file nào. Kéo-thả file vào cửa sổ hoặc bấm "Chọn file...".
          </li>
        )}
        {files.map((f) => (
          <li
            key={f}
            onClick={(e) => toggleSelect(f, e)}
            className={`cursor-pointer truncate border-b border-border px-3 py-1.5 ${
              selected.has(f) ? 'bg-accent/20 text-accent' : 'text-ink'
            }`}
          >
            {f}
          </li>
        ))}
      </ul>
      <div className="mt-2 flex gap-2">
        <button
          onClick={reload}
          disabled={isProcessing}
          className="inline-flex flex-1 items-center justify-center gap-2 rounded-lg bg-accent/10 px-3 py-2 text-sm font-medium text-accent hover:bg-accent/20 disabled:opacity-40"
        >
          <FaArrowsRotate /> Tải lại đơn hàng
        </button>
        <button
          onClick={pickFiles}
          disabled={isProcessing}
          className="inline-flex flex-1 items-center justify-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium text-ink hover:border-accent disabled:opacity-40"
        >
          <FaFolderOpen /> Chọn file...
        </button>
      </div>
    </section>
  )
}
```

- [ ] **Step 2: Wire vào `ProcessTab.tsx`**

Trong `GO/frontend/src/components/ProcessTab.tsx`, thêm import và thay placeholder đầu tiên:

```tsx
import { FileListPanel } from './FileListPanel'
```

```tsx
<div className="grid grid-cols-[3fr_1fr] gap-4 overflow-hidden">
  <FileListPanel />
  <Placeholder label="ControlPanel (Task 11)" />
</div>
```

- [ ] **Step 3: Xác minh thủ công**

```bash
cd GO && wails dev
```

Kiểm tra: bấm "Tải lại đơn hàng" → thư mục `đơn hàng/MM-YYYY` (theo tháng hiện tại) được tạo dưới `GO/` nếu chưa có, danh sách file hiển thị nếu có sẵn file `.pdf/.xlsx/.txt`. Bấm "Chọn file..." → dialog native mở lên, chọn file bất kỳ → xuất hiện trong danh sách. Kéo-thả một file PDF thật từ Explorer vào cửa sổ → file xuất hiện trong danh sách (qua sự kiện `files:dropped`). Click chọn 1 dòng, nhấn phím Delete → dòng biến mất. Thử bôi đen text trong danh sách file — chọn được (vùng này có class `.selectable`). Đóng cửa sổ / Ctrl+C.

- [ ] **Step 4: Commit**

```bash
git add GO/frontend/src/components/FileListPanel.tsx GO/frontend/src/components/ProcessTab.tsx
git commit -m "feat(frontend): add FileListPanel with select, drag-drop, delete"
```

---

### Task 9: `LogPanel` — nhật ký hệ thống realtime

**Files:**
- Create: `GO/frontend/src/components/LogPanel.tsx`
- Modify: `GO/frontend/src/components/ProcessTab.tsx` (thay placeholder log)

**Interfaces:**
- Consumes: `useAppStore` (`logLines, clearLog`).
- Produces: `<LogPanel />`.

- [ ] **Step 1: Tạo `src/components/LogPanel.tsx`**

```tsx
import { useEffect, useRef } from 'react'
import { FaTrashCan } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'

export function LogPanel() {
  const logLines = useAppStore((s) => s.logLines)
  const clearLog = useAppStore((s) => s.clearLog)
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [logLines])

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-muted">3. Nhật ký hệ thống</h2>
        <button
          onClick={clearLog}
          className="inline-flex items-center gap-1 text-xs text-muted hover:text-accent"
        >
          <FaTrashCan /> Xóa nhật ký
        </button>
      </div>
      <div className="selectable flex-1 overflow-auto rounded-lg border border-border bg-bg p-2 font-mono text-xs text-ink">
        {logLines.map((line, i) => (
          <div key={i} className="whitespace-pre-wrap py-0.5">
            {line}
          </div>
        ))}
        <div ref={bottomRef} />
      </div>
    </section>
  )
}
```

- [ ] **Step 2: Wire vào `ProcessTab.tsx`**

```tsx
import { LogPanel } from './LogPanel'
```

```tsx
<div className="grid grid-rows-[1fr_1fr] gap-4 overflow-hidden">
  <LogPanel />
  <Placeholder label="ResultTable (Task 10)" />
</div>
```

- [ ] **Step 3: Xác minh thủ công**

```bash
cd GO && wails dev
```

Chưa có nút xử lý (đến Task 11), nên xác nhận UI hiển thị đúng khung log rỗng, bấm "Xóa nhật ký" không lỗi (danh sách vẫn rỗng), bôi đen được text trong vùng log nếu có nội dung. Đóng cửa sổ / Ctrl+C.

- [ ] **Step 4: Commit**

```bash
git add GO/frontend/src/components/LogPanel.tsx GO/frontend/src/components/ProcessTab.tsx
git commit -m "feat(frontend): add realtime LogPanel"
```

---

### Task 10: `ResultTable` — bảng kết quả có màu theo trạng thái

**Files:**
- Create: `GO/frontend/src/components/ResultTable.tsx`
- Modify: `GO/frontend/src/components/ProcessTab.tsx` (thay placeholder bảng kết quả)

**Interfaces:**
- Consumes: `useAppStore` (`rows`), `OrderRow` từ `../types`.
- Produces: `<ResultTable />`.

- [ ] **Step 1: Tạo `src/components/ResultTable.tsx`**

```tsx
import { FaCircleCheck, FaCircleXmark, FaTriangleExclamation } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import type { OrderRow } from '../types'

const columns: { key: keyof OrderRow; label: string }[] = [
  { key: 'fileName', label: 'Tên file' },
  { key: 'page', label: 'Trang' },
  { key: 'system', label: 'Hệ thống' },
  { key: 'maKhachHang', label: 'Mã khách hàng' },
  { key: 'po', label: 'PO' },
  { key: 'donGia', label: 'Đơn giá' },
  { key: 'status', label: 'Trạng thái' },
]

function statusMeta(status: string): { icon: JSX.Element | null; classes: string; label: string } {
  if (status.includes('❌')) {
    return { icon: <FaCircleXmark />, classes: 'bg-danger/20 text-danger', label: status.replace('❌', '').trim() }
  }
  if (status.includes('⚠️')) {
    return {
      icon: <FaTriangleExclamation />,
      classes: 'bg-warning/20 text-warning',
      label: status.replace('⚠️', '').trim(),
    }
  }
  if (status.includes('✅')) {
    return { icon: <FaCircleCheck />, classes: 'bg-success/20 text-success', label: status.replace('✅', '').trim() }
  }
  return { icon: null, classes: 'bg-border text-muted', label: status }
}

export function ResultTable() {
  const rows = useAppStore((s) => s.rows)

  return (
    <section className="flex h-full flex-col rounded-xl border border-border bg-panel p-3">
      <h2 className="mb-2 text-sm font-semibold text-muted">4. Kết quả xử lý chi tiết</h2>
      <div className="selectable flex-1 overflow-auto rounded-lg border border-border">
        <table className="w-full border-collapse font-mono text-xs">
          <thead className="sticky top-0 bg-bg">
            <tr>
              {columns.map((c) => (
                <th key={c.key} className="border-b border-border px-2 py-1.5 text-left text-muted">
                  {c.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => {
              const meta = statusMeta(row.status)
              return (
                <tr key={i} className="odd:bg-bg/40">
                  {columns.map((c) => (
                    <td key={c.key} className="border-b border-border px-2 py-1.5 text-ink">
                      {c.key === 'status' ? (
                        <span
                          className={`inline-flex items-center gap-1.5 rounded px-2 py-0.5 font-semibold ${meta.classes}`}
                        >
                          {meta.icon}
                          {meta.label}
                        </span>
                      ) : (
                        row[c.key]
                      )}
                    </td>
                  ))}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </section>
  )
}
```

- [ ] **Step 2: Wire vào `ProcessTab.tsx`**

```tsx
import { ResultTable } from './ResultTable'
```

```tsx
<div className="grid grid-rows-[1fr_1fr] gap-4 overflow-hidden">
  <LogPanel />
  <ResultTable />
</div>
```

- [ ] **Step 3: Xác minh thủ công**

```bash
cd GO && wails dev
```

Xác nhận bảng hiển thị đúng 7 cột tiêu đề, chưa có dòng dữ liệu (vì chưa nối nút xử lý). Đóng cửa sổ / Ctrl+C.

- [ ] **Step 4: Commit**

```bash
git add GO/frontend/src/components/ResultTable.tsx GO/frontend/src/components/ProcessTab.tsx
git commit -m "feat(frontend): add ResultTable with status color coding"
```

---

### Task 11: `ControlPanel` — STT, nút xử lý, nút Zalo/MISA (disabled)

**Files:**
- Create: `GO/frontend/src/components/ControlPanel.tsx`
- Modify: `GO/frontend/src/components/ProcessTab.tsx` (thay placeholder cuối cùng)

**Interfaces:**
- Consumes: `useAppStore` (`stt, setStt, files, isProcessing, setProcessing, appendLog, resetRows`), Wails binding `GetSTT()`, `SetSTT(v)`, `ProcessFiles(files, stt)`.
- Produces: `<ControlPanel />` — sau task này pipeline mock end-to-end hoàn chỉnh.

- [ ] **Step 1: Tạo `src/components/ControlPanel.tsx`**

```tsx
import { useEffect } from 'react'
import { FaPaperPlane, FaCloudArrowUp, FaRocket } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { GetSTT, SetSTT, ProcessFiles } from '../../wailsjs/go/main/App'

export function ControlPanel() {
  const stt = useAppStore((s) => s.stt)
  const setStt = useAppStore((s) => s.setStt)
  const files = useAppStore((s) => s.files)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const appendLog = useAppStore((s) => s.appendLog)
  const resetRows = useAppStore((s) => s.resetRows)

  useEffect(() => {
    GetSTT()
      .then(setStt)
      .catch((err) => appendLog(`❌ Lỗi đọc STT: ${String(err)}`))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function handleSttBlur() {
    try {
      await SetSTT(stt)
    } catch (err) {
      appendLog(`❌ Lỗi ghi STT: ${String(err)}`)
    }
  }

  async function handleProcess() {
    if (files.length === 0) {
      appendLog('Không có file nào để xử lý!')
      return
    }
    resetRows()
    setProcessing(true)
    appendLog('🚀 Bắt đầu xử lý...')
    try {
      await ProcessFiles(files, stt)
    } catch (err) {
      appendLog(`❌ Lỗi xử lý: ${String(err)}`)
      setProcessing(false)
    }
  }

  return (
    <section className="flex h-full flex-col justify-between rounded-xl border border-border bg-panel p-3">
      <div>
        <h2 className="mb-2 text-sm font-semibold text-muted">2. Cấu hình &amp; Thực thi</h2>
        <label className="text-xs text-muted">Số thứ tự đơn hàng bắt đầu</label>
        <input
          type="number"
          value={stt}
          disabled={isProcessing}
          onChange={(e) => setStt(Number(e.target.value))}
          onBlur={handleSttBlur}
          className="selectable mt-1 w-full rounded-lg border border-border bg-bg px-3 py-2 text-center font-mono text-lg text-ink focus:border-accent focus:outline-none disabled:opacity-40"
        />
      </div>
      <div className="mt-4 flex flex-col gap-2">
        <button
          disabled
          title="Sẽ có ở giai đoạn sau"
          className="inline-flex items-center justify-center gap-2 rounded-lg bg-[#0068ff]/20 px-3 py-2 text-sm font-medium text-[#0068ff] opacity-40"
        >
          <FaPaperPlane /> Gửi thông báo Zalo
        </button>
        <button
          disabled
          title="Sẽ có ở giai đoạn sau"
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium text-muted opacity-40"
        >
          <FaCloudArrowUp /> Push MISA
        </button>
        <button
          onClick={handleProcess}
          disabled={isProcessing}
          className="inline-flex items-center justify-center gap-2 rounded-lg bg-success px-3 py-3 text-sm font-bold text-bg hover:brightness-110 disabled:opacity-40"
        >
          <FaRocket /> XỬ LÝ ĐƠN HÀNG
        </button>
      </div>
    </section>
  )
}
```

- [ ] **Step 2: Wire vào `ProcessTab.tsx`**

```tsx
import { ControlPanel } from './ControlPanel'
```

```tsx
<div className="grid grid-cols-[3fr_1fr] gap-4 overflow-hidden">
  <FileListPanel />
  <ControlPanel />
</div>
```

Xoá hàm `Placeholder` khỏi `ProcessTab.tsx` (không còn chỗ nào dùng).

- [ ] **Step 3: Xác minh thủ công — pipeline end-to-end**

```bash
cd GO && wails dev
```

1. Tab "Xử lý Đơn hàng": ô STT tự điền giá trị đọc từ `config.txt` (mặc định `1` nếu chưa có file).
2. Bấm "Chọn file..." chọn 2-3 file PDF/XLSX bất kỳ (nội dung không quan trọng, `MockProcessor` không đọc nội dung file).
3. Bấm "🚀 XỬ LÝ ĐƠN HÀNG": các nút bị khoá (`disabled`), log hiện lần lượt "Đang xử lý ...", bảng kết quả xuất hiện dần từng dòng với badge màu xanh lá/vàng/đỏ + icon tương ứng, sau ~1-2s mỗi file. Khi xong, nút mở khoá lại.
4. Đổi giá trị ô STT, click ra ngoài (blur) → mở `GO/config.txt`, xác nhận `current_row=` đã đổi theo giá trị mới cộng số file vừa xử lý.
5. Nút "Gửi thông báo Zalo" và "Push MISA" hiển thị mờ, không bấm được, hover hiện tooltip "Sẽ có ở giai đoạn sau".

Đóng cửa sổ / Ctrl+C.

- [ ] **Step 4: Commit**

```bash
git add GO/frontend/src/components/ControlPanel.tsx GO/frontend/src/components/ProcessTab.tsx
git commit -m "feat(frontend): add ControlPanel, complete end-to-end mock pipeline"
```

---

### Task 12: Hoàn thiện & kiểm tra build production

**Files:**
- Không tạo file mới; chỉ chạy lệnh xác minh và (nếu phát sinh) sửa lỗi nhỏ ở các file đã tạo.

**Interfaces:**
- Không có interface mới — task này khoá chất lượng của toàn bộ Phase 1.

- [ ] **Step 1: Chạy toàn bộ test Go**

```bash
cd GO && go test ./... -v
```

Expected: PASS toàn bộ (config, fileset, processing, App.runBatch).

- [ ] **Step 2: Build production**

```bash
cd GO && wails build
```

Expected: build thành công, sinh `GO/build/bin/order-processor.exe`, icon file .exe đúng là `blue.ico` đã copy ở Task 6.

- [ ] **Step 3: Chạy thử bản build production**

Mở trực tiếp `GO/build/bin/order-processor.exe` (double-click hoặc từ PowerShell `& "GO/build/bin/order-processor.exe"`). Xác nhận:
- Không có DevTools/menu chuột phải khi bấm phải chuột trong cửa sổ.
- Ctrl+cuộn chuột và Ctrl+/-/=/0 không zoom giao diện.
- Toàn bộ luồng ở Task 11 Step 3 (chọn file → xử lý → bảng kết quả → STT lưu lại) hoạt động giống hệt bản `wails dev`.

- [ ] **Step 4: Cập nhật `.gitignore` gốc nếu cần**

Kiểm tra file `.gitignore` ở gốc repo đã loại trừ artefact build của Go/Node chưa:

```bash
cd "c:/Users/Admin/Desktop/code py/Xử lý đơn hàng" && grep -E "GO/frontend/node_modules|GO/frontend/dist|GO/build/bin" .gitignore
```

Nếu không có, thêm 3 dòng sau vào cuối `.gitignore`:

```
GO/frontend/node_modules/
GO/frontend/dist/
GO/build/bin/
```

- [ ] **Step 5: Commit cuối Phase 1**

```bash
git add .gitignore
git commit -m "chore(go): ignore build artefacts, close out Phase 1 skeleton"
```

---

## Sau khi hoàn tất

Phase 1 xong khi cả 12 task pass. Phase 2 (spec riêng, brainstorm riêng) sẽ thêm `processing.RealProcessor` implement cùng interface `Processor` đã định nghĩa ở Task 4, bắt đầu với vendor Coop và BigC — không cần sửa `App`, event contract, hay bất kỳ component frontend nào ở trên.
