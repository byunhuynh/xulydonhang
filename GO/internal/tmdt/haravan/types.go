package haravan

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Number chấp nhận cả number lẫn string trong JSON (Haravan trả về không đồng nhất
// giữa các endpoint: 13200000.0000 hoặc "13200000.0000").
type Number float64

func (n *Number) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*n = 0
		return nil
	}
	s := strings.Trim(string(b), `"`)
	if s == "" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*n = Number(f)
	return nil
}

func (n Number) Float() float64 { return float64(n) }

// Time parse ISO 8601 với nhiều biến thể mà Haravan dùng.
type Time struct{ time.Time }

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func (t *Time) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(bytes.TrimSpace(b)), `"`)
	if s == "" || s == "null" {
		t.Time = time.Time{}
		return nil
	}
	for _, l := range timeLayouts {
		if v, err := time.Parse(l, s); err == nil {
			t.Time = v
			return nil
		}
	}
	// Không parse được thì bỏ qua thay vì làm hỏng cả response.
	t.Time = time.Time{}
	return nil
}

func (t Time) IsZero() bool { return t.Time.IsZero() }

// In theo giờ Việt Nam cho dễ đối chiếu với trang quản trị.
var VNLocation = time.FixedZone("ICT", 7*3600)

func (t Time) InVN() time.Time { return t.Time.In(VNLocation) }

type Order struct {
	ID          int64  `json:"id"`
	OrderNumber string `json:"order_number"`
	Name        string `json:"name"`
	Number      int64  `json:"number"`

	CreatedAt   Time `json:"created_at"`
	UpdatedAt   Time `json:"updated_at"`
	ConfirmedAt Time `json:"confirmed_at"`
	ClosedAt    Time `json:"closed_at"`
	CancelledAt Time `json:"cancelled_at"`

	SourceName     string `json:"source_name"`
	Source         string `json:"source"`
	RefOrderID     int64  `json:"ref_order_id"`
	RefOrderNumber string `json:"ref_order_number"`
	UTMSource      string `json:"utm_source"`
	UTMMedium      string `json:"utm_medium"`
	UTMCampaign    string `json:"utm_campaign"`
	LandingSite    string `json:"landing_site"`

	FinancialStatus   string `json:"financial_status"`
	FulfillmentStatus string `json:"fulfillment_status"`
	ClosedStatus      string `json:"closed_status"`
	CancelledStatus   string `json:"cancelled_status"`
	ConfirmedStatus   string `json:"confirmed_status"`
	CancelReason      string `json:"cancel_reason"`

	Email        string `json:"email"`
	ContactEmail string `json:"contact_email"`
	Phone        string `json:"phone"`

	Customer        *Customer `json:"customer"`
	BillingAddress  *Address  `json:"billing_address"`
	ShippingAddress *Address  `json:"shipping_address"`

	TotalPrice          Number `json:"total_price"`
	SubtotalPrice       Number `json:"subtotal_price"`
	TotalLineItemsPrice Number `json:"total_line_items_price"`
	TotalDiscounts      Number `json:"total_discounts"`
	TotalTax            Number `json:"total_tax"`
	TotalWeight         Number `json:"total_weight"`
	Currency            string `json:"currency"`

	Gateway          string `json:"gateway"`
	GatewayCode      string `json:"gateway_code"`
	ProcessingMethod string `json:"processing_method"`

	Tags           string          `json:"tags"`
	Note           string          `json:"note"`
	NoteAttributes []NoteAttribute `json:"note_attributes"`

	LocationID   int64  `json:"location_id"`
	LocationName string `json:"location_name"`
	UserID       int64  `json:"user_id"`

	ShippingLines []ShippingLine `json:"shipping_lines"`
	LineItems     []LineItem     `json:"line_items"`
	Fulfillments  []Fulfillment  `json:"fulfillments"`
	DiscountCodes []DiscountCode `json:"discount_codes"`
}

type NoteAttribute struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

func (na NoteAttribute) StringValue() string {
	s := strings.TrimSpace(string(na.Value))
	if s == "" || s == "null" {
		return ""
	}
	var str string
	if err := json.Unmarshal(na.Value, &str); err == nil {
		return str
	}
	return strings.Trim(s, `"`)
}

type ShippingLine struct {
	Code  string `json:"code"`
	Title string `json:"title"`
	Price Number `json:"price"`
}

type Customer struct {
	ID          int64  `json:"id"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	OrdersCount int    `json:"orders_count"`
	TotalSpent  Number `json:"total_spent"`
}

func (c *Customer) FullName() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimSpace(c.LastName + " " + c.FirstName))
}

type Address struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Name         string `json:"name"`
	Phone        string `json:"phone"`
	Address1     string `json:"address1"`
	Address2     string `json:"address2"`
	Ward         string `json:"ward"`
	District     string `json:"district"`
	City         string `json:"city"`
	Province     string `json:"province"`
	Country      string `json:"country"`
	Zip          string `json:"zip"`
	ProvinceCode string `json:"province_code"`
	CountryCode  string `json:"country_code"`
}

func (a *Address) Receiver() string {
	if a == nil {
		return ""
	}
	if n := strings.TrimSpace(a.Name); n != "" {
		return n
	}
	return strings.TrimSpace(strings.TrimSpace(a.LastName + " " + a.FirstName))
}

func (a *Address) Full() string {
	if a == nil {
		return ""
	}
	parts := []string{a.Address1, a.Address2, a.Ward, a.District, a.City, a.Province, a.Country}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

type LineItem struct {
	ID                int64  `json:"id"`
	ProductID         int64  `json:"product_id"`
	VariantID         int64  `json:"variant_id"`
	Title             string `json:"title"`
	VariantTitle      string `json:"variant_title"`
	Name              string `json:"name"`
	SKU               string `json:"sku"`
	Barcode           string `json:"barcode"`
	Vendor            string `json:"vendor"`
	Quantity          int    `json:"quantity"`
	Price             Number `json:"price"`
	PriceOriginal     Number `json:"price_original"`
	PricePromotion    Number `json:"price_promotion"`
	TotalDiscount     Number `json:"total_discount"`
	Grams             Number `json:"grams"`
	FulfillmentStatus string `json:"fulfillment_status"`
	// Properties là thuộc tính tuỳ biến của dòng hàng. Với đơn sàn, Haravan nhét
	// thêm khoá nội bộ "X-Haravan-*" vào đây nên phải lọc trước khi hiển thị.
	Properties []NoteAttribute `json:"properties"`
}

type Fulfillment struct {
	ID              int64  `json:"id"`
	Status          string `json:"status"`
	TrackingNumber  string `json:"tracking_number"`
	TrackingCompany string `json:"tracking_company"`
	TrackingURL     string `json:"tracking_url"`
	CreatedAt       Time   `json:"created_at"`
}

type DiscountCode struct {
	Code   string `json:"code"`
	Amount Number `json:"amount"`
	Type   string `json:"type"`
}

type ordersResponse struct {
	Orders []Order `json:"orders"`
}

type countResponse struct {
	Count int `json:"count"`
}
