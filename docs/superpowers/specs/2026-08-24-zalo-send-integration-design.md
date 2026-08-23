# Gửi tin Zalo thật từ GO app (chromedp) — Thiết kế

## Mục tiêu

Nút "Gửi Zalo" ở `ControlPanel.tsx` hiện là placeholder disabled
("SẮP RA MẮT") — phần xem trước nội dung tin nhắn
(`OrderContentModal.tsx` + `zaloMessage.ts`, nút "Xem nội dung Zalo" ở
`ResultTable.tsx`) đã xây xong và đang hoạt động, nhưng chưa có gì gửi
tin thật cả.

Mục tiêu: khi người dùng tick chọn 1+ PO trong bảng kết quả, nút to ở
`ControlPanel` (hiện là "XỬ LÝ ĐƠN HÀNG") đổi thành nút gửi tin Zalo;
bấm vào sẽ gửi thật nội dung đã build (giống hệt nội dung xem trước)
tới đúng liên hệ/nhóm Zalo tương ứng từng hệ thống (`system` field),
bằng cách điều khiển 1 trình duyệt Chrome thật (qua `chromedp`, viết
bằng Go — không phụ thuộc Python/Playwright của
`sendmessage_zalo/send_message.py` nữa, dù logic tham khảo lại từ đó).

## Bối cảnh kỹ thuật hiện tại

- **`sendmessage_zalo/send_message.py`** (Python, Playwright, đã chạy
  thật, không phải phần sửa trong thiết kế này — chỉ tham khảo logic):
  mở `chromium.launch_persistent_context(user_data_dir=...)` để giữ
  đăng nhập qua các lần chạy; lần đầu chưa đăng nhập
  (`"id.zalo.me" in page.url`) thì chờ tối đa 120s để người dùng quét
  QR; tìm hội thoại bằng cách gõ vào `#contact-search-input`, chờ
  `.txt-highlight` xuất hiện rồi click; gõ nội dung vào `#richInput`
  (mỗi dòng `\n` → `Shift+Enter`); bấm Enter, xác nhận gửi thành công
  bằng cách bắt response HTTP có URL chứa `message/sms` và body chứa
  `"error_code":0`. `rich_paste_engine.py` (chế độ `--rich`, dựng HTML
  rồi giả lập sự kiện `paste`) — **không port trong thiết kế này**, xem
  Phạm vi.
- **`GO/frontend/src/lib/zaloMessage.ts`**: `buildZaloMessageForPO(rows,
  processedAt, priceBasisBySku)` build sẵn đúng nội dung text thuần
  (không có cú pháp markup nào) cho 1 PO — đây là hàm DUY NHẤT tạo nội
  dung tin nhắn, `OrderContentModal.tsx` (modal xem trước, đã xong)
  đang gọi hàm này để hiển thị. Thiết kế này KHÔNG viết thêm 1 nơi build
  text thứ 2 ở Go — nội dung gửi đi luôn là nội dung frontend đã build
  và người dùng đã thấy trong modal xem trước.
- **`GO/frontend/src/components/ResultTable.tsx`**: `selectedPOs:
  Set<string>` hiện là `useState` cục bộ trong component này (dòng
  ~103), điều khiển checkbox từng dòng + nút "Xem nội dung Zalo".
  `ControlPanel.tsx` không có quyền đọc state này.
- **`GO/frontend/src/components/ControlPanel.tsx`**: nút "Gửi Zalo"
  hiện là 1 `div` disabled tĩnh (dòng 44-52), không có handler.
- **`GO/internal/appsettings`**: `Settings.Zalo map[string]string` map
  tên hệ thống (khớp `OrderRow.system`, ví dụ `MNCOOPMART`) → tên hiển
  thị liên hệ/nhóm Zalo thật (ví dụ `"Đơn hàng Co-op Miền Nam"`) — ĐÃ
  có sẵn, sửa được qua popup Cài đặt > tab Zalo
  (`SettingsModal.tsx`/`KeyValueEditor.tsx`). Đây chính là tham số
  `contact_query` mà `send_message.py` nhận qua command-line.
- **Pattern batch chạy nền + phát sự kiện** đã có sẵn ở `app.go`:
  `ProcessFiles` → `a.processing atomic.Bool` chặn chạy trùng →
  `go a.runBatch(...)` → phát `process:log`/`process:row`/`process:done`
  qua `Emitter` interface (`wailsEmitter` bọc `runtime.EventsEmit`,
  test được bằng fake `Emitter`) → `useWailsEvents.ts` lắng nghe, đổ vào
  `appStore`. Thiết kế này dùng lại nguyên pattern này cho việc gửi
  Zalo, không phát minh cơ chế mới.
