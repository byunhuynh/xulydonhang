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
	// Nhận diện HTML TRƯỚC khi xét mã HTTP. Trước đây phép thử này nằm
	// sau, nên mọi lỗi không-2xx đều đổ 400 ký tự HTML thô của trang lỗi
	// Google thẳng vào giao diện và nhật ký — vô dụng với người dùng, mà
	// còn che mất nguyên nhân thật.
	if looksLikeHTML(raw) {
		return nil, htmlEndpointError(resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("endpoint cấp phiên trả HTTP %d: %s", resp.StatusCode, truncate(raw, 400))
	}

	// Apps Script trả lỗi kèm HTTP 200, phải xem trong body.
	var probe struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("phản hồi không phải JSON: %s", truncate(raw, 400))
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

// looksLikeHTML báo body là một trang web chứ không phải JSON. Chỉ soi phần
// đầu: trang lỗi của Google mở bằng doctype hoặc thẻ <html> ngay dòng đầu.
func looksLikeHTML(raw []byte) bool {
	head := strings.ToLower(strings.TrimSpace(string(raw)))
	if len(head) > 200 {
		head = head[:200]
	}
	return strings.Contains(head, "<!doctype html") || strings.Contains(head, "<html")
}

// htmlEndpointError dịch mã HTTP kèm trang HTML thành việc người dùng
// phải làm, thay vì dán lại mã nguồn trang lỗi.
func htmlEndpointError(status int) error {
	switch status {
	case http.StatusNotFound:
		return fmt.Errorf("endpoint cấp phiên trả HTTP 404 (Google trả trang lỗi, không phải JSON) — " +
			"URL Apps Script sai hoặc bản triển khai không còn. Kiểm tra sid_url trong Cài đặt > MISA: " +
			"phải là URL kết thúc bằng /exec (không phải /dev), lấy từ Deploy > Manage deployments của " +
			"bản triển khai đang bật; sửa code rồi tạo deployment mới thì URL cũng đổi")
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("endpoint cấp phiên trả HTTP %d kèm trang đăng nhập Google — "+
			"mở Deploy > Manage deployments và đặt \"Who has access\" thành \"Anyone with the link\"", status)
	default:
		return fmt.Errorf("endpoint cấp phiên trả HTML chứ không phải JSON (HTTP %d) — "+
			"kiểm tra quyền truy cập của Apps Script (phải là \"Anyone with the link\") và URL kết thúc bằng /exec", status)
	}
}
