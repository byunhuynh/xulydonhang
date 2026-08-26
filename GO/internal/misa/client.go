// Package misa gọi API nội bộ của AMIS Kế toán (actapp.misa.vn) bằng phiên
// đăng nhập bắt được từ trình duyệt. Đây là API riêng của MISA, không có hợp
// đồng công khai — mọi thứ ở đây suy ra từ traffic thật và có thể đổi bất cứ lúc nào.
package misa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL là host của AMIS Kế toán bản web.
const DefaultBaseURL = "https://actapp.misa.vn"

// ErrUnauthorized báo phiên đăng nhập đã hết hạn.
var ErrUnauthorized = errors.New("phiên đăng nhập hết hạn hoặc thiếu header xác thực")

// Client giữ base URL và bộ header xác thực dùng cho mọi request.
type Client struct {
	BaseURL string
	Headers map[string]string
	HTTP    *http.Client

	// Log nhận thông báo tiến độ; để nil thì im lặng.
	Log func(format string, args ...any)

	// session cho phép tự cấp lại token khi hết hạn; nil thì không tự cấp.
	session *Session

	// Renew xin một phiên hoàn toàn mới khi phiên hiện tại đã chết.
	// Để nil thì phiên chết là dừng.
	Renew func(ctx context.Context) (*Session, error)
}

// NewClient tạo client chưa có thông tin xác thực.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Headers: map[string]string{},
		HTTP:    &http.Client{Timeout: 3 * time.Minute},
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.Log != nil {
		c.Log(format, args...)
	}
}

// Envelope là khung phản hồi chung của mọi API g2 của MISA.
type Envelope struct {
	Success       bool              `json:"Success"`
	Code          int               `json:"Code"`
	SubCode       int               `json:"SubCode"`
	UserMessage   string            `json:"UserMessage"`
	ErrorsMessage []json.RawMessage `json:"ErrorsMessage"`
	Data          json.RawMessage   `json:"Data"`
	LogStep       []string          `json:"LogStep"` // chỉ có ở worker/check_step*
}

// Err trả lỗi nếu server báo thất bại.
func (e *Envelope) Err() error {
	if e.Success {
		return nil
	}
	msg := e.UserMessage
	if msg == "" && len(e.ErrorsMessage) > 0 {
		parts := make([]string, 0, len(e.ErrorsMessage))
		for _, m := range e.ErrorsMessage {
			parts = append(parts, string(m))
		}
		msg = strings.Join(parts, "; ")
	}
	if msg == "" {
		msg = "không rõ nguyên nhân"
	}
	return fmt.Errorf("MISA trả lỗi (Code=%d SubCode=%d): %s", e.Code, e.SubCode, msg)
}

// Decode giải mã trường Data vào v.
func (e *Envelope) Decode(v any) error {
	if len(e.Data) == 0 {
		return fmt.Errorf("phản hồi không có trường Data")
	}
	return json.Unmarshal(e.Data, v)
}

// APIError mô tả một phản hồi HTTP không thành công.
type APIError struct {
	Method string
	URL    string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.URL, e.Status, e.Body)
}

func (e *APIError) Unwrap() error {
	if e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden {
		return ErrUnauthorized
	}
	return nil
}

func (c *Client) endpoint(path string, q url.Values) string {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// loginPath là endpoint tự cấp token — không được tự đăng nhập lại quanh nó,
// nếu không sẽ đệ quy vô hạn khi phiên chết.
const loginPath = "/g2/api/auth/v1/account/login/misa_id"

// doRaw gửi đúng một request, không xử lý xác thực.
func (c *Client) doRaw(ctx context.Context, method, path string, q url.Values, contentType string, body []byte) (*Envelope, error) {
	target := c.endpoint(path, q)

	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, r)
	if err != nil {
		return nil, err
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, target, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Method: method, URL: target, Status: resp.StatusCode, Body: truncate(raw, 800)}
	}

	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%s %s: phản hồi không phải JSON hợp lệ: %w (body: %s)",
			method, target, err, truncate(raw, 400))
	}
	return &env, nil
}

