package osifood

import (
	"reflect"
	"testing"
)

// realPageText is the one real order available (store 68014, 12/10/2026)
// exactly as this repo's extractPageTexts returns it: one spreadsheet
// cell per line, cell padding kept, column headers after the table.
const realPageText = "\n##########\n########\nNhân viên đặt hàng :\nSTORE\n68014\nPhụ trách cửa hàng :\nTÊN CH\nBình Lợi\n" +
	"Ngày đặt hàng :\n12/10/2026 14:11\nĐỊA CHỈ \nGIAO HÀNG\n127 Bình Lợi Phường 13 Quận Bình Thạnh Thành phố Hồ Chí \nMinh\n" +
	"Hạn chót CH nhận đơn ngày :\n19/10/2026\nĐƠN ĐẶT HÀNG NCC\n00758\nDH-HÀ THÀNH LA1\n" +
	"(1)\n(2)\n(3)\n(4)\n(5)\n(6)\n(7)\n(8)\n(9)\n(10)\n" +
	"1\n8936156730244\n79758001\nNước giặt xả Blue đậm đặc H. Thảo mộc 2.1L\n8.00\n110,501\n    \n \n-\n            \n \n Mua 1 tặng 1 \n(Quấn kèm) \n8.00\n1.00\n                        884,004 \n0\n" +
	"2\n8936156731647\n79758006\nNước rửa chén Blue phiên bản không mùi túi 3.2L\n4.00\n80,900\n      \n \n56,630\n      \n \n Giảm 30% \n4.00\n1.00\n                        226,520 \n0\n" +
	"3\n8936156731630\n79758007\nNước rửa chén Blue chiết xuất chanh  túi 3.2L\n4.00\n80,900\n      \n \n56,630\n      \n \n Giảm 30% \n4.00\n1.00\n                        226,520 \n0\n" +
	"NOTE\nAnh chị NCC vui lòng kèm theo đơn đặt hàng khi đến giao hàng tại cửa hàng.Cám ơn Anh Chị\n" +
	"TOTAL\n                 16 \n                            3 \n                     1,337,044 \n" +
	"CỬA HÀNG \nGHI CHÚ\nGIÁ VỐN \nĐVỊ (0VAT)\nGIÁ VỐN \nKM\nCƠ CẤU KM\nLƯỢNG LẺ\nLƯỢNG THÙNG\nTHÀNH TIỀN ĐƠN \nGIÁ THƯỜNG\n" +
	"Nguyễn Trần Ánh Tuyết\nAnh Kiên: 0937.701.339\nSTT\n BARCODE \nMÃ GỢI NHỚ\nTÊN HÀNG\nQUI CÁCH \nTHÙNG\n"

func TestParseOrderInfo_ReadsTheRealOrderHeader(t *testing.T) {
	got, ok := ParseOrderInfo(realPageText)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false for the real order")
	}
	want := OrderInfo{
		// No number is printed on the order: store code + order date.
		PONumber:        "68014-121026",
		StoreCode:       "68014",
		SupplierCode:    "00758",
		EntryDate:       "12/10/2026",
		Deadline:        "19/10/2026",
		DeliveryAddress: "127 Bình Lợi Phường 13 Quận Bình Thạnh Thành phố Hồ Chí Minh",
	}
	if got != want {
		t.Errorf("ParseOrderInfo =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestParseOrderInfo_PadsASingleDigitDayAndMonthInThePONumber(t *testing.T) {
	text := "STORE\n79001\nNgày đặt hàng :\n5/9/2026 08:00\nGIAO HÀNG\n1 Đường A\nHạn chót CH nhận đơn ngày :\n12/9/2026\n"
	got, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false")
	}
	if got.PONumber != "79001-050926" || got.EntryDate != "05/09/2026" {
		t.Errorf("PONumber = %q, EntryDate = %q, want %q and %q", got.PONumber, got.EntryDate, "79001-050926", "05/09/2026")
	}
}

func TestParseOrderInfo_RejectsAPageWithNoStoreCode(t *testing.T) {
	if _, ok := ParseOrderInfo("Ngày đặt hàng :\n12/10/2026\nGIAO HÀNG\n1 Đường A\nHạn chót CH nhận đơn ngày :\n19/10/2026\n"); ok {
		t.Error("ParseOrderInfo returned ok=true for a page with no STORE code")
	}
}

func TestExtractProducts_ReadsEveryRowOfTheRealOrder(t *testing.T) {
	got := ExtractProducts(realPageText)
	want := []Product{
		{
			Barcode: "8936156730244", Code: "79758001", Name: "Nước giặt xả Blue đậm đặc H. Thảo mộc 2.1L",
			PackSize: "8.00", Price: "110,501", PromoPrice: "-", PromoText: "Mua 1 tặng 1 (Quấn kèm)",
			Qty: "8.00", Cartons: "1.00", Total: "884,004",
		},
		{
			Barcode: "8936156731647", Code: "79758006", Name: "Nước rửa chén Blue phiên bản không mùi túi 3.2L",
			PackSize: "4.00", Price: "80,900", PromoPrice: "56,630", PromoText: "Giảm 30%",
			Qty: "4.00", Cartons: "1.00", Total: "226,520",
		},
		{
			Barcode: "8936156731630", Code: "79758007", Name: "Nước rửa chén Blue chiết xuất chanh túi 3.2L",
			PackSize: "4.00", Price: "80,900", PromoPrice: "56,630", PromoText: "Giảm 30%",
			Qty: "4.00", Cartons: "1.00", Total: "226,520",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractProducts =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestExtractProducts_ReadsARowWithNoPromotionAtAll(t *testing.T) {
	// Not seen on a real order yet: a line with neither a promotional
	// price nor promotion text, whether the empty price cell prints "-"
	// or nothing.
	for _, promoCell := range []string{"-\n", ""} {
		text := "ĐƠN ĐẶT HÀNG NCC\n00758\nDH-HÀ THÀNH LA1\n" +
			"1\n8936156731647\n79758006\nNước rửa chén Blue không mùi túi 3.2L\n4.00\n80,900\n" + promoCell +
			"8.00\n2.00\n 647,200 \n0\nNOTE\nTOTAL\n 8 \n 2 \n 647,200 \n"
		got := ExtractProducts(text)
		if len(got) != 1 {
			t.Fatalf("promo cell %q: got %d products, want 1: %+v", promoCell, len(got), got)
		}
		p := got[0]
		if p.Price != "80,900" || p.PromoText != "" || p.Qty != "8.00" || p.Cartons != "2.00" || p.Total != "647,200" {
			t.Errorf("promo cell %q: got %+v", promoCell, p)
		}
		if p.PromoPrice != "-" && p.PromoPrice != "" {
			t.Errorf("promo cell %q: PromoPrice = %q, want none", promoCell, p.PromoPrice)
		}
	}
}

func TestParseTotals_ReadsTheRealOrdersTotalRow(t *testing.T) {
	got, ok := ParseTotals(realPageText)
	if !ok {
		t.Fatal("ParseTotals returned ok=false for the real order")
	}
	if want := (Totals{Qty: "16", Cartons: "3", Amount: "1,337,044"}); got != want {
		t.Errorf("ParseTotals = %+v, want %+v", got, want)
	}
}

func TestParseTotals_ReportsAPageWithNoTotalRow(t *testing.T) {
	if _, ok := ParseTotals("ĐƠN ĐẶT HÀNG NCC\n00758\n1\n8936156731647\n"); ok {
		t.Error("ParseTotals returned ok=true for a page with no TOTAL row")
	}
}
