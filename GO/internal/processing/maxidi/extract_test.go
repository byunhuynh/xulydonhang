package maxidi

import "testing"

// Both fixtures below are the EXACT text this repo's own extractPageTexts
// produces for two real archived delivery notes — captured verbatim, not
// hand-written. They differ in the one way that matters for parsing:
// binhDuongText keeps a newline on each side of the "số lượng lẻ" value,
// while dongNaiText has the entire product row fused onto a single line
// with no newline anywhere in it. Any line-position-based parser would
// pass on one and fail on the other, which is exactly why the extraction
// below is anchored on labels and value shapes instead of line offsets.
const binhDuongText = "\nUoMCHI NHÁNH BÌNH DƯƠNG - CÔNG TY TNHH MAXIDI VIỆT NAMKhu A, kho Liên Anh, số 189/8 Lê Hồng Phong, KP Tân Phước, P. Tân Đông HiệpTax Code: 0317899481-002Delivery note(Phiếu giao hàng)\nCÔNG TY CP HÀ THÀNH LONG AN 1Supplier (Nhà cung cấp):Purchase Date (Ngày đặt hàng):Document No.(Số đơn hàng):\n26/08/2026\nHO-PO00085936Barcode(Mã vạch)PLU(Mã hàng)Quantity(Số lượng)Ship Date (Ngày giao hàng):Ship To (Nơi giao hàng):\n24/09/2026\nKhu A, Kho Liên Anh, số 189/8 Lê Hồng Phong, KP Tân Phước, P.Tân Đông Hiệp, Hồ Chí Minh, Bình DươngQuantityper Unit(Số lượng mỗi thùng)\nT170042\nSupplier code (Mã nhà cung cấp):Description(Mô tả hàng hoá)Num per UoMPhysical state(Tiêu chuẩn ngoại quan)8935355302344280085 900.00Thung\n 10,800.00\nNước tẩy Javel Cleanwise 550G 12.00NSX thấp nhất 17/02/2026\nTuan-0913522900Created by:Approved by:\n 900.00THỜI GIAN GIAO HÀNG BUỔI SÁNG + CHIỀURemarks:"

const dongNaiText = "\nUoMCHI NHÁNH ĐỒNG NAI-CÔNG TY TNHH MAXIDI VIỆT NAMKho 12,ICD Tân Cảng Long Bình,Phường Long Bình, Region 1, Đồng NaiTax Code: 0317899481-001Delivery note(Phiếu giao hàng)\nCÔNG TY CP HÀ THÀNH LONG AN 1Supplier (Nhà cung cấp):Purchase Date (Ngày đặt hàng):Document No.(Số đơn hàng):\n26/08/2026\nHO-PO00085935Barcode(Mã vạch)PLU(Mã hàng)Quantity(Số lượng)Ship Date (Ngày giao hàng):Ship To (Nơi giao hàng):\n25/09/2026\nKho số 12, ICD Tân Cảng Long Bình, số 10 Phan Đăng Lưu, phường Long Bình, Bien Hoa, ĐồNg NaiQuantityper Unit(Số lượng mỗi thùng)\nT170042\nSupplier code (Mã nhà cung cấp):Description(Mô tả hàng hoá)Num per UoMPhysical state(Tiêu chuẩn ngoại quan)8935355302344280085 650.00Thung 7,800.00Nước tẩy Javel Cleanwise 550G 12.00NSX thấp nhất 18/02/2026\nTuan-0913522900Created by:Approved by:\n 650.00THỜI GIAN GIAO HÀNG BUỔI SÁNG + CHIỀURemarks:"

