# Gửi tin Zalo thật (chromedp) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Nút to ở `ControlPanel.tsx` đổi thành nút gửi tin Zalo thật khi có PO được chọn trong `ResultTable.tsx`, gửi đúng nội dung đã build sẵn (giống hệt modal xem trước) tới đúng liên hệ Zalo, bằng cách điều khiển 1 Chrome thật qua `chromedp` (Go), không phụ thuộc Python.

**Architecture:** Package Go mới `internal/zalosend` (interface `ZaloSender` + cài đặt `ChromedpSender`) cắm vào `App.SendZaloMessages`, chạy nền và phát sự kiện `zalo:log`/`zalo:sent`/`zalo:done` theo đúng pattern `ProcessFiles`/`runBatch` đã có. Frontend nâng `selectedPOs` + `resolvedChoice` (lựa chọn giá đã xác nhận) từ state cục bộ của `ResultTable.tsx` lên `appStore.ts` để `ControlPanel.tsx` build được đúng job list rồi gọi `SendZaloMessages`.

**Tech Stack:** Go 1.25, `github.com/chromedp/chromedp` (mới), Wails v2, React + Zustand + TypeScript.

**Spec:** [docs/superpowers/specs/2026-08-24-zalo-send-integration-design.md](../specs/2026-08-24-zalo-send-integration-design.md)

## Global Constraints

- Không port `rich_paste_engine.py` (chế độ rich/paste) — chỉ gửi text thuần, khớp nội dung `buildZaloMessageForPO` hiện tại.
- Không headless — trình duyệt luôn hiện cửa sổ (cần cho QR lần đầu).
- Gửi tuần tự, không song song — 1 trình duyệt dùng chung.
- `zalo_profile/` (thư mục ở REPO ROOT, cùng cấp `settings.ini`/`zalo_state.json` — xem Task 1 vì sao KHÔNG phải `GO/zalo_profile/`; chứa cookie phiên đăng nhập thật) PHẢI vào `.gitignore` trước khi tính năng chạy lần đầu trên bất kỳ máy nào — sự cố rò rỉ `zalo_state.json` trước đây không được lặp lại.
- Không có UI trạng thái gửi riêng từng PO ở v1 — chỉ log text qua `LogPanel` + tự bỏ chọn khi xong.
- Frontend không có test suite tự động trong repo này — mỗi task frontend verify bằng `npx tsc --noEmit` (đúng cách CI đang implicit check qua `npm run build`), việc gửi thật chỉ verify bằng `wails dev` thủ công ở Task 9.

---

### Task 1: `.gitignore` — chặn rò rỉ profile Zalo trước khi nó tồn tại

**Files:**
- Modify: `.gitignore`

**Interfaces:**
- Không có (thay đổi cấu hình git thuần tuý, không phải code).

- [ ] **Step 1: Thêm dòng gitignore**

`ProfileDir` của `ChromedpSender` (xem Task 4) được tính bằng
`filepath.Join(resolveRepoDir("settings.ini"), "zalo_profile")` —
`resolveRepoDir("settings.ini")` trả về THƯ MỤC GỐC REPO (đã xác nhận
`settings.ini` nằm ở `./settings.ini`, cùng cấp `zalo_state.json`, KHÔNG
nằm trong `GO/`) — nên thư mục thật sự được tạo ra là `zalo_profile/` ở
gốc repo, KHÔNG phải `GO/zalo_profile/`. Mở `.gitignore` (repo root),
thêm ngay dưới dòng `zalo_state.json`/`log.log` (gần cuối file, cùng
nhóm với ghi chú "Superpowers scratch"):

```gitignore
zalo_state.json
log.log

# ===== Chromedp Zalo session (chứa cookie đăng nhập thật, KHÔNG commit — xem
# ghi nhớ dự án "Zalo session leak incident") =====
zalo_profile/
```

- [ ] **Step 2: Xác nhận bằng git status**

Run: `git status --short .gitignore`
Expected: hiện đúng 1 dòng ` M .gitignore` (file đã sửa, chưa stage).

- [ ] **Step 3: Commit**

```bash
git add .gitignore
git commit -m "chore: gitignore the future chromedp Zalo session profile dir"
```

---

### Task 2: `internal/zalosend` — interface `ZaloSender` + resolve liên hệ

**Files:**
- Create: `GO/internal/zalosend/sender.go`
- Test: `GO/internal/zalosend/sender_test.go`

**Interfaces:**
- Produces: `type ZaloSender interface { EnsureLoggedIn(ctx context.Context) error; SendMessage(ctx context.Context, contactQuery, message string) error; Close() error }`, `var ErrNoContact error`, `func ResolveContact(system string, zaloMap map[string]string) (string, error)`.

- [ ] **Step 1: Viết test trước (sẽ fail vì package chưa tồn tại)**

Tạo `GO/internal/zalosend/sender_test.go`:

```go
package zalosend

import (
	"errors"
	"testing"
)

func TestResolveContact_Found(t *testing.T) {
	zaloMap := map[string]string{"MNCOOPMART": "Đơn hàng Co-op Miền Nam"}
	got, err := ResolveContact("MNCOOPMART", zaloMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Đơn hàng Co-op Miền Nam" {
		t.Fatalf("got %q, want %q", got, "Đơn hàng Co-op Miền Nam")
	}
}

func TestResolveContact_NotConfigured(t *testing.T) {
	_, err := ResolveContact("UNKNOWN", map[string]string{"MNCOOPMART": "x"})
	if !errors.Is(err, ErrNoContact) {
		t.Fatalf("got err = %v, want errors.Is(err, ErrNoContact)", err)
	}
}

func TestResolveContact_EmptyValueTreatedAsNotConfigured(t *testing.T) {
	_, err := ResolveContact("MNCOOPMART", map[string]string{"MNCOOPMART": ""})
	if !errors.Is(err, ErrNoContact) {
		t.Fatalf("got err = %v, want errors.Is(err, ErrNoContact)", err)
	}
}
```

- [ ] **Step 2: Chạy test, xác nhận fail**

Run: `cd GO && go test ./internal/zalosend/... -v`
Expected: FAIL — package `zalosend` không có `ResolveContact`/`ErrNoContact` (chưa tồn tại file `sender.go`).

- [ ] **Step 3: Viết `sender.go`**

Tạo `GO/internal/zalosend/sender.go`:

