// Command zalosend tu dong gui tin nhan tren chat.zalo.me bang Playwright
// (dieu khien UI that, khong goi thang API noi bo vi API do dung payload
// ma hoa rieng cua Zalo). Ban port tu send_message.py + rich_paste_engine.py
// (Python) sang Go.
//
// Cach dung (luu y: --flag phai dat TRUOC tham so vi tri, dung quy uoc
// cua goi "flag" chuan trong Go - khac voi argparse cua Python):
//
//	zalosend "Ten lien he hoac ten nhom" "Noi dung tin nhan"
//	zalosend "Ten lien he" "**Dam** *nghieng* {red:mau do}"
//	zalosend --headless "Ten lien he" "Noi dung"
//
// Moi tin nhan deu duoc gui bang cach DAN (paste) 1 lan - khong go tung
// ky tu qua ban phim - nhanh va on dinh hon nhieu. Cu phap dinh dang
// (dam/nghieng/gach chan/gach ngang/mau chu/danh sach, ho tro thut le)
// duoc dien giai tu dong neu co trong noi dung, xem RICH_TEXT_SYNTAX.md;
// noi dung khong co cu phap nao thi paste nhu text thuong, khong can
// tuy chon rieng.
//
// Tuy chon:
//
//	--headless   Chay an (khong hien cua so trinh duyet)
//
// Lan dau chay se can dang nhap bang QR code (session duoc luu lai o
// ../zalo_profile - DUNG CHUNG voi ban Python - nen cac lan sau tu dong
// dang nhap).
//
// LUU Y: Chi nen dung voi tai khoan cua chinh ban, cho muc dich ca nhan.
// Khong dung de spam / gui hang loat vi co the vi pham dieu khoan Zalo
// va bi khoa tai khoan.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mxschmitt/playwright-go"

	"zalosend/richtext"
)

// indentClicksPerLevel: nut "Lui dau dong" tren <ul> tao 1 muc thut le
// ro rang chi voi 1 lan bam. Tren <ol> no chi cong don text-indent:10px
// moi lan bam, phai bam nhieu lan moi thay ro - da do dac qua thu
// nghiem thuc te (xem RICH_TEXT_SYNTAX.md).
var indentClicksPerLevel = map[string]int{
	"bullet":   1,
	"numbered": 3,
}

func ensureFormatMode(page playwright.Page) error {
	res, err := page.Evaluate(`
		() => {
			const btn = document.querySelector('[title="Định dạng tin nhắn (Ctrl + Shift + X)"]');
			return !!(btn && btn.className.includes('focused'));
		}
	`)
	if err != nil {
		return err
	}
	alreadyOn, _ := res.(bool)
	if !alreadyOn {
		if err := page.Keyboard().Press("Control+Shift+X"); err != nil {
			return err
		}
		if err := page.GetByTitle("In đậm", playwright.PageGetByTitleOptions{Exact: playwright.Bool(true)}).
			WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(5000)}); err != nil {
			return err
		}
		page.WaitForTimeout(300)
	}
	return nil
}

// applyIndents la buoc 2 cua co che hybrid: sau khi da dan (paste) noi
// dung dang list PHANG, tim tung dong can thut le (Indent > 0) trong o
// soan tin bang cach khop text, dat con tro vao dong do, roi bam nut
// "Lui dau dong" dung so lan. Khac voi thut le qua HTML long nhau (bi
// Zalo lam phang khi dan), thut le qua nut bam nay duoc giu nguyen sau
// khi gui. Dung "occurrence index" de xu ly dung khi co nhieu dong
// trung text.
func applyIndents(page playwright.Page, lines []richtext.ParsedLine) error {
	seenCounts := map[string]int{}
	for _, line := range lines {
		if line.ListType == "" || line.Indent <= 0 || line.PlainText == "" {
			continue
		}
		skip := seenCounts[line.PlainText]
		seenCounts[line.PlainText] = skip + 1

		res, err := page.Evaluate(`
			({ text, skip }) => {
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
			}
		`, map[string]any{"text": line.PlainText, "skip": skip})
		if err != nil {
			return err
		}
		if res == nil {
			continue
		}
		coords, ok := res.(map[string]any)
		if !ok {
			continue
		}
		x, _ := coords["x"].(float64)
		y, _ := coords["y"].(float64)

		if err := page.Mouse().Click(x, y); err != nil {
			return err
		}
		page.WaitForTimeout(150)

		clicksPerLevel := indentClicksPerLevel[line.ListType]
		if clicksPerLevel == 0 {
			clicksPerLevel = 1
		}
		for i := 0; i < line.Indent*clicksPerLevel; i++ {
			if err := page.GetByTitle("Lùi đầu dòng", playwright.PageGetByTitleOptions{Exact: playwright.Bool(true)}).Click(); err != nil {
				return err
			}
			page.WaitForTimeout(150)
		}
	}
	return nil
}