- **Sự cố rò rỉ session Zalo trước đây**: `zalo_state.json` (cookie
  phiên đăng nhập Zalo Web) từng bị commit nhầm vào git, đã xử lý phần
  khẩn cấp (session bị vô hiệu hoá, file được untrack) nhưng lịch sử git
  trên `origin` vẫn còn — xem chi tiết trong ghi nhớ dự án liên quan.
  **Bài học áp dụng trực tiếp vào thiết kế này**: thư mục profile trình
  duyệt mới (`zalo_profile/`, sẽ chứa cookie phiên đăng nhập thật
  ngay khi người dùng quét QR lần đầu) PHẢI được thêm vào
  `.gitignore` **trước khi** bất kỳ ai chạy tính năng này lần đầu — xem
  mục Rủi ro.

## Kiến trúc

### 1. Package mới `internal/zalosend`

```go
// GO/internal/zalosend/sender.go
package zalosend

import "context"

// ZaloSender trừu tượng hoá việc gửi 1 tin nhắn Zalo, để app.go test
// được logic vòng lặp gửi hàng loạt (SendZaloMessages) bằng fake sender,
// không cần trình duyệt thật — cùng lý do processing.Processor là
// interface (xem app.go hiện có).
type ZaloSender interface {
	// EnsureLoggedIn mở trình duyệt (nếu chưa mở) và đảm bảo đã đăng
	// nhập chat.zalo.me, chờ quét QR nếu cần (tối đa loginTimeout).
	// Gọi 1 LẦN trước cả batch, không gọi lại cho từng tin (khác với
	// send_message.py, vốn coi login là 1 phần của mỗi lần chạy script
	// riêng lẻ — ở đây trình duyệt sống xuyên suốt vòng đời App).
	EnsureLoggedIn(ctx context.Context) error

	// SendMessage tìm đúng hội thoại theo contactQuery (tên liên hệ/nhóm
	// hiển thị trên Zalo) rồi gửi message dạng text thuần (mỗi dòng
	// message thành 1 dòng trong ô soạn tin, giống send_message
	// (không phải send_pasted_message) bên Python — xem Phạm vi vì sao
	// không port chế độ rich).
	SendMessage(ctx context.Context, contactQuery, message string) error

	// Close đóng trình duyệt — gọi lúc App shutdown.
	Close() error
}
```

```go
// GO/internal/zalosend/chromedp_sender.go
package zalosend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// ChromedpSender là cài đặt thật của ZaloSender, dùng chromedp điều
// khiển 1 Chrome thật (không headless — cần hiện cửa sổ để quét QR lần
// đầu, và để người dùng quan sát/can thiệp nếu có gì bất thường, đúng
// hành vi mặc định của send_message.py khi KHÔNG truyền --headless).
type ChromedpSender struct {
	ProfileDir string // thư mục user-data-dir riêng, giữ đăng nhập qua các lần chạy app

	mu      sync.Mutex
	allocCancel context.CancelFunc
	browserCancel context.CancelFunc
	browserCtx context.Context
}

// EnsureLoggedIn — mở allocator + context 1 lần (idempotent, các lần
// gọi sau chỉ kiểm tra lại URL hiện tại, không mở trình duyệt mới),
// điều hướng chat.zalo.me, nếu URL rơi vào id.zalo.me thì chờ tối đa
// 120s (khớp send_message.py) cho tới khi người dùng quét QR xong và
// URL chuyển về chat.zalo.me.
func (c *ChromedpSender) EnsureLoggedIn(ctx context.Context) error {
	// ... mở NewExecAllocator với chromedp.UserDataDir(c.ProfileDir),
	// chromedp.Flag("headless", false); Navigate; kiểm tra Location();
	// nếu cần đăng nhập, poll Location() mỗi ~1s tới khi đổi hoặc hết
	// 120s (chromedp không có wait_for_url built-in tương đương Playwright,
	// polling thủ công đơn giản hơn dùng đúng — xác nhận lại lúc code).
}

// SendMessage — Click + SendKeys vào #contact-search-input, chờ
// ".txt-highlight" VISIBLE rồi Click thẳng vào selector đó (chromedp tự
// scroll+click, KHÔNG cần tính toạ độ tay như send_message.py —
// send_message.py tính toạ độ vì lý do lịch sử/độ tin cậy riêng của
// Playwright script đó, chromedp.Click(sel, chromedp.NodeVisible) đã đủ
// tin cậy cho use case tương tự trong hệ sinh thái chromedp). Chờ
// "#richInput" visible, click, gõ từng dòng của message (Shift+Enter
// giữa các dòng), rồi gọi waitForSendConfirmation trước khi bấm Enter
// cuối để gửi.
func (c *ChromedpSender) SendMessage(ctx context.Context, contactQuery, message string) error {
	// ...
}

// waitForSendConfirmation bật network.Enable() 1 lần, lắng nghe
// network.EventResponseReceived qua chromedp.ListenTarget, khớp URL
// chứa "message/sms", lấy body qua network.GetResponseBody(requestID),
// xác nhận chứa `"error_code":0` — tương đương page.expect_response
// bên Playwright, nhưng chromedp không có API chờ-response tiện lợi
// tương đương nên phần này cần channel + timeout tự viết tay. ĐÂY LÀ
// PHẦN KỸ THUẬT CHƯA CHẮC CHẮN NHẤT của thiết kế — xem Rủi ro.
func waitForSendConfirmation(ctx context.Context, browserCtx context.Context, timeout time.Duration) error {
	// ...
}

func (c *ChromedpSender) Close() error {
	// cancel browserCtx rồi allocCtx, theo đúng thứ tự chromedp yêu cầu.
}
```