```go
package zalosend

import (
	"context"
	"errors"
	"fmt"
)

// ZaloSender trừu tượng hoá việc gửi 1 tin nhắn Zalo, để app.go test được
// logic vòng lặp gửi hàng loạt (SendZaloMessages/runZaloBatch) bằng fake
// sender, không cần trình duyệt thật — cùng lý do processing.Processor
// là interface trong app.go hiện có.
type ZaloSender interface {
	// EnsureLoggedIn mở trình duyệt (nếu chưa mở) và đảm bảo đã đăng nhập
	// chat.zalo.me, chờ quét QR nếu cần. Gọi 1 LẦN trước cả batch gửi,
	// không gọi lại cho từng tin — trình duyệt sống xuyên suốt vòng đời
	// App, khác với send_message.py (Python), vốn coi login là 1 phần
	// của mỗi lần chạy script riêng lẻ.
	EnsureLoggedIn(ctx context.Context) error

	// SendMessage tìm đúng hội thoại theo contactQuery (tên liên hệ/nhóm
	// hiển thị trên Zalo) rồi gửi message dạng text thuần.
	SendMessage(ctx context.Context, contactQuery, message string) error

	// Close đóng trình duyệt — gọi lúc App shutdown.
	Close() error
}

// ErrNoContact báo hệ thống (OrderRow.System) chưa có mapping Zalo trong
// settings.Zalo — job tương ứng bị SKIP (không dừng cả batch), người
// dùng sửa qua Cài đặt > tab Zalo rồi gửi lại đúng PO đó.
var ErrNoContact = errors.New("zalosend: no zalo contact configured for this system")

// ResolveContact tra map settings.Zalo (đã có sẵn, sửa được qua popup
// Cài đặt) theo tên hệ thống. Giá trị rỗng bị coi như CHƯA cấu hình
// (không phải "gửi tới tên rỗng") — khớp cách KeyValueEditor bỏ qua
// dòng key/value rỗng lúc lưu.
func ResolveContact(system string, zaloMap map[string]string) (string, error) {
	contact, ok := zaloMap[system]
	if !ok || contact == "" {
		return "", fmt.Errorf("%w: %s", ErrNoContact, system)
	}
	return contact, nil
}
```

- [ ] **Step 4: Chạy lại test, xác nhận pass**

Run: `cd GO && go test ./internal/zalosend/... -v`
Expected: PASS (3/3 test).

- [ ] **Step 5: Commit**

```bash
git add GO/internal/zalosend/sender.go GO/internal/zalosend/sender_test.go
git commit -m "feat(go): add ZaloSender interface and contact resolution"
```

---

### Task 3: `internal/zalosend` — `ChromedpSender` (cài đặt thật)

**Files:**
- Modify: `GO/go.mod`, `GO/go.sum` (thêm dependency)
- Create: `GO/internal/zalosend/chromedp_sender.go`

**Interfaces:**
- Consumes: `ZaloSender` interface (Task 2).
- Produces: `type ChromedpSender struct { ProfileDir string; ... }` implementing `ZaloSender` — dùng ở Task 4 (`app.go`).

- [ ] **Step 1: Thêm dependency chromedp**

Run: `cd GO && go get github.com/chromedp/chromedp@latest`
Expected: `go.mod`/`go.sum` được cập nhật tự động (thêm `github.com/chromedp/chromedp` + `github.com/chromedp/cdproto` + các dependency gián tiếp).

- [ ] **Step 2: Viết `chromedp_sender.go`**

Tạo `GO/internal/zalosend/chromedp_sender.go`:

```go
package zalosend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

const (
	loginPollInterval  = 1 * time.Second
	loginTimeout       = 120 * time.Second
	sendConfirmTimeout = 15 * time.Second
)

// ChromedpSender là cài đặt thật của ZaloSender, dùng chromedp điều
// khiển 1 Chrome thật (không headless — cần hiện cửa sổ để quét QR lần
// đầu, và để người dùng quan sát/can thiệp nếu có gì bất thường, đúng
// hành vi mặc định của send_message.py khi KHÔNG truyền --headless).
// ProfileDir PHẢI là 1 thư mục nằm trong .gitignore (xem Task 1) —
// chứa cookie phiên đăng nhập thật ngay sau lần quét QR đầu tiên.
type ChromedpSender struct {
	ProfileDir string

	mu            sync.Mutex
	allocCancel   context.CancelFunc
	browserCtx    context.Context
	browserCancel context.CancelFunc
}

// ensureBrowser mở allocator + context 1 LẦN (idempotent — các lần gọi
// sau chỉ thấy c.browserCtx đã khác nil và trả về ngay), giữ trình
// duyệt sống xuyên suốt vòng đời ChromedpSender thay vì mở/đóng mỗi
// lần gửi.
func (c *ChromedpSender) ensureBrowser() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.browserCtx != nil {
		return nil
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir(c.ProfileDir),
		chromedp.Flag("headless", false),
		chromedp.WindowSize(1280, 900),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	// chromedp.Run với 0 action vẫn buộc trình duyệt thật sự khởi động
	// (chromedp mặc định lazy-start lúc action đầu tiên chạy) — làm
	// ngay ở đây để lỗi khởi động Chrome (ví dụ chưa cài Chrome) lộ ra
	// ngay tại EnsureLoggedIn, không phải lúc SendMessage đầu tiên.
	if err := chromedp.Run(browserCtx); err != nil {
		browserCancel()
		allocCancel()
		return fmt.Errorf("zalosend: khởi động trình duyệt: %w", err)
	}

	c.allocCancel = allocCancel
	c.browserCtx, c.browserCancel = browserCtx, browserCancel
	return nil
}

// EnsureLoggedIn mở trình duyệt (nếu chưa), điều hướng chat.zalo.me,
// chờ tối đa loginTimeout (120s, khớp send_message.py) nếu URL rơi vào
// id.zalo.me (chưa đăng nhập — cần quét QR trong cửa sổ vừa mở).
func (c *ChromedpSender) EnsureLoggedIn(ctx context.Context) error {
	if err := c.ensureBrowser(); err != nil {
		return err
	}
	if err := chromedp.Run(c.browserCtx, chromedp.Navigate("https://chat.zalo.me/")); err != nil {
		return fmt.Errorf("zalosend: điều hướng chat.zalo.me: %w", err)
	}
	// Chờ trang render xong trước khi đọc URL lần đầu — khớp
	// send_message.py's page.wait_for_timeout(4000) sau Navigate.
	time.Sleep(4 * time.Second)

	deadline := time.Now().Add(loginTimeout)
	for {
		var url string
		if err := chromedp.Run(c.browserCtx, chromedp.Location(&url)); err != nil {
			return fmt.Errorf("zalosend: đọc URL hiện tại: %w", err)
		}
		if !strings.Contains(url, "id.zalo.me") {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("zalosend: hết thời gian chờ đăng nhập — vui lòng quét QR trong cửa sổ trình duyệt vừa mở rồi thử gửi lại")
		}
		time.Sleep(loginPollInterval)
	}
}

// SendMessage tìm hội thoại theo contactQuery rồi gửi message. PHẢI gọi
// EnsureLoggedIn trước (ít nhất 1 lần trong vòng đời ChromedpSender này).
func (c *ChromedpSender) SendMessage(ctx context.Context, contactQuery, message string) error {
	if c.browserCtx == nil {
		return errors.New("zalosend: chưa gọi EnsureLoggedIn")
	}

	if err := chromedp.Run(c.browserCtx,
		chromedp.Click("#contact-search-input", chromedp.ByID),
		// Chọn hết nội dung cũ (nếu có, từ lần tìm trước) rồi gõ đè lên —
		// tránh dùng chromedp.SetValue (set thẳng DOM .value, không chắc
		// kích hoạt đúng sự kiện input/onChange của khung tìm kiếm này,
		// khác hẳn field text thường): Ctrl+A rồi gõ mới là cách chắc
		// chắn hoạt động giống thao tác tay thật, cùng cách kb.Control+"a"
		// đã dùng ở SendMessage bên dưới cho "select all" trên tin nhắn.
		chromedp.KeyEvent(kb.Control+"a"),
		chromedp.SendKeys("#contact-search-input", contactQuery, chromedp.ByID),
	); err != nil {
		return fmt.Errorf("zalosend: gõ tìm kiếm %q: %w", contactQuery, err)
	}
	// Khớp send_message.py's page.wait_for_timeout(1500) sau khi gõ tìm
	// kiếm — cho kết quả tìm kiếm kịp render trước khi tìm .txt-highlight.
	time.Sleep(1500 * time.Millisecond)

	if err := chromedp.Run(c.browserCtx,
		chromedp.WaitVisible(".txt-highlight", chromedp.ByQuery),
		chromedp.Click(".txt-highlight", chromedp.ByQuery),
		chromedp.WaitVisible("#richInput", chromedp.ByID),
	); err != nil {
		return fmt.Errorf("zalosend: không tìm thấy hội thoại %q: %w", contactQuery, err)
	}

	lines := strings.Split(message, "\n")
	actions := []chromedp.Action{chromedp.Click("#richInput", chromedp.ByID)}
	for i, line := range lines {
		if i > 0 {
			// Shift+Enter = xuống dòng TRONG cùng 1 tin (Enter không kèm
			// Shift = gửi luôn) — khớp send_message.py's
			// page.keyboard.press("Shift+Enter") giữa các dòng.
			actions = append(actions, chromedp.KeyEvent(kb.Shift+kb.Enter))
		}
		if line != "" {
			actions = append(actions, chromedp.KeyEvent(line))
		}
	}
	if err := chromedp.Run(c.browserCtx, actions...); err != nil {
		return fmt.Errorf("zalosend: gõ nội dung tin nhắn: %w", err)
	}

	confirmed := make(chan error, 1)
	if err := watchForSendConfirmation(c.browserCtx, confirmed); err != nil {
		return fmt.Errorf("zalosend: chuẩn bị xác nhận gửi: %w", err)
	}
	if err := chromedp.Run(c.browserCtx, chromedp.KeyEvent(kb.Enter)); err != nil {
		return fmt.Errorf("zalosend: bấm Enter để gửi: %w", err)
	}
	select {
	case err := <-confirmed:
		if err != nil {
			return fmt.Errorf("zalosend: gửi tới %q thất bại: %w", contactQuery, err)
		}
		return nil
	case <-time.After(sendConfirmTimeout):
		return fmt.Errorf("zalosend: không nhận được xác nhận gửi tới %q sau %s (tin có thể đã gửi, kiểm tra lại thủ công)", contactQuery, sendConfirmTimeout)
	}
}

// watchForSendConfirmation bật network.Enable() rồi đăng ký 1 listener
// (qua chromedp.ListenTarget) khớp response có URL chứa "message/sms" —
// tương đương page.expect_response bên Playwright, nhưng chromedp không
// có API chờ-response tiện lợi tương đương nên phải tự viết bằng
// channel. ĐÂY LÀ PHẦN CHƯA CHẮC CHẮN NHẤT của toàn bộ package (xem
// mục Rủi ro trong spec) — nếu qua kiểm thử thủ công (Task 9) mà
// listener không bao giờ khớp (ví dụ Zalo đổi URL endpoint), phương án
// dự phòng đã ghi trong spec: thay bằng kiểm tra DOM (bong bóng tin
// nhắn cuối cùng) thay vì network response.
func watchForSendConfirmation(browserCtx context.Context, result chan<- error) error {
	if err := chromedp.Run(browserCtx, network.Enable()); err != nil {
		return err
	}

	var once sync.Once
	chromedp.ListenTarget(browserCtx, func(ev interface{}) {
		respEv, ok := ev.(*network.EventResponseReceived)
		if !ok || !strings.Contains(respEv.Response.URL, "message/sms") {
			return
		}
		once.Do(func() {
			// ListenTarget gọi callback ĐỒNG BỘ trên goroutine xử lý sự
			// kiện CDP — không được block/chạy Action ở đây (sẽ deadlock),
			// nên đọc response body trong 1 goroutine riêng.
			go func() {
				var body []byte
				readErr := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					b, err := network.GetResponseBody(respEv.RequestID).Do(ctx)
					if err != nil {
						return err
					}
					body = b
					return nil
				}))
				if readErr != nil {
					result <- fmt.Errorf("đọc response gửi tin: %w", readErr)
					return
				}
				if !strings.Contains(string(body), `"error_code":0`) {
					snippet := string(body)
					if len(snippet) > 300 {
						snippet = snippet[:300]
					}
					result <- fmt.Errorf("Zalo báo lỗi: %s", snippet)
					return
				}
				result <- nil
			}()
		})
	})
	return nil
}

// Close đóng trình duyệt — an toàn gọi nhiều lần, gọi khi chưa từng mở
// trình duyệt (browserCtx nil) cũng không lỗi.
func (c *ChromedpSender) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.browserCancel != nil {
		c.browserCancel()
	}
	if c.allocCancel != nil {
		c.allocCancel()
	}
	c.browserCtx = nil
	return nil
}
```