// do gửi request kèm xác thực: tự cấp token nếu chưa có, và cấp lại đúng một
// lần nếu server trả 401/403 giữa chừng (token 24h hết hạn khi đang chạy).
func (c *Client) do(ctx context.Context, method, path string, q url.Values, contentType string, body []byte) (*Envelope, error) {
	if c.Headers["Authorization"] == "" {
		if err := c.login(ctx); err != nil {
			return nil, err
		}
	}

	env, err := c.doRaw(ctx, method, path, q, contentType, body)
	if err == nil || (c.session == nil && c.Renew == nil) {
		return env, err
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) || (apiErr.Status != http.StatusUnauthorized && apiErr.Status != http.StatusForbidden) {
		return env, err
	}

	c.logf("token hết hạn, tự cấp lại…")
	if relogin := c.login(ctx); relogin != nil {
		return nil, relogin
	}
	return c.doRaw(ctx, method, path, q, contentType, body)
}

// Login cấp token ngay, không đợi tới request đầu tiên. Dùng để phát hiện sớm
// phiên hỏng thay vì để lỗi nổ ra giữa lúc đang đẩy dữ liệu.
func (c *Client) Login(ctx context.Context) error { return c.login(ctx) }

// login cấp token từ phiên đang có; phiên chết thì xin phiên mới qua Renew.
func (c *Client) login(ctx context.Context) error {
	if c.session != nil {
		err := c.LoginWithSession(ctx, c.session)
		if err == nil {
			return nil
		}
		if c.Renew == nil || !errors.Is(err, ErrUnauthorized) {
			return err
		}
		c.logf("phiên AMIS đã chết: %v", err)
	} else if c.Renew == nil {
		return fmt.Errorf("%w: chưa nạp thông tin đăng nhập (dùng LoginWithSession, LoadHeadersFromCapture hoặc SetRenewFromURL)", ErrUnauthorized)
	}

	s, err := c.Renew(ctx)
	if err != nil {
		return fmt.Errorf("xin phiên mới: %w", err)
	}
	return c.LoginWithSession(ctx, s)
}

// postForm gửi form urlencoded tới endpoint cấp token (không cần Authorization).
func (c *Client) postForm(ctx context.Context, path string, form url.Values) (*Envelope, error) {
	return c.doRaw(ctx, http.MethodPost, path, nil,
		"application/x-www-form-urlencoded", []byte(form.Encode()))
}

// Get gọi một endpoint GET.
func (c *Client) Get(ctx context.Context, path string, q url.Values) (*Envelope, error) {
	return c.do(ctx, http.MethodGet, path, q, "", nil)
}

// PostJSON gọi một endpoint POST với body JSON. body = nil thì không gửi body.
func (c *Client) PostJSON(ctx context.Context, path string, q url.Values, body any) (*Envelope, error) {
	var raw []byte
	ct := ""
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		raw = buf
		ct = "application/json"
	}
	return c.do(ctx, http.MethodPost, path, q, ct, raw)
}

// PostRawJSON gửi nguyên một khối JSON đã có sẵn, không marshal lại.
// Dùng khi cần bắn ngược y hệt payload server vừa trả về.
func (c *Client) PostRawJSON(ctx context.Context, path string, q url.Values, body json.RawMessage) (*Envelope, error) {
	return c.do(ctx, http.MethodPost, path, q, "application/json", body)
}

// FileUpload là một file gửi kèm trong multipart/form-data.
type FileUpload struct {
	Field       string
	FileName    string
	ContentType string
	Data        []byte
}

// PostMultipart gửi form multipart kèm file.
func (c *Client) PostMultipart(ctx context.Context, path string, q url.Values, fields map[string]string, files []FileUpload) (*Envelope, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	for _, f := range files {
		h := make(map[string][]string)
		ct := f.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		h["Content-Disposition"] = []string{
			fmt.Sprintf("form-data; name=%q; filename=%q", f.Field, f.FileName),
		}
		h["Content-Type"] = []string{ct}
		part, err := mw.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(f.Data); err != nil {
			return nil, err
		}
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	return c.do(ctx, http.MethodPost, path, q, mw.FormDataContentType(), buf.Bytes())
}

// SetHeader gắn thêm một header cố định cho mọi request.
func (c *Client) SetHeader(key, value string) { c.Headers[key] = value }

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