### 2. Resolve liên hệ theo `system`

```go
// GO/internal/zalosend/sender.go (tiếp)

// ErrNoContact báo hệ thống chưa có mapping Zalo trong settings — job
// tương ứng bị SKIP (không dừng cả batch), người dùng sửa qua Cài đặt >
// tab Zalo rồi thử gửi lại.
var ErrNoContact = errors.New("zalosend: no zalo contact configured for this system")

func ResolveContact(system string, zaloMap map[string]string) (string, error) {
	contact, ok := zaloMap[system]
	if !ok || contact == "" {
		return "", fmt.Errorf("%w: %s", ErrNoContact, system)
	}
	return contact, nil
}
```

### 3. `app.go` — `SendZaloMessages`

Frontend đã build sẵn text từng PO (bằng `buildZaloMessageForPO`, y hệt
modal xem trước) — Go chỉ nhận `{po, system, message}` từng job, resolve
liên hệ, gửi tuần tự, phát sự kiện tiến trình.

```go
// GO/app.go — thêm

type ZaloJob struct {
	PO      string `json:"po"`
	System  string `json:"system"`
	Message string `json:"message"`
}

// App struct thêm 2 field mới:
//   zaloSender zalosend.ZaloSender
//   sending    atomic.Bool

// SendZaloMessages gửi tuần tự từng job (KHÔNG song song — cùng 1 trang
// trình duyệt, giống lý do handleApplyAll ở ResultTable.tsx gọi
// ConfirmPrice tuần tự chứ không đồng thời). Đăng nhập được đảm bảo 1
// LẦN trước cả batch (EnsureLoggedIn), không chờ QR riêng cho từng tin —
// nếu bước này lỗi/timeout, huỷ toàn bộ batch (không gửi được tin nào
// nếu chưa đăng nhập, khác với lỗi resolve-contact/gửi-từng-tin, vốn chỉ
// skip đúng job đó).
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

`NewApp()` khởi tạo `zaloSender: &zalosend.ChromedpSender{ProfileDir:
filepath.Join(resolveRepoDir("settings.ini"), "zalo_profile")}` (theo
đúng pattern `resolveRepoDir` đã dùng cho `excelPath`/`orderDir` — ổn
định giữa `wails dev` và bản `.exe` đã build). `startup()` hoặc 1 hàm
`shutdown(ctx)` mới (Wails hỗ trợ hook này, app hiện chưa dùng) gọi
`a.zaloSender.Close()` lúc app đóng.

### 4. Frontend — nâng `selectedPOs` lên `appStore`

```ts
// GO/frontend/src/store/appStore.ts — thêm vào AppState
selectedPOs: Set<string>
togglePOSelection: (po: string) => void
toggleAllPOs: (allPOs: string[], checked: boolean) => void
clearSelection: () => void
```

`ResultTable.tsx` đổi `useState<Set<string>>` (dòng 103) và
`togglePOSelection`/`toggleAllPOs` (dòng 203-215) sang gọi store —
UI/JSX không đổi, chỉ đổi nguồn state. `resetRows()`'s effect (dòng
126-134, clear state khi batch mới bắt đầu) gọi thêm `clearSelection()`
từ store thay vì `setSelectedPOs(new Set())` cục bộ.

### 5. `ControlPanel.tsx` — đổi nút theo `selectedPOs`

```tsx
const selectedPOs = useAppStore((s) => s.selectedPOs)
const rows = useAppStore((s) => s.rows)
const appendLog = useAppStore((s) => s.appendLog)
const hasSelection = selectedPOs.size > 0

