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

// EnsureLoggedIn — port của phần Navigate + chờ QR trong run() (main.go
// dòng 347-373), không đổi logic/thời gian chờ.
func (c *ChromedpSender) EnsureLoggedIn(ctx context.Context) error {
	if err := c.ensureBrowser(); err != nil {
		return err
	}

	if err := chromedp.Run(c.ctx,
		chromedp.EmulateViewport(1280, 900),
		chromedp.Navigate("https://chat.zalo.me/"),
	); err != nil {
		return fmt.Errorf("zalosend: không mở được trang: %w", err)
	}
	chromedp.Run(c.ctx, chromedp.Sleep(4*time.Second))

	var curURL string
	chromedp.Run(c.ctx, chromedp.Location(&curURL))
	if !strings.Contains(curURL, "id.zalo.me") {
		return nil
	}

	deadline := time.Now().Add(loginTimeout)
	for time.Now().Before(deadline) {
		chromedp.Run(c.ctx, chromedp.Location(&curURL))
		if strings.Contains(curURL, "chat.zalo.me") && !strings.Contains(curURL, "id.zalo.me") {
			chromedp.Run(c.ctx, chromedp.Sleep(3*time.Second))
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
	if c.ctx == nil {
		return fmt.Errorf("zalosend: chưa gọi EnsureLoggedIn")
	}

	opened, err := openConversation(c.ctx, contactQuery)
	if err != nil {
		return fmt.Errorf("zalosend: tìm hội thoại %q: %w", contactQuery, err)
	}
	if !opened {
		return fmt.Errorf("zalosend: không tìm thấy hội thoại %q", contactQuery)
	}

	ok, body, err := sendPastedMessage(c.ctx, message)
	if err != nil {
		return fmt.Errorf("zalosend: gửi tin tới %q: %w", contactQuery, err)
	}
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
func (c *ChromedpSender) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	if c.allocCancel != nil {
		c.allocCancel()
	}
	c.ctx = nil
	return nil
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
