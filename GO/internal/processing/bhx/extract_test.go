package bhx

import (
	"reflect"
	"testing"
)

// realPageText is the one real order available (store 7300, PO
// 07300PO2611606019, 10/11/2026) exactly as this repo's extractPageTexts
// returns it, trimmed to the parts the parser reads.
const realPageText = "\nTên chi nhánh\nNCC:\n36560 - BHX <> CÔNG TY CP HÀ THÀNH LONG\nAN 1\nTên NCC:\n86999 - CÔNG TY CP HÀ THÀNH LONG AN 1\n" +
	"Kho NCC giao\nhàng:\n7300 - BHX_HCM_Q12 - 31 Vườn Lài\nĐịa chỉ kho NCC giao:\n29-31 Vườn Lài, Phường An Phú Đông, Thành phố Hồ Chí Minh, Việt\nNam\n" +
	"Mã số thuế:\n1102018142\nNgày NCC giao hàng:\n10/11/2026 - 13/11/2026\n" +
	"STT\nLayout\nBarcode\nMã sản phẩm/ \nBarcode\nTên sản phẩm\nĐVT\nSL\nĐơn giá\nĐơn giá chiết\nkhấu\nVAT\n(%)\nThành tiền\nHình thức nhập\nGhi chú\n" +
	"1\n1013059000699 \n8936156730886\nNƯỚC GIẶT XẢ BLUE HƯƠNG\nNƯỚC HOA TÚI 3.6L \nTúi\n252.00\n71,296.30\n71,296.30\n8\n17,966,666.67\n4 - Nhập mua\nhàng (trong\nnước)\n" +
	"2\n1013059000698 \n8936156730879\nNƯỚC GIẶT XẢ BLUE HƯƠNG\nTHẢO MỘC TÚI 3.6L \nTúi\n252.00\n71,296.30\n71,296.30\n8\n17,966,666.67\n4 - Nhập mua\nhàng (trong\nnước)\n" +
	"Tổng\n504\n35,933,333.34\nMã đơn hàng:\n \n07300PO2611606019\nNgày đặt hàng:\n \n10/11/2026 16:14\nVP:\nCông Ty Cổ Phần Thương Mại Bách Hóa Xanh\n" +
	"ĐƠN ĐẶT HÀNG\nTổng tiền ĐH:\n35,933,333\nTổng tiền (sau VAT):\n38,808,000\n"

func TestParseOrderInfo_ReadsTheRealOrderHeader(t *testing.T) {
	got, ok := ParseOrderInfo(realPageText)
	if !ok {
		t.Fatalf("ParseOrderInfo returned ok=false: %+v", got)
	}
	want := OrderInfo{
		PONumber:   "07300PO2611606019",
		BranchCode: "36560",
		StoreCode:  "7300",
		StoreName:  "7300 - BHX_HCM_Q12 - 31 Vườn Lài",
		EntryDate:  "10/11/2026",
		CancelDate: "13/11/2026", // the END of the delivery window, as in Python
	}
	if got != want {
		t.Errorf("ParseOrderInfo =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestParseOrderInfo_AcceptsValuesOnTheLabelLine(t *testing.T) {
	text := "Tên chi nhánh NCC: 36560 - BHX <> HTLA1\nKho NCC giao hàng: 7300 - BHX_HCM_Q12\nNgày NCC giao hàng: 1/12/2026 - 3/12/2026\nMã đơn hàng: 07300PO26.1\nNgày đặt hàng: 1/12/2026 08:00\n"
	got, ok := ParseOrderInfo(text)
	if !ok {
		t.Fatalf("ParseOrderInfo returned ok=false: %+v", got)
	}
	// Python turns "." in the PO into "/"; dates are zero-padded.
	if got.PONumber != "07300PO26/1" || got.EntryDate != "01/12/2026" || got.CancelDate != "03/12/2026" || got.StoreCode != "7300" || got.BranchCode != "36560" {
		t.Errorf("ParseOrderInfo = %+v", got)
	}
}

func TestParseOrderInfo_RejectsAPageWithNoPONumber(t *testing.T) {
	text := "Kho NCC giao\nhàng:\n7300 - BHX\nNgày NCC giao hàng:\n10/11/2026 - 13/11/2026\nNgày đặt hàng:\n10/11/2026 16:14\n"
	if _, ok := ParseOrderInfo(text); ok {
		t.Error("ParseOrderInfo returned ok=true for a page with no Mã đơn hàng")
	}
}

func TestExtractProducts_ReadsEveryRowOfTheRealOrder(t *testing.T) {
	got := ExtractProducts(realPageText)
	want := []Product{
		{Barcode: "8936156730886", Qty: "252.00", Price: "71,296.30", DiscountPrice: "71,296.30", Total: "17,966,666.67"},
		{Barcode: "8936156730879", Qty: "252.00", Price: "71,296.30", DiscountPrice: "71,296.30", Total: "17,966,666.67"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractProducts =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestParseTotals_ReadsTheTongRow(t *testing.T) {
	got, ok := ParseTotals(realPageText)
	if !ok || got != (Totals{Qty: "504", Amount: "35,933,333.34"}) {
		t.Errorf("ParseTotals = %+v, %v", got, ok)
	}
}

func TestParseTotals_FailsWithoutATongRow(t *testing.T) {
	if _, ok := ParseTotals("Thành tiền\n1\n1013059000699\n8936156730886\n252.00\n"); ok {
		t.Error("ParseTotals returned ok=true for a page with no Tổng row")
	}
}
