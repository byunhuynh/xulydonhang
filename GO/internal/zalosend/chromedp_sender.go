package zalosend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"order-processor/internal/zalosend/richtext"
)

const (
	loginTimeout       = 120 * time.Second
	sendConfirmTimeout = 15 * time.Second

	// browserOpTimeout là hạn giờ mặc định cho MỘT lượt tương tác trình
	// duyệt (mở trang, tìm hội thoại + dán + gửi 1 tin) khi caller không
	// tự đặt deadline. Không có nó, một chromedp.WaitVisible chờ selector
	// mà DOM Zalo đã đổi (rủi ro đã ghi nhận với `.txt-highlight`/
	// `#richInput`) sẽ block VĨNH VIỄN: a.sending (app.go) không bao giờ
	// được trả về false và nút gửi chết cho tới khi khởi động lại app.
	// KHÔNG áp cho vòng chờ quét QR 120s trong EnsureLoggedIn — đó là
	// khoảng chờ người dùng CỐ Ý, không phải treo.
	browserOpTimeout = 90 * time.Second
)

var indentClicksPerLevel = map[string]int{
	"bullet":   1,
	"numbered": 3,
}

// ChromedpSender là cài đặt thật của ZaloSender — port TRỰC TIẾP từ
// sendmessage_zalo/go/cmd/chromedp/main.go (đã tự chạy thật, xác nhận
// gửi tin thành công, KHÔNG dùng playwright-go). Khác biệt DUY NHẤT so
// với bản CLI gốc: bản gốc mở/đóng 1 browser context MỚI cho mỗi lần
// chạy (1 lệnh = 1 tin); ChromedpSender giữ browser SỐNG XUYÊN SUỐT
// nhiều lần SendMessage trong cùng 1 lượt gửi hàng loạt (EnsureLoggedIn
// mở + đăng nhập 1 lần, SendMessage chỉ làm phần openConversation +
// sendPastedMessage, Close đóng lúc App shutdown). Mọi logic tương tác
// DOM/CDP bên trong (selector, thời gian chờ, cách paste/gửi) giữ
// NGUYÊN không đổi so với bản đã test.
type ChromedpSender struct {
	ProfileDir string

	mu          sync.Mutex
	allocCancel context.CancelFunc
	ctx         context.Context
	cancel      context.CancelFunc
}

// ensureBrowser mở allocator + context 1 LẦN (idempotent) — port của
// phần đầu run() (main.go dòng 326-346), bỏ phần Navigate/login (chuyển
// sang EnsureLoggedIn).
func (c *ChromedpSender) ensureBrowser() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx != nil {
		return nil
	}
	if err := os.MkdirAll(c.ProfileDir, 0o755); err != nil {
		return fmt.Errorf("zalosend: tạo thư mục profile %s: %w", c.ProfileDir, err)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir(c.ProfileDir),
		chromedp.Flag("headless", false),
		chromedp.WindowSize(1280, 900),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	c.allocCancel = allocCancel
	c.ctx, c.cancel = ctx, cancel
	return nil
}

// browserContext snapshot c.ctx dưới c.mu thay vì để caller đọc trực
// tiếp field nhiều lần — Close() có thể chạy đồng thời (vd shutdown app
// trong lúc batch gửi vẫn còn đang chạy ở goroutine khác) và ghi
// c.ctx=nil dưới cùng 1 lock; đọc field trực tiếp không lock sẽ là data
// race và có thể truyền context nil vào chromedp.Run gây panic. Trả nil
// nếu trình duyệt chưa mở hoặc đã đóng — caller tự quyết thông báo lỗi.
func (c *ChromedpSender) browserContext() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ctx
}

