package misa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchSession xin một phiên mới từ endpoint ngoài — thường là một Google Apps
// Script tự đăng nhập MISA và đọc mã OTP trong Gmail.
//
// Endpoint phải trả JSON đúng dạng file phiên: {"sid","tid","mid","dbid","x_device"},
// hoặc {"error": "..."} khi hỏng.
func FetchSession(ctx context.Context, endpoint string, hc *http.Client) (*Session, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("chưa có URL cấp phiên")
	}
	if hc == nil {
		// Đăng nhập kèm chờ mã OTP về mail nên chậm, cho hạn rộng.
		hc = &http.Client{Timeout: 5 * time.Minute}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gọi endpoint cấp phiên: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("endpoint cấp phiên trả HTTP %d: %s", resp.StatusCode, truncate(raw, 400))
	}

	// Apps Script trả lỗi kèm HTTP 200, phải xem trong body.
	var probe struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		body := truncate(raw, 400)
		if strings.Contains(strings.ToLower(body), "<html") {
			return nil, fmt.Errorf("endpoint trả HTML chứ không phải JSON — kiểm tra quyền truy cập của Apps Script (phải là \"Anyone with the link\")")
		}
		return nil, fmt.Errorf("phản hồi không phải JSON: %s", body)
	}
	if probe.Error != "" {
		return nil, fmt.Errorf("endpoint cấp phiên báo lỗi: %s", probe.Error)
	}

	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("đọc phiên: %w", err)
	}
	if err := s.Valid(); err != nil {
		return nil, err
	}
	return &s, nil
}

// SetRenewFromURL cho client tự xin phiên mới ở endpoint khi phiên hiện tại chết.
// onNew (có thể nil) nhận phiên mới để lưu lại xuống đĩa.
func (c *Client) SetRenewFromURL(endpoint string, onNew func(*Session) error) {
	c.Renew = func(ctx context.Context) (*Session, error) {
		c.logf("xin phiên mới từ endpoint cấp phiên…")
		s, err := FetchSession(ctx, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if onNew != nil {
			if err := onNew(s); err != nil {
				c.logf("cảnh báo: không lưu được phiên mới: %v", err)
			}
		}
		return s, nil
	}
}
