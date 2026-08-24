package zalosend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"order-processor/internal/zalosend/richtext"
)

const (
	loginTimeout = 120 * time.Second
	// ensureFormatModeConfirmTimeout giới hạn RIÊNG bước xác nhận bật chế
	// độ định dạng trong ensureFormatMode - KHÔNG dùng chung hạn giờ
	// browserOpTimeout/deadline của cả job. Trước đây WaitVisible ở đó kế
	// thừa hạn giờ còn lại của runCtx (tới 90s); nếu phím tắt Ctrl+Shift+X
	// vì lý do nào đó không kích hoạt được, cả lượt gửi treo tới khi hết
	// giờ và báo "context deadline exceeded" dù hội thoại đã mở đúng -
	// đúng lỗi quan sát thực tế ở tin thứ 2/3 trong 1 batch cùng hội
	// thoại. Cắt xuống 5s + coi thất bại là best-effort (không lỗi cả
	// hàm) để 1 lần bật định dạng không thành công không làm mất luôn cả
	// tin nhắn.
	ensureFormatModeConfirmTimeout = 5 * time.Second

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
	// lastContact là tên liên hệ (contactQuery) đã MỞ THÀNH CÔNG gần
	// nhất — cho phép SendMessage bỏ qua bước tìm+mở lại hội thoại khi
	// job kế tiếp cùng liên hệ (runZaloBatch, app.go, đã sắp xếp gộp
	// nhóm các job cùng liên hệ đứng cạnh nhau trước khi gọi), tránh
	// trình duyệt gõ tìm kiếm + chờ + click lại y hệt hội thoại đang mở
	// sẵn. An toàn vì trình duyệt này chỉ do chính vòng lặp gửi điều
	// khiển tuần tự, không có thao tác nào khác xen vào giữa 2 lần gửi
	// liên tiếp có thể làm đổi hội thoại đang mở.
	lastContact string
}

func findBrowserExecutable(
	getenv func(string) string,
	fileExists func(string) bool,
	lookPath func(string) (string, error),
) (string, error) {
	paths := findBrowserExecutables(getenv, fileExists, lookPath)
	if len(paths) > 0 {
		return paths[0], nil
	}
	return "", fmt.Errorf("zalosend: không tìm thấy Google Chrome hoặc Microsoft Edge; vui lòng cài một trong hai trình duyệt rồi mở lại ứng dụng")
}

func findBrowserExecutables(
	getenv func(string) string,
	fileExists func(string) bool,
	lookPath func(string) (string, error),
) []string {
	type browserLocation struct {
		envVar string
		parts  []string
	}
	locations := []browserLocation{
		{"LOCALAPPDATA", []string{"Google", "Chrome", "Application", "chrome.exe"}},
		{"ProgramFiles", []string{"Google", "Chrome", "Application", "chrome.exe"}},
		{"ProgramFiles(x86)", []string{"Google", "Chrome", "Application", "chrome.exe"}},
		{"LOCALAPPDATA", []string{"Microsoft", "Edge", "Application", "msedge.exe"}},
		{"ProgramFiles", []string{"Microsoft", "Edge", "Application", "msedge.exe"}},
		{"ProgramFiles(x86)", []string{"Microsoft", "Edge", "Application", "msedge.exe"}},
	}
	var paths []string
	seen := make(map[string]bool)
	add := func(path string) {
		key := strings.ToLower(filepath.Clean(path))
		if path != "" && !seen[key] {
			seen[key] = true
			paths = append(paths, path)
		}
	}
	for _, location := range locations {
		base := getenv(location.envVar)
		if base == "" {
			continue
		}
		candidate := filepath.Join(append([]string{base}, location.parts...)...)
		if fileExists(candidate) {
			add(candidate)
		}
	}
	for _, name := range []string{"chrome.exe", "chrome", "msedge.exe", "msedge"} {
		if path, err := lookPath(name); err == nil && path != "" {
			add(path)
		}
	}
	return paths
}

func installedBrowserExecutable() (string, error) {
	return findBrowserExecutable(
		os.Getenv,
		func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		},
		exec.LookPath,
	)
}

func installedBrowserExecutables() []string {
	return findBrowserExecutables(
		os.Getenv,
		func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		},
		exec.LookPath,
	)
}

