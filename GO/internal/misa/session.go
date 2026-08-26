package misa

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"unicode/utf16"
)

// Session là bộ định danh phiên đủ để tự cấp lại token mà không cần mật khẩu.
//
// Không chứa mật khẩu. Chép sang máy khác là chạy được, miễn phiên AMIS (SID)
// còn sống. SID hết hạn thì phải đăng nhập lại bằng trình duyệt một lần.
type Session struct {
	SID        string `json:"sid"`  // phiên AMIS, chính là AmisSessionId đã giải mã
	TenantID   string `json:"tid"`  // TenantId
	MisaID     string `json:"mid"`  // MISA ID của người dùng
	DatabaseID string `json:"dbid"` // bộ dữ liệu kế toán mặc định
	XDevice    string `json:"x_device"`
	Email      string `json:"email,omitempty"`
	Note       string `json:"note,omitempty"`
}

// Valid kiểm tra đủ trường bắt buộc chưa.
func (s *Session) Valid() error {
	var missing []string
	for _, f := range []struct {
		name, val string
	}{
		{"sid", s.SID}, {"tid", s.TenantID}, {"mid", s.MisaID}, {"dbid", s.DatabaseID},
	} {
		if f.val == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("phiên thiếu trường: %s", strings.Join(missing, ", "))
	}
	return nil
}

// LoadSession đọc file phiên đã lưu.
func LoadSession(path string) (*Session, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := s.Valid(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}

// Save ghi phiên ra file với quyền 0600 — nó thay được mật khẩu, giữ kín.
func (s *Session) Save(path string) error {
	buf, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(buf, '\n'), 0o600)
}

// captureSessionEntry là phần cần đọc của một dòng requests.jsonl.
type captureSessionEntry struct {
	Seq            int               `json:"seq"`
	Host           string            `json:"host"`
	RequestHeaders map[string]string `json:"request_headers"`
}

type misaContext struct {
	TenantID      string `json:"TenantId"`
	DatabaseID    string `json:"DatabaseId"`
	UserID        string `json:"UserId"`
	AmisSessionID string `json:"AmisSessionId"`
}

// SessionFromCapture dựng Session từ file requests.jsonl mà misasniff ghi ra.
func SessionFromCapture(path, host string) (*Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if host == "" {
		host = "actapp.misa.vn"
	}

	var best *Session
	bestSeq := -1

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 64<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e captureSessionEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if !strings.EqualFold(e.Host, host) || e.Seq < bestSeq {
			continue
		}

		var ctxRaw, device string
		for k, v := range e.RequestHeaders {
			switch strings.ToLower(k) {
			case "x-misa-context":
				ctxRaw = v
			case "x-device":
				device = v
			}
		}
		if ctxRaw == "" {
			continue
		}
		s, err := SessionFromContext(ctxRaw, device)
		if err != nil {
			continue
		}
		best, bestSeq = s, e.Seq
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if best == nil {
		return nil, fmt.Errorf("%s: không tìm thấy request nào tới %s có X-MISA-Context kèm AmisSessionId",
			path, host)
	}
	return best, best.Valid()
}

// SessionFromContext dựng Session từ một giá trị header X-MISA-Context.
func SessionFromContext(ctxJSON, xDevice string) (*Session, error) {
	var mc misaContext
	if err := json.Unmarshal([]byte(ctxJSON), &mc); err != nil {
		return nil, fmt.Errorf("X-MISA-Context không phải JSON: %w", err)
	}
	if mc.AmisSessionID == "" {
		return nil, fmt.Errorf("X-MISA-Context không có AmisSessionId")
	}
	sid, err := decodeAmisSessionID(mc.AmisSessionID)
	if err != nil {
		return nil, err
	}
	s := &Session{
		SID:        sid,
		TenantID:   mc.TenantID,
		MisaID:     mc.UserID,
		DatabaseID: mc.DatabaseID,
		XDevice:    xDevice,
	}
	return s, s.Valid()
}

// decodeAmisSessionID giải mã AmisSessionId: base64 của chuỗi UTF-16LE.
func decodeAmisSessionID(b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("AmisSessionId không phải base64: %w", err)
	}
	if len(raw)%2 != 0 {
		return "", fmt.Errorf("AmisSessionId không phải UTF-16LE (độ dài lẻ)")
	}
	u := make([]uint16, len(raw)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	sid := string(utf16.Decode(u))
	if sid == "" {
		return "", fmt.Errorf("AmisSessionId rỗng")
	}
	return sid, nil
}

type loginMisaIDData struct {
	AccessToken struct {
		Token        string  `json:"Token"`
		TokenExpired float64 `json:"TokenExpired"`
	} `json:"AccessToken"`
	Context json.RawMessage `json:"Context"`
	Env     string          `json:"Env"`
}

// LoginWithSession cấp một token mới từ phiên AMIS và nạp luôn ngữ cảnh dữ liệu.
//
// Một lần gọi trả về cả token 24h lẫn X-MISA-Context, nên sau đó client dùng
// được ngay. Không cần mật khẩu, không cần OTP.
func (c *Client) LoginWithSession(ctx context.Context, s *Session) error {
	if err := s.Valid(); err != nil {
		return err
	}

	form := url.Values{
		"sid":  {s.SID},
		"dbid": {s.DatabaseID},
		"lang": {"vi"},
		"tid":  {s.TenantID},
		"mid":  {s.MisaID},
	}
	if s.XDevice != "" {
		c.Headers["X-Device"] = s.XDevice
	}

	env, err := c.postForm(ctx, loginPath, form)
	if err != nil {
		return fmt.Errorf("cấp token từ phiên: %w", err)
	}
	if err := env.Err(); err != nil {
		return fmt.Errorf("%w: phiên AMIS (sid) có thể đã hết hạn, đăng nhập lại bằng misasniff — %v",
			ErrUnauthorized, err)
	}

	// sid sai/hết hạn: MISA vẫn trả Success = true nhưng Data rỗng, không hề báo
	// lỗi. Phải tự nhận ra, nếu không nhánh xin phiên mới sẽ không chạy.
	const deadSID = "%w: sid không được chấp nhận — lấy phiên mới bằng `misasniff -refresh-session`"

	var data loginMisaIDData
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return fmt.Errorf(deadSID, ErrUnauthorized)
	}
	if err := env.Decode(&data); err != nil {
		return fmt.Errorf("đọc phản hồi cấp token: %w", err)
	}
	if data.AccessToken.Token == "" {
		return fmt.Errorf(deadSID, ErrUnauthorized)
	}

	c.Headers["Authorization"] = "Bearer " + data.AccessToken.Token
	if len(data.Context) > 0 {
		c.setContextHeader(string(data.Context))
	}
	c.session = s
	c.logf("đã cấp token mới, còn %.1f giờ", data.AccessToken.TokenExpired/3600)
	return nil
}

// UseSession gắn phiên để client tự cấp lại token khi gặp 401.
func (c *Client) UseSession(s *Session) { c.session = s }

// Session trả về phiên đang gắn, hoặc nil.
func (c *Client) Session() *Session { return c.session }
