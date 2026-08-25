package haravan

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://apis.haravan.com/com"
	// Haravan giới hạn 50 bản ghi / trang.
	MaxLimit = 50
)

type Client struct {
	BaseURL     string
	AccessToken string
	HTTP        *http.Client
	Logger      *log.Logger
	// MaxRetries cho lỗi 429 / 5xx.
	MaxRetries int
}

func NewClient(accessToken string) *Client {
	return &Client{
		BaseURL:     DefaultBaseURL,
		AccessToken: accessToken,
		HTTP:        &http.Client{Timeout: 60 * time.Second},
		Logger:      log.Default(),
		MaxRetries:  5,
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.Logger != nil {
		c.Logger.Printf(format, args...)
	}
}

// do gọi API, tự động retry khi bị 429 (Retry-After) hoặc lỗi 5xx,
// và tự giảm tốc khi gần chạm giới hạn leaky-bucket (80 request / 4 req/s).
func (c *Client) do(ctx context.Context, path string, q url.Values, out any) error {
	endpoint := strings.TrimRight(c.BaseURL, "/") + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			if !sleepCtx(ctx, backoff(attempt)) {
				return ctx.Err()
			}
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if !sleepCtx(ctx, backoff(attempt)) {
				return ctx.Err()
			}
			continue
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			wait := backoff(attempt)
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil && secs > 0 {
					wait = time.Duration(secs) * time.Second
				}
			}
			c.logf("429 Too Many Requests, chờ %s rồi thử lại (%s)", wait, path)
			if !sleepCtx(ctx, wait) {
				return ctx.Err()
			}
			lastErr = fmt.Errorf("429 too many requests")
			continue

		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("haravan trả về %d: %s", resp.StatusCode, snippet(body))
			c.logf("%v — thử lại", lastErr)
			if !sleepCtx(ctx, backoff(attempt)) {
				return ctx.Err()
			}
			continue

		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return fmt.Errorf("xác thực thất bại (%d): kiểm tra access token và scope com.read_orders — %s",
				resp.StatusCode, snippet(body))

		case resp.StatusCode >= 400:
			return fmt.Errorf("gọi %s lỗi %d: %s", path, resp.StatusCode, snippet(body))
		}

		c.throttle(ctx, resp.Header.Get("X-Haravan-Api-Call-Limit"))

		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("không đọc được JSON từ %s: %w — %s", path, err, snippet(body))
		}
		return nil
	}
	return fmt.Errorf("hết số lần thử lại cho %s: %w", path, lastErr)
}

// throttle đọc header dạng "32/80"; nếu dùng quá 80% bucket thì nghỉ một nhịp.
func (c *Client) throttle(ctx context.Context, header string) {
	used, total, ok := parseCallLimit(header)
	if !ok || total == 0 {
		return
	}
	if float64(used)/float64(total) >= 0.8 {
		c.logf("gần chạm giới hạn API (%d/%d), tạm nghỉ 1s", used, total)
		sleepCtx(ctx, time.Second)
	}
}

func parseCallLimit(h string) (used, total int, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(h), "/", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	u, err1 := strconv.Atoi(parts[0])
	t, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return u, t, true
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// ListOptions ánh xạ query params của GET /com/orders.json
type ListOptions struct {
	CreatedAtMin      time.Time
	CreatedAtMax      time.Time
	UpdatedAtMin      time.Time
	UpdatedAtMax      time.Time
	Status            string // open, closed, cancelled, any
	FinancialStatus   string
	FulfillmentStatus string
	Limit             int
	// MaxOrders = 0 nghĩa là lấy hết.
	MaxOrders int
}

// filterTime định dạng mốc thời gian cho các tham số *_at_min/max.
//
// Haravan so sánh các mốc này theo GIỜ CỬA HÀNG (GMT+7). Kiểm chứng trên API thật:
//
//   - "2026-08-24T00:00:00"       → 761 đơn (đúng ngày 24/08 giờ VN)
//   - "2026-08-24T00:00:00Z"      → 761 đơn — hậu tố Z bị bỏ qua, giờ ghi trong
//     chuỗi được hiểu luôn là giờ VN
//   - "2026-08-24T00:00:00+07:00" → 786 đơn — offset số thì LẠI được quy đổi về
//     UTC trước, rồi giờ UTC đó mới đem so như giờ VN ⇒ lệch 7 tiếng
//
// Nói cách khác: phải gửi giờ VN dạng "trần". Sai lầm nguy hiểm nhất là đổi mốc
// sang UTC rồi mới format (bản đầu của tool làm vậy) — cửa sổ lệch 7 tiếng, mất
// 267 đơn và lấy thừa 242 đơn mà không có lỗi nào báo ra.
func filterTime(t time.Time) string {
	return t.In(VNLocation).Format("2006-01-02T15:04:05")
}

func (o ListOptions) values() url.Values {
	q := url.Values{}
	if !o.CreatedAtMin.IsZero() {
		q.Set("created_at_min", filterTime(o.CreatedAtMin))
	}
	if !o.CreatedAtMax.IsZero() {
		q.Set("created_at_max", filterTime(o.CreatedAtMax))
	}
	if !o.UpdatedAtMin.IsZero() {
		q.Set("updated_at_min", filterTime(o.UpdatedAtMin))
	}
	if !o.UpdatedAtMax.IsZero() {
		q.Set("updated_at_max", filterTime(o.UpdatedAtMax))
	}
	if o.Status != "" {
		q.Set("status", o.Status)
	}
	if o.FinancialStatus != "" {
		q.Set("financial_status", o.FinancialStatus)
	}
	if o.FulfillmentStatus != "" {
		q.Set("fulfillment_status", o.FulfillmentStatus)
	}
	return q
}

// CountOrders gọi GET /com/orders/count.json
func (c *Client) CountOrders(ctx context.Context, opt ListOptions) (int, error) {
	var out countResponse
	if err := c.do(ctx, "/orders/count.json", opt.values(), &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

// ListOrders duyệt hết các trang của GET /com/orders.json.
// fn được gọi cho từng trang để caller có thể xử lý dần thay vì giữ hết trong RAM.
func (c *Client) ListOrders(ctx context.Context, opt ListOptions, fn func(page int, orders []Order) error) error {
	limit := opt.Limit
	if limit <= 0 || limit > MaxLimit {
		limit = MaxLimit
	}

	fetched := 0
	for page := 1; ; page++ {
		q := opt.values()
		q.Set("page", strconv.Itoa(page))
		q.Set("limit", strconv.Itoa(limit))
		q.Set("order", "created_at asc")

		var out ordersResponse
		if err := c.do(ctx, "/orders.json", q, &out); err != nil {
			return fmt.Errorf("trang %d: %w", page, err)
		}

		n := len(out.Orders)
		c.logf("trang %d: %d đơn", page, n)
		if n == 0 {
			return nil
		}

		orders := out.Orders
		if opt.MaxOrders > 0 && fetched+n > opt.MaxOrders {
			orders = orders[:opt.MaxOrders-fetched]
		}
		if err := fn(page, orders); err != nil {
			return err
		}
		fetched += len(orders)

		if opt.MaxOrders > 0 && fetched >= opt.MaxOrders {
			return nil
		}
		if n < limit {
			return nil
		}
	}
}