func startFirstWorkingBrowser(paths []string, start func(string) error) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("zalosend: không tìm thấy Google Chrome hoặc Microsoft Edge; vui lòng cài một trong hai trình duyệt rồi mở lại ứng dụng")
	}
	var failures []string
	for _, path := range paths {
		if err := start(path); err == nil {
			return path, nil
		} else {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
		}
	}
	return "", fmt.Errorf("không khởi động được Chrome/Edge: %s", strings.Join(failures, "; "))
}

func resolveProfileDir(profileDir string, executable func() (string, error)) (string, error) {
	if filepath.IsAbs(profileDir) {
		return filepath.Clean(profileDir), nil
	}
	exePath, err := executable()
	if err == nil && exePath != "" {
		exePath, err = filepath.Abs(exePath)
		if err == nil {
			return filepath.Join(filepath.Dir(exePath), profileDir), nil
		}
	}
	absPath, absErr := filepath.Abs(profileDir)
	if absErr != nil {
		return "", fmt.Errorf("zalosend: không xác định được đường dẫn profile %q: %w", profileDir, absErr)
	}
	return absPath, nil
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
	profileDir, err := resolveProfileDir(c.ProfileDir, os.Executable)
	if err != nil {
		return err
	}
	c.ProfileDir = profileDir
	if err := os.MkdirAll(c.ProfileDir, 0o755); err != nil {
		return fmt.Errorf("zalosend: tạo thư mục profile %s: %w", c.ProfileDir, err)
	}
	_, err = startFirstWorkingBrowser(installedBrowserExecutables(), func(browserPath string) error {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath(browserPath),
			chromedp.UserDataDir(c.ProfileDir),
			// Chạy ẩn hoàn toàn — mã QR đăng nhập hiện qua onQR (EnsureLoggedIn)
			// ngay trong popup của app, không còn lý do gì phải mở cửa sổ Chrome
			// thật cho người dùng thấy nữa (mọi thao tác còn lại đều tự động qua
			// selector, không cần quan sát bằng mắt).
			chromedp.Flag("headless", true),
			chromedp.WindowSize(1280, 900),
		)
		allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
		ctx, cancel := chromedp.NewContext(allocCtx)
		if startErr := chromedp.Run(ctx); startErr != nil {
			cancel()
			allocCancel()
			return startErr
		}
		c.allocCancel = allocCancel
		c.ctx, c.cancel = ctx, cancel
		return nil
	})
	if err != nil {
		return fmt.Errorf("zalosend: %w (profile: %s)", err, c.ProfileDir)
	}
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
// loginTimeout=120s); bọc thêm hạn giờ cho các lời gọi tương tác trình
// duyệt, thêm 1 lần mở lại trình duyệt nếu nó bị mất (trình duyệt chạy
// ẩn nên "mất" giờ chỉ còn do crash/bị kill ngoài ý muốn, không còn do
// người dùng tự tay đóng cửa sổ), và trong lúc chờ quét QR: đọc mã QR
// mới (nếu có) rồi báo qua onQR, tự bấm "Lấy mã mới" khi phát hiện mã đã
// hết hạn.
func (c *ChromedpSender) EnsureLoggedIn(ctx context.Context, onQR func(svgMarkup string)) error {
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

	var lastQR string
	deadline := time.Now().Add(loginTimeout)
	for time.Now().Before(deadline) {
		// ctx (tham số của EnsureLoggedIn, KHÁC browserCtx của trình duyệt)
		// bị huỷ khi người dùng bấm "Đóng" trên popup QR (CancelZaloLogin,
		// app.go) — kiểm tra ngay đầu mỗi vòng để dừng chờ trong tối đa ~1s
		// thay vì phải đợi hết 120s. Vòng lặp còn lại vẫn chạy trên
		// browserCtx THÔ (không hạn giờ) vì 120s chờ người dùng quét QR là
		// khoảng chờ cố ý; lỗi Run ở dưới thì không phải "chưa quét xong" mà
		// là trình duyệt đã chết/bị đóng — nuốt nó sẽ quay 120 vòng vô ích
		// rồi báo "hết giờ đăng nhập" sai.
		if ctx.Err() != nil {
			if onQR != nil {
				onQR("")
			}
			return fmt.Errorf("zalosend: đã huỷ đăng nhập: %w", ctx.Err())
		}
		if err := chromedp.Run(browserCtx, chromedp.Location(&curURL)); err != nil {
			return fmt.Errorf("zalosend: mất kết nối trình duyệt trong lúc chờ đăng nhập: %w", err)
		}
		if strings.Contains(curURL, "chat.zalo.me") && !strings.Contains(curURL, "id.zalo.me") {
			chromedp.Run(browserCtx, chromedp.Sleep(3*time.Second))
			if onQR != nil {
				onQR("")
			}
			return nil
		}

		// Đọc/gửi QR và kiểm tra hết hạn là "best effort": lỗi ở 2 bước này
		// (vd trang đang giữa lúc chuyển tiếp) không đủ nghiêm trọng để coi
		// là mất trình duyệt như lỗi Location ở trên — bỏ qua, thử lại vòng
		// sau, KHÔNG return.
		if expired, err := isQRExpired(browserCtx); err == nil && expired {
			_ = clickRefreshQR(browserCtx)
			chromedp.Run(browserCtx, chromedp.Sleep(1*time.Second))
		}
		if onQR != nil {
			if svg, err := readQRSVG(browserCtx); err == nil && svg != "" && svg != lastQR {
				lastQR = svg
				onQR(svg)
			}
		}

		time.Sleep(1 * time.Second)
	}
	if onQR != nil {
		onQR("")
	}
	return fmt.Errorf("zalosend: hết thời gian chờ đăng nhập (quét mã QR trong popup app rồi thử gửi lại)")
}

