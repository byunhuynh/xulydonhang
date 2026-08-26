package misa

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// authHeaderPrefixes là các header cần mang theo để MISA chấp nhận request.
// Ngoài Authorization, X-MISA-Context mang TenantId/TenantCode nên bắt buộc.
// X-Device là vân tay thiết bị — chính là đoạn giữa của SessionId; thiếu nó thì
// các endpoint đổi ngữ cảnh dữ liệu trả về "Error while process request".
var authHeaderPrefixes = []string{"authorization", "x-misa", "x-amis", "x-device", "cookie"}

func isAuthHeader(key string) bool {
	k := strings.ToLower(key)
	for _, p := range authHeaderPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// captureEntry là phần tối thiểu của một dòng requests.jsonl do misasniff ghi.
type captureEntry struct {
	Seq            int               `json:"seq"`
	Host           string            `json:"host"`
	RequestHeaders map[string]string `json:"request_headers"`
}

// LoadHeadersFromCapture rút header xác thực từ file requests.jsonl của misasniff.
// Lấy request mới nhất tới host mong muốn có mang Authorization.
func (c *Client) LoadHeadersFromCapture(path, host string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if host == "" {
		host = "actapp.misa.vn"
	}

	var best map[string]string
	bestSeq := -1

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 64<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e captureEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // dòng hỏng thì bỏ qua, không làm chết cả file
		}
		if !strings.EqualFold(e.Host, host) || e.Seq < bestSeq {
			continue
		}

		picked := map[string]string{}
		for k, v := range e.RequestHeaders {
			if v != "" && isAuthHeader(k) {
				picked[k] = v
			}
		}
		if picked["Authorization"] == "" && picked["authorization"] == "" {
			continue
		}
		best, bestSeq = picked, e.Seq
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if best == nil {
		return fmt.Errorf("%s: không tìm thấy request nào tới %s có header Authorization", path, host)
	}

	for k, v := range best {
		// Chuẩn hoá tên Authorization để Client.do kiểm tra được.
		if strings.EqualFold(k, "authorization") {
			c.Headers["Authorization"] = v
			continue
		}
		c.Headers[k] = v
	}
	return nil
}

// LoadHeadersFile nạp header từ một file JSON dạng {"Authorization": "...", ...}.
func (c *Client) LoadHeadersFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	for k, v := range m {
		if strings.EqualFold(k, "authorization") {
			c.Headers["Authorization"] = v
			continue
		}
		c.Headers[k] = v
	}
	return nil
}

// SaveHeaders ghi bộ header hiện tại ra file để lần sau dùng lại.
// File chứa token thật nên đặt quyền 0600.
func (c *Client) SaveHeaders(path string) error {
	buf, err := json.MarshalIndent(c.Headers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, buf, 0o600)
}

// TenantInfo là phần đọc được từ header X-MISA-Context.
type TenantInfo struct {
	TenantID   string `json:"TenantId"`
	TenantCode string `json:"TenantCode"`
	DatabaseID string `json:"DatabaseId"`
	BranchID   string `json:"BranchId"`
	UserID     string `json:"UserId"`
	SubCorpID  string `json:"SubCorpID"`
}

// Tenant giải mã X-MISA-Context để biết đang thao tác trên đơn vị nào.
func (c *Client) Tenant() (TenantInfo, error) {
	var t TenantInfo
	for k, v := range c.Headers {
		if strings.EqualFold(k, "x-misa-context") {
			if err := json.Unmarshal([]byte(v), &t); err != nil {
				return t, fmt.Errorf("X-MISA-Context không phải JSON: %w", err)
			}
			return t, nil
		}
	}
	return t, fmt.Errorf("không có header X-MISA-Context")
}