- [ ] **Step 3: Build + vet (không có test tự động — trình duyệt thật, xem Task 9)**

Run: `cd GO && go build ./... && go vet ./...`
Expected: build + vet sạch, không lỗi. (Đây KHÔNG xác nhận logic gửi tin đúng — chỉ xác nhận code compile. Xác nhận thật chỉ có ở Task 9.)

- [ ] **Step 4: Commit**

```bash
git add GO/go.mod GO/go.sum GO/internal/zalosend/chromedp_sender.go
git commit -m "feat(go): add ChromedpSender — real Zalo send via chromedp"
```

---

### Task 4: `app.go` — `SendZaloMessages` + phát sự kiện tiến trình

**Files:**
- Modify: `GO/app.go`
- Modify: `GO/main.go`
- Test: `GO/app_test.go`

**Interfaces:**
- Consumes: `zalosend.ZaloSender`, `zalosend.ResolveContact`, `zalosend.ErrNoContact`, `appsettings.Settings.Zalo` (Task 2/3, đã có sẵn).
- Produces: `type ZaloJob struct { PO, System, Message string }`, `func (a *App) SendZaloMessages(jobs []ZaloJob)`, sự kiện `zalo:log` (string), `zalo:sent` (`map[string]any{"po": string, "ok": bool}`), `zalo:done` (nil) — dùng ở Task 8 (`useWailsEvents.ts`).

- [ ] **Step 1: Viết test trước (fail vì `runZaloBatch`/`ZaloJob` chưa tồn tại)**

Thêm vào cuối `GO/app_test.go`:

```go
type fakeZaloSender struct {
	loginErr   error
	sendErrs   map[string]error // key = contactQuery
	loginCalls int
	sentTo     []string
}

func (f *fakeZaloSender) EnsureLoggedIn(ctx context.Context) error {
	f.loginCalls++
	return f.loginErr
}

func (f *fakeZaloSender) SendMessage(ctx context.Context, contactQuery, message string) error {
	f.sentTo = append(f.sentTo, contactQuery)
	if f.sendErrs != nil {
		if err, ok := f.sendErrs[contactQuery]; ok {
			return err
		}
	}
	return nil
}

func (f *fakeZaloSender) Close() error { return nil }

func newTestAppForZalo(t *testing.T, sender zalosend.ZaloSender, zaloMap map[string]string) *App {
	t.Helper()
	store := appsettings.NewStore(filepath.Join(t.TempDir(), "settings.bhconfig"))
	if err := store.Save(appsettings.Settings{Zalo: zaloMap}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	return &App{appSettingsStore: store, zaloSender: sender}
}

func sentEventsOf(t *testing.T, events []emittedEvent) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range events {
		if e.name == "zalo:sent" {
			data, ok := e.data[0].(map[string]any)
			if !ok {
				t.Fatalf("zalo:sent data is not map[string]any: %#v", e.data)
			}
			out = append(out, data)
		}
	}
	return out
}

func TestRunZaloBatch_SendsEachJobAndEmitsEvents(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop", "BIGC": "Nhom BigC"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "COOP", Message: "noi dung 1"},
		{PO: "PO2", System: "BIGC", Message: "noi dung 2"},
	})

	if sender.loginCalls != 1 {
		t.Fatalf("loginCalls = %d, want 1", sender.loginCalls)
	}
	wantSentTo := []string{"Nhom Coop", "Nhom BigC"}
	if !reflect.DeepEqual(sender.sentTo, wantSentTo) {
		t.Fatalf("sentTo = %#v, want %#v", sender.sentTo, wantSentTo)
	}

	lastEvent := emitter.events[len(emitter.events)-1]
	if lastEvent.name != "zalo:done" {
		t.Fatalf("last event = %q, want zalo:done", lastEvent.name)
	}

	sent := sentEventsOf(t, emitter.events)
	if len(sent) != 2 || sent[0]["po"] != "PO1" || sent[0]["ok"] != true || sent[1]["po"] != "PO2" || sent[1]["ok"] != true {
		t.Fatalf("zalo:sent events = %#v", sent)
	}
}

func TestRunZaloBatch_SkipsJobWithoutContact(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "UNKNOWN", Message: "noi dung 1"},
		{PO: "PO2", System: "COOP", Message: "noi dung 2"},
	})

	if !reflect.DeepEqual(sender.sentTo, []string{"Nhom Coop"}) {
		t.Fatalf("sentTo = %#v, want only the configured contact attempted", sender.sentTo)
	}

	sent := sentEventsOf(t, emitter.events)
	if len(sent) != 2 || sent[0]["po"] != "PO1" || sent[0]["ok"] != false || sent[1]["po"] != "PO2" || sent[1]["ok"] != true {
		t.Fatalf("zalo:sent events = %#v", sent)
	}
}

func TestRunZaloBatch_ContinuesAfterOneJobFails(t *testing.T) {
	sender := &fakeZaloSender{sendErrs: map[string]error{"Nhom Loi": errors.New("boom")}}
	a := newTestAppForZalo(t, sender, map[string]string{"LOI": "Nhom Loi", "COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{
		{PO: "PO1", System: "LOI", Message: "x"},
		{PO: "PO2", System: "COOP", Message: "y"},
	})

	if !reflect.DeepEqual(sender.sentTo, []string{"Nhom Loi", "Nhom Coop"}) {
		t.Fatalf("sentTo = %#v, want both contacts attempted despite the first failing", sender.sentTo)
	}
	sent := sentEventsOf(t, emitter.events)
	if len(sent) != 2 || sent[0]["ok"] != false || sent[1]["ok"] != true {
		t.Fatalf("zalo:sent events = %#v", sent)
	}
}

func TestRunZaloBatch_AbortsWholeBatchIfLoginFails(t *testing.T) {
	sender := &fakeZaloSender{loginErr: errors.New("login timeout")}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}

	a.runZaloBatch(emitter, []ZaloJob{{PO: "PO1", System: "COOP", Message: "x"}})

	if len(sender.sentTo) != 0 {
		t.Fatalf("sentTo = %#v, want no send attempted after login failure", sender.sentTo)
	}
	lastEvent := emitter.events[len(emitter.events)-1]
	if lastEvent.name != "zalo:done" {
		t.Fatalf("last event = %q, want zalo:done", lastEvent.name)
	}
}

func TestApp_SendZaloMessages_RejectsWhileAlreadySending(t *testing.T) {
	sender := &fakeZaloSender{}
	a := newTestAppForZalo(t, sender, map[string]string{"COOP": "Nhom Coop"})
	emitter := &fakeEmitter{}
	a.emitter = emitter
	a.sending.Store(true)

	a.SendZaloMessages([]ZaloJob{{PO: "PO1", System: "COOP", Message: "x"}})

	if len(emitter.events) != 1 || emitter.events[0].name != "zalo:log" {
		t.Fatalf("events = %#v, want a single zalo:log warning", emitter.events)
	}
	if sender.loginCalls != 0 {
		t.Fatalf("loginCalls = %d, want 0 (must not start a new batch while one is running)", sender.loginCalls)
	}
}
```

Thêm import còn thiếu vào đầu `app_test.go` (`"order-processor/internal/appsettings"`, `"order-processor/internal/zalosend"`) nếu chưa có.

- [ ] **Step 2: Chạy test, xác nhận fail**

Run: `cd GO && go test ./... -run TestRunZaloBatch -v` và `go test ./... -run TestApp_SendZaloMessages -v`
Expected: FAIL để build — `App` chưa có field `zaloSender`/`sending`, chưa có `runZaloBatch`/`SendZaloMessages`/`ZaloJob`.

- [ ] **Step 3: Thêm field vào `App` struct + khởi tạo trong `NewApp`**

Trong `GO/app.go`, sửa import (thêm `"sync/atomic"` đã có sẵn ở dòng 9, thêm dòng mới):

```go
	"order-processor/internal/zalosend"
```

Sửa struct `App` (dòng 43-54), thêm 2 field:

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
	zaloSender       zalosend.ZaloSender
	sending          atomic.Bool
}
```

Trong `NewApp()` (dòng 154-160), thêm khởi tạo `zaloSender`:

```go
	app := &App{
		cfg:              config.NewStore(configFileName),
		appSettingsStore: appSettingsStore,
		processor:        processor,
		orderDir:         orderDir,
		excelPath:        excelPath,
		zaloSender: &zalosend.ChromedpSender{
			ProfileDir: filepath.Join(resolveRepoDir("settings.ini"), "zalo_profile"),
		},
	}
```

- [ ] **Step 4: Thêm `ZaloJob`, `SendZaloMessages`, `runZaloBatch`**

Thêm vào cuối `GO/app.go`:

```go
// ZaloJob là 1 lần gửi cần thực hiện: nội dung tin nhắn ĐÃ được frontend
// build sẵn bằng buildZaloMessageForPO (y hệt nội dung modal xem trước
// đã hiển thị cho người dùng) — Go không build lại text, chỉ resolve
// liên hệ (theo System) rồi gửi.
type ZaloJob struct {
	PO      string `json:"po"`
	System  string `json:"system"`
	Message string `json:"message"`
}

// SendZaloMessages gửi tuần tự từng job trong 1 goroutine nền, phát sự
// kiện zalo:log/zalo:sent/zalo:done — cùng pattern ProcessFiles/runBatch.
// Từ chối nếu đang có 1 lượt gửi khác chạy (atomic.Bool, giống
// a.processing) — không cho 2 batch gửi chồng lên nhau trên cùng 1
// trình duyệt.
func (a *App) SendZaloMessages(jobs []ZaloJob) {
	if !a.sending.CompareAndSwap(false, true) {
		a.emitter.Emit("zalo:log", "⚠️ Đã có một lượt gửi Zalo đang chạy, vui lòng đợi hoàn tất.")
		return
	}
	go a.runZaloBatch(a.emitter, jobs)
}

