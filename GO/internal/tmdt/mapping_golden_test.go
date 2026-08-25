package tmdt

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"order-processor/internal/tmdt/lookup"
)

const goldenDir = "testdata/golden"

func readCSV(t *testing.T, name string) ([]string, [][]string) {
	t.Helper()
	fh, err := os.Open(filepath.Join(goldenDir, name))
	if err != nil {
		t.Fatalf("mở %s: %v", name, err)
	}
	defer fh.Close()
	r := csv.NewReader(fh)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("đọc %s: %v", name, err)
	}
	if len(rows) < 2 {
		t.Fatalf("%s không có dữ liệu", name)
	}
	return rows[0], rows[1:]
}

// parseVNTime chấp nhận các biến thể ngày mà Excel/Haravan sinh ra.
func parseVNTime(t *testing.T, s string) time.Time {
	t.Helper()
	s = strings.TrimSpace(s)
	layouts := []string{
		time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05-07:00", "2006-01-02T15:04:05", "2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if v, err := time.Parse(l, s); err == nil {
			return v
		}
	}
	t.Fatalf("không parse được thời gian %q", s)
	return time.Time{}
}

func num(t *testing.T, s string) float64 {
	t.Helper()
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("không parse được số %q: %v", s, err)
	}
	return v
}

// goldenKey ghép 12 cột CHỮ mà tầng quy đổi sinh ra thành một khoá, đúng
// thứ tự header của expected_dondathang.csv: A,B,D,E,G,L,Q,U,V,AJ,AM,AO.
// Bốn cột còn lại (C,T,AE,AV) là hằng số, kiểm riêng.
func goldenKey(a, b, d, e, g, l, q, u, v, aj, am, ao string) string {
	return strings.Join([]string{a, b, d, e, g, l, q, u, v, aj, am, ao}, "\x1f")
}

