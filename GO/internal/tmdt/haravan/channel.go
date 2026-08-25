package haravan

import (
	"sort"
	"strings"
)

// Haravan không tài liệu hoá một trường "sàn" riêng cho đơn Shopee / TikTok Shop.
// Khi Haravan Omnichannel đồng bộ đơn từ sàn về, dấu vết của sàn nằm rải rác ở
// source_name, tags, utm_source, ref_order_number, note hoặc note_attributes —
// tuỳ cấu hình kết nối của từng shop. Vì vậy ta dò theo từ khoá trên tất cả các
// trường đó, và cho phép chỉnh danh sách từ khoá qua -keywords.
type ChannelRule struct {
	Name     string   // tên hiển thị trong Excel, ví dụ "Shopee"
	Keywords []string // so khớp không phân biệt hoa thường
}

var DefaultChannelRules = []ChannelRule{
	{Name: "Shopee", Keywords: []string{"shopee", "shoppe", "spx"}},
	{Name: "TikTok Shop", Keywords: []string{"tiktok", "tik tok", "tiktokshop", "tts"}},
}

// ChannelHaystack gom tất cả các trường có thể chứa dấu vết sàn của một đơn.
func ChannelHaystack(o *Order) string {
	var sb strings.Builder
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			sb.WriteString(s)
			sb.WriteByte('\n')
		}
	}
	add(o.SourceName)
	add(o.Source)
	add(o.Tags)
	add(o.UTMSource)
	add(o.UTMMedium)
	add(o.UTMCampaign)
	add(o.LandingSite)
	add(o.RefOrderNumber)
	add(o.Note)
	add(o.Gateway)
	add(o.GatewayCode)
	for _, na := range o.NoteAttributes {
		add(na.Name)
		add(na.StringValue())
	}
	for _, f := range o.Fulfillments {
		add(f.TrackingCompany)
	}
	return strings.ToLower(sb.String())
}

// DetectChannel trả về tên sàn khớp đầu tiên, hoặc "" nếu không khớp rule nào.
func DetectChannel(o *Order, rules []ChannelRule) string {
	hay := ChannelHaystack(o)
	for _, r := range rules {
		for _, kw := range r.Keywords {
			kw = strings.ToLower(strings.TrimSpace(kw))
			if kw != "" && strings.Contains(hay, kw) {
				return r.Name
			}
		}
	}
	return ""
}

// MarketplaceOrderCode cố lấy mã đơn gốc trên sàn (nếu Haravan có lưu lại).
//
// Chỉ trả về khi Haravan lưu mã sàn ở một trường riêng (ref_order_number hoặc
// note_attributes). Nhiều store thấy Haravan đặt thẳng mã sàn vào order.name —
// trường hợp đó cột "Mã đơn" đã là mã sàn nên không nhân bản sang cột này.
func MarketplaceOrderCode(o *Order) string {
	if s := strings.TrimSpace(o.RefOrderNumber); s != "" {
		return s
	}
	wanted := []string{"order_code", "ordercode", "ma_don_san", "marketplace_order_id",
		"channel_order_id", "order_sn", "ordersn", "external_order_id", "order_id"}
	for _, na := range o.NoteAttributes {
		name := strings.ToLower(strings.TrimSpace(na.Name))
		for _, w := range wanted {
			if strings.Contains(name, w) {
				if v := strings.TrimSpace(na.StringValue()); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// Khoá note_attributes mà Haravan Omnichannel gắn vào đơn đồng bộ từ sàn.
// BranchName chính là tên shop trên sàn (một store Haravan có thể gắn nhiều shop
// Shopee/TikTok); BranchId là id shop bên sàn.
const (
	attrBranchName = "X-Haravan-SalesChannel-BranchName"
	attrBranchID   = "X-Haravan-SalesChannel-BranchId"
)

// ShopName trả về tên shop trên sàn của đơn.
func ShopName(o *Order) string { return noteAttr(o.NoteAttributes, attrBranchName) }

// ShopID trả về id shop trên sàn của đơn.
func ShopID(o *Order) string { return noteAttr(o.NoteAttributes, attrBranchID) }

func noteAttr(attrs []NoteAttribute, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(strings.TrimSpace(a.Name), name) {
			return strings.TrimSpace(a.StringValue())
		}
	}
	return ""
}

// LineItemAttributes gộp thuộc tính hiển thị được của một dòng hàng: variant_title
// (ví dụ "Combo 3 Túi (Bán chạy nhất)") cộng với properties do sàn gửi kèm, bỏ qua
// các khoá nội bộ "X-Haravan-*".
func LineItemAttributes(li *LineItem) string {
	parts := []string{}
	if s := strings.TrimSpace(li.VariantTitle); s != "" {
		parts = append(parts, s)
	}
	for _, p := range li.Properties {
		name := strings.TrimSpace(p.Name)
		if name == "" || strings.HasPrefix(name, "X-Haravan-") {
			continue
		}
		if v := strings.TrimSpace(p.StringValue()); v != "" {
			parts = append(parts, name+": "+v)
		}
	}
	return strings.Join(parts, " | ")
}

// ShippingService trả về tên dịch vụ vận chuyển và tổng phí vận chuyển của đơn.
func ShippingService(o *Order) (service string, fee float64) {
	names := []string{}
	for _, sl := range o.ShippingLines {
		if s := strings.TrimSpace(firstNonBlank(sl.Title, sl.Code)); s != "" {
			names = append(names, s)
		}
		fee += sl.Price.Float()
	}
	return strings.Join(uniq(names), ", "), fee
}

func firstNonBlank(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ShopFilter là tập tên shop cần loại khỏi kết quả xuất, so khớp không phân biệt
// hoa thường (giống cách VLOOKUP của Excel so tên shop).
type ShopFilter struct {
	set   map[string]bool
	names []string // giữ nguyên cách viết người dùng nhập, để log cho dễ đọc
}

// NewShopFilter dựng bộ lọc từ danh sách tên shop ngăn bởi dấu phẩy.
func NewShopFilter(list string) ShopFilter {
	f := ShopFilter{set: map[string]bool{}}
	for _, s := range strings.Split(list, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		k := strings.ToLower(s)
		if !f.set[k] {
			f.set[k] = true
			f.names = append(f.names, s)
		}
	}
	sort.Strings(f.names)
	return f
}

// Len là số shop bị loại; 0 nghĩa là không loại đơn nào.
func (f ShopFilter) Len() int { return len(f.set) }

// Excluded cho biết đơn của shop này có bị loại không.
func (f ShopFilter) Excluded(shop string) bool {
	if len(f.set) == 0 {
		return false
	}
	return f.set[strings.ToLower(strings.TrimSpace(shop))]
}

// Names trả về danh sách tên shop trong bộ lọc — dùng để log.
func (f ShopFilter) Names() []string { return f.names }