func (a *App) runZaloBatch(emitter Emitter, jobs []ZaloJob) {
	defer func() {
		if r := recover(); r != nil {
			emitter.Emit("zalo:log", fmt.Sprintf("❌ Lỗi không mong muốn: %v", r))
		}
		a.sending.Store(false)
		emitter.Emit("zalo:done", nil)
	}()

	ctx := context.Background()
	emitter.Emit("zalo:log", "🔐 Đang kiểm tra đăng nhập Zalo (quét QR trên cửa sổ trình duyệt nếu được yêu cầu)...")
	if err := a.zaloSender.EnsureLoggedIn(ctx); err != nil {
		emitter.Emit("zalo:log", fmt.Sprintf("❌ Không đăng nhập được Zalo: %v", err))
		return
	}

	settings, err := a.appSettingsStore.Load(resolveRepoFile("settings.ini"))
	if err != nil {
		emitter.Emit("zalo:log", fmt.Sprintf("❌ Không đọc được cấu hình liên hệ Zalo: %v", err))
		return
	}

	for _, job := range jobs {
		contact, err := zalosend.ResolveContact(job.System, settings.Zalo)
		if err != nil {
			emitter.Emit("zalo:log", fmt.Sprintf("❌ %s: chưa cấu hình liên hệ Zalo cho %s (sửa ở Cài đặt > Zalo)", job.PO, job.System))
			emitter.Emit("zalo:sent", map[string]any{"po": job.PO, "ok": false})
			continue
		}
		emitter.Emit("zalo:log", fmt.Sprintf("📤 Đang gửi %s → %s...", job.PO, contact))
		if err := a.zaloSender.SendMessage(ctx, contact, job.Message); err != nil {
			emitter.Emit("zalo:log", fmt.Sprintf("❌ Gửi %s thất bại: %v", job.PO, err))
			emitter.Emit("zalo:sent", map[string]any{"po": job.PO, "ok": false})
			continue
		}
		emitter.Emit("zalo:log", fmt.Sprintf("✅ Đã gửi %s", job.PO))
		emitter.Emit("zalo:sent", map[string]any{"po": job.PO, "ok": true})
	}
}
```

- [ ] **Step 5: Đóng trình duyệt lúc app tắt**

Thêm vào cuối `GO/app.go`:

```go
// shutdown đóng trình duyệt Zalo (nếu đã mở) khi app thoát — tránh để
// lại 1 tiến trình Chrome mồ côi chạy nền sau khi đóng cửa sổ chính.
func (a *App) shutdown(ctx context.Context) {
	if a.zaloSender != nil {
		_ = a.zaloSender.Close()
	}
}
```

Sửa `GO/main.go`, thêm `OnShutdown: app.shutdown,` vào `options.App{...}` (ngay dưới dòng `OnStartup: app.startup,`, dòng 35):

```go
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
```

- [ ] **Step 6: Chạy test, xác nhận pass**

Run: `cd GO && go build ./... && go test ./... -v`
Expected: build sạch, TOÀN BỘ test pass (bao gồm cả 5 test mới + test cũ không bị vỡ).

- [ ] **Step 7: Commit**

```bash
git add GO/app.go GO/app_test.go GO/main.go
git commit -m "feat(go): wire SendZaloMessages batch send + zalo:* events into App"
```

---

### Task 5: Frontend — helper thuần (`zaloMessage.ts`) + nâng state lên `appStore.ts`

**Files:**
- Modify: `GO/frontend/src/lib/zaloMessage.ts`
- Modify: `GO/frontend/src/store/appStore.ts`

**Interfaces:**
- Produces: `resolveEffectivePrice(rowIndex, detail, resolvedChoice)`, `buildPriceBasisForRow(rowIndex, row, resolvedChoice)`, `buildPriceBasisForPO(rows, rowIndices, resolvedChoice)` (đều export từ `zaloMessage.ts`); `appStore`'s `selectedPOs: Set<string>`, `togglePOSelection(po)`, `toggleAllPOs(allPOs, checked)`, `clearSelection()`, `resolvedChoice: Record<string, PriceBasis>`, `setResolvedChoice(key, choice)`, `clearResolvedChoice()` — dùng ở Task 6/7.

- [ ] **Step 1: Thêm 3 hàm thuần vào `zaloMessage.ts`**

Thêm vào `GO/frontend/src/lib/zaloMessage.ts`, ngay sau `effectivePrice` (dòng 27-29) — đây là 3 hàm được kéo ra từ `ResultTable.tsx` (nơi chúng đang là closure đóng gói `rows`/`resolvedChoice` cục bộ) để cả `ResultTable.tsx` và `ControlPanel.tsx` dùng chung mà không lặp logic:

```ts
// resolveEffectivePrice là whichever giá đang tính vào DonGia của dòng
// này cho SKU này: giá PO nếu người dùng đã xác nhận chọn, ngược lại
// giá hệ thống (mặc định của DonGia — xem PriceMismatchDetail's doc).
// resolvedChoice key là `${rowIndex}-${excelRow}` (khớp cách
// ResultTable.tsx đã dùng, giữ nguyên qua lần refactor này).
export function resolveEffectivePrice(
  rowIndex: number,
  detail: PriceMismatchDetail,
  resolvedChoice: Record<string, PriceBasis>,
): number {
  const choice = resolvedChoice[`${rowIndex}-${detail.excelRow}`]
  return choice === 'po' ? detail.invoicePrice : detail.systemPrice
}

// buildPriceBasisForRow rút gọn resolvedChoice (key theo rowIndex, có
// thể lặp excelRow giữa các dòng khác nhau) xuống 1 map theo excelRow
// riêng của 1 dòng — đúng dạng buildZaloMessage cần.
export function buildPriceBasisForRow(
  rowIndex: number,
  row: OrderRow,
  resolvedChoice: Record<string, PriceBasis>,
): Record<number, PriceBasis> {
  const result: Record<number, PriceBasis> = {}
  for (const d of row.priceMismatchDetails ?? []) {
    result[d.excelRow] = resolvedChoice[`${rowIndex}-${d.excelRow}`] ?? 'system'
  }
  return result
}

// buildPriceBasisForPO gộp buildPriceBasisForRow của mọi dòng thuộc 1 PO
// (BigC có thể có nhiều dòng/PO) thành 1 map duy nhất cho
// buildZaloMessageForPO — an toàn gộp vì excelRow là số dòng Excel thật,
// không bao giờ trùng giữa 2 OrderRow khác nhau.
export function buildPriceBasisForPO(
  rows: OrderRow[],
  rowIndices: number[],
  resolvedChoice: Record<string, PriceBasis>,
): Record<number, PriceBasis> {
  const result: Record<number, PriceBasis> = {}
  for (const idx of rowIndices) {
    Object.assign(result, buildPriceBasisForRow(idx, rows[idx], resolvedChoice))
  }
  return result
}
```

Thêm `OrderRow` vào import ở đầu file (dòng 10 hiện tại `import type { OrderRow, PriceMismatchDetail } from '../types'` — đã có sẵn `OrderRow`, không cần sửa).

- [ ] **Step 2: Nâng state lên `appStore.ts`**

Sửa `GO/frontend/src/store/appStore.ts` — thêm import `PriceBasis`:

```ts
import type { LogEntry, OrderRow } from '../types'
import type { PriceBasis } from '../lib/zaloMessage'
```

Thêm vào interface `AppState` (sau `lockStatus: LockStatus`, dòng 12):

```ts
  selectedPOs: Set<string>
  resolvedChoice: Record<string, PriceBasis>
  togglePOSelection: (po: string) => void
  toggleAllPOs: (allPOs: string[], checked: boolean) => void
  clearSelection: () => void
  setResolvedChoice: (key: string, choice: PriceBasis) => void
  clearResolvedChoice: () => void