func TestParseOrderInfo_BinhDuongDeliveryNote(t *testing.T) {
	got, ok := ParseOrderInfo(binhDuongText)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false for a real Bình Dương delivery note")
	}
	want := OrderInfo{
		PONumber:        "HO-PO00085936",
		EntryDate:       "26/08/2026",
		ShipDate:        "24/09/2026",
		DeliveryAddress: "Khu A, Kho Liên Anh, số 189/8 Lê Hồng Phong, KP Tân Phước, P.Tân Đông Hiệp, Hồ Chí Minh, Bình Dương",
		Remarks:         "THỜI GIAN GIAO HÀNG BUỔI SÁNG + CHIỀU",
		TaxCode:         "0317899481-002",
	}
	if got != want {
		t.Errorf("ParseOrderInfo =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseOrderInfo_DongNaiDeliveryNote(t *testing.T) {
	got, ok := ParseOrderInfo(dongNaiText)
	if !ok {
		t.Fatal("ParseOrderInfo returned ok=false for a real Đồng Nai delivery note")
	}
	want := OrderInfo{
		PONumber:        "HO-PO00085935",
		EntryDate:       "26/08/2026",
		ShipDate:        "25/09/2026",
		DeliveryAddress: "Kho số 12, ICD Tân Cảng Long Bình, số 10 Phan Đăng Lưu, phường Long Bình, Bien Hoa, ĐồNg Nai",
		Remarks:         "THỜI GIAN GIAO HÀNG BUỔI SÁNG + CHIỀU",
		TaxCode:         "0317899481-001",
	}
	if got != want {
		t.Errorf("ParseOrderInfo =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseOrderInfo_RejectsTextMissingAField(t *testing.T) {
	// Everything up to (but not including) the ship-date block. A page
	// this truncated must fail cleanly rather than return a half-filled
	// OrderInfo that would go on to be written into the accounting
	// workbook with a blank delivery date.
	truncated := binhDuongText[:len(binhDuongText)/2]
	if _, ok := ParseOrderInfo(truncated); ok {
		t.Error("ParseOrderInfo returned ok=true for a truncated page")
	}
}

func TestExtractProducts_ReadsBothQuantitiesAndPackSize(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Product
	}{
		{
			// Newlines around the "số lượng lẻ" value.
			name: "Bình Dương",
			text: binhDuongText,
			want: Product{
				Barcode: "8935355302344", PLU: "280085",
				Cartons: "900.00", UoM: "Thung", Qty: "10,800.00",
				Name: "Nước tẩy Javel Cleanwise 550G", PackSize: "12.00",
			},
		},
		{
			// The whole product row fused onto one line, no newlines.
			name: "Đồng Nai",
			text: dongNaiText,
			want: Product{
				Barcode: "8935355302344", PLU: "280085",
				Cartons: "650.00", UoM: "Thung", Qty: "7,800.00",
				Name: "Nước tẩy Javel Cleanwise 550G", PackSize: "12.00",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractProducts(c.text)
			if len(got) != 1 {
				t.Fatalf("ExtractProducts returned %d products, want 1: %+v", len(got), got)
			}
			if got[0] != c.want {
				t.Errorf("ExtractProducts[0] =\n%+v\nwant\n%+v", got[0], c.want)
			}
		})
	}
}

func TestExtractProducts_ReturnsNothingWhenTheTableHeaderIsAbsent(t *testing.T) {
	if got := ExtractProducts("a page with no product table at all"); len(got) != 0 {
		t.Errorf("ExtractProducts = %+v, want none", got)
	}
}

func TestBranchForTaxCode(t *testing.T) {
	cases := []struct {
		taxCode  string
		wantName string
		wantOK   bool
	}{
		{"0317899481-002", "CHI NHÁNH BÌNH DƯƠNG - CÔNG TY TNHH MAXIDI VIỆT NAM", true},
		{"0317899481-001", "CHI NHÁNH ĐỒNG NAI - CÔNG TY TNHH MAXIDI VIỆT NAM", true},
		{"0317899481-003", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		t.Run(c.taxCode, func(t *testing.T) {
			got, ok := BranchForTaxCode(c.taxCode)
			if ok != c.wantOK {
				t.Fatalf("BranchForTaxCode(%q) ok = %v, want %v", c.taxCode, ok, c.wantOK)
			}
			if got.CustomerName != c.wantName {
				t.Errorf("CustomerName = %q, want %q", got.CustomerName, c.wantName)
			}
			if ok && (got.TaxCode != c.taxCode || got.InvoiceAddress == "") {
				t.Errorf("branch for %q = %+v, want its own tax code and a non-empty invoice address", c.taxCode, got)
			}
		})
	}
}