func TestBuildGoldenAgainstMauChuan(t *testing.T) {
	_, orderRows := readCSV(t, "orders.csv")
	lines := make([]OrderLine, 0, len(orderRows))
	for _, r := range orderRows {
		// order_code,shop,kho_ban,kenh_ban_hang,ngay_dat_hang,so_luong,
		// ten_san_pham,gia_tri_thuoc_tinh_1,gia_san_pham,ma_san_pham
		lines = append(lines, OrderLine{
			OrderCode: r[0], Shop: r[1], KhoBan: r[2], KenhBanHang: r[3],
			CreatedAt: parseVNTime(t, r[4]), Quantity: num(t, r[5]),
			Title: r[6], VariantTitle: r[7], Price: num(t, r[8]), SKU: r[9],
		})
	}
	if len(lines) != 1585 {
		t.Fatalf("fixture orders.csv có %d dòng, muốn 1585 — sinh lại fixture", len(lines))
	}

	tables, err := lookup.Load(filepath.Join(goldenDir, "lookup.xlsx"))
	if err != nil {
		t.Fatalf("nạp bảng tra cứu: %v", err)
	}

	// ProductName dựng từ fixture: cột S tra productdata.Store (qua mạng),
	// không suy được từ dữ liệu Haravan — xem testdata/golden/README.md.
	_, nameRows := readCSV(t, "ten_hang.csv")
	names := map[string]string{}
	for _, r := range nameRows {
		names[r[0]] = r[1]
	}

	got := Build(lines, tables, Options{ProductName: func(tp string) string { return names[tp] }})

	if len(got.SheetRows) != 1585 {
		t.Errorf("SheetRows = %d, muốn 1585 (một dòng sheet cho mỗi dòng hàng)", len(got.SheetRows))
	}
	if len(got.Missing) != 0 {
		t.Errorf("golden không được thiếu mã nào, nhưng thiếu %d: %+v", len(got.Missing), got.Missing)
	}

	_, want := readCSV(t, "expected_dondathang.csv")
	if len(got.OrderRows) != len(want) {
		t.Fatalf("OrderRows = %d, muốn %d", len(got.OrderRows), len(want))
	}

	// Mẫu chuẩn do người dùng dựng THỦ CÔNG: họ lọc sheet "Đơn hàng haravan"
	// theo từng ngày, từng shop, từng kho rồi dán thành khối, nên thứ tự dòng
	// của "Don dat hang" là (ngày × mã MISA × kho) — kiểm thật thấy 15 khối
	// liên tiếp như vậy. Build thì cố ý giữ NGUYÊN thứ tự dòng hàng đầu vào
	// (Task 7/11 ghi theo thứ tự đó; AMIS import không quan tâm thứ tự dòng).
	// Nên so theo VỊ TRÍ sẽ đỏ ở gần như mọi dòng dù nội dung đúng từng ô.
	// Vì vậy so như TẬP HỢP CÓ LẶP: mỗi dòng mẫu chuẩn phải được ĐÚNG MỘT
	// dòng Build khớp và ngược lại — vẫn là so cell-by-cell, chỉ bỏ ràng buộc
	// thứ tự. Dung sai giữ nguyên 1e-9 cho Số lượng, 1e-6 cho Đơn giá; nới
	// rộng hơn là che công thức sai.
	type wantRow struct {
		qty, price float64
		matched    bool
	}
	wants := make([]wantRow, len(want))
	buckets := map[string][]int{} // khoá chữ → chỉ số các dòng mẫu chuẩn
	for i, w := range want {
		// Bốn cột hằng số: mẫu chuẩn phải đúng như excelwriter ghi cứng.
		for _, c := range []struct{ col, got, want string }{
			{"C", "Chưa thực hiện", w[2]},
			{"T", "Không", w[8]},
			{"AE", "8", strings.TrimSpace(w[13])},
			{"AV", "15", strings.TrimSpace(w[17])},
		} {
			if c.got != c.want {
				t.Errorf("mẫu chuẩn dòng %d cột %s = %q, hằng số ghi ra là %q", i+1, c.col, c.want, c.got)
			}
		}
		wants[i] = wantRow{qty: num(t, w[11]), price: num(t, w[12])}
		k := goldenKey(w[0], w[1], w[3], w[4], w[5], w[6], w[7], w[9], w[10], w[14], w[15], w[16])
		buckets[k] = append(buckets[k], i)
	}

	matched := 0
	var lechChu, lechSo []string
	for i, g := range got.OrderRows {
		promo := "Không"
		if g.IsPromoItem {
			promo = "Có"
		}
		k := goldenKey(g.EntryDate, g.OrderNumber, g.EntryDate, g.ShipTo, g.CustomerCode,
			g.Description, g.SKU, promo, g.Warehouse, g.RegionCode, g.StatCode, g.Note)
		idxs, ok := buckets[k]
		if !ok {
			if len(lechChu) < 5 {
				lechChu = append(lechChu, strconv.Itoa(i+1)+": "+strings.ReplaceAll(k, "\x1f", " | "))
			}
			continue
		}
		hit := -1
		for _, j := range idxs {
			if wants[j].matched {
				continue
			}
			if math.Abs(g.Qty-wants[j].qty) > 1e-9 {
				continue
			}
			if math.Abs(g.UnitPrice-wants[j].price) > 1e-6 {
				continue
			}
			hit = j
			break
		}
		if hit < 0 {
			if len(lechSo) < 5 {
				// Chỉ báo con số của dòng mẫu chuẩn đầu tiên cùng khoá — đủ để
				// thấy công thức Số lượng / Đơn giá sai ở đâu.
				j := idxs[0]
				lechSo = append(lechSo, strconv.Itoa(i+1)+": "+strings.ReplaceAll(k, "\x1f", " | ")+
					" — được X="+strconv.FormatFloat(g.Qty, 'f', -1, 64)+
					" Y="+strconv.FormatFloat(g.UnitPrice, 'f', -1, 64)+
					", muốn X="+strconv.FormatFloat(wants[j].qty, 'f', -1, 64)+
					" Y="+strconv.FormatFloat(wants[j].price, 'f', -1, 64))
			}
			continue
		}
		wants[hit].matched = true
		matched++
	}

	t.Logf("khớp %d/%d dòng mẫu chuẩn", matched, len(want))
	if matched != len(want) {
		t.Errorf("khớp %d/%d dòng mẫu chuẩn", matched, len(want))
		for _, s := range lechChu {
			t.Errorf("dòng Build %s — không có dòng mẫu chuẩn nào cùng bộ cột chữ", s)
		}
		for _, s := range lechSo {
			t.Errorf("dòng Build %s — lệch số", s)
		}
		for j, w := range wants {
			if !w.matched {
				t.Errorf("dòng mẫu chuẩn %d KHÔNG được dòng Build nào khớp: %s | X=%v Y=%v",
					j+1, strings.Join(want[j][:11], " | "), w.qty, w.price)
				break
			}
		}
	}
}