// RefreshQR bấm nút "Lấy mã mới" trên trang đăng nhập nếu đang có (kể cả
// khi đang ẩn — .click() qua JS vẫn kích hoạt handler bất kể CSS
// display, đã xác nhận qua thử nghiệm thực tế trên trang thật). Không
// lỗi nếu chưa mở trình duyệt hay không có mã QR nào đang chờ (đã đăng
// nhập rồi) — chỉ đơn giản không làm gì.
func (c *ChromedpSender) RefreshQR(ctx context.Context) error {
	browserCtx := c.browserContext()
	if browserCtx == nil {
		return nil
	}
	runCtx, cancel := context.WithTimeout(browserCtx, opTimeout(ctx, browserOpTimeout))
	defer cancel()
	return clickRefreshQR(runCtx)
}

// readQRSVG đọc outerHTML của mã QR hiện tại trên trang đăng nhập, trả
// về "" (không lỗi) nếu chưa có/không tìm thấy — chuỗi SVG lấy trực tiếp
// từ DOM (không chụp ảnh) để gửi thẳng cho frontend render, độ nét không
// đổi ở mọi kích thước hiển thị.
func readQRSVG(ctx context.Context) (string, error) {
	var svg string
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		(() => {
			const el = document.querySelector('.qrcode svg');
			return el ? el.outerHTML : '';
		})()
	`, &svg))
	return svg, err
}

// isQRExpired kiểm tra khối ".qrcode-expired" (chứa chữ "Mã QR hết hạn"
// + link "Lấy mã mới") — khối này LUÔN tồn tại sẵn trong DOM ngay cả khi
// QR còn hiệu lực, chỉ ẩn qua CSS display:none nên phải kiểm tra
// getComputedStyle thay vì chỉ kiểm tra phần tử có tồn tại hay không (đã
// xác nhận qua thử nghiệm thực tế — kiểm tra sai kiểu này sẽ luôn báo
// "hết hạn" ngay từ đầu).
func isQRExpired(ctx context.Context) (bool, error) {
	var expired bool
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		(() => {
			const el = document.querySelector('.qrcode-expired');
			if (!el) return false;
			return getComputedStyle(el).display !== 'none';
		})()
	`, &expired))
	return expired, err
}