```

Thêm vào state khởi tạo (sau `lockStatus: 'checking',`, dòng 36):

```ts
  selectedPOs: new Set(),
  resolvedChoice: {},
```

Thêm vào cuối object store (trước dòng đóng `}))`, sau `setLockStatus`):

```ts
  togglePOSelection: (po) =>
    set((state) => {
      const next = new Set(state.selectedPOs)
      if (next.has(po)) next.delete(po)
      else next.add(po)
      return { selectedPOs: next }
    }),
  toggleAllPOs: (allPOs, checked) => set({ selectedPOs: checked ? new Set(allPOs) : new Set() }),
  clearSelection: () => set({ selectedPOs: new Set() }),
  setResolvedChoice: (key, choice) =>
    set((state) => ({ resolvedChoice: { ...state.resolvedChoice, [key]: choice } })),
  clearResolvedChoice: () => set({ resolvedChoice: {} }),
```

- [ ] **Step 3: Kiểm tra kiểu (không có test tự động ở frontend)**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: KHÔNG sạch ở bước này — `ResultTable.tsx` vẫn còn khai báo `resolvedChoice`/`selectedPOs` cục bộ trùng tên với store (chưa xung đột kiểu vì chưa import từ store), nhưng file `zaloMessage.ts`/`appStore.ts` tự chúng phải type-check sạch. Nếu `tsc` báo lỗi TRONG 2 file này, sửa trước khi qua bước tiếp.

- [ ] **Step 4: Commit**

```bash
git add GO/frontend/src/lib/zaloMessage.ts GO/frontend/src/store/appStore.ts
git commit -m "feat(frontend): lift PO selection + price-basis choice into appStore"
```

---

### Task 6: `ResultTable.tsx` — chuyển sang dùng state từ `appStore`

**Files:**
- Modify: `GO/frontend/src/components/ResultTable.tsx`

**Interfaces:**
- Consumes: `appStore`'s `selectedPOs`/`togglePOSelection`/`toggleAllPOs`/`clearSelection`/`resolvedChoice`/`setResolvedChoice`/`clearResolvedChoice` (Task 5), `resolveEffectivePrice`/`buildPriceBasisForRow`/`buildPriceBasisForPO` từ `zaloMessage.ts` (Task 5).

- [ ] **Step 1: Xoá state cục bộ, đọc từ store**

Sửa dòng 93-95 (khai báo `useState`) thành:

```tsx
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  const [expandedRow, setExpandedRow] = useState<number | null>(null)
  const selectedPOs = useAppStore((s) => s.selectedPOs)
  const togglePOSelection = useAppStore((s) => s.togglePOSelection)
  const toggleAllPOs = useAppStore((s) => s.toggleAllPOs)
  const clearSelection = useAppStore((s) => s.clearSelection)
  const resolvedChoice = useAppStore((s) => s.resolvedChoice)
  const setResolvedChoiceKey = useAppStore((s) => s.setResolvedChoice)
  const clearResolvedChoice = useAppStore((s) => s.clearResolvedChoice)
  const [flashCount, setFlashCount] = useState<Record<number, number>>({})
  const [contentModalGroups, setContentModalGroups] = useState<POContentGroup[] | null>(null)
```

Xoá dòng 103 cũ (`const [selectedPOs, setSelectedPOs] = useState<Set<string>>(new Set())`) — đã thay bằng dòng trên.

- [ ] **Step 2: Xoá 2 hàm cục bộ giờ dư thừa (`togglePOSelection`/`toggleAllPOs`)**

Xoá định nghĩa `function togglePOSelection(po: string) {...}` và `function toggleAllPOs(checked: boolean) {...}` (dòng 203-215 hiện tại) — đã thay bằng action từ store ở Step 1.

- [ ] **Step 3: Cập nhật effect reset khi batch mới**

Sửa effect dòng 126-134 (`if (rows.length === 0) {...}`):

```tsx
  useEffect(() => {
    if (rows.length === 0) {
      setExpandedRow(null)
      clearResolvedChoice()
      setContentModalGroups(null)
      clearSelection()
      receivedAtRef.current = {}
    }
  }, [rows.length, clearResolvedChoice, clearSelection])
```

- [ ] **Step 4: Đổi `effectivePrice`/`priceBasisForRow`/`priceBasisForPO` sang gọi helper dùng chung**

Xoá định nghĩa module-level `function effectivePrice(...)` (dòng 80-87) — thay lời gọi DUY NHẤT của nó, ở dòng 145 trong `handleApplyPrice` (`const previousPrice = effectivePrice(rowIndex, detail, resolvedChoice)`), bằng `resolveEffectivePrice(rowIndex, detail, resolvedChoice)` (import từ `zaloMessage.ts`).

Xoá 2 hàm cục bộ `priceBasisForRow`/`priceBasisForPO` (dòng 178-227 — `priceBasisForRow` chỉ được gọi nội bộ bởi `priceBasisForPO`, không có nơi khác gọi trực tiếp). `priceBasisForPO` có ĐÚNG 1 nơi gọi, trong IIFE dựng `OrderContentModal` ở cuối file (dòng 494-506) — sửa khối này thành:

```tsx
      {contentModalGroups && contentModalGroups.length > 0 && (() => {
        const firstRowIndex = rowsForPO(contentModalGroups[0].po)[0]
        const combinedPriceBasis: Record<number, PriceBasis> = {}
        for (const g of contentModalGroups) {
          Object.assign(combinedPriceBasis, buildPriceBasisForPO(rows, rowsForPO(g.po), resolvedChoice))
        }
        return (
          <OrderContentModal
            groups={contentModalGroups}
            processedAt={receivedAtRef.current[firstRowIndex] ?? ''}
            priceBasisBySku={combinedPriceBasis}
            onClose={() => setContentModalGroups(null)}
          />
        )
      })()}
```

(Đổi kiểu `Record<number, 'po' | 'system'>` thành `Record<number, PriceBasis>` — cùng kiểu, chỉ đổi tên qua alias đã import từ `zaloMessage.ts`; thêm `PriceBasis` vào import ở Step tiếp theo.)

Sửa `handleApplyPrice` (dòng 142-157) — đổi dòng gọi `setResolvedChoice` cục bộ (dòng 148) thành gọi action store:

```tsx
  async function handleApplyPrice(rowIndex: number, detail: PriceMismatchDetail, useInvoicePrice: boolean) {
    const price = useInvoicePrice ? detail.invoicePrice : detail.systemPrice
    const key = `${rowIndex}-${detail.excelRow}`
    const previousPrice = resolveEffectivePrice(rowIndex, detail, resolvedChoice)
    try {
      await ConfirmPrice(detail.excelRow, price)
      setResolvedChoiceKey(key, useInvoicePrice ? 'po' : 'system')
      const delta = (price - previousPrice) * detail.qty
      if (delta !== 0) {
        adjustRowDonGia(rowIndex, delta)
        setFlashCount((prev) => ({ ...prev, [rowIndex]: (prev[rowIndex] ?? 0) + 1 }))
      }
    } catch (err) {
      appendLog(`❌ Lỗi áp dụng giá cho ${detail.sku}: ${String(err)}`)
    }
  }