async function handleSendZalo() {
  const jobs = [...selectedPOs].map((po) => {
    const poRows = rows.filter((r) => r.po === po)
    return { po, system: poRows[0]?.system ?? '', message: buildZaloMessageForPO(poRows, /* processedAt */ '', /* priceBasisBySku */ {}) }
  })
  await SendZaloMessages(jobs)
}
```

Nút to cuối cùng (dòng 62-78 hiện tại) đổi thành:

```tsx
{hasSelection ? (
  <button onClick={handleSendZalo} style={{ backgroundColor: '#0068FF' }} className="ml-auto ...">
    <FaPaperPlane /> GỬI {selectedPOs.size} TIN ZALO
  </button>
) : (
  <button onClick={handleProcess} disabled={isProcessing} className="ml-auto ...">
    ... (giữ nguyên như hiện tại)
  </button>
)}
```

Placeholder "Gửi Zalo"/"SẮP RA MẮT" (dòng 44-52) bị xoá — chức năng
chuyển hẳn sang nút to, không còn 2 chỗ nói về "gửi Zalo" cùng lúc.
`priceBasisBySku`/`processedAt` chính xác cần truyền gì (lấy từ đâu,
có cần giữ lại `receivedAtRef`/`resolvedChoice` hiện đang sống trong
`ResultTable.tsx` hay không) quyết định lúc viết kế hoạch triển khai
chi tiết — đây chỉ là sketch kiến trúc.

### 6. `useWailsEvents.ts` — 3 sự kiện mới

```ts
const offZaloLog = EventsOn('zalo:log', (line: string) => appendLog(line))
const offZaloSent = EventsOn('zalo:sent', (r: { po: string; ok: boolean }) => { /* xem Phạm vi — không có UI trạng thái riêng ở v1, chỉ log */ })
const offZaloDone = EventsOn('zalo:done', () => clearSelection())
```

`zalo:sent` được phát nhưng v1 CHƯA có UI nào đọc nó ngoài log text
(xem Phạm vi) — phát sẵn để việc thêm UI trạng thái từng PO sau này
không cần sửa lại Go, chỉ cần frontend lắng nghe thêm.

### 7. `go.mod`

Thêm `github.com/chromedp/chromedp` (kéo theo
`github.com/chromedp/cdproto`). Yêu cầu máy chạy app đã cài Chrome/Edge
(chromedp tự tìm binary Chrome hệ thống theo mặc định trên Windows) —
chấp nhận được, đây là công cụ nội bộ chạy trên máy đã biết trước.

## Phạm vi

### Làm thật

- Package `internal/zalosend`: interface `ZaloSender`, cài đặt
  `ChromedpSender` (đăng nhập + giữ phiên qua `user-data-dir` riêng, tìm
  hội thoại, gõ + gửi text thuần, xác nhận qua response `message/sms`),
  `ResolveContact`.
- `App.SendZaloMessages` + `zalo:log`/`zalo:sent`/`zalo:done`, đăng
  nhập 1 lần trước cả batch, gửi tuần tự, lỗi từng job chỉ skip job đó.
- Nâng `selectedPOs` lên `appStore`, đổi nút to ở `ControlPanel` theo
  state đó, xoá placeholder "SẮP RA MẮT".
- Thêm `zalo_profile/` (hoặc tên tương đương) vào `.gitignore`
  **trước khi** tính năng này chạy lần đầu trên bất kỳ máy nào.

### Không làm (YAGNI)

- **Không port `rich_paste_engine.py`** (chế độ `--rich`, dán HTML định
  dạng) — nội dung `buildZaloMessageForPO` hiện tại là text thuần,
  không có cú pháp markup nào cần dịch. Nếu sau này nội dung tin nhắn
  cần định dạng (đậm/màu/list), port phần này là 1 việc riêng.
- **Không headless** — luôn hiện cửa sổ Chrome (cần cho QR lần đầu +
  quan sát), không thêm tuỳ chọn ẩn cửa sổ ở v1.
- **Không gửi song song nhiều tin cùng lúc** — tuần tự, 1 trình duyệt.
- **Không có UI trạng thái gửi riêng từng PO** (spinner/tick trong modal
  xem trước) — v1 chỉ có log text qua `LogPanel` (`zalo:log`) và tự
  động bỏ chọn khi xong (`zalo:done`). `zalo:sent` được phát sẵn cho
  UI này thêm sau, nhưng không xây ở v1.
- **Không retry tự động** khi 1 job gửi thất bại — người dùng tick chọn
  lại PO đó và bấm gửi lại.
- **Không đóng/mở lại trình duyệt giữa các lần gửi trong cùng phiên
  app** — mở 1 lần (lazy, lúc `SendZaloMessages` đầu tiên được gọi),
  sống tới khi app đóng.

## Rủi ro / lưu ý

- **`zalo_profile/` PHẢI vào `.gitignore` trước khi tính năng chạy
  lần đầu** — thư mục này sẽ chứa cookie phiên đăng nhập Zalo Web thật
  ngay sau lần quét QR đầu tiên. Repo này từng rò rỉ đúng loại dữ liệu
  này qua `zalo_state.json` (xem ghi nhớ dự án "Zalo session leak
  incident") — không được lặp lại. Việc thêm dòng gitignore này làm
  TRƯỚC, độc lập với phần còn lại của code, verify bằng `git status`
  sau khi thư mục được tạo lần đầu (`wails dev` test thật) để chắc chắn
  nó không xuất hiện trong danh sách untracked.
- **`waitForSendConfirmation` (xác nhận gửi thành công qua network
  response) là phần kỹ thuật chưa chắc chắn nhất** — chromedp không có
  API tiện lợi tương đương `page.expect_response` của Playwright; cần
  tự viết `chromedp.ListenTarget` + channel + timeout. Nếu cách này
  chứng minh không ổn định lúc triển khai, phương án dự phòng: chờ 1
  khoảng thời gian cố định rồi kiểm tra bong bóng tin nhắn CUỐI CÙNG
  trong khung chat có khớp nội dung vừa gửi hay không (kiểm tra DOM
  thay vì network) — kém chính xác hơn nhưng đơn giản hơn nhiều.
- **Chọn lựa hội thoại qua `.txt-highlight` có thể fragile giống hệt
  bản Python** — đây là DOM nội bộ không chính thức của Zalo Web, có
  thể đổi bất cứ lúc nào Zalo cập nhật giao diện. Rủi ro đã tồn tại sẵn
  ở bản Python, không phải rủi ro MỚI do việc port sang Go.
  `waitForSendConfirmation`/`SendMessage` nên trả lỗi rõ ràng (không
  silent fail) để `zalo:log` báo đúng job nào thất bại, thay vì cả batch
  treo.
- **Không kiểm thử được luồng chromedp thật bằng automated test** (cần
  Chrome thật + tài khoản Zalo thật + tương tác quét QR thủ công) —
  chấp nhận, verify bằng `wails dev` thủ công (xem Kiểm thử).
- **`sendmessage_zalo/zalo.rar`** (file nén chưa rõ nội dung, đang
  untracked) không nằm trong phạm vi thiết kế này — không đụng tới, chỉ
  ghi nhận nó tồn tại.

## Kiểm thử

- `internal/zalosend`: `TestResolveContact_Found`/`_NotConfigured`
  (map thường + rỗng).
- `app.go` (`runZaloBatch`): test bằng fake `ZaloSender` (implement
  interface, trả lỗi/thành công theo kịch bản) + fake `Emitter` (đã có
  sẵn pattern test cho `runBatch`, xem `app_test.go`) —
  `TestRunZaloBatch_SkipsJobWithoutContact`,
  `TestRunZaloBatch_ContinuesAfterOneJobFails`,
  `TestRunZaloBatch_AbortsWholeBatchIfLoginFails`,
  `TestRunZaloBatch_RejectsWhileAlreadySending` (guard
  `atomic.Bool`, mirror pattern test hiện có cho `a.processing` nếu có).
- Frontend: verify bằng `wails dev` thật — tick chọn PO, xác nhận nút
  to đổi thành "GỬI TIN ZALO", bấm gửi, quan sát `LogPanel` hiện tiến
  trình, xác nhận tin thật tới đúng hội thoại Zalo test, xác nhận
  `zalo_profile/` được tạo và KHÔNG xuất hiện trong `git status`.
