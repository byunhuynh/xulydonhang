// Command haravan-export lấy đơn hàng từ Haravan Omni API (đơn đồng bộ từ
// Shopee / TikTok Shop) rồi ghi ra file Excel.
//
//	go run ./cmd/haravan-export -from 2026-08-01 -to 2026-08-25 -out donhang.xlsx
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"order-processor/internal/tmdt/export"
	"order-processor/internal/tmdt/haravan"
	"order-processor/internal/tmdt/lookup"
)

func main() {
	log.SetFlags(log.Ltime)
	loadDotEnv(".env")
	if err := run(); err != nil {
		log.Fatalf("LỖI: %v", err)
	}
}

// loadDotEnv đọc file .env (KEY=VALUE mỗi dòng) và nạp vào môi trường của tiến
// trình. Biến đã có sẵn trong môi trường được giữ nguyên. File không tồn tại thì
// bỏ qua. Không log nội dung để token không lọt ra output.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		os.Setenv(key, val)
	}
}

func run() error {
	var (
		token            = flag.String("token", os.Getenv("HARAVAN_ACCESS_TOKEN"), "Access token (mặc định lấy từ biến môi trường HARAVAN_ACCESS_TOKEN)")
		baseURL          = flag.String("base-url", haravan.DefaultBaseURL, "Base URL của Haravan Omni API")
		from             = flag.String("from", "", "Từ ngày (YYYY-MM-DD, giờ VN). Mặc định: 30 ngày trước")
		to               = flag.String("to", "", "Đến ngày (YYYY-MM-DD, giờ VN, tính hết ngày). Mặc định: hôm nay")
		dateField        = flag.String("date-field", "created", "Lọc theo ngày tạo (created) hay ngày cập nhật (updated)")
		status           = flag.String("status", "any", "Trạng thái đơn: open, closed, cancelled, any")
		financial        = flag.String("financial-status", "", "Lọc theo trạng thái thanh toán: paid, pending, refunded...")
		fulfill          = flag.String("fulfillment-status", "", "Lọc theo trạng thái giao: shipped, unshipped, partial")
		channels         = flag.String("channels", "shopee,tiktok", "Sàn cần lấy, phân cách bởi dấu phẩy: shopee, tiktok, all")
		keywords         = flag.String("keywords", "", `Ghi đè bộ từ khoá nhận diện sàn, dạng "Shopee=shopee,spx;TikTok Shop=tiktok,tts"`)
		includeUnmatched = flag.Bool("include-other", false, "Xuất luôn cả đơn không nhận diện được sàn (gắn nhãn Khác)")
		excludeShops     = flag.String("exclude-shop", "CLEVY VIỆT NAM", "Bỏ qua đơn của các shop này, ngăn bởi dấu phẩy. Để rỗng nếu muốn lấy hết")
		format           = flag.String("format", export.FormatStandard, "Bố cục file: chuan (đã tính sẵn MÃ TP/SLTP/Shop/Mã misa), haravan (thô như Haravan xuất ra), full (3 sheet)")
		mapping          = flag.String("mapping", "XUẤT HÀNG HN-LA MỚI.xlsx", `Workbook chứa 2 sheet tra cứu "data shop" và "Mã misa" (chỉ cần cho -format chuan)`)
		out              = flag.String("out", "", "Đường dẫn file Excel đầu ra (mặc định donhang-<from>-<to>.xlsx)")
		maxOrders        = flag.Int("max", 0, "Giới hạn số đơn tải về (0 = không giới hạn), dùng để test")
		discover         = flag.Bool("discover", false, "Chỉ liệt kê các giá trị source_name / tags gặp được, không ghi Excel")
	)
	flag.Parse()

	if strings.TrimSpace(*token) == "" {
		return fmt.Errorf("thiếu access token: đặt biến môi trường HARAVAN_ACCESS_TOKEN hoặc dùng cờ -token")
	}

	fromT, toT, err := parseRange(*from, *to)
	if err != nil {
		return err
	}

	rules, err := buildRules(*channels, *keywords)
	if err != nil {
		return err
	}

	opt := haravan.ListOptions{
		Status:            *status,
		FinancialStatus:   *financial,
		FulfillmentStatus: *fulfill,
		MaxOrders:         *maxOrders,
	}
	switch strings.ToLower(*dateField) {
	case "created":
		opt.CreatedAtMin, opt.CreatedAtMax = fromT, toT
	case "updated":
		opt.UpdatedAtMin, opt.UpdatedAtMax = fromT, toT
	default:
		return fmt.Errorf("-date-field chỉ nhận created hoặc updated")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := haravan.NewClient(*token)
	client.BaseURL = *baseURL

	log.Printf("Khoảng thời gian (%s): %s → %s",
		*dateField,
		fromT.In(haravan.VNLocation).Format("02/01/2006 15:04"),
		toT.In(haravan.VNLocation).Format("02/01/2006 15:04"))

	if total, err := client.CountOrders(ctx, opt); err == nil {
		log.Printf("Tổng số đơn khớp bộ lọc trên Haravan: %d", total)
	} else {
		log.Printf("Không lấy được số đơn (bỏ qua): %v", err)
	}

	shopFilter := haravan.NewShopFilter(*excludeShops)
	if shopFilter.Len() > 0 {
		log.Printf("Bỏ qua đơn của shop: %s", strings.Join(shopFilter.Names(), ", "))
	}

	var (
		scanned   int
		matched   int
		skipped   int
		sourceHit = map[string]int{}
		tagHit    = map[string]int{}
		writer    export.OrderWriter
	)

	path := *out
	if strings.TrimSpace(path) == "" {
		path = fmt.Sprintf("donhang-%s-%s.xlsx",
			fromT.In(haravan.VNLocation).Format("20060102"),
			toT.In(haravan.VNLocation).Format("20060102"))
	}

	var tables *lookup.Tables
	if !*discover && (*format == export.FormatStandard || *format == "") {
		if tables, err = lookup.Load(*mapping); err != nil {
			return fmt.Errorf("%w — dùng -mapping để trỏ tới workbook chứa 2 sheet tra cứu, "+
				"hoặc -format haravan để xuất thô không cần tra cứu", err)
		}
		log.Printf("Bảng tra cứu %q: %d dòng sản phẩm, %d shop", *mapping, tables.Combos, tables.Misa)
	}

	if !*discover {
		// Ghi streaming: đơn được đẩy thẳng vào file theo từng trang API nên bộ
		// nhớ không phình theo số đơn.
		if writer, err = export.NewOrderWriter(*format, path, tables); err != nil {
			return fmt.Errorf("tạo file Excel: %w", err)
		}
	}

	err = client.ListOrders(ctx, opt, func(page int, orders []haravan.Order) error {
		for i := range orders {
			o := &orders[i]
			scanned++

			if *discover {
				sourceHit[emptyAs(o.SourceName, "(trống)")]++
				for _, t := range strings.Split(o.Tags, ",") {
					if t = strings.TrimSpace(t); t != "" {
						tagHit[t]++
					}
				}
				continue
			}

			if shopFilter.Excluded(haravan.ShopName(o)) {
				skipped++
				continue
			}

			ch := haravan.DetectChannel(o, rules)
			if ch == "" {
				if !*includeUnmatched {
					continue
				}
				ch = "Khác"
			}
			if err := writer.AddOrder(ch, o); err != nil {
				return fmt.Errorf("ghi đơn %s: %w", o.Name, err)
			}
			matched++
		}
		if !*discover && page%20 == 0 {
			log.Printf("… đã quét %d đơn, ghi %d đơn", scanned, matched)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if *discover {
		printDiscovery(scanned, sourceHit, tagHit)
		return nil
	}

	if skipped > 0 {
		log.Printf("Đã bỏ qua %d đơn thuộc shop bị loại (%s)", skipped, strings.Join(shopFilter.Names(), ", "))
	}
	log.Printf("Đã quét %d đơn, khớp %d đơn theo sàn đã chọn", scanned, matched)
	if matched == 0 {
		os.Remove(path)
		log.Printf("Không có đơn nào khớp. Chạy lại với -discover để xem shop của bạn gắn nhãn sàn thế nào,")
		log.Printf("rồi truyền bộ từ khoá đúng qua -keywords.")
		return nil
	}

	if sw, ok := writer.(*export.StandardWriter); ok {
		if warns := sw.Warnings(); len(warns) > 0 {
			sort.Strings(warns)
			log.Printf("CẢNH BÁO — %d mục chưa khai báo trong bảng tra cứu:", len(warns))
			for _, w := range warns {
				log.Printf("  - %s", w)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("lưu Excel: %w", err)
	}

	abs, _ := os.Getwd()
	log.Printf("Đã ghi %d đơn vào %s", matched, fmt.Sprintf("%s%c%s", abs, os.PathSeparator, path))
	return nil
}

func parseRange(from, to string) (time.Time, time.Time, error) {
	loc := haravan.VNLocation
	now := time.Now().In(loc)

	toT := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, loc)
	if strings.TrimSpace(to) != "" {
		d, err := time.ParseInLocation("2006-01-02", to, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("-to sai định dạng (cần YYYY-MM-DD): %w", err)
		}
		toT = d.Add(24*time.Hour - time.Second)
	}

	fromT := toT.AddDate(0, 0, -30).Truncate(24 * time.Hour)
	if strings.TrimSpace(from) != "" {
		d, err := time.ParseInLocation("2006-01-02", from, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("-from sai định dạng (cần YYYY-MM-DD): %w", err)
		}
		fromT = d
	}
	if fromT.After(toT) {
		return time.Time{}, time.Time{}, fmt.Errorf("-from (%s) phải trước -to (%s)", from, to)
	}
	return fromT, toT, nil
}

// buildRules dựng bộ luật nhận diện sàn từ cờ -channels và -keywords.
func buildRules(channels, keywords string) ([]haravan.ChannelRule, error) {
	if s := strings.TrimSpace(keywords); s != "" {
		var rules []haravan.ChannelRule
		for _, group := range strings.Split(s, ";") {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			name, kws, ok := strings.Cut(group, "=")
			if !ok {
				return nil, fmt.Errorf("-keywords sai định dạng ở %q, cần dạng Tên=tu,khoa", group)
			}
			var list []string
			for _, k := range strings.Split(kws, ",") {
				if k = strings.TrimSpace(k); k != "" {
					list = append(list, k)
				}
			}
			if len(list) == 0 {
				return nil, fmt.Errorf("-keywords: %q không có từ khoá nào", name)
			}
			rules = append(rules, haravan.ChannelRule{Name: strings.TrimSpace(name), Keywords: list})
		}
		if len(rules) == 0 {
			return nil, fmt.Errorf("-keywords rỗng")
		}
		return rules, nil
	}

	want := map[string]bool{}
	for _, c := range strings.Split(channels, ",") {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" {
			want[c] = true
		}
	}
	if want["all"] || len(want) == 0 {
		return haravan.DefaultChannelRules, nil
	}

	var rules []haravan.ChannelRule
	for _, r := range haravan.DefaultChannelRules {
		key := strings.ToLower(strings.Fields(r.Name)[0])
		if want[key] {
			rules = append(rules, r)
		}
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("-channels %q không khớp sàn nào (hỗ trợ: shopee, tiktok, all)", channels)
	}
	return rules, nil
}

func printDiscovery(scanned int, sources, tags map[string]int) {
	fmt.Printf("\nĐã quét %d đơn.\n\n", scanned)
	fmt.Println("== Giá trị source_name gặp được ==")
	printCounts(sources)
	fmt.Println("\n== Tag gặp được ==")
	printCounts(tags)
	fmt.Println("\nDùng các giá trị ở trên để đặt -keywords, ví dụ:")
	fmt.Println(`  -keywords "Shopee=shopee;TikTok Shop=tiktok"`)
}

func printCounts(m map[string]int) {
	if len(m) == 0 {
		fmt.Println("  (không có)")
		return
	}
	type kv struct {
		k string
		n int
	}
	list := make([]kv, 0, len(m))
	for k, n := range m {
		list = append(list, kv{k, n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].k < list[j].k
	})
	for _, e := range list {
		fmt.Printf("  %-40s %d đơn\n", e.k, e.n)
	}
}

func emptyAs(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