```

Thêm import ở đầu file (cạnh import `type { OrderRow, PriceMismatchDetail } from '../types'`, dòng 14):

```tsx
import {
  resolveEffectivePrice,
  buildPriceBasisForRow,
  buildPriceBasisForPO,
  type PriceBasis,
} from '../lib/zaloMessage'
```

(`buildPriceBasisForRow` chỉ dùng gián tiếp qua `buildPriceBasisForPO`, xem Step tiếp theo — vẫn cần import vì `PriceMismatchDetail`/kiểu trả về xuất hiện trong chữ ký hàm mà TypeScript cần resolve được.)

- [ ] **Step 5: Cập nhật call site `toggleAllPOs`**

Sửa dòng 281 (`onChange={(e) => toggleAllPOs(e.target.checked)}`) thành:

```tsx
                  onChange={(e) => toggleAllPOs(uniquePOs, e.target.checked)}
```

Sửa nút "Bỏ chọn hết" (`onClick={() => setSelectedPOs(new Set())}`, gần dòng 254) thành:

```tsx
            onClick={clearSelection}
```

- [ ] **Step 6: Kiểm tra kiểu**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: 0 lỗi.

- [ ] **Step 7: Smoke test thủ công (chưa cần Zalo thật — chỉ xác nhận KHÔNG regress phần đã có)**

Run: `cd GO && wails dev` — chờ app mở, xử lý thử 1-2 file đơn hàng có sẵn trong thư mục `đơn hàng/`, tick chọn 1 PO, xác nhận:
- Checkbox tick/bỏ tick vẫn hoạt động đúng như trước (kiểm tra bằng mắt, so với hành vi hiện tại trước khi sửa).
- Nút "Xem nội dung Zalo" vẫn mở đúng modal, nội dung không đổi.
- Nếu có PO bị sai giá, bấm áp dụng giá PO/hệ thống vẫn cập nhật đúng DonGia (không vỡ do đổi `effectivePrice`→`resolveEffectivePrice`).

- [ ] **Step 8: Commit**

```bash
git add GO/frontend/src/components/ResultTable.tsx
git commit -m "refactor(frontend): source PO selection + price-basis choice from appStore"
```

---

### Task 7: `ControlPanel.tsx` — đổi nút to thành nút gửi Zalo khi có chọn

**Files:**
- Modify: `GO/frontend/src/components/ControlPanel.tsx`

**Interfaces:**
- Consumes: `appStore`'s `selectedPOs`/`rows`/`clearSelection`/`resolvedChoice` (Task 5), `buildZaloMessageForPO`/`buildPriceBasisForPO` (Task 5/đã có), `SendZaloMessages` (Task 4, sinh binding qua `wailsjs/go/main/App` — xem Step 1).

- [ ] **Step 1: Sinh binding Wails cho `SendZaloMessages`**

Run: `cd GO && wails dev` rồi tắt ngay sau khi app mở xong (Ctrl+C) — Wails tự quét `app.go`, sinh lại `GO/frontend/wailsjs/go/main/App.d.ts`/`App.js` với hàm `SendZaloMessages` mới (giống cách `ProcessFiles`/`ConfirmPrice` đã có sẵn trong 2 file này).

Run: `grep -n "SendZaloMessages" GO/frontend/wailsjs/go/main/App.d.ts`
Expected: có 1 dòng khai báo `export function SendZaloMessages(...)`.

- [ ] **Step 2: Sửa `ControlPanel.tsx`**

Thay toàn bộ nội dung `GO/frontend/src/components/ControlPanel.tsx`:

```tsx
import { useEffect } from 'react'
import { FaPaperPlane, FaCloudArrowUp, FaRocket, FaSpinner } from 'react-icons/fa6'
import { useAppStore } from '../store/appStore'
import { GetSTT, ProcessFiles, SendZaloMessages } from '../../wailsjs/go/main/App'
import { buildZaloMessageForPO, buildPriceBasisForPO } from '../lib/zaloMessage'

export function ControlPanel() {
  const stt = useAppStore((s) => s.stt)
  const setStt = useAppStore((s) => s.setStt)
  const files = useAppStore((s) => s.files)
  const isProcessing = useAppStore((s) => s.isProcessing)
  const setProcessing = useAppStore((s) => s.setProcessing)
  const appendLog = useAppStore((s) => s.appendLog)
  const resetRows = useAppStore((s) => s.resetRows)
  const rows = useAppStore((s) => s.rows)
  const selectedPOs = useAppStore((s) => s.selectedPOs)
  const resolvedChoice = useAppStore((s) => s.resolvedChoice)

  useEffect(() => {
    GetSTT()
      .then(setStt)
      .catch((err) => appendLog(`❌ Lỗi đọc STT: ${String(err)}`))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

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

  // rowsForPO cục bộ - CHỈ cần đọc, không cần state riêng - lặp toàn
  // bộ rows tìm đúng những dòng thuộc PO này (khớp cách
  // ResultTable.tsx's rowsForPO đã dùng, không tái sử dụng trực tiếp vì
  // hàm đó vẫn là closure riêng của ResultTable.tsx, xem Task 6).
  function rowsForPO(po: string): number[] {
    return rows.reduce<number[]>((acc, row, idx) => {
      if (row.po === po) acc.push(idx)
      return acc
    }, [])
  }

  async function handleSendZalo() {
    const jobs = [...selectedPOs].map((po) => {
      const indices = rowsForPO(po)
      const poRows = indices.map((idx) => rows[idx])
      const priceBasis = buildPriceBasisForPO(rows, indices, resolvedChoice)
      const processedAt = new Date().toLocaleTimeString('vi-VN', { hour12: false })
      return {
        po,
        system: poRows[0]?.system ?? '',
        message: buildZaloMessageForPO(poRows, processedAt, priceBasis),
      }
    })
    appendLog(`📨 Bắt đầu gửi ${jobs.length} tin Zalo...`)
    try {
      await SendZaloMessages(jobs)
    } catch (err) {
      appendLog(`❌ Lỗi gửi Zalo: ${String(err)}`)
    }
  }

  const hasSelection = selectedPOs.size > 0

  return (
    <section className="flex flex-shrink-0 items-center gap-3 rounded-xl border border-border bg-panel px-4 py-3">
      <div
        title="Sẽ có ở giai đoạn sau"
        className="flex items-center gap-2 rounded-lg border border-dashed border-border px-3 py-2 text-xs font-medium text-muted opacity-60"
      >
        <FaCloudArrowUp /> Push MISA
        <span className="rounded-full bg-white/5 px-1.5 py-0.5 font-mono text-[8px] font-bold tracking-wide">
          SẮP RA MẮT
        </span>
      </div>
      {hasSelection ? (
        <button
          onClick={handleSendZalo}
          className="ml-auto inline-flex items-center justify-center gap-2 rounded-lg px-5 py-2.5 text-sm font-extrabold tracking-wide text-white transition-transform hover:brightness-110 active:scale-[0.98]"
          style={{ backgroundColor: '#0068FF' }}
        >
          <FaPaperPlane /> GỬI {selectedPOs.size} TIN ZALO
        </button>
      ) : (
        <button
          onClick={handleProcess}
          disabled={isProcessing}
          className={`ml-auto inline-flex items-center justify-center gap-2 rounded-lg bg-gradient-to-br from-accent to-[#1a9dc4] px-5 py-2.5 text-sm font-extrabold tracking-wide text-[#0a1620] transition-transform hover:brightness-110 active:scale-[0.98] disabled:opacity-60 ${
            !isProcessing ? 'animate-pulse-glow' : ''
          }`}
        >
          {isProcessing ? (
            <>
              <FaSpinner className="animate-spin" /> ĐANG XỬ LÝ...
            </>
          ) : (
            <>
              <FaRocket /> XỬ LÝ ĐƠN HÀNG
            </>
          )}
        </button>
      )}
    </section>
  )
}
```

(Placeholder "Gửi Zalo"/"SẮP RA MẮT" cũ bị xoá hoàn toàn — chức năng chuyển hẳn sang nút to, giữ lại đúng placeholder "Push MISA".)

- [ ] **Step 3: Kiểm tra kiểu**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: 0 lỗi.

- [ ] **Step 4: Commit**

```bash
git add GO/frontend/src/components/ControlPanel.tsx GO/frontend/wailsjs
git commit -m "feat(frontend): swap main action button to real Zalo send when POs selected"
```

---

### Task 8: `useWailsEvents.ts` — lắng nghe `zalo:log`/`zalo:sent`/`zalo:done`

**Files:**
- Modify: `GO/frontend/src/hooks/useWailsEvents.ts`

**Interfaces:**
- Consumes: sự kiện `zalo:log`/`zalo:sent`/`zalo:done` (Task 4), `appendLog`/`clearSelection` (Task 5/đã có).

- [ ] **Step 1: Thêm listener**

Sửa `GO/frontend/src/hooks/useWailsEvents.ts` — thêm hook store cần dùng (đầu hàm, cạnh các hook hiện có, sau dòng 12):

```ts
  const clearSelection = useAppStore((s) => s.clearSelection)
```

Sửa `useEffect` (thêm 3 listener mới, ngay sau `offLock`, trước `OnFileDrop(...)`, khoảng dòng 22-27):

```ts
    const offLock = EventsOn('applock:status', (status: LockStatus) => setLockStatus(status))
    const offZaloLog = EventsOn('zalo:log', (line: string) => appendLog(line))
    // zalo:sent (po/ok) được phát nhưng CHƯA có UI trạng thái riêng ở
    // v1 (xem spec/Phạm vi) - không lắng nghe ở đây, chỉ zalo:log/done.
    const offZaloDone = EventsOn('zalo:done', () => clearSelection())
    OnFileDrop(() => {}, false)
```

Sửa `return` cleanup (dòng 29-36) để gọi thêm 2 hàm off mới:

```ts
    return () => {
      offLog()
      offRow()
      offDone()
      offDrop()
      offLock()
      offZaloLog()
      offZaloDone()
      OnFileDropOff()
    }
```

Sửa mảng dependency của `useEffect` (dòng 37) thêm `clearSelection`:

```ts
  }, [appendLog, appendRow, setProcessing, setStt, addFiles, setLockStatus, clearSelection])
