package haravan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const sampleOrder = `{
  "id": 1138637099,
  "order_number": "#100020",
  "name": "#100020",
  "created_at": "2026-08-19T01:12:10.062Z",
  "updated_at": "2026-08-19T04:12:21.837Z",
  "source_name": "shopee",
  "ref_order_number": "2508190XYZABC",
  "financial_status": "paid",
  "fulfillment_status": "fulfilled",
  "tags": "Shopee, Sale88",
  "total_price": "13200000.0000",
  "subtotal_price": 13200000.0000,
  "total_discounts": 0,
  "currency": "VND",
  "gateway": "Thanh toán khi giao hàng (COD)",
  "customer": {"id": 1, "first_name": "Demo", "last_name": "Haravan", "phone": "0900000000"},
  "shipping_address": {"first_name": "Demo", "address1": "182 Lê Đại Hành", "province": "Hồ Chí Minh", "country": "Vietnam"},
  "note_attributes": [{"name": "order_sn", "value": "2508190XYZABC"}],
  "line_items": [
    {"id": 11, "product_id": 21, "variant_id": 31, "title": "Đầm babydoll", "quantity": 2, "price": 600000.0000, "sku": "SKU-1"}
  ],
  "fulfillments": [{"id": 5, "tracking_number": "SPXVN123", "tracking_company": "SPX Express"}]
}`

func TestUnmarshalOrderMixedTypes(t *testing.T) {
	var o Order
	if err := json.Unmarshal([]byte(sampleOrder), &o); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := o.TotalPrice.Float(); got != 13200000 {
		t.Errorf("total_price dạng string: got %v, want 13200000", got)
	}
	if got := o.SubtotalPrice.Float(); got != 13200000 {
		t.Errorf("subtotal_price dạng number: got %v, want 13200000", got)
	}
	if o.CreatedAt.IsZero() {
		t.Error("created_at không parse được")
	}
	if got := o.Customer.FullName(); got != "Haravan Demo" {
		t.Errorf("FullName: got %q", got)
	}
	if got := MarketplaceOrderCode(&o); got != "2508190XYZABC" {
		t.Errorf("MarketplaceOrderCode: got %q", got)
	}
}

func TestDetectChannel(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"source_name shopee", `{"source_name":"shopee"}`, "Shopee"},
		{"tag TikTok", `{"source_name":"web","tags":"TikTok Shop, hot"}`, "TikTok Shop"},
		{"note_attribute tiktok", `{"note_attributes":[{"name":"channel","value":"tiktok_shop"}]}`, "TikTok Shop"},
		{"tracking SPX", `{"fulfillments":[{"tracking_company":"SPX Express"}]}`, "Shopee"},
		{"đơn website", `{"source_name":"web","tags":"vip"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var o Order
			if err := json.Unmarshal([]byte(tc.raw), &o); err != nil {
				t.Fatal(err)
			}
			if got := DetectChannel(&o, DefaultChannelRules); got != tc.want {
				t.Errorf("DetectChannel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestListOrdersPaginates(t *testing.T) {
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		w.Header().Set("X-Haravan-Api-Call-Limit", "5/80")
		switch page {
		case "1":
			// Trả đủ limit -> client phải xin trang tiếp theo.
			items := make([]json.RawMessage, 0, MaxLimit)
			for i := 0; i < MaxLimit; i++ {
				items = append(items, json.RawMessage(sampleOrder))
			}
			b, _ := json.Marshal(map[string]any{"orders": items})
			w.Write(b)
		case "2":
			fmt.Fprintf(w, `{"orders":[%s]}`, sampleOrder)
		default:
			w.Write([]byte(`{"orders":[]}`))
		}
	}))
	defer srv.Close()

	c := NewClient("tok")
	c.BaseURL = srv.URL
	c.Logger = nil

	total := 0
	err := c.ListOrders(context.Background(), ListOptions{}, func(page int, orders []Order) error {
		total += len(orders)
		return nil
	})
	if err != nil {
		t.Fatalf("ListOrders: %v", err)
	}
	if total != MaxLimit+1 {
		t.Errorf("lấy được %d đơn, want %d", total, MaxLimit+1)
	}
	if len(pages) != 2 {
		t.Errorf("gọi %d trang (%v), want 2 — phải dừng khi trang trả về ít hơn limit", len(pages), pages)
	}
}

func TestRetriesOn429(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"count": 7}`))
	}))
	defer srv.Close()

	c := NewClient("tok")
	c.BaseURL = srv.URL
	c.Logger = nil

	n, err := c.CountOrders(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("CountOrders: %v", err)
	}
	if n != 7 || calls != 2 {
		t.Errorf("count=%d calls=%d, want 7 và 2", n, calls)
	}
}

func TestUnauthorizedNotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient("tok")
	c.BaseURL = srv.URL
	c.Logger = nil

	if _, err := c.CountOrders(context.Background(), ListOptions{}); err == nil {
		t.Fatal("mong đợi lỗi 401")
	}
	if calls != 1 {
		t.Errorf("gọi %d lần, want 1 — không retry lỗi xác thực", calls)
	}
}

func TestMarketplaceOrderCodeFallbacks(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"ref_order_number", `{"name":"#100020","ref_order_number":"REF-1"}`, "REF-1"},
		{"note_attribute", `{"name":"#100020","note_attributes":[{"name":"order_sn","value":"SN-9"}]}`, "SN-9"},
		{"khong co ma san rieng", `{"name":"585206619912505206"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var o Order
			if err := json.Unmarshal([]byte(tc.raw), &o); err != nil {
				t.Fatal(err)
			}
			if got := MarketplaceOrderCode(&o); got != tc.want {
				t.Errorf("MarketplaceOrderCode = %q, want %q", got, tc.want)
			}
		})
	}
}

// Haravan so sánh created_at_min/max theo giờ cửa hàng (GMT+7) và bỏ qua hậu tố
// múi giờ. Gửi kèm "Z" hay "+07:00" là lệch cửa sổ 7 tiếng và mất đơn — đã kiểm
// chứng trên API thật, nên khoá lại bằng test.
func TestListOptionsSendsShopLocalTime(t *testing.T) {
	min := time.Date(2026, 8, 22, 0, 0, 0, 0, VNLocation)
	max := time.Date(2026, 8, 23, 23, 59, 59, 0, VNLocation)
	opt := ListOptions{CreatedAtMin: min, CreatedAtMax: max, UpdatedAtMin: min}
	q := opt.values()

	for _, tc := range []struct{ key, want string }{
		{"created_at_min", "2026-08-22T00:00:00"},
		{"created_at_max", "2026-08-23T23:59:59"},
		{"updated_at_min", "2026-08-22T00:00:00"},
	} {
		if got := q.Get(tc.key); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
		}
	}

	// Mốc cho sẵn ở UTC vẫn phải được đổi sang giờ VN trước khi gửi.
	utc := time.Date(2026, 8, 23, 17, 0, 0, 0, time.UTC) // = 00:00 ngày 24/08 giờ VN
	utcOpt := ListOptions{CreatedAtMin: utc}
	if got := utcOpt.values().Get("created_at_min"); got != "2026-08-24T00:00:00" {
		t.Errorf("mốc UTC: created_at_min = %q, want 2026-08-24T00:00:00", got)
	}
}

func TestShopFilter(t *testing.T) {
	f := NewShopFilter("CLEVY VIỆT NAM, GH Mart ")
	if got := f.Len(); got != 2 {
		t.Fatalf("bộ lọc có %d shop, want 2", got)
	}
	for _, tc := range []struct {
		shop string
		want bool
	}{
		{"CLEVY VIỆT NAM", true},
		{"clevy việt nam", true}, // không phân biệt hoa thường
		{"  GH Mart  ", true},    // bỏ khoảng trắng thừa
		{"Tẩy lồng máy giặt Blue", false},
		{"", false},
	} {
		if got := f.Excluded(tc.shop); got != tc.want {
			t.Errorf("Excluded(%q) = %v, want %v", tc.shop, got, tc.want)
		}
	}

	// Danh sách rỗng thì không loại đơn nào.
	empty := NewShopFilter("  ,  ")
	if empty.Len() != 0 || empty.Excluded("CLEVY VIỆT NAM") {
		t.Error("bộ lọc rỗng phải giữ lại mọi đơn")
	}
}
