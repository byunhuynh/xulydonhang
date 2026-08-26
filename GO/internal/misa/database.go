package misa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Database là một bộ dữ liệu kế toán (một "công ty" trong menu chọn dữ liệu).
type Database struct {
	DatabaseID   string `json:"database_id"`
	DatabaseName string `json:"database_name"`
	TenantID     string `json:"tenant_id"`
	TenantCode   string `json:"tenant_code"`
	TenantName   string `json:"tenant_name"`
	TaxCode      string `json:"tax_code"`
	SubCorpID    string `json:"sub_corp_id"`
	MisaID       string `json:"misa_id"`
	LastActive   string `json:"last_active"`
	Installed    bool   `json:"installed"`
}

// Databases liệt kê các bộ dữ liệu kế toán mà tài khoản hiện tại xem được.
func (c *Client) Databases(ctx context.Context) ([]Database, error) {
	t, err := c.Tenant()
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"UserId":                 t.UserID,
		"TenantId":               t.TenantID,
		"AuthType":               0,
		"IsCheckConnectOtherApp": true,
	}

	env, err := c.PostJSON(ctx, "/g2/api/system/v1/database/databases_user_can_see", nil, body)
	if err != nil {
		return nil, err
	}
	if err := env.Err(); err != nil {
		return nil, err
	}
	var out []Database
	if err := env.Decode(&out); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DatabaseName < out[j].DatabaseName })
	return out, nil
}

// CurrentDatabaseID đọc DatabaseId trong X-MISA-Context đang dùng.
func (c *Client) CurrentDatabaseID() string {
	t, err := c.Tenant()
	if err != nil {
		return ""
	}
	return t.DatabaseID
}

// FindDatabase tìm một bộ dữ liệu theo id đầy đủ hoặc theo một phần tên.
// Trả lỗi nếu không khớp cái nào, hoặc khớp nhiều hơn một.
func FindDatabase(dbs []Database, needle string) (Database, error) {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return Database{}, fmt.Errorf("chưa nêu tên hoặc id bộ dữ liệu")
	}

	for _, d := range dbs {
		if strings.EqualFold(d.DatabaseID, needle) {
			return d, nil
		}
	}

	lower := strings.ToLower(needle)
	var hits []Database
	for _, d := range dbs {
		if strings.Contains(strings.ToLower(d.DatabaseName), lower) {
			hits = append(hits, d)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		names := make([]string, 0, len(dbs))
		for _, d := range dbs {
			names = append(names, d.DatabaseName)
		}
		return Database{}, fmt.Errorf("không có bộ dữ liệu nào khớp %q; hiện có: %s",
			needle, strings.Join(names, " | "))
	default:
		names := make([]string, 0, len(hits))
		for _, d := range hits {
			names = append(names, d.DatabaseName)
		}
		return Database{}, fmt.Errorf("%q khớp nhiều bộ dữ liệu: %s — nêu rõ hơn hoặc dùng database_id",
			needle, strings.Join(names, " | "))
	}
}

type databaseContextData struct {
	IsSwitchTenant bool            `json:"IsSwitchTenant"`
	Context        json.RawMessage `json:"Context"`
}

// SwitchDatabase đổi ngữ cảnh sang một bộ dữ liệu kế toán khác.
//
// Server cấp một X-MISA-Context mới (DatabaseId, BranchId, SessionId đều đổi);
// client thay header rồi mọi request sau đó chạy trên dữ liệu mới. Token
// Authorization giữ nguyên. Endpoint này bắt buộc phải có header X-Device.
func (c *Client) SwitchDatabase(ctx context.Context, databaseID string) error {
	t, err := c.Tenant()
	if err != nil {
		return err
	}
	if t.UserID == "" {
		return fmt.Errorf("X-MISA-Context không có UserId")
	}

	q := url.Values{"isContinueAccessDBBackup": {"false"}}
	env, err := c.Get(ctx, "/g2/api/auth/v1/account/database-context/"+databaseID+"/"+t.UserID, q)
	if err != nil {
		return err
	}
	if err := env.Err(); err != nil {
		if !c.hasHeader("x-device") {
			return fmt.Errorf("%w — thiếu header X-Device, bắt lại phiên bằng misasniff bản mới", err)
		}
		return err
	}

	var data databaseContextData
	if err := env.Decode(&data); err != nil {
		return fmt.Errorf("đọc ngữ cảnh mới: %w", err)
	}
	if len(data.Context) == 0 {
		return fmt.Errorf("server không trả về ngữ cảnh cho bộ dữ liệu %s", databaseID)
	}

	c.setContextHeader(string(data.Context))
	c.logf("đã chuyển sang bộ dữ liệu %s", databaseID)
	return nil
}

// SwitchDatabaseByName tra tên rồi chuyển sang bộ dữ liệu đó.
func (c *Client) SwitchDatabaseByName(ctx context.Context, needle string) (Database, error) {
	dbs, err := c.Databases(ctx)
	if err != nil {
		return Database{}, err
	}
	db, err := FindDatabase(dbs, needle)
	if err != nil {
		return Database{}, err
	}
	if db.DatabaseID == c.CurrentDatabaseID() {
		return db, nil // đã ở đúng bộ dữ liệu
	}
	if err := c.SwitchDatabase(ctx, db.DatabaseID); err != nil {
		return db, err
	}
	return db, nil
}

func (c *Client) hasHeader(name string) bool {
	for k, v := range c.Headers {
		if strings.EqualFold(k, name) && v != "" {
			return true
		}
	}
	return false
}

// setContextHeader thay X-MISA-Context, xoá mọi biến thể hoa/thường cũ.
func (c *Client) setContextHeader(value string) {
	for k := range c.Headers {
		if strings.EqualFold(k, "x-misa-context") {
			delete(c.Headers, k)
		}
	}
	c.Headers["X-MISA-Context"] = value
}