```

- [ ] **Step 2: Kiểm tra kiểu**

Run: `cd GO/frontend && npx tsc --noEmit`
Expected: 0 lỗi.

- [ ] **Step 3: Commit**

```bash
git add GO/frontend/src/hooks/useWailsEvents.ts
git commit -m "feat(frontend): wire zalo:log/zalo:done events into log panel + selection reset"
```

---

### Task 9: Xác nhận end-to-end thật (thủ công, cần tài khoản Zalo thật)

**Files:** không sửa file nào — chỉ chạy và quan sát.

**Interfaces:** không có (task xác nhận, không sinh interface mới).

- [ ] **Step 1: Xác nhận `.gitignore` có hiệu lực TRƯỚC khi chạy**

Run: `git check-ignore -v zalo_profile/anything` (thư mục chưa tồn tại vẫn check được pattern; đây là đường dẫn REPO ROOT, không phải trong `GO/` — xem giải thích ở Task 1)
Expected: in ra dòng khớp `.gitignore:<n>:zalo_profile/`.

- [ ] **Step 2: Chạy app, xử lý 1 file đơn hàng thật có PO hợp lệ**

Run: `cd GO && wails dev`
- Kéo/chọn 1 file đơn hàng thật (hoặc file test có sẵn trong `đơn hàng/`), bấm "XỬ LÝ ĐƠN HÀNG".
- Xác nhận `system` của PO đó ĐÃ có mapping trong Cài đặt > tab Zalo (nếu chưa, thêm 1 mapping trỏ tới 1 hội thoại Zalo TEST — không dùng nhóm khách hàng thật cho lần thử đầu).

- [ ] **Step 3: Tick chọn PO, xác nhận nút đổi**

Tick checkbox của PO đó trong `ResultTable.tsx` — xác nhận nút to ở `ControlPanel` đổi từ "XỬ LÝ ĐƠN HÀNG" sang "GỬI 1 TIN ZALO" (nền xanh Zalo).

- [ ] **Step 4: Bấm gửi, xác nhận luồng đăng nhập/gửi thật**

Bấm nút "GỬI 1 TIN ZALO":
- Lần đầu: 1 cửa sổ Chrome mới hiện ra, điều hướng `chat.zalo.me`, yêu cầu quét QR — quét bằng điện thoại đã đăng nhập Zalo.
- `LogPanel` hiện `🔐 Đang kiểm tra đăng nhập...` rồi `📤 Đang gửi PO... → <tên liên hệ>...`.
- Xác nhận tin nhắn THẬT xuất hiện trong đúng hội thoại Zalo test (đúng nội dung, khớp những gì modal "Xem nội dung Zalo" đã hiển thị trước đó).
- `LogPanel` hiện `✅ Đã gửi <PO>`.
- Sau khi xong, selection tự bỏ chọn (nút quay lại "XỬ LÝ ĐƠN HÀNG").

- [ ] **Step 5: Gửi lần 2 trong cùng phiên app — xác nhận KHÔNG cần quét lại QR**

Xử lý/tick chọn 1 PO khác, bấm gửi lần nữa — xác nhận KHÔNG bị yêu cầu quét QR lại (session giữ nguyên qua `ProfileDir`), tin gửi thành công bình thường.

- [ ] **Step 6: Thử trường hợp lỗi — hệ thống chưa có mapping Zalo**

Chọn 1 PO có `system` CHƯA có trong Cài đặt > Zalo (hoặc tạm xoá mapping đi) — bấm gửi, xác nhận `LogPanel` hiện đúng dòng lỗi "chưa cấu hình liên hệ Zalo cho ..." và KHÔNG làm treo/lỗi các job khác trong cùng lượt gửi (nếu gửi nhiều PO cùng lúc, PO còn lại vẫn gửi được).

- [ ] **Step 7: Xác nhận `zalo_profile/` không bị git track**

Run: `git status --short`
Expected: `zalo_profile/` (ở gốc repo — xác nhận bằng `ls zalo_profile` là thư mục đã có thật trên đĩa) KHÔNG xuất hiện trong danh sách output của `git status`.

- [ ] **Step 8: Đóng app, xác nhận không còn tiến trình Chrome mồ côi**

Đóng cửa sổ app chính — kiểm tra Task Manager, xác nhận tiến trình `chrome.exe` được mở bởi `wails dev`/app cũng đã đóng theo (không còn treo nền).