// opTimeout lấy hạn giờ cho 1 lượt tương tác trình duyệt: ưu tiên phần
// thời gian còn lại của deadline caller đặt (runZaloBatch trong app.go
// đặt 1 deadline riêng cho mỗi job), ngược lại dùng mặc định
// browserOpTimeout. Trả về 0 nếu deadline đã trôi qua — context dẫn xuất
// sẽ hết hạn ngay, đúng ý caller là "đã hết giờ".
func opTimeout(ctx context.Context, fallback time.Duration) time.Duration {
	if ctx == nil {
		return fallback
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return fallback
	}
	remaining := time.Until(deadline)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// warmUpBrowser BẮT BUỘC chạy trước MỌI chromedp.Run có hạn giờ trên 1
// browser context vừa tạo. chromedp cấp phát tiến trình Chrome LƯỜI
// (lazy): lần Run ĐẦU TIÊN trên 1 context mới là lần thật sự khởi động
// Chrome, và vòng đời tiến trình đó bị buộc vào context của CHÍNH lần Run
// đầu tiên ấy. Nếu lần Run đầu tiên chạy trên 1 context.WithTimeout con
// thì `defer cancel()` của nó (luôn chạy khi hàm trả về, dù thành công
// hay hết giờ) sẽ giết luôn cả trình duyệt — đúng như tài liệu chromedp
// ghi: "it's generally a bad idea to use a context timeout on the first
// Run call, as it will stop the entire browser". Vì vậy ở đây ta gọi
// chromedp.Run KHÔNG kèm action nào trên context trình duyệt THÔ (không
// bọc timeout) để việc cấp phát xảy ra trên context sống lâu; sau đó mọi
// lần Run có hạn giờ đều an toàn.
func warmUpBrowser(browserCtx context.Context) error {
	if err := chromedp.Run(browserCtx); err != nil {
		return fmt.Errorf("zalosend: không khởi động được trình duyệt: %w", err)
	}
	return nil
}

// navigateToZalo chạy cặp EmulateViewport+Navigate mở đầu trên 1 context
// CÓ HẠN GIỜ dẫn xuất từ context trình duyệt. context.WithTimeout lồng
// trên chromedp context là cách chuẩn để chặn giờ 1 lời gọi mà KHÔNG phá
// huỷ cả browser context (cancel con không lan ngược lên cha) — VỚI ĐIỀU
// KIỆN trình duyệt đã được warmUpBrowser cấp phát trước đó.
func navigateToZalo(browserCtx context.Context, timeout time.Duration) error {
	runCtx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()
	return chromedp.Run(runCtx,
		chromedp.EmulateViewport(1280, 900),
		chromedp.Navigate("https://chat.zalo.me/"),
	)
}

// waitAndReadURL chờ trang ổn định rồi đọc URL hiện tại, cũng trên 1
// context có hạn giờ dẫn xuất từ context trình duyệt. Lỗi được TRẢ VỀ chứ
// không nuốt: nếu không đọc nổi URL thì ta KHÔNG biết đang ở trang đăng
// nhập hay trang chat, và coi chuỗi rỗng là "đã đăng nhập" chính là cách
// EnsureLoggedIn báo thành công giả trong khi trình duyệt đã chết.
func waitAndReadURL(browserCtx context.Context, timeout time.Duration) (string, error) {
	runCtx, cancel := context.WithTimeout(browserCtx, timeout)
	defer cancel()
	if err := chromedp.Run(runCtx, chromedp.Sleep(4*time.Second)); err != nil {
		return "", err
	}
	var curURL string
	if err := chromedp.Run(runCtx, chromedp.Location(&curURL)); err != nil {
		return "", err
	}
	return curURL, nil
}

// EnsureLoggedIn — port của phần Navigate + chờ QR trong run() (main.go
// dòng 347-373), không đổi logic/thời gian chờ (vòng chờ QR vẫn đúng
// loginTimeout=120s); chỉ bọc thêm hạn giờ cho các lời gọi tương tác
// trình duyệt và thêm 1 lần mở lại trình duyệt nếu nó đã bị đóng tay.
func (c *ChromedpSender) EnsureLoggedIn(ctx context.Context) error {
	if err := c.ensureBrowser(); err != nil {
		return err
	}

	browserCtx := c.browserContext()
	if browserCtx == nil {
		return fmt.Errorf("zalosend: trình duyệt đã bị đóng")
	}
	// PHẢI hâm nóng TRƯỚC navigateToZalo: ensureBrowser mới chỉ dựng
	// allocator + context chứ chưa chạy gì, nên nếu không có bước này thì
	// lần Run đầu tiên của cả vòng đời trình duyệt lại chính là lần Run có
	// hạn giờ trong navigateToZalo — và cancel của nó sẽ giết Chrome ngay
	// sau lần điều hướng đầu tiên (xem chú thích warmUpBrowser).
	if err := warmUpBrowser(browserCtx); err != nil {
		return err
	}
	timeout := opTimeout(ctx, browserOpTimeout)

	if navErr := navigateToZalo(browserCtx, timeout); navErr != nil {
		// Trình duyệt này CỐ Ý hiện cửa sổ (cần cho QR) nên người dùng tự
		// tay đóng nó giữa chừng là chuyện thường. Khi đó c.ctx vẫn khác
		// nil (ensureBrowser tưởng "đang mở") nhưng mọi lời gọi chromedp
		// đều hỏng vĩnh viễn. Coi Navigate hỏng = trình duyệt đã mất: reset
		// trạng thái bằng Close() rồi mở lại 1 trình duyệt mới và thử lại
		// ĐÚNG 1 LẦN (không lặp vô hạn).
		_ = c.Close()
		if reopenErr := c.ensureBrowser(); reopenErr != nil {
			return fmt.Errorf("zalosend: không mở được trang (%v) và mở lại trình duyệt cũng thất bại: %w", navErr, reopenErr)
		}
		browserCtx = c.browserContext()
		if browserCtx == nil {
			return fmt.Errorf("zalosend: trình duyệt đã bị đóng")
		}
		// Trình duyệt mới ⇒ lại là 1 context chromedp chưa từng Run: phải
		// hâm nóng lần nữa trước khi navigateToZalo bọc hạn giờ.
		if warmErr := warmUpBrowser(browserCtx); warmErr != nil {
			return fmt.Errorf("zalosend: không mở được trang (%v) và trình duyệt mở lại cũng không khởi động được: %w", navErr, warmErr)
		}
		if retryErr := navigateToZalo(browserCtx, timeout); retryErr != nil {
			return fmt.Errorf("zalosend: không mở được trang kể cả sau khi mở lại trình duyệt: %w", retryErr)
		}
	}

	curURL, err := waitAndReadURL(browserCtx, timeout)
	if err != nil {
		return fmt.Errorf("zalosend: không đọc được địa chỉ trang sau khi mở: %w", err)
	}
	if !strings.Contains(curURL, "id.zalo.me") {
		return nil
	}

	deadline := time.Now().Add(loginTimeout)
	for time.Now().Before(deadline) {
		// Vòng lặp này chạy trên browserCtx THÔ (không hạn giờ) vì 120s chờ
		// người dùng quét QR là khoảng chờ cố ý; nhưng lỗi Run ở đây thì
		// không phải "chưa quét xong" mà là trình duyệt đã chết/bị đóng —
		// nuốt nó sẽ quay 120 vòng vô ích rồi báo "hết giờ đăng nhập" sai.
		if err := chromedp.Run(browserCtx, chromedp.Location(&curURL)); err != nil {
			return fmt.Errorf("zalosend: mất kết nối trình duyệt trong lúc chờ đăng nhập: %w", err)
		}
		if strings.Contains(curURL, "chat.zalo.me") && !strings.Contains(curURL, "id.zalo.me") {
			chromedp.Run(browserCtx, chromedp.Sleep(3*time.Second))
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("zalosend: hết thời gian chờ đăng nhập (quét QR trong cửa sổ trình duyệt vừa mở rồi thử gửi lại)")
}

// SendMessage — port của phần openConversation+sendPastedMessage trong
// run() (main.go dòng 375-386), chỉ đổi cách báo lỗi: bản CLI gốc in ra
// console rồi return nil khi thất bại (chấp nhận được cho 1 lệnh chạy
// tay); ở đây PHẢI trả error thật để runZaloBatch (app.go, Task 4) log
// đúng job nào thất bại và tiếp tục job sau.
func (c *ChromedpSender) SendMessage(ctx context.Context, contactQuery, message string) error {
	browserCtx := c.browserContext()
	if browserCtx == nil {
		return fmt.Errorf("zalosend: chưa gọi EnsureLoggedIn")
	}

	// Chặn giờ cho TOÀN BỘ lượt gửi này (tìm hội thoại + dán + gửi) trên 1
	// context con của browser context — nếu 1 selector nào đó không còn
	// khớp DOM Zalo thì WaitVisible bên trong sẽ thất bại sau timeout thay
	// vì treo mãi mãi và làm kẹt cờ a.sending của app. Cancel context con
	// KHÔNG đóng trình duyệt: job sau vẫn dùng lại browserCtx bình thường.
	runCtx, cancel := context.WithTimeout(browserCtx, opTimeout(ctx, browserOpTimeout))
	defer cancel()

	opened, err := openConversation(runCtx, contactQuery)
	if err != nil {
		return fmt.Errorf("zalosend: tìm hội thoại %q: %w", contactQuery, err)
	}
	if !opened {
		return fmt.Errorf("zalosend: không tìm thấy hội thoại %q", contactQuery)
	}

	ok, body, err := sendPastedMessage(runCtx, message)
	if err != nil {
		return fmt.Errorf("zalosend: gửi tin tới %q: %w", contactQuery, err)
	}
	// Port của chromedp.Run(ctx, chromedp.Sleep(1*time.Second)) ngay
	// trước return nil trong run() (main.go dòng 398) — chạy sau MỌI lần
	// sendPastedMessage trả về không lỗi, bất kể ok true/false, trước khi
	// SendMessage return.
	chromedp.Run(runCtx, chromedp.Sleep(1*time.Second))
	if !ok {
		if len(body) > 300 {
			body = body[:300]
		}
		return fmt.Errorf("zalosend: Zalo báo lỗi khi gửi tới %q: %s", contactQuery, body)
	}
	return nil
}

// Close đóng trình duyệt — an toàn gọi nhiều lần, gọi khi chưa từng mở
// trình duyệt (c.ctx nil) cũng không lỗi.
//
// Dùng chromedp.Cancel (đường tắt shutdown "lịch sự" mà chromedp
// khuyến nghị) thay vì gọi thẳng cancel closure: nó CHỜ tiến trình
// Chrome thật sự thoát hẳn, không chỉ cắt context. Quan trọng vì Wails
// gọi hàm này trong OnShutdown rồi thoát tiến trình chính ngay sau đó —
// cắt context suông có thể để lại 1 Chrome mồ côi chưa kịp bị thu hồi.
// Vì chromedp.Cancel có thể block một chút, nó được gọi NGOÀI c.mu (sau
// khi đã snapshot + xoá các field dưới lock) để không chặn goroutine gửi
// đang cùng đọc c.ctx.
func (c *ChromedpSender) Close() error {
	c.mu.Lock()
	ctx := c.ctx
	cancel := c.cancel
	allocCancel := c.allocCancel
	c.ctx, c.cancel, c.allocCancel = nil, nil, nil
	c.mu.Unlock()

	if ctx == nil {
		return nil
	}

	err := chromedp.Cancel(ctx)
	if err != nil && cancel != nil {
		// Không thoát êm được thì cắt thẳng context để chắc chắn không rò
		// rỉ goroutine của chromedp.
		cancel()
	}
	if allocCancel != nil {
		allocCancel()
	}
	return err
}

// ---- Phần dưới đây port GẦN NHƯ NGUYÊN VĂN từ cmd/chromedp/main.go
// (đã chạy thật, xác nhận gửi tin thành công) — chỉ đổi import
// "zalosend/richtext" → "order-processor/internal/zalosend/richtext",
// KHÔNG đổi bất kỳ selector/thời gian chờ/logic tương tác nào.

func pressKeyCombo(ctx context.Context, mods input.Modifier, code, key string, vkey int64) error {
	down := input.DispatchKeyEvent(input.KeyDown).
		WithModifiers(mods).WithCode(code).WithKey(key).
		WithWindowsVirtualKeyCode(vkey).WithNativeVirtualKeyCode(vkey)
	up := input.DispatchKeyEvent(input.KeyUp).
		WithModifiers(mods).WithCode(code).WithKey(key).
		WithWindowsVirtualKeyCode(vkey).WithNativeVirtualKeyCode(vkey)
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error { return down.Do(ctx) }),
		chromedp.ActionFunc(func(ctx context.Context) error { return up.Do(ctx) }),
	)
}

func pressEnter(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.KeyEvent(kb.Enter))
}