// clickRefreshQR bấm link "Lấy mã mới" trong khối ".qrcode-expired" nếu
// có — không lỗi, không làm gì nếu không tìm thấy link đó (chưa cần làm
// mới, hoặc đã đăng nhập nên trang không còn khối này nữa).
func clickRefreshQR(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.Evaluate(`
		(() => {
			const links = Array.from(document.querySelectorAll('.qrcode-expired a'));
			const btn = links.find(a => a.textContent.trim() === 'Lấy mã mới') || links[0];
			if (btn) btn.click();
		})()
	`, nil))
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

	c.mu.Lock()
	alreadyOpen := c.lastContact != "" && c.lastContact == contactQuery
	c.mu.Unlock()

	if !alreadyOpen {
		opened, err := openConversation(runCtx, contactQuery)
		if err != nil {
			return fmt.Errorf("zalosend: tìm hội thoại %q: %w", contactQuery, err)
		}
		if !opened {
			return fmt.Errorf("zalosend: không tìm thấy hội thoại %q", contactQuery)
		}
		c.mu.Lock()
		c.lastContact = contactQuery
		c.mu.Unlock()
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
	// Trình duyệt đóng lại thì "hội thoại đang mở" không còn ý nghĩa gì
	// nữa — xoá để lần SendMessage kế tiếp (trên trình duyệt MỚI, xem
	// EnsureLoggedIn's cơ chế mở lại) luôn tìm+mở hội thoại thật thay vì
	// tưởng nhầm là đã mở sẵn.
	c.lastContact = ""
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

// isFormatModeOn dò bằng class "--rtf-mode" trên #chat-box-input-container-id
// - tín hiệu người dùng tự soi DevTools thật tìm ra (so sánh HTML thật
// lúc TẮT/BẬT): container đó CHỈ có class này khi định dạng đang bật,
// đi kèm y hệt với nút bật/tắt gắn thêm class "focused" và ô soạn tin
// đổi hẳn cấu trúc (từ <div id="richInput"> phẳng sang
// <div data-component="rtf-container" id="{ngẫu nhiên}"> có toolbar rtf
// riêng). Đã thử 2 cách dò trước đó và đều SAI - lần lượt: className
// 'focused' của nút bật/tắt (không nhất quán, có lúc lệch với chế độ
// thật), rồi độ hiển thị nút "In đậm" (nút đó có thể "hiển thị" trong
// khi ô soạn RÕ RÀNG vẫn ở chế độ thường) - mỗi lần dò sai khiến hàm
// tưởng "đã bật sẵn" rồi bỏ qua bước bấm Ctrl+Shift+X, dán ra tin không
// định dạng - đúng nguyên nhân lỗi lúc được lúc không quan sát thấy
// trong thực tế trước khi có class "--rtf-mode" này làm tín hiệu.
func isFormatModeOn(ctx context.Context) (bool, error) {
	var on bool
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		(() => {
			const el = document.querySelector('#chat-box-input-container-id');
			return !!el && el.classList.contains('--rtf-mode');
		})()
	`, &on))
	return on, err
}

func ensureFormatMode(ctx context.Context) error {
	on, err := isFormatModeOn(ctx)
	if err != nil {
		return err
	}
	if on {
		return nil
	}
	// Ctrl+Shift+X: modifier bitmask Ctrl(2)+Shift(8)=10, phím chính 'X'.
	if err := pressKeyCombo(ctx, input.ModifierCtrl|input.ModifierShift, "KeyX", "X", 88); err != nil {
		return err
	}
	// Xác nhận bằng CHÍNH tín hiệu isFormatModeOn (không dùng WaitVisible
	// trên 1 selector khác) - đợi tới khi class "--rtf-mode" xuất hiện
	// hoặc hết giờ, best-effort: không chặn cả lượt gửi nếu chưa kịp
	// trong ensureFormatModeConfirmTimeout (xem giải thích ở hằng số đó).
	deadline := time.Now().Add(ensureFormatModeConfirmTimeout)
	for time.Now().Before(deadline) {
		on, err := isFormatModeOn(ctx)
		if err == nil && on {
			chromedp.Run(ctx, chromedp.Sleep(150*time.Millisecond))
			return nil
		}
		chromedp.Run(ctx, chromedp.Sleep(150*time.Millisecond))
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

const (
	pasteInitialSettleDelay = time.Second
	pastePollDelay          = 200 * time.Millisecond
	pasteReadyAttempts      = 10
)

func waitForPastedContent(
	lines []richtext.ParsedLine,
	read func() (string, error),
	sleep func(time.Duration) error,
) error {
	if err := sleep(pasteInitialSettleDelay); err != nil {
		return err
	}
	for attempt := 0; attempt < pasteReadyAttempts; attempt++ {
		actual, err := read()
		if err != nil {
			return err
		}
		if pastedLinesPresent(lines, actual) {
			return nil
		}
		if attempt+1 < pasteReadyAttempts {
			if err := sleep(pastePollDelay); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("nội dung dán vào ô soạn tin chưa ổn định sau %s", pasteInitialSettleDelay+(pasteReadyAttempts-1)*pastePollDelay)
}

func pastedLinesPresent(lines []richtext.ParsedLine, actual string) bool {
	normalizedActual := strings.Join(strings.Fields(actual), " ")
	cursor := 0
	for _, line := range lines {
		expected := strings.Join(strings.Fields(line.PlainText), " ")
		if expected == "" {
			continue
		}
		index := strings.Index(normalizedActual[cursor:], expected)
		if index < 0 {
			return false
		}
		cursor += index + len(expected)
	}
	return cursor > 0
}

func waitForPastedContentInBrowser(ctx context.Context, lines []richtext.ParsedLine) error {
	return waitForPastedContent(
		lines,
		func() (string, error) {
			var content string
			err := chromedp.Run(ctx, chromedp.Evaluate(`
				(() => {
					const el = document.activeElement;
					return el ? (el.innerText || el.textContent || '') : '';
				})()
			`, &content))
			return content, err
		},
		func(delay time.Duration) error {
			return chromedp.Run(ctx, chromedp.Sleep(delay))
		},
	)
}

// sendPastedMessage dán (paste) TOÀN BỘ message dưới dạng 1 khối HTML
// (qua richtext.BuildHTML — mỗi dòng thành 1 <div>, không có cú pháp
// markup nào thì vẫn ra <div> thường) trong 1 sự kiện paste DUY NHẤT,
// KHÔNG gõ từng dòng/Shift+Enter như send_message.py bản không --rich —
// nhanh và ổn định hơn, và giúp nội dung có markup (đậm/nghiêng/màu/
// list) hiển thị đúng định dạng — ĐÃ XÁC NHẬN LẠI bằng thử nghiệm thực tế
// (chạy thẳng bản tham chiếu sendmessage_zalo/go/cmd/chromedp trên đúng
// profile đang dùng, gửi vào "Cloud của tôi") rằng cách dispatch
// ClipboardEvent giả bằng JS này vẫn ra đúng <b>/color/<ul><li>.
func sendPastedMessage(ctx context.Context, markupText string) (bool, string, error) {
	// "#richInput" chỉ đúng cho LẦN GÕ ĐẦU TIÊN sau khi mở hội thoại -
	// xác nhận qua thử nghiệm thực tế: Zalo Web GỠ BỎ hẳn phần tử
	// #richInput và DỰNG LẠI ô soạn tin với 1 id ngẫu nhiên mới (vd
	// "3g183") ngay sau khi gửi xong 1 tin, id cũ biến mất hoàn toàn
	// (không phải chỉ đổi thuộc tính) - đây chính là lý do MỌI tin từ
	// tin thứ 2 trở đi trong cùng 1 hội thoại (kịch bản chỉ xảy ra với
	// ChromedpSender giữ browser sống xuyên nhiều lần gửi, bản CLI tham
	// chiếu 1 tin/1 lần chạy không bao giờ gặp) đều hỏng ở NGAY BƯỚC
	// CLICK ĐẦU TIÊN (treo tới hết giờ chờ vì #richInput không còn tồn
	// tại nữa) - không liên quan gì tới định dạng/mạng như các lần dò
	// trước. "[contenteditable=\"true\"]" vẫn luôn khớp ô soạn tin dù id
	// đổi (đã xác nhận: phần tử thay thế vẫn giữ contenteditable="true"),
	// dùng danh sách chọn gộp cả 2 để đúng ở CẢ lần gõ đầu lẫn các lần
	// sau.
	const composeInputSelector = `#richInput, [contenteditable="true"]`
	if err := chromedp.Run(ctx, chromedp.Click(composeInputSelector, chromedp.ByQuery)); err != nil {
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

	args, _ := json.Marshal(map[string]any{"html": html, "text": plainFallback})
	err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`
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
	if err := waitForPastedContentInBrowser(ctx, lines); err != nil {
		return false, "", err
	}

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
	// Không còn chờ xác nhận qua network response (bản trước chờ tới 25s
	// mỗi tin, luôn timeout với hội thoại NHÓM vì endpoint gửi tin nhóm
	// không khớp chuỗi "message/sms" từng xác nhận cho chat 1-1 — vừa báo
	// thất bại giả vừa làm chậm hẳn tin kế tiếp trong cùng batch). paste
	// qua clipboard thật + Ctrl+V thật + Enter không lỗi ở trên là đủ tín
	// hiệu tin đã gửi.
	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
	return true, "", nil
}

// openConversation tìm hội thoại theo tên qua ô tìm kiếm và mở nó ra,
// trả về false (không lỗi) nếu không tìm thấy kết quả nào hoặc không
// thấy ô soạn tin sau khi chọn — SendMessage (ở trên) chuyển 2 trường
// hợp "false, nil" này thành error thật cho caller.
func openConversation(ctx context.Context, contactQuery string) (bool, error) {
	// Xoá nội dung tìm kiếm CŨ trước khi gõ tên mới — thiếu bước này (bị
	// bỏ sót khi port từ bản send_message.py/cmd/playwright, cả 2 bản đó
	// đều có search_box.fill("")/searchBox.Fill("") trước khi gõ) khiến
	// từ lần gửi THỨ 2 trong cùng phiên trở đi, ô tìm kiếm còn nguyên tên
	// liên hệ của lần gửi trước, gõ nối vào thành 1 chuỗi vô nghĩa không
	// khớp được liên hệ nào.
	//
	// Dùng JS set thẳng thuộc tính "value" qua property setter GỐC của
	// HTMLInputElement (không phải Ctrl+A rồi gõ đè bằng phím tắt) rồi tự
	// bắn sự kiện "input" — LÝ DO đổi từ Ctrl+A: thử qua Ctrl+A từng gây
	// "không tìm thấy hội thoại" ngay từ LẦN TÌM ĐẦU TIÊN của phiên
	// (không phải chỉ lần 2 trở đi), rất có thể do Zalo Web tự bắt phím
	// tắt Ctrl+A cho mục đích khác (không phải "chọn hết text trong ô
	// tìm kiếm"). Cách set value qua JS mô phỏng ĐÚNG cơ chế
	// search_box.fill("") của Playwright — không phụ thuộc phím tắt nào
	// của trang, chắc chắn hơn. Dùng property setter gốc (không gán
	// el.value = '' trực tiếp) vì Zalo Web (giống nhiều SPA hiện đại)
	// theo dõi thay đổi qua setter bị ghi đè — gán trực tiếp có thể
	// không kích hoạt đúng logic nội bộ của ô tìm kiếm.
	if err := chromedp.Run(ctx,
		chromedp.Click("#contact-search-input", chromedp.ByQuery),
		chromedp.Evaluate(`
			(() => {
				const el = document.querySelector('#contact-search-input');
				if (!el) return;
				const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
				setter.call(el, '');
				el.dispatchEvent(new Event('input', { bubbles: true }));
			})()
		`, nil),
	); err != nil {
		return false, err
	}
	if err := chromedp.Run(ctx, chromedp.SendKeys("#contact-search-input", contactQuery, chromedp.ByQuery)); err != nil {
		return false, err
	}
	// Chờ lâu hơn bản gốc 1500ms: send_message.py gõ CÓ chủ đích delay
	// 40ms GIỮA MỖI phím (page.keyboard.type(..., delay=40)) để Zalo kịp
	// xử lý từng ký tự trước khi tới ký tự tiếp theo; SendKeys của
	// chromedp không có tuỳ chọn delay tương đương, gõ gần như tức thời
	// - với chuỗi tìm kiếm dài (vd tên nhóm đã đổi dài hơn), Zalo có thể
	// chưa kịp cập nhật kết quả tìm kiếm ngay khi ta kiểm tra
	// ".txt-highlight". Tăng lên 2500ms để bù lại, không đổi gì khác.
	chromedp.Run(ctx, chromedp.Sleep(2500*time.Millisecond))

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

	if err := chromedp.Run(ctx, chromedp.WaitVisible(`#richInput, [contenteditable="true"]`, chromedp.ByQuery)); err != nil {
		return false, nil
	}
	return true, nil
}
