// Command zalosend-chromedp la ban port cua zalosend (cmd/playwright) sang
// thu vien chromedp thay vi playwright-go - giao tiep TRUC TIEP voi Chrome
// qua Chrome DevTools Protocol (CDP), khong can driver rieng nhu Playwright.
//
// Cach dung giong het ban playwright:
//
//	zalosend-chromedp "Ten lien he" "Noi dung tin nhan"
//	zalosend-chromedp --headless "Ten lien he" "**Dam** *nghieng* {red:mau do}"
//
// Dung chung profile ../zalo_profile voi ban Python va ban playwright (deu
// la thu muc user-data-dir chuan cua Chromium, khong phu thuoc cong cu nao
// tao ra no).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"zalosend/richtext"
)

var indentClicksPerLevel = map[string]int{
	"bullet":   1,
	"numbered": 3,
}

// pressKeyCombo gui mot phim kem modifier (vd Ctrl+Shift+X) bang cach
// dispatch truc tiep qua CDP input domain - chromedp khong co ham tien ich
// san cho to hop phim nhu Playwright's Keyboard.Press("Control+Shift+X"),
// nen phai tu dung input.DispatchKeyEvent voi bitmask Modifiers.
func pressKeyCombo(ctx context.Context, mods input.Modifier, code, key string, vkey int64) error {
	down := input.DispatchKeyEvent(input.KeyDown).
		WithModifiers(mods).
		WithCode(code).
		WithKey(key).
		WithWindowsVirtualKeyCode(vkey).
		WithNativeVirtualKeyCode(vkey)
	up := input.DispatchKeyEvent(input.KeyUp).
		WithModifiers(mods).
		WithCode(code).
		WithKey(key).
		WithWindowsVirtualKeyCode(vkey).
		WithNativeVirtualKeyCode(vkey)
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
		// Ctrl+Shift+X: modifier bitmask Ctrl(2)+Shift(8)=10, phim chinh la 'X'.
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

// waitMessageResponse dang ky truoc mot listener CDP cho network domain,
// tra ve mot ham "cho ket qua" se block toi khi thay response voi URL
// chua `urlSubstr` tai load xong (LoadingFinished) - roi tu goi
// network.GetResponseBody de lay noi dung. Chromedp khong co tien ich
// "ExpectResponse" nhu Playwright nen phai tu dung sync qua channel.
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
			return 0, "", fmt.Errorf("het thoi gian cho response chua %q", urlSubstr)
		}
	}
	return wait, nil
}

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
	status, body, err := wait(15 * time.Second)
	if err != nil {
		return false, "", err
	}
	ok := status == 200 && strings.Contains(body, `"error_code":0`)
	return ok, body, nil
}

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
		fmt.Printf("Khong tim thay ket qua nao cho %q.\n", contactQuery)
		return false, nil
	}

	if err := chromedp.Run(ctx, chromedp.MouseClickXY(coords.X, coords.Y)); err != nil {
		return false, err
	}
	chromedp.Run(ctx, chromedp.Sleep(1500*time.Millisecond))

	err = chromedp.Run(ctx, chromedp.WaitVisible("#richInput", chromedp.ByQuery))
	if err != nil {
		fmt.Println("Khong thay o nhap tin nhan sau khi chon ket qua tim kiem.")
		return false, nil
	}
	return true, nil
}

func run(contactQuery, messageText string, headless bool) error {
	baseDir, err := os.Getwd()
	if err != nil {
		return err
	}
	userDataDir := filepath.Join(baseDir, "..", "zalo_profile")
	if err := os.MkdirAll(userDataDir, 0o755); err != nil {
		return err
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir(userDataDir),
		chromedp.Flag("headless", headless),
		chromedp.WindowSize(1280, 900),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1280, 900),
		chromedp.Navigate("https://chat.zalo.me/"),
	); err != nil {
		return fmt.Errorf("khong mo duoc trang: %w", err)
	}
	chromedp.Run(ctx, chromedp.Sleep(4*time.Second))

	var curURL string
	chromedp.Run(ctx, chromedp.Location(&curURL))
	if strings.Contains(curURL, "id.zalo.me") {
		fmt.Println("Chua dang nhap. Vui long quet QR code trong cua so trinh duyet (chay lai khong co --headless).")
		fmt.Println("Dang cho dang nhap (toi da 120s)...")
		deadline := time.Now().Add(120 * time.Second)
		for time.Now().Before(deadline) {
			chromedp.Run(ctx, chromedp.Location(&curURL))
			if strings.Contains(curURL, "chat.zalo.me") && !strings.Contains(curURL, "id.zalo.me") {
				break
			}
			time.Sleep(1 * time.Second)
		}
		if strings.Contains(curURL, "id.zalo.me") {
			fmt.Println("Het thoi gian cho dang nhap.")
			return nil
		}
		chromedp.Run(ctx, chromedp.Sleep(3*time.Second))
	}

	opened, err := openConversation(ctx, contactQuery)
	if err != nil {
		return err
	}
	if !opened {
		return nil
	}

	ok, body, err := sendPastedMessage(ctx, messageText)
	if err != nil {
		return err
	}

	status := "THAT BAI"
	if ok {
		status = "THANH CONG"
	}
	fmt.Println("Gui tin:", status)
	if len(body) > 300 {
		body = body[:300]
	}
	fmt.Println("  Response:", body)

	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	return nil
}

func main() {
	headless := flag.Bool("headless", false, "Chay an, khong hien cua so trinh duyet")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Println(`Cach dung: zalosend-chromedp [--headless] "Ten lien he" "Noi dung tin nhan"`)
		os.Exit(1)
	}
	contact, message := args[0], args[1]

	if err := run(contact, message, *headless); err != nil {
		fmt.Fprintln(os.Stderr, "Loi:", err)
		os.Exit(1)
	}
}