type point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func ensureFormatMode(ctx context.Context) error {
	var alreadyOn bool
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		(() => {
			const btn = document.querySelector('[title="Định dạng tin nhắn (Ctrl + Shift + X)"]');
			return !!(btn && btn.className.includes('focused'));
		})()
	`, &alreadyOn))
	if err != nil {
		return err
	}
	if !alreadyOn {
		// Ctrl+Shift+X: modifier bitmask Ctrl(2)+Shift(8)=10, phím chính 'X'.
		if err := pressKeyCombo(ctx, input.ModifierCtrl|input.ModifierShift, "KeyX", "X", 88); err != nil {
			return err
		}
		if err := chromedp.Run(ctx, chromedp.WaitVisible(`[title="In đậm"]`, chromedp.ByQuery)); err != nil {
			return err
		}
		chromedp.Run(ctx, chromedp.Sleep(300*time.Millisecond))
	}
	return nil
}

func applyIndents(ctx context.Context, lines []richtext.ParsedLine) error {
	seenCounts := map[string]int{}
	for _, line := range lines {
		if line.ListType == "" || line.Indent <= 0 || line.PlainText == "" {
			continue
		}
		skip := seenCounts[line.PlainText]
		seenCounts[line.PlainText] = skip + 1

		args, _ := json.Marshal(map[string]any{"text": line.PlainText, "skip": skip})
		var coords *point
		err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`
			(() => {
				const args = %s;
				const text = args.text, skip = args.skip;
				const root = document.activeElement;
				const blocks = root.querySelectorAll('[data-component="rtf-block"]');
				let matchCount = 0;
				for (const node of blocks) {
					if (node.textContent.trim() === text) {
						if (matchCount === skip) {
							let target = node;
							const spans = node.querySelectorAll('[data-text="true"]');
							if (spans.length > 0) {
								target = spans[spans.length - 1];
							}
							target.scrollIntoView({ block: "center" });
							const r = target.getBoundingClientRect();
							return { x: r.x + r.width - 2, y: r.y + r.height / 2 };
						}
						matchCount += 1;
					}
				}
				return null;
			})()
		`, string(args)), &coords))
		if err != nil {
			return err
		}
		if coords == nil {
			continue
		}

		if err := chromedp.Run(ctx, chromedp.MouseClickXY(coords.X, coords.Y)); err != nil {
			return err
		}
		chromedp.Run(ctx, chromedp.Sleep(150*time.Millisecond))

		clicksPerLevel := indentClicksPerLevel[line.ListType]
		if clicksPerLevel == 0 {
			clicksPerLevel = 1
		}
		for i := 0; i < line.Indent*clicksPerLevel; i++ {
			if err := chromedp.Run(ctx, chromedp.Click(`[title="Lùi đầu dòng"]`, chromedp.ByQuery)); err != nil {
				return err
			}
			chromedp.Run(ctx, chromedp.Sleep(150*time.Millisecond))
		}
	}
	return nil
}

// waitMessageResponse đăng ký trước 1 listener CDP cho network domain,
// trả về 1 hàm "chờ kết quả" sẽ block tới khi thấy response khớp
// urlSubstr ĐÃ TẢI XONG (khớp cặp EventResponseReceived +
// EventLoadingFinished cùng RequestID, không chỉ ResponseReceived —
// tránh đọc body khi chưa tải xong hẳn) rồi tự gọi
// network.GetResponseBody để lấy nội dung.
func waitMessageResponse(ctx context.Context, urlSubstr string) (func(timeout time.Duration) (int64, string, error), error) {
	type matched struct {
		id     network.RequestID
		status int64
	}
	resultCh := make(chan matched, 1)
	var mu sync.Mutex
	var pending *matched

	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *network.EventResponseReceived:
			if strings.Contains(e.Response.URL, urlSubstr) {
				mu.Lock()
				pending = &matched{id: e.RequestID, status: e.Response.Status}
				mu.Unlock()
			}
		case *network.EventLoadingFinished:
			mu.Lock()
			if pending != nil && pending.id == e.RequestID {
				m := *pending
				pending = nil
				mu.Unlock()
				select {
				case resultCh <- m:
				default:
				}
				return
			}
			mu.Unlock()
		}
	})

	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		return nil, err
	}

	wait := func(timeout time.Duration) (int64, string, error) {
		select {
		case m := <-resultCh:
			var body []byte
			err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
				b, err := network.GetResponseBody(m.id).Do(ctx)
				body = b
				return err
			}))
			if err != nil {
				return m.status, "", err
			}
			return m.status, string(body), nil
		case <-time.After(timeout):
			return 0, "", fmt.Errorf("hết thời gian chờ response chứa %q", urlSubstr)
		}
	}
	return wait, nil
}

// sendPastedMessage dán (paste) TOÀN BỘ message dưới dạng 1 khối HTML
// (qua richtext.BuildHTML — mỗi dòng thành 1 <div>, không có cú pháp
// markup nào thì vẫn ra <div> thường) trong 1 sự kiện paste DUY NHẤT,
// KHÔNG gõ từng dòng/Shift+Enter như send_message.py bản không --rich —
// nhanh và ổn định hơn, và giúp nội dung có markup (đậm/nghiêng/màu/
// list) hiển thị đúng định dạng nếu sau này zaloMessage.ts dùng tới cú
// pháp đó (xem RICH_TEXT_SYNTAX.md) — không cần đổi gì thêm ở đây.
func sendPastedMessage(ctx context.Context, markupText string) (bool, string, error) {
	if err := chromedp.Run(ctx, chromedp.Click("#richInput", chromedp.ByQuery)); err != nil {
		return false, "", err
	}
	chromedp.Run(ctx, chromedp.Sleep(150*time.Millisecond))

	if err := ensureFormatMode(ctx); err != nil {
		return false, "", err
	}

	lines := richtext.ParseDocument(markupText)
	html := richtext.BuildHTML(lines)
	plainParts := make([]string, len(lines))
	for i, l := range lines {
		plainParts[i] = l.PlainText
	}
	plainFallback := strings.Join(plainParts, "\n")

	wait, err := waitMessageResponse(ctx, "message/sms")
	if err != nil {
		return false, "", err
	}

	args, _ := json.Marshal(map[string]any{"html": html, "text": plainFallback})
	err = chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`
		(() => {
			const args = %s;
			const el = document.activeElement;
			const dt = new DataTransfer();
			dt.setData('text/html', args.html);
			dt.setData('text/plain', args.text);
			const evt = new ClipboardEvent('paste', {
				clipboardData: dt,
				bubbles: true,
				cancelable: true,
			});
			el.dispatchEvent(evt);
		})()
	`, string(args)), nil))
	if err != nil {
		return false, "", err
	}
	chromedp.Run(ctx, chromedp.Sleep(300*time.Millisecond))

	needsIndent := false
	for _, l := range lines {
		if l.ListType != "" && l.Indent > 0 {
			needsIndent = true
			break
		}
	}
	if needsIndent {
		if err := applyIndents(ctx, lines); err != nil {
			return false, "", err
		}
	}

	if err := pressEnter(ctx); err != nil {
		return false, "", err
	}
	status, body, err := wait(sendConfirmTimeout)
	if err != nil {
		return false, "", err
	}
	ok := status == 200 && strings.Contains(body, `"error_code":0`)
	return ok, body, nil
}

// openConversation tìm hội thoại theo tên qua ô tìm kiếm và mở nó ra,
// trả về false (không lỗi) nếu không tìm thấy kết quả nào hoặc không
// thấy ô soạn tin sau khi chọn — SendMessage (ở trên) chuyển 2 trường
// hợp "false, nil" này thành error thật cho caller.
func openConversation(ctx context.Context, contactQuery string) (bool, error) {
	if err := chromedp.Run(ctx, chromedp.Click("#contact-search-input", chromedp.ByQuery)); err != nil {
		return false, err
	}
	if err := chromedp.Run(ctx, chromedp.SendKeys("#contact-search-input", contactQuery, chromedp.ByQuery)); err != nil {
		return false, err
	}
	chromedp.Run(ctx, chromedp.Sleep(1500*time.Millisecond))

	var coords *point
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		(() => {
			const el = document.querySelector('.txt-highlight');
			if (!el) return null;
			const r = el.getBoundingClientRect();
			return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
		})()
	`, &coords))
	if err != nil {
		return false, err
	}
	if coords == nil {
		return false, nil
	}

	if err := chromedp.Run(ctx, chromedp.MouseClickXY(coords.X, coords.Y)); err != nil {
		return false, err
	}
	chromedp.Run(ctx, chromedp.Sleep(1500*time.Millisecond))

	if err := chromedp.Run(ctx, chromedp.WaitVisible("#richInput", chromedp.ByQuery)); err != nil {
		return false, nil
	}
	return true, nil
}