// sendPastedMessage dan (paste) toan bo markupText (cu phap
// RICH_TEXT_SYNTAX.md) vao hoi thoai dang mo, roi gui. Neu markup co
// dong list voi thut le, se tu dong ap dung thut le sau khi paste (xem
// applyIndents). Yeu cau: da mo dung hoi thoai (#richInput dang hien
// dien truoc khi goi).
func sendPastedMessage(page playwright.Page, markupText string) (bool, string, error) {
	richInput := page.Locator("#richInput")
	if err := richInput.Click(); err != nil {
		return false, "", err
	}
	page.WaitForTimeout(150)

	if err := ensureFormatMode(page); err != nil {
		return false, "", err
	}

	lines := richtext.ParseDocument(markupText)
	html := richtext.BuildHTML(lines)
	plainParts := make([]string, len(lines))
	for i, l := range lines {
		plainParts[i] = l.PlainText
	}
	plainFallback := strings.Join(plainParts, "\n")

	_, err := page.Evaluate(`
		({ html, text }) => {
			const el = document.activeElement;
			const dt = new DataTransfer();
			dt.setData('text/html', html);
			dt.setData('text/plain', text);
			const evt = new ClipboardEvent('paste', {
				clipboardData: dt,
				bubbles: true,
				cancelable: true,
			});
			el.dispatchEvent(evt);
		}
	`, map[string]any{"html": html, "text": plainFallback})
	if err != nil {
		return false, "", err
	}
	page.WaitForTimeout(300)

	needsIndent := false
	for _, l := range lines {
		if l.ListType != "" && l.Indent > 0 {
			needsIndent = true
			break
		}
	}
	if needsIndent {
		if err := applyIndents(page, lines); err != nil {
			return false, "", err
		}
	}

	resp, err := page.ExpectResponse(func(r playwright.Response) bool {
		return strings.Contains(r.URL(), "message/sms")
	}, func() error {
		return page.Keyboard().Press("Enter")
	}, playwright.PageExpectResponseOptions{Timeout: playwright.Float(15000)})
	if err != nil {
		return false, "", err
	}
	body, err := resp.Text()
	if err != nil {
		return false, "", err
	}
	ok := resp.Status() == 200 && strings.Contains(body, `"error_code":0`)
	return ok, body, nil
}

// openConversation tim hoi thoai theo ten qua o tim kiem va mo no ra,
// tra ve false neu khong tim thay ket qua nao.
func openConversation(page playwright.Page, contactQuery string) (bool, error) {
	searchBox := page.Locator("#contact-search-input")
	if err := searchBox.Click(); err != nil {
		return false, err
	}
	if err := searchBox.Fill(""); err != nil {
		return false, err
	}
	if err := page.Keyboard().Type(contactQuery, playwright.KeyboardTypeOptions{Delay: playwright.Float(40)}); err != nil {
		return false, err
	}
	page.WaitForTimeout(1500)

	// Tim ket qua dau tien trong danh sach ket qua tim kiem (doan text duoc highlight)
	res, err := page.Evaluate(`
		() => {
			const el = document.querySelector('.txt-highlight');
			if (!el) return null;
			const r = el.getBoundingClientRect();
			return { x: r.x + r.width / 2, y: r.y + r.height / 2 };
		}
	`)
	if err != nil {
		return false, err
	}
	if res == nil {
		fmt.Printf("Khong tim thay ket qua nao cho %q.\n", contactQuery)
		return false, nil
	}
	coords := res.(map[string]any)
	x, _ := coords["x"].(float64)
	y, _ := coords["y"].(float64)

	if err := page.Mouse().Click(x, y); err != nil {
		return false, err
	}
	page.WaitForTimeout(1500)

	richInput := page.Locator("#richInput")
	if err := richInput.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(8000)}); err != nil {
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
	// Dung chung profile voi ban Python (../zalo_profile tinh tu thu muc go/)
	userDataDir := filepath.Join(baseDir, "..", "zalo_profile")
	if err := os.MkdirAll(userDataDir, 0o755); err != nil {
		return err
	}

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("khong khoi dong duoc playwright: %w", err)
	}
	defer pw.Stop()

	context, err := pw.Chromium.LaunchPersistentContext(userDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(headless),
		Viewport: &playwright.Size{Width: 1280, Height: 900},
	})
	if err != nil {
		return fmt.Errorf("khong mo duoc trinh duyet: %w", err)
	}
	defer context.Close()

	page, err := context.NewPage()
	if err != nil {
		return err
	}

	if _, err := page.Goto("https://chat.zalo.me/", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return err
	}
	page.WaitForTimeout(4000)

	if strings.Contains(page.URL(), "id.zalo.me") {
		fmt.Println("Chua dang nhap. Vui long quet QR code trong cua so trinh duyet (chay lai khong co --headless).")
		fmt.Println("Dang cho dang nhap (toi da 120s)...")
		if err := page.WaitForURL("https://chat.zalo.me/**", playwright.PageWaitForURLOptions{Timeout: playwright.Float(120000)}); err != nil {
			fmt.Println("Het thoi gian cho dang nhap.")
			return nil
		}
		page.WaitForTimeout(3000)
	}

	opened, err := openConversation(page, contactQuery)
	if err != nil {
		return err
	}
	if !opened {
		return nil
	}

	ok, body, err := sendPastedMessage(page, messageText)
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

	page.WaitForTimeout(1000)
	return nil
}

func main() {
	headless := flag.Bool("headless", false, "Chay an, khong hien cua so trinh duyet")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Println(`Cach dung: zalosend [--headless] "Ten lien he" "Noi dung tin nhan"`)
		fmt.Println(`(luu y: --flag phai dat TRUOC 2 tham so vi tri)`)
		os.Exit(1)
	}
	contact, message := args[0], args[1]

	if err := run(contact, message, *headless); err != nil {
		fmt.Fprintln(os.Stderr, "Loi:", err)
		os.Exit(1)
	}
}
